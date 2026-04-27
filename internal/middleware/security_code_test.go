package middleware

import (
	"testing"

	codes "api/common/codes"
	authlogic "api/internal/logic/auth"
	"api/internal/security"

	"github.com/Is999/go-utils/errors"
)

func TestResolveSecurityFailureCodeMapsReasons(t *testing.T) {
	tests := []struct {
		name     string
		reason   string
		fallback int
		err      error
		want     int
	}{
		{
			name:     "app id invalid",
			reason:   authlogic.AuthEventReasonSecurityAppIDInvalid,
			fallback: codes.ParamError,
			want:     codes.SecurityAppIDInvalid,
		},
		{
			name:     "signature failed",
			reason:   authlogic.AuthEventReasonSignatureFailed,
			fallback: codes.AuthFailed,
			want:     codes.SecuritySignatureFailed,
		},
		{
			name:     "request decrypt failed",
			reason:   authlogic.AuthEventReasonRequestDecryptFailed,
			fallback: codes.AuthFailed,
			want:     codes.SecurityRequestDecryptFailed,
		},
		{
			name:     "response sign failed",
			reason:   authlogic.AuthEventReasonResponseSignFailed,
			fallback: codes.InternalError,
			want:     codes.SecurityResponseSignFailed,
		},
		{
			name:     "response encrypt failed",
			reason:   authlogic.AuthEventReasonResponseEncryptFailed,
			fallback: codes.InternalError,
			want:     codes.SecurityResponseEncryptFailed,
		},
		{
			name:     "fallback",
			reason:   "custom_reason",
			fallback: codes.InternalError,
			want:     codes.InternalError,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := resolveSecurityFailureCode(tt.reason, tt.fallback, tt.err); got != tt.want {
				t.Fatalf("resolveSecurityFailureCode() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestResolveSecurityFailureCodePrefersPayloadLimit(t *testing.T) {
	err := errors.Wrapf(security.ErrSecurityPayloadTooLarge, "响应字段超过上限")
	got := resolveSecurityFailureCode(authlogic.AuthEventReasonResponseSignFailed, codes.InternalError, err)
	if got != codes.SecurityPayloadTooLarge {
		t.Fatalf("resolveSecurityFailureCode() = %d, want %d", got, codes.SecurityPayloadTooLarge)
	}
}

func TestResolveSecurityFailureReasonPrefersPayloadLimit(t *testing.T) {
	err := errors.Wrapf(security.ErrSecurityPayloadTooLarge, "请求字段超过上限")
	got := resolveSecurityFailureReason(authlogic.AuthEventReasonSignatureFailed, err)
	if got != authlogic.AuthEventReasonSecurityPayloadTooLarge {
		t.Fatalf("resolveSecurityFailureReason() = %q, want %q", got, authlogic.AuthEventReasonSecurityPayloadTooLarge)
	}
}

func TestResolveSecurityFailureReasonFallback(t *testing.T) {
	if got := resolveSecurityFailureReason("", nil); got != authlogic.AuthEventReasonSecurityFailed {
		t.Fatalf("resolveSecurityFailureReason(empty) = %q, want %q", got, authlogic.AuthEventReasonSecurityFailed)
	}
	if got := resolveSecurityFailureReason(" custom_reason ", nil); got != "custom_reason" {
		t.Fatalf("resolveSecurityFailureReason(custom) = %q, want custom_reason", got)
	}
}
