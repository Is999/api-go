package hotreload

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"api/internal/config"
	"api/internal/svc"
)

// State 保存配置热加载运行态资源，零值可用。
type State struct {
	cancel    context.CancelFunc // 配置热加载后台协程取消函数
	wg        sync.WaitGroup     // 等待配置热加载后台协程退出
	stateMu   sync.RWMutex       // 保护 watcher 生命周期
	statusMu  sync.Mutex         // 保护热加载状态快照更新
	execMu    sync.Mutex         // 串行化实际配置重载
	logMu     sync.Mutex         // 保护重复失败日志限频状态
	lastError string             // 最近一次失败日志签名
	lastLogAt time.Time          // 最近一次失败日志输出时间
}

// LockExec 锁定配置重载执行通道。
func (s *State) LockExec() {
	if s == nil {
		return
	}
	s.execMu.Lock()
}

// UnlockExec 释放配置重载执行通道。
func (s *State) UnlockExec() {
	if s == nil {
		return
	}
	s.execMu.Unlock()
}

// StartWatcher 启动热加载 watcher，并保证同一时间只有一个 watcher。
func (s *State) StartWatcher(run func(context.Context)) bool {
	if s == nil || run == nil {
		return false
	}
	s.stateMu.Lock()
	if s.cancel != nil {
		s.stateMu.Unlock()
		return false
	}
	ctx, cancel := context.WithCancel(context.Background())
	s.cancel = cancel
	s.stateMu.Unlock()
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		run(ctx)
	}()
	return true
}

// StopWatcher 停止热加载 watcher 并等待退出。
func (s *State) StopWatcher() {
	if s == nil {
		return
	}
	s.stateMu.Lock()
	if s.cancel == nil {
		s.stateMu.Unlock()
		return
	}
	cancel := s.cancel
	s.cancel = nil
	s.stateMu.Unlock()
	cancel()
	s.wg.Wait()
}

// WatcherRunning 返回当前是否已有热加载 watcher 在运行。
func (s *State) WatcherRunning() bool {
	if s == nil {
		return false
	}
	s.stateMu.RLock()
	defer s.stateMu.RUnlock()
	return s.cancel != nil
}

// UpdateStatus 在当前状态基础上执行原子更新。
func (s *State) UpdateStatus(svcCtx *svc.ServiceContext, mutator func(svc.HotReloadStatus) svc.HotReloadStatus) {
	if s == nil || svcCtx == nil || mutator == nil {
		return
	}
	s.statusMu.Lock()
	defer s.statusMu.Unlock()
	status := svcCtx.CurrentHotReloadStatus()
	svcCtx.UpdateHotReloadStatus(mutator(status))
}

// SuppressFailure 判断本次失败日志是否应被限频抑制。
func (s *State) SuppressFailure(errorKey string, now time.Time, window time.Duration) bool {
	if s == nil {
		return false
	}
	s.logMu.Lock()
	defer s.logMu.Unlock()
	sameError := errorKey == s.lastError && !s.lastLogAt.IsZero() && now.Sub(s.lastLogAt) < window
	if sameError {
		s.lastError = errorKey
		return true
	}
	s.lastError = errorKey
	s.lastLogAt = now
	return false
}

// ResetFailureLog 清理重复失败限频状态。
func (s *State) ResetFailureLog() {
	if s == nil {
		return
	}
	s.logMu.Lock()
	s.lastError = ""
	s.lastLogAt = time.Time{}
	s.logMu.Unlock()
}

// Summary 生成运行期配置摘要，便于接口展示和日志排查。
func Summary(cfg config.Config) string {
	return fmt.Sprintf("mode=%s app_id=%s user_route=%d sign=%d crypto=%d collector=%t", cfg.Mode, strings.TrimSpace(cfg.AppID), cfg.User.RouteShardCount, cfg.Security.SecretKey.SignStatus, cfg.Security.SecretKey.CryptoStatus, cfg.Collector.Enabled)
}

// CheckInterval 返回热加载轮询间隔，默认 5 秒。
func CheckInterval(seconds int) time.Duration {
	if seconds <= 0 {
		seconds = 5
	}
	if seconds < 1 {
		seconds = 1
	}
	return time.Duration(seconds) * time.Second
}

// Source 归一化热加载触发来源。
func Source(source string) string {
	source = strings.TrimSpace(source)
	if source == "" {
		return "manual"
	}
	return source
}

// FailureCategory 归一化热加载失败分类。
func FailureCategory(category string) string {
	category = strings.TrimSpace(category)
	if category == "" {
		return "reload"
	}
	return category
}
