package requestctx

import (
	"context"
	"testing"
	"time"
)

// TestRefreshLatencyUsesStartedAt 验证按创建时间刷新请求耗时。
func TestRefreshLatencyUsesStartedAt(t *testing.T) {
	ctx, meta := New(context.Background())
	meta.StartedAt = time.Now().Add(-5 * time.Millisecond)

	RefreshLatency(ctx)

	if meta.LatencyMS <= 0 {
		t.Fatalf("RefreshLatency() latency_ms = %d, want positive", meta.LatencyMS)
	}
}

// TestSetLatencyRoundsSubMillisecondToOne 验证亚毫秒耗时按 1ms 记录。
func TestSetLatencyRoundsSubMillisecondToOne(t *testing.T) {
	ctx, meta := New(context.Background())

	SetLatency(ctx, time.Nanosecond)

	if meta.LatencyMS != 1 {
		t.Fatalf("SetLatency(1ns) latency_ms = %d, want 1", meta.LatencyMS)
	}
}
