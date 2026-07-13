package hotreload

import (
	"context"
	"testing"
	"time"
)

// TestStateWatcherLifecycle 确保零值状态可以启动和停止 watcher。
func TestStateWatcherLifecycle(t *testing.T) {
	var state State
	started := make(chan struct{})
	done := make(chan struct{})
	if ok := state.StartWatcher(func(ctx context.Context) {
		close(started)
		<-ctx.Done()
		close(done)
	}); !ok {
		t.Fatal("expected watcher to start")
	}
	<-started
	if !state.WatcherRunning() {
		t.Fatal("expected watcher to be running")
	}
	if ok := state.StartWatcher(func(context.Context) {}); ok {
		t.Fatal("expected duplicate watcher start to be ignored")
	}
	if err := state.StopWatcher(context.Background()); err != nil {
		t.Fatalf("StopWatcher() error = %v", err)
	}
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("watcher did not stop")
	}
	if state.WatcherRunning() {
		t.Fatal("expected watcher to stop")
	}
}

// TestStateStartWatcherClearsAfterRunReturns 确保 watcher 自然退出后可重新启动。
func TestStateStartWatcherClearsAfterRunReturns(t *testing.T) {
	var state State
	done := make(chan struct{})
	if ok := state.StartWatcher(func(context.Context) {
		close(done)
	}); !ok {
		t.Fatal("expected watcher to start")
	}
	<-done
	waitForWatcherState(t, func() bool {
		return !state.WatcherRunning()
	})

	started := make(chan struct{})
	stopped := make(chan struct{})
	if ok := state.StartWatcher(func(ctx context.Context) {
		close(started)
		<-ctx.Done()
		close(stopped)
	}); !ok {
		t.Fatal("expected watcher to restart after natural exit")
	}
	<-started
	if err := state.StopWatcher(context.Background()); err != nil {
		t.Fatalf("StopWatcher() error = %v", err)
	}
	<-stopped
}

// TestStateRejectsRestartUntilStoppingWatcherExits 验证 Stop 等待清理期间不会发布新的 watcher。
func TestStateRejectsRestartUntilStoppingWatcherExits(t *testing.T) {
	var state State
	started := make(chan struct{})
	cancelled := make(chan struct{})
	release := make(chan struct{})
	if ok := state.StartWatcher(func(ctx context.Context) {
		close(started)
		<-ctx.Done()
		close(cancelled)
		<-release
	}); !ok {
		t.Fatal("expected watcher to start")
	}
	<-started
	stopDone := make(chan struct{})
	go func() {
		_ = state.StopWatcher(context.Background())
		close(stopDone)
	}()
	<-cancelled
	if ok := state.StartWatcher(func(context.Context) {}); ok {
		t.Fatal("stopping watcher must keep the lifecycle slot")
	}
	select {
	case <-stopDone:
		t.Fatal("StopWatcher returned before watcher cleanup completed")
	default:
	}
	close(release)
	select {
	case <-stopDone:
	case <-time.After(time.Second):
		t.Fatal("StopWatcher did not finish after cleanup release")
	}
	if ok := state.StartWatcher(func(context.Context) {}); !ok {
		t.Fatal("expected watcher restart after stop completed")
	}
	if err := state.StopWatcher(context.Background()); err != nil {
		t.Fatalf("StopWatcher() error = %v", err)
	}
}

// TestStateImmediateStartStopStress 验证快速启动停止不会触发 Add/Wait 类生命周期竞态。
func TestStateImmediateStartStopStress(t *testing.T) {
	var state State
	for range 500 {
		if ok := state.StartWatcher(func(ctx context.Context) { <-ctx.Done() }); !ok {
			t.Fatal("expected watcher to start")
		}
		_ = state.StopWatcher(context.Background())
	}
}

// TestCheckInterval 验证热加载轮询间隔默认值和显式配置值。
func TestCheckInterval(t *testing.T) {
	if got := CheckInterval(0); got != 5*time.Second {
		t.Fatalf("interval 0 = %s, want 5s", got)
	}
	if got := CheckInterval(2); got != 2*time.Second {
		t.Fatalf("interval 2 = %s, want 2s", got)
	}
}

// waitForWatcherState 等待异步 watcher 状态进入预期。
func waitForWatcherState(t *testing.T, ok func() bool) {
	t.Helper()
	deadline := time.After(time.Second)
	ticker := time.NewTicker(time.Millisecond)
	defer ticker.Stop()
	for {
		if ok() {
			return
		}
		select {
		case <-deadline:
			t.Fatal("等待 watcher 状态变化超时")
		case <-ticker.C:
		}
	}
}
