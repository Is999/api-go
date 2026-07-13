package tracing

import (
	"context"
	"testing"

	"api/internal/config"

	"go.opentelemetry.io/otel"
)

// TestSetupDisabledSkipsExporter 验证关闭 trace 时不会因废弃或不可达的 OTLP 配置阻塞启动。
func TestSetupDisabledSkipsExporter(t *testing.T) {
	shutdown, err := Setup(context.Background(), config.ObservabilityConfig{
		TraceEnabled: false,
		OTLPProtocol: "invalid",
		OTLPEndpoint: "blackhole.invalid:4317",
		SampleRatio:  1,
	})
	if err != nil {
		t.Fatalf("Setup(disabled) error = %v", err)
	}
	t.Cleanup(func() { _ = shutdown(context.Background()) })
}

// TestSetupPreservesZeroSampleRatio 验证显式零采样不会被静默改成全量采样。
func TestSetupPreservesZeroSampleRatio(t *testing.T) {
	shutdown, err := Setup(context.Background(), config.ObservabilityConfig{
		TraceEnabled: true,
		SampleRatio:  0,
	})
	if err != nil {
		t.Fatalf("Setup(sample=0) error = %v", err)
	}
	t.Cleanup(func() { _ = shutdown(context.Background()) })
	_, span := otel.Tracer("test").Start(context.Background(), "zero-sample")
	defer span.End()
	if span.IsRecording() {
		t.Fatal("sample_ratio=0 should not record spans")
	}
}

// TestNormalizeOTLPProtocolRejectsHTTPJSON 验证 OTLP HTTP 只接受真实 protobuf 协议别名。
func TestNormalizeOTLPProtocolRejectsHTTPJSON(t *testing.T) {
	if got := normalizeOTLPProtocol("http/json"); got == "http" {
		t.Fatalf("http/json should not normalize to protobuf HTTP: %q", got)
	}
}
