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

// TestCheckInterval 验证热加载轮询间隔默认值和显式配置值。
func TestCheckInterval(t *testing.T) {
	if got := CheckInterval(0); got != 5*time.Second {
		t.Fatalf("interval 0 = %s, want 5s", got)
	}
	if got := CheckInterval(2); got != 2*time.Second {
		t.Fatalf("interval 2 = %s, want 2s", got)
	}
}
