package middleware

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"api/internal/config"
	"api/internal/infra/collectorx"
	authlogic "api/internal/logic/auth"
	"api/internal/requestctx"
	"api/internal/svc"
)

// TestAuthMiddlewareMissingBearerEmitsAuthSecurityEvent 确保鉴权失败也会投递脱敏风控事件。
func TestAuthMiddlewareMissingBearerEmitsAuthSecurityEvent(t *testing.T) {
	svcCtx, seen := newAuthMiddlewareEventService(t)
	middleware := NewAuthMiddleware(svcCtx)
	nextCalled := false
	handler := middleware.Handle(func(w http.ResponseWriter, r *http.Request) {
		nextCalled = true
	}, RouteAlias("user.profile"))

	req := httptest.NewRequest(http.MethodGet, "/api/user/profile", nil)
	req.RemoteAddr = "127.0.0.1:12345"
	rec := httptest.NewRecorder()
	handler(rec, req)

	if nextCalled {
		t.Fatal("next handler should not be called")
	}
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
	if len(*seen) != 1 {
		t.Fatalf("collector events = %d, want 1", len(*seen))
	}
	event := (*seen)[0]
	if event.BizType != authlogic.AuthCollectorBizType {
		t.Fatalf("biz type = %q, want %q", event.BizType, authlogic.AuthCollectorBizType)
	}
	var payload map[string]any
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		t.Fatalf("Unmarshal(payload) error = %v", err)
	}
	if payload["action"] != authlogic.AuthEventActionAuthFailed || payload["reason"] != authlogic.AuthEventReasonMissingBearer {
		t.Fatalf("payload action/reason = %+v", payload)
	}
	if payload["route"] != "user.profile" {
		t.Fatalf("payload route = %v, want user.profile", payload["route"])
	}
	if _, ok := payload["client_ip_hash"].(string); !ok {
		t.Fatalf("payload client_ip_hash missing: %+v", payload)
	}
	raw := string(event.Payload)
	if strings.Contains(raw, "127.0.0.1") {
		t.Fatalf("payload leaked raw client ip: %s", raw)
	}
}

// TestEmitAuthFailureEventIncludesKnownIdentity 确保已解析身份的失败事件可按用户聚合。
func TestEmitAuthFailureEventIncludesKnownIdentity(t *testing.T) {
	svcCtx, seen := newAuthMiddlewareEventService(t)
	middleware := NewAuthMiddleware(svcCtx)

	middleware.emitAuthFailureEvent(context.Background(), authlogic.AuthEventReasonSessionExpired, &UserTokenIdentity{
		UserID:   42,
		UserName: "Demo_User",
		JTI:      "session-jti",
	})

	if len(*seen) != 1 {
		t.Fatalf("collector events = %d, want 1", len(*seen))
	}
	event := (*seen)[0]
	if event.PartitionKey != "site-a:42" {
		t.Fatalf("partition key = %q, want site-a:42", event.PartitionKey)
	}
	var payload map[string]any
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		t.Fatalf("Unmarshal(payload) error = %v", err)
	}
	if payload["user_id"].(float64) != 42 {
		t.Fatalf("payload user_id = %v, want 42", payload["user_id"])
	}
	if payload["reason"] != authlogic.AuthEventReasonSessionExpired {
		t.Fatalf("payload reason = %v, want %s", payload["reason"], authlogic.AuthEventReasonSessionExpired)
	}
	raw := string(event.Payload)
	for _, forbidden := range []string{"Demo_User", "session-jti"} {
		if strings.Contains(raw, forbidden) {
			t.Fatalf("payload leaked raw value %q: %s", forbidden, raw)
		}
	}
}

// TestEmitSecurityFailureEvent 确保签名和加密失败也进入 auth.security 脱敏事件。
func TestEmitSecurityFailureEvent(t *testing.T) {
	svcCtx, seen := newAuthMiddlewareEventService(t)
	ctx, _ := requestctx.New(context.Background())
	requestctx.SetRoute(ctx, "auth.login")
	requestctx.SetRequest(ctx, http.MethodPost, "/api/auth/login", "127.0.0.1")
	requestctx.SetTrace(ctx, "trace-id", "span-id")

	emitSecurityFailureEvent(ctx, svcCtx, authlogic.AuthEventReasonRequestDecryptFailed)

	if len(*seen) != 1 {
		t.Fatalf("collector events = %d, want 1", len(*seen))
	}
	var payload map[string]any
	if err := json.Unmarshal((*seen)[0].Payload, &payload); err != nil {
		t.Fatalf("Unmarshal(payload) error = %v", err)
	}
	if payload["action"] != authlogic.AuthEventActionSecurityFailed {
		t.Fatalf("payload action = %v, want %s", payload["action"], authlogic.AuthEventActionSecurityFailed)
	}
	if payload["reason"] != authlogic.AuthEventReasonRequestDecryptFailed {
		t.Fatalf("payload reason = %v, want %s", payload["reason"], authlogic.AuthEventReasonRequestDecryptFailed)
	}
	if payload["route"] != "auth.login" {
		t.Fatalf("payload route = %v, want auth.login", payload["route"])
	}
	raw := string((*seen)[0].Payload)
	if strings.Contains(raw, "127.0.0.1") {
		t.Fatalf("payload leaked raw client ip: %s", raw)
	}
}

func newAuthMiddlewareEventService(t *testing.T) (*svc.ServiceContext, *[]collectorx.Event) {
	t.Helper()
	cfg := config.Config{
		AppID:     "site-a",
		AppKey:    "event-secret",
		JwtSecret: "jwt-secret",
		Collector: config.CollectorConfig{
			Enabled:   true,
			Transport: "sync",
		},
	}
	manager, err := collectorx.New(config.CollectorConfig{
		Enabled:   true,
		Transport: "sync",
	}, nil)
	if err != nil {
		t.Fatalf("collectorx.New() error = %v", err)
	}
	seen := make([]collectorx.Event, 0, 1)
	if err := manager.RegisterProcessorFunc(authlogic.AuthCollectorBizType, func(ctx context.Context, events []collectorx.Event) ([]collectorx.ProcessResult, error) {
		seen = append(seen, events...)
		results := make([]collectorx.ProcessResult, 0, len(events))
		for _, event := range events {
			results = append(results, collectorx.ProcessResult{EventID: event.EventID, Success: true})
		}
		return results, nil
	}); err != nil {
		t.Fatalf("RegisterProcessorFunc() error = %v", err)
	}
	svcCtx := svc.NewServiceContext(cfg, "v1", svc.Dependencies{})
	svcCtx.Collector = manager
	return svcCtx, &seen
}
