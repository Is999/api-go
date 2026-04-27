package bootstrap

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"api/internal/config"
	"api/internal/infra/loggerx"
	"api/internal/svc"

	utils "github.com/Is999/go-utils"
	"github.com/Is999/go-utils/errors"
	"github.com/zeromicro/go-zero/core/logx"
)

// configHotReloadState 保存配置热加载运行态资源，零值可用。
type configHotReloadState struct {
	cancel    context.CancelFunc // 配置热加载后台协程取消函数
	wg        sync.WaitGroup     // 等待配置热加载后台协程退出
	stateMu   sync.RWMutex       // 保护 watcher 生命周期
	statusMu  sync.Mutex         // 保护热加载状态快照更新
	execMu    sync.Mutex         // 串行化实际配置重载
	logMu     sync.Mutex         // 保护重复失败日志限频状态
	lastError string             // 最近一次失败日志签名
	lastLogAt time.Time          // 最近一次失败日志输出时间
}

// ReloadConfig 手动触发一次配置重载，供 handler/logic 通过接口调用。
func (a *App) ReloadConfig(ctx context.Context, source string) error {
	_, err := a.reloadConfigFile(ctx, source, a.boundConfigFile())
	return errors.Tag(err)
}

// startConfigHotReload 在启用时启动后台配置轮询协程。
func (a *App) startConfigHotReload() {
	if a == nil || a.ServiceContext == nil {
		return
	}
	cfg := a.ServiceContext.CurrentConfig()
	interval := normalizeHotReloadCheckInterval(cfg.HotReload.CheckIntervalSeconds)
	configFile := a.boundConfigFile()
	a.refreshHotReloadStatus(func(status svc.HotReloadStatus) svc.HotReloadStatus {
		status.Enabled = cfg.HotReload.Enabled
		status.Watching = false
		status.ConfigFile = configFile
		status.CheckIntervalSeconds = int(interval / time.Second)
		status.ConfigVersion = a.ServiceContext.CurrentVersion()
		status.ConfigSummary = buildHotReloadConfigSummary(cfg)
		if status.LastStatus == "" {
			status.LastStatus = "idle"
			status.LastMessage = "热加载监听尚未启动"
		}
		return status
	})
	if configFile == "" || !cfg.HotReload.Enabled {
		return
	}
	a.hotReload.stateMu.Lock()
	if a.hotReload.cancel != nil {
		a.hotReload.stateMu.Unlock()
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	a.hotReload.cancel = cancel
	a.hotReload.stateMu.Unlock()
	a.hotReload.wg.Add(1)
	go func() {
		defer a.hotReload.wg.Done()
		a.watchConfigFile(ctx, configFile)
	}()
	loggerx.Infow(ctx, "配置 热加载已启用",
		logx.Field("file", configFile),
		logx.Field(loggerx.FieldIntervalSeconds, int(interval/time.Second)),
	)
}

// stopConfigHotReload 停止配置热加载后台协程。
func (a *App) stopConfigHotReload() {
	if a == nil {
		return
	}
	a.hotReload.stateMu.Lock()
	if a.hotReload.cancel == nil {
		a.hotReload.stateMu.Unlock()
		return
	}
	cancel := a.hotReload.cancel
	a.hotReload.cancel = nil
	a.hotReload.stateMu.Unlock()
	cancel()
	a.hotReload.wg.Wait()
}

// isConfigHotReloadRunning 返回当前是否已有热加载 watcher 在运行。
func (a *App) isConfigHotReloadRunning() bool {
	if a == nil {
		return false
	}
	a.hotReload.stateMu.RLock()
	defer a.hotReload.stateMu.RUnlock()
	return a.hotReload.cancel != nil
}

// watchConfigFile 轮询配置文件指纹，检测到变化后重新解析并刷新配置快照。
func (a *App) watchConfigFile(ctx context.Context, configFile string) {
	interval := normalizeHotReloadCheckInterval(a.ServiceContext.CurrentConfig().HotReload.CheckIntervalSeconds)
	lastFingerprint, err := configBundleFingerprint(configFile)
	if err != nil {
		a.markHotReloadFailure("初始化配置文件指纹失败", err, "", "startup", "fingerprint", configFile)
		a.refreshHotReloadStatus(func(status svc.HotReloadStatus) svc.HotReloadStatus {
			status.Enabled = a.ServiceContext.CurrentConfig().HotReload.Enabled
			status.Watching = false
			status.ConfigFile = configFile
			status.CheckIntervalSeconds = int(interval / time.Second)
			return status
		})
		return
	}
	a.refreshHotReloadStatus(func(status svc.HotReloadStatus) svc.HotReloadStatus {
		status.Enabled = true
		status.Watching = true
		status.ConfigFile = configFile
		status.CheckIntervalSeconds = int(interval / time.Second)
		status.ConfigVersion = a.ServiceContext.CurrentVersion()
		status.ConfigSummary = buildHotReloadConfigSummary(a.ServiceContext.CurrentConfig())
		status.LastTriggerSource = "startup"
		if status.LastStatus == "" || status.LastStatus == "idle" {
			status.LastStatus = "idle"
			status.LastMessage = "热加载监听运行中"
		}
		return status
	})
	timer := time.NewTimer(interval)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			a.refreshHotReloadStatus(func(status svc.HotReloadStatus) svc.HotReloadStatus {
				status.Watching = false
				if status.LastMessage == "" {
					status.LastMessage = "热加载监听已停止"
				}
				return status
			})
			return
		case <-timer.C:
			now := time.Now()
			a.refreshHotReloadStatus(func(status svc.HotReloadStatus) svc.HotReloadStatus {
				status.LastCheckedAt = now
				status.CheckIntervalSeconds = int(normalizeHotReloadCheckInterval(a.ServiceContext.CurrentConfig().HotReload.CheckIntervalSeconds) / time.Second)
				return status
			})
			currentFingerprint, statErr := configBundleFingerprint(configFile)
			if statErr != nil {
				a.markHotReloadFailure("读取配置文件状态失败", statErr, "", "watcher", "fingerprint", configFile)
				timer.Reset(normalizeHotReloadCheckInterval(a.ServiceContext.CurrentConfig().HotReload.CheckIntervalSeconds))
				continue
			}
			if currentFingerprint != lastFingerprint {
				if _, reloadErr := a.reloadConfigFile(ctx, "watcher", configFile); reloadErr == nil {
					lastFingerprint = currentFingerprint
				}
			}
			if !a.ServiceContext.CurrentConfig().HotReload.Enabled {
				a.refreshHotReloadStatus(func(status svc.HotReloadStatus) svc.HotReloadStatus {
					status.Enabled = false
					status.Watching = false
					status.LastStatus = "idle"
					status.LastMessage = "热加载监听已关闭"
					return status
				})
				return
			}
			timer.Reset(normalizeHotReloadCheckInterval(a.ServiceContext.CurrentConfig().HotReload.CheckIntervalSeconds))
		}
	}
}

