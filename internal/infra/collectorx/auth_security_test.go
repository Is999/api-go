package collectorx

import (
	"context"
	"encoding/json"
	"testing"

	"api/internal/config"
)

// TestRegisterDefaultProcessorsEnqueuesAuthSecurity 确保内置认证风控 Processor 可消费 sync 事件。
func TestRegisterDefaultProcessorsEnqueuesAuthSecurity(t *testing.T) {
	manager, err := New(config.CollectorConfig{
		Enabled:   true,
		Transport: "sync",
	}, nil)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if err := RegisterDefaultProcessors(manager); err != nil {
		t.Fatalf("RegisterDefaultProcessors() error = %v", err)
	}

	eventID, err := manager.Enqueue(context.Background(), Event{
		BizType: BizTypeAuthSecurity,
		Payload: json.RawMessage(`{
			"action":"login_failed",
			"reason":"invalid_password",
			"app_id":"site-a"
		}`),
	})
	if err != nil {
		t.Fatalf("Enqueue() error = %v", err)
	}
	if eventID == "" {
		t.Fatal("event id is empty")
	}
}

// TestAuthSecurityProcessorRejectsInvalidPayload 确保异常事件按单条结果失败，不中断批处理。
func TestAuthSecurityProcessorRejectsInvalidPayload(t *testing.T) {
	processor := NewAuthSecurityProcessor()
	results, err := processor.ProcessBatch(context.Background(), []Event{
		{
			EventID: "bad",
			BizType: BizTypeAuthSecurity,
			Payload: json.RawMessage(`{bad`),
		},
		{
			EventID: "ok",
			BizType: BizTypeAuthSecurity,
			Payload: json.RawMessage(`{"action":"auth_failed","reason":"token_invalid","app_id":"site-a"}`),
		},
	})
	if err != nil {
		t.Fatalf("ProcessBatch() error = %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("results len = %d, want 2", len(results))
	}
	if results[0].Success || results[0].Error == "" {
		t.Fatalf("first result = %+v, want failed", results[0])
	}
	if !results[1].Success || results[1].Error != "" {
		t.Fatalf("second result = %+v, want success", results[1])
	}
}

// TestNormalizeAuthSecurityLabels 确保认证事件指标标签不会被异常值撑爆。
func TestNormalizeAuthSecurityLabels(t *testing.T) {
	if got := normalizeAuthSecurityAction("login_success"); got != "login_success" {
		t.Fatalf("normalizeAuthSecurityAction() = %q", got)
	}
	if got := normalizeAuthSecurityAction("dynamic_action"); got != authSecurityLabelOther {
		t.Fatalf("normalizeAuthSecurityAction(dynamic) = %q, want other", got)
	}
	if got := normalizeAuthSecurityReason(""); got != authSecurityLabelUnknown {
		t.Fatalf("normalizeAuthSecurityReason(empty) = %q, want unknown", got)
	}
	if got := normalizeAuthSecurityReason("security_payload_too_large"); got != authSecurityReasonSecurityPayloadTooLarge {
		t.Fatalf("normalizeAuthSecurityReason(payload limit) = %q, want %q", got, authSecurityReasonSecurityPayloadTooLarge)
	}
	if got := normalizeAuthSecurityAppID("site-a"); got != "site-a" {
		t.Fatalf("normalizeAuthSecurityAppID() = %q", got)
	}
	if got := normalizeAuthSecurityAppID("site/a"); got != authSecurityLabelOther {
		t.Fatalf("normalizeAuthSecurityAppID(invalid) = %q, want other", got)
	}
}

// TestNormalizeAuthSecurityCategory 确保认证安全指标按低基数分类聚合。
func TestNormalizeAuthSecurityCategory(t *testing.T) {
	tests := []struct {
		name   string
		reason string
		want   string
	}{
		{
			name:   "auth",
			reason: authSecurityReasonInvalidPassword,
			want:   authSecurityCategoryAuth,
		},
		{
			name:   "token",
			reason: authSecurityReasonTokenInvalid,
			want:   authSecurityCategoryToken,
		},
		{
			name:   "rate limit",
			reason: authSecurityReasonLoginIPRateLimited,
			want:   authSecurityCategoryRateLimit,
		},
		{
			name:   "security client",
			reason: authSecurityReasonSignatureFailed,
			want:   authSecurityCategorySecurityClient,
		},
		{
			name:   "security config",
			reason: authSecurityReasonSecurityKeyUnavailable,
			want:   authSecurityCategorySecurityConfig,
		},
		{
			name:   "payload limit",
			reason: authSecurityReasonSecurityPayloadTooLarge,
			want:   authSecurityCategorySecurityPayloadLimit,
		},
		{
			name:   "security response",
			reason: authSecurityReasonResponseEncryptFailed,
			want:   authSecurityCategorySecurityResponse,
		},
		{
			name:   "session lifecycle",
			reason: authSecurityReasonSessionRotated,
			want:   authSecurityCategorySessionLifecycle,
		},
		{
			name:   "unknown",
			reason: "",
			want:   authSecurityLabelUnknown,
		},
		{
			name:   "other",
			reason: "dynamic_reason",
			want:   authSecurityLabelOther,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := normalizeAuthSecurityCategory(tt.reason); got != tt.want {
				t.Fatalf("normalizeAuthSecurityCategory() = %q, want %q", got, tt.want)
			}
		})
	}
}
