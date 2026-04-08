package logic

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"gozero_api/internal/collector"
	"gozero_api/internal/config"
	"gozero_api/internal/requestctx"
	"gozero_api/internal/svc"
)

// TestRecordAuthEventEnqueuesSanitizedPayload 确保认证事件只投递脱敏后的结构化负载。
func TestRecordAuthEventEnqueuesSanitizedPayload(t *testing.T) {
	cfg := authEventTestConfig(true)
	svcCtx, seen := newAuthEventTestService(t, cfg, true)
	ctx, _ := requestctx.New(context.Background())
	requestctx.SetTrace(ctx, "trace-demo", "span-demo")
	requestctx.SetRoute(ctx, "auth.login")
	requestctx.SetRequest(ctx, "POST", "/api/auth/login", "127.0.0.1")
	requestctx.SetNode(ctx, "node-a")
	requestctx.SetMode(ctx, "dev")

	RecordAuthEvent(ctx, svcCtx, AuthEventInput{
		Action:   AuthEventActionLoginSuccess,
		UserID:   42,
		Username: "Demo_User",
		JTI:      "session-jti",
		Reason:   AuthEventReasonSessionCreated,
	})

	if len(*seen) != 1 {
		t.Fatalf("collector events = %d, want 1", len(*seen))
	}
	event := (*seen)[0]
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
	if payload.AppID != "site-a" || payload.Route != "auth.login" || payload.TraceID != "trace-demo" || payload.SpanID != "span-demo" {
		t.Fatalf("payload trace fields = %+v", payload)
	}
	if payload.UsernameHash != authEventHash(cfg, "demo_user") {
		t.Fatalf("username hash = %q, want deterministic hmac", payload.UsernameHash)
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
	svcCtx, seen := newAuthEventTestService(t, authEventTestConfig(false), true)

	RecordAuthEvent(context.Background(), svcCtx, AuthEventInput{
		Action:   AuthEventActionLoginFailed,
		Username: "demo",
		Reason:   AuthEventReasonInvalidPassword,
	})

	if len(*seen) != 0 {
		t.Fatalf("collector events = %d, want 0", len(*seen))
	}
}

// TestRecordAuthEventIgnoresMissingProcessor 确保未注册 Processor 不影响认证主流程。
func TestRecordAuthEventIgnoresMissingProcessor(t *testing.T) {
	svcCtx, seen := newAuthEventTestService(t, authEventTestConfig(true), false)

	RecordAuthEvent(context.Background(), svcCtx, AuthEventInput{
		Action:   AuthEventActionRateLimited,
		Username: "demo",
		Reason:   AuthEventReasonLoginUsernameRateLimited,
	})

	if len(*seen) != 0 {
		t.Fatalf("collector events = %d, want 0", len(*seen))
	}
}

func authEventTestConfig(enabled bool) config.Config {
	return config.Config{
		AppID:     "site-a",
		AppKey:    "event-secret",
		JwtSecret: "jwt-secret",
		Collector: config.CollectorConfig{
			Enabled:   enabled,
			Transport: "sync",
		},
	}
}

func newAuthEventTestService(t *testing.T, cfg config.Config, registerProcessor bool) (*svc.ServiceContext, *[]collector.Event) {
	t.Helper()
	manager, err := collector.New(config.CollectorConfig{
		Enabled:   true,
		Transport: "sync",
	}, nil)
	if err != nil {
		t.Fatalf("collector.New() error = %v", err)
	}
	seen := make([]collector.Event, 0, 1)
	if registerProcessor {
		if err := manager.RegisterProcessorFunc(AuthCollectorBizType, func(ctx context.Context, events []collector.Event) ([]collector.ProcessResult, error) {
			seen = append(seen, events...)
			results := make([]collector.ProcessResult, 0, len(events))
			for _, event := range events {
				results = append(results, collector.ProcessResult{EventID: event.EventID, Success: true})
			}
			return results, nil
		}); err != nil {
			t.Fatalf("RegisterProcessorFunc() error = %v", err)
		}
	}
	svcCtx := svc.NewServiceContext(cfg, "v1", svc.Dependencies{})
	svcCtx.Collector = manager
	return svcCtx, &seen
}