// reloadConfigFile 串行执行一次配置文件重载，供 watcher 和手动接口共用。
func (a *App) reloadConfigFile(ctx context.Context, source string, configFile string) (string, error) {
	if a == nil || a.ServiceContext == nil {
		return "", errors.Errorf("应用实例为空")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	configFile = strings.TrimSpace(configFile)
	if configFile == "" {
		notBoundErr := errors.Errorf("未绑定配置文件路径")
		a.markHotReloadFailure("配置热加载未绑定文件", notBoundErr, "", source, "not_bound", configFile)
		return "", notBoundErr
	}
	a.hotReload.execMu.Lock()
	defer a.hotReload.execMu.Unlock()
	select {
	case <-ctx.Done():
		cancelErr := errors.Tag(ctx.Err())
		a.markHotReloadFailure("配置热加载已取消", cancelErr, "", source, "cancelled", configFile)
		return "", cancelErr
	default:
	}

	beforeCfg := a.ServiceContext.CurrentConfig()
	previousVersion := a.ServiceContext.CurrentVersion()
	currentFingerprint, err := configBundleFingerprint(configFile)
	if err != nil {
		a.markHotReloadFailure("读取配置文件指纹失败", err, "", source, "fingerprint", configFile)
		return "", errors.Tag(err)
	}
	cfg, version, err := LoadConfig(configFile)
	if err != nil {
		a.markHotReloadFailure("配置热加载失败", err, currentFingerprint, source, "load", configFile)
		return "", errors.Tag(err)
	}
	restartRequired, restartReason := detectHotReloadRestartImpact(beforeCfg, cfg)
	effectiveCfg := cfg
	if restartRequired {
		effectiveCfg = buildHotReloadEffectiveConfig(beforeCfg, cfg)
	}
	a.ServiceContext.UpdateConfig(effectiveCfg)
	a.ServiceContext.UpdateVersion(version)
	now := time.Now()
	message := "配置热加载成功"
	if restartRequired {
		message = "配置热加载成功，部分启动期配置需重启后生效"
	}
	a.refreshHotReloadStatus(func(status svc.HotReloadStatus) svc.HotReloadStatus {
		status.Enabled = effectiveCfg.HotReload.Enabled
		status.ConfigFile = configFile
		status.CheckIntervalSeconds = int(normalizeHotReloadCheckInterval(effectiveCfg.HotReload.CheckIntervalSeconds) / time.Second)
		status.ConfigVersion = version
		status.ConfigSummary = buildHotReloadConfigSummary(effectiveCfg)
		status.RestartRequired = restartRequired
		status.RestartReason = restartReason
		status.LastStatus = "success"
		status.LastMessage = message
		status.LastTriggerSource = normalizeHotReloadSource(source)
		status.LastFailureCategory = ""
		status.LastReloadAt = now
		status.LastSuccessAt = now
		status.ReloadCount++
		return status
	})
	a.hotReload.logMu.Lock()
	a.hotReload.lastError = ""
	a.hotReload.lastLogAt = time.Time{}
	a.hotReload.logMu.Unlock()
	loggerx.Infow(ctx, "配置 热加载成功",
		logx.Field("file", configFile),
		logx.Field("from_version", previousVersion),
		logx.Field("to_version", version),
		logx.Field("restart_required", restartRequired),
		logx.Field("restart_reason", restartReason),
	)
	if effectiveCfg.HotReload.Enabled && !a.isConfigHotReloadRunning() {
		a.startConfigHotReload()
	}
	if !effectiveCfg.HotReload.Enabled && normalizeHotReloadSource(source) != "watcher" {
		a.stopConfigHotReload()
	}
	return currentFingerprint, nil
}

// boundConfigFile 返回当前 App 绑定的配置文件路径。
func (a *App) boundConfigFile() string {
	if a == nil {
		return ""
	}
	return strings.TrimSpace(a.ConfigFile)
}

// refreshHotReloadStatus 在当前状态基础上执行原子更新。
func (a *App) refreshHotReloadStatus(mutator func(svc.HotReloadStatus) svc.HotReloadStatus) {
	if a == nil || a.ServiceContext == nil || mutator == nil {
		return
	}
	a.hotReload.statusMu.Lock()
	defer a.hotReload.statusMu.Unlock()
	status := a.ServiceContext.CurrentHotReloadStatus()
	a.ServiceContext.UpdateHotReloadStatus(mutator(status))
}

// markHotReloadFailure 记录最近一次热加载失败状态，并对重复错误限频。
func (a *App) markHotReloadFailure(message string, err error, fingerprint, source, category, configFile string) {
	if a == nil {
		return
	}
	now := time.Now()
	errText := ""
	if err != nil {
		errText = err.Error()
	}
	a.refreshHotReloadStatus(func(status svc.HotReloadStatus) svc.HotReloadStatus {
		status.LastStatus = "failed"
		status.LastMessage = strings.TrimSpace(message)
		status.LastReloadAt = now
		status.LastFailureAt = now
		status.LastTriggerSource = normalizeHotReloadSource(source)
		status.LastFailureCategory = normalizeHotReloadFailureCategory(category)
		if fingerprint != "" {
			status.ConfigVersion = fingerprint
		}
		return status
	})
	errorKey := message + "|" + errText + "|" + source + "|" + category
	a.hotReload.logMu.Lock()
	sameError := errorKey == a.hotReload.lastError && !a.hotReload.lastLogAt.IsZero() && now.Sub(a.hotReload.lastLogAt) < 30*time.Second
	if sameError {
		a.hotReload.lastError = errorKey
		a.hotReload.logMu.Unlock()
		a.refreshHotReloadStatus(func(status svc.HotReloadStatus) svc.HotReloadStatus {
			status.SuppressedFailureCount++
			return status
		})
		return
	}
	a.hotReload.lastError = errorKey
	a.hotReload.lastLogAt = now
	a.hotReload.logMu.Unlock()
	loggerx.ErrorTextw(nil, "配置 热加载失败", errText,
		logx.Field("file", configFile),
		logx.Field("detail", message),
		logx.Field("version", fingerprint),
		logx.Field("source", normalizeHotReloadSource(source)),
		logx.Field("category", normalizeHotReloadFailureCategory(category)),
	)
}

// configFileFingerprint 返回配置文件当前的稳定指纹。
func configFileFingerprint(file string) (string, error) {
	cleanFile := filepath.Clean(strings.TrimSpace(file))
	info, err := os.Stat(cleanFile)
	if err != nil {
		return "", errors.Tag(err)
	}
	data, err := os.ReadFile(cleanFile)
	if err != nil {
		return "", errors.Tag(err)
	}
	realPath, err := filepath.EvalSymlinks(cleanFile)
	if err != nil {
		realPath = cleanFile
	}
	return fmt.Sprintf("%s|%d|%d|%s", realPath, info.Size(), info.ModTime().UnixNano(), utils.Sha256(string(data))), nil
}

// configBundleFingerprint 返回主配置及外部配置文件组成的配置包指纹。
func configBundleFingerprint(file string) (string, error) {
	mainFingerprint, err := configFileFingerprint(file)
	if err != nil {
		return "", errors.Tag(err)
	}
	cfg, err := loadBaseConfig(file)
	if err != nil {
		return mainFingerprint, nil
	}
	parts := []string{mainFingerprint}
	for _, include := range configIncludePaths(file, cfg.ConfigFiles) {
		fingerprint, innerErr := configFileFingerprint(include)
		if innerErr != nil {
			return "", errors.Wrapf(innerErr, "读取外部配置文件指纹失败 file=%s", include)
		}
		parts = append(parts, fingerprint)
	}
	return strings.Join(parts, "\n"), nil
}

// normalizeHotReloadCheckInterval 返回热加载轮询间隔，默认 5 秒。
func normalizeHotReloadCheckInterval(seconds int) time.Duration {
	if seconds <= 0 {
		seconds = 5
	}
	if seconds < 1 {
		seconds = 1
	}
	return time.Duration(seconds) * time.Second
}

// normalizeHotReloadSource 归一化热加载触发来源。
func normalizeHotReloadSource(source string) string {
	source = strings.TrimSpace(source)
	if source == "" {
		return "manual"
	}
	return source
}

// normalizeHotReloadFailureCategory 归一化热加载失败分类。
func normalizeHotReloadFailureCategory(category string) string {
	category = strings.TrimSpace(category)
	if category == "" {
		return "reload"
	}
	return category
}

// buildHotReloadConfigSummary 生成运行期配置摘要，便于接口展示和日志排查。
func buildHotReloadConfigSummary(cfg config.Config) string {
	return fmt.Sprintf("mode=%s app_id=%s sign=%d crypto=%d collector=%t", cfg.Mode, strings.TrimSpace(cfg.AppID), cfg.Security.SecretKey.SignStatus, cfg.Security.SecretKey.CryptoStatus, cfg.Collector.Enabled)
}

// detectHotReloadRestartImpact 判断新配置是否包含启动期依赖变化。
func detectHotReloadRestartImpact(oldCfg config.Config, newCfg config.Config) (bool, string) {
	reasons := make([]string, 0, 4)
	if oldCfg.Host != newCfg.Host || oldCfg.Port != newCfg.Port {
		reasons = append(reasons, "HTTP监听地址变更")
	}
	if fmt.Sprint(oldCfg.MySQL) != fmt.Sprint(newCfg.MySQL) || fmt.Sprint(oldCfg.SiteMySQL) != fmt.Sprint(newCfg.SiteMySQL) {
		reasons = append(reasons, "MySQL连接配置变更")
	}
	if fmt.Sprint(oldCfg.Redis) != fmt.Sprint(newCfg.Redis) {
		reasons = append(reasons, "Redis连接配置变更")
	}
	if oldCfg.Observability.OTLPEndpoint != newCfg.Observability.OTLPEndpoint || oldCfg.Observability.OTLPProtocol != newCfg.Observability.OTLPProtocol {
		reasons = append(reasons, "OTLP导出配置变更")
	}
	if len(reasons) == 0 {
		return false, ""
	}
	return true, strings.Join(reasons, "；")
}

// buildHotReloadEffectiveConfig 保留启动期依赖配置，运行期配置仍按新文件刷新。
func buildHotReloadEffectiveConfig(oldCfg config.Config, newCfg config.Config) config.Config {
	newCfg.RestConf = oldCfg.RestConf
	newCfg.MySQL = oldCfg.MySQL
	newCfg.SiteMySQL = oldCfg.SiteMySQL
	newCfg.Redis = oldCfg.Redis
	newCfg.Observability.OTLPEndpoint = oldCfg.Observability.OTLPEndpoint
	newCfg.Observability.OTLPProtocol = oldCfg.Observability.OTLPProtocol
	return newCfg
}
