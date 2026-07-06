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
	state.StopWatcher()
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
	state.StopWatcher()
	<-stopped
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
