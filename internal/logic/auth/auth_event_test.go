package auth

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"api/internal/config"
	"api/internal/infra/collectorx"
	"api/internal/requestctx"
	"api/internal/routealias"
	"api/internal/svc"

	"github.com/Is999/go-utils/errors"
)

// TestRecordAuthEventEnqueuesSanitizedPayload 确保认证事件只投递脱敏后的结构化负载。
func TestRecordAuthEventEnqueuesSanitizedPayload(t *testing.T) {
	cfg := authEventTestConfig(true)
	svcCtx, collector := newAuthEventTestService(cfg)
	ctx, _ := requestctx.New(context.Background())
	requestctx.SetTrace(ctx, "trace-demo", "span-demo")
	requestctx.SetRoute(ctx, string(routealias.AuthLogin))
	requestctx.SetRequest(ctx, "POST", "/api/auth/login", "127.0.0.1")
	requestctx.SetNode(ctx, "node-a")
	requestctx.SetMode(ctx, "dev")

	RecordAuthEvent(ctx, svcCtx, AuthEventInput{
		Action:   AuthEventActionLoginSuccess,
		UserID:   42,
		Identity: "username:Demo_User",
		JTI:      "session-jti",
		Reason:   AuthEventReasonSessionCreated,
	})

	if len(collector.events) != 1 {
		t.Fatalf("collector events = %d, want 1", len(collector.events))
	}
	event := collector.events[0]
	if event.BizType != AuthCollectorBizType {
		t.Fatalf("biz type = %q, want %q", event.BizType, AuthCollectorBizType)
	}
	if event.PartitionKey != "site-a:42" {
		t.Fatalf("partition key = %q, want site-a:42", event.PartitionKey)
	}
	var payload authEventPayload
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		t.Fatalf("Unmarshal(payload) error = %v", err)
	}
	if payload.Action != AuthEventActionLoginSuccess || payload.UserID != 42 || payload.Reason != AuthEventReasonSessionCreated {
		t.Fatalf("payload core fields = %+v", payload)
	}
	if payload.AppID != "site-a" || payload.Route != string(routealias.AuthLogin) || payload.TraceID != "trace-demo" || payload.SpanID != "span-demo" {
		t.Fatalf("payload trace fields = %+v", payload)
	}
	if payload.IdentityHash != authEventHash(cfg, "username:demo_user") {
		t.Fatalf("identity hash = %q, want deterministic hmac", payload.IdentityHash)
	}
	if payload.ClientIPHash == "" || payload.SessionHash == "" {
		t.Fatalf("payload hashes missing = %+v", payload)
	}
	raw := string(event.Payload)
	for _, forbidden := range []string{"Demo_User", "127.0.0.1", "session-jti"} {
		if strings.Contains(raw, forbidden) {
			t.Fatalf("payload leaked raw value %q: %s", forbidden, raw)
		}
	}
}

// TestRecordAuthEventSkipsWhenCollectorDisabled 确保关闭 Collector 时认证主流程不会产生副作用。
func TestRecordAuthEventSkipsWhenCollectorDisabled(t *testing.T) {
	svcCtx, collector := newAuthEventTestService(authEventTestConfig(false))

	RecordAuthEvent(context.Background(), svcCtx, AuthEventInput{
		Action:   AuthEventActionLoginFailed,
		Identity: "username:demo",
		Reason:   AuthEventReasonInvalidPassword,
	})

	if len(collector.events) != 0 {
		t.Fatalf("collector events = %d, want 0", len(collector.events))
	}
}

// TestRecordAuthEventIgnoresCollectorError 确保 Collector 投递失败不影响认证主流程。
func TestRecordAuthEventIgnoresCollectorError(t *testing.T) {
	svcCtx, collector := newAuthEventTestService(authEventTestConfig(true))
	collector.enqueueErr = errors.Errorf("kafka timeout")

	RecordAuthEvent(context.Background(), svcCtx, AuthEventInput{
		Action:   AuthEventActionRateLimited,
		Identity: "username:demo",
		Reason:   AuthEventReasonLoginIdentityRateLimited,
	})

	if len(collector.events) != 0 {
		t.Fatalf("collector events = %d, want 0", len(collector.events))
	}
}

// TestRuntimeRegistrySpecsValid 确保认证业务运行时注册入口规格完整且名称唯一。
func TestRuntimeRegistrySpecsValid(t *testing.T) {
	specs := RuntimeRegistrySpecs()
	if len(specs) == 0 {
		t.Fatal("RuntimeRegistrySpecs() 不能为空")
	}
	seen := make(map[string]struct{}, len(specs))
	for _, spec := range specs {
		if spec.Name == "" || spec.File == "" || spec.Method == "" || spec.Description == "" {
			t.Fatalf("运行时注册规格字段不完整: %+v", spec)
		}
		if _, ok := seen[spec.Name]; ok {
			t.Fatalf("运行时注册规格名称重复: %s", spec.Name)
		}
		seen[spec.Name] = struct{}{}
	}
}

// authEventTestConfig 表示测试辅助逻辑。
func authEventTestConfig(enabled bool) config.Config {
	return config.Config{
		AppID:     "site-a",
		AppKey:    "event-secret",
		JwtSecret: "jwt-secret",
		Collector: config.CollectorConfig{
			Enabled: enabled,
		},
	}
}

// fakeCollector 记录业务投递的 Collector 事件。
type fakeCollector struct {
	events     []collectorx.Event   // 已投递事件
	enqueueErr error                // 投递时返回的错误
	alertHook  collectorx.AlertHook // 运行异常告警钩子
	closed     bool                 // 是否已关闭
}

// Enqueue 记录一条事件。
func (f *fakeCollector) Enqueue(_ context.Context, event collectorx.Event) (string, error) {
	if f.enqueueErr != nil {
		return "", f.enqueueErr
	}
	if event.EventID == "" {
		event.EventID = "test-event"
	}
	f.events = append(f.events, event)
	return event.EventID, nil
}

// SetAlertHook 保存告警钩子。
func (f *fakeCollector) SetAlertHook(hook collectorx.AlertHook) {
	f.alertHook = hook
}

// Close 标记收集器已关闭。
func (f *fakeCollector) Close(context.Context) error {
	f.closed = true
	return nil
}

// newAuthEventTestService 构造测试依赖。
func newAuthEventTestService(cfg config.Config) (*svc.ServiceContext, *fakeCollector) {
	collector := &fakeCollector{events: make([]collectorx.Event, 0, 1)}
	svcCtx := svc.NewServiceContext(cfg, "v1", svc.Dependencies{})
	svcCtx.Collector = collector
	return svcCtx, collector
}
