package middleware

import (
	"strings"

	codes "gozero_api/common/codes"
	"gozero_api/internal/logic"
	"gozero_api/internal/security"

	"github.com/Is999/go-utils/errors"
)

// resolveSecurityFailureCode 将安全链路失败原因转换为可观测的业务码。
func resolveSecurityFailureCode(reason string, fallback int, err error) int {
	reason = resolveSecurityFailureReason(reason, err)
	if errors.Is(err, security.ErrSecurityPayloadTooLarge) {
		return codes.SecurityPayloadTooLarge
	}
	switch strings.TrimSpace(reason) {
	case logic.AuthEventReasonSecurityAppIDInvalid:
		return codes.SecurityAppIDInvalid
	case logic.AuthEventReasonSecurityKeyUnavailable:
		return codes.SecurityKeyUnavailable
	case logic.AuthEventReasonSignatureFailed:
		return codes.SecuritySignatureFailed
	case logic.AuthEventReasonSecurityPayloadTooLarge:
		return codes.SecurityPayloadTooLarge
	case logic.AuthEventReasonResponseSignFailed:
		return codes.SecurityResponseSignFailed
	case logic.AuthEventReasonCryptoDisabled:
		return codes.SecurityCryptoDisabled
	case logic.AuthEventReasonRequestDecryptFailed:
		return codes.SecurityRequestDecryptFailed
	case logic.AuthEventReasonResponseEncryptFailed:
		return codes.SecurityResponseEncryptFailed
	default:
		if fallback != codes.Undefined {
			return fallback
		}
		return codes.AuthFailed
	}
}

// resolveSecurityFailureReason 将安全链路内部错误归并为稳定风控原因。
func resolveSecurityFailureReason(reason string, err error) string {
	if errors.Is(err, security.ErrSecurityPayloadTooLarge) {
		return logic.AuthEventReasonSecurityPayloadTooLarge
	}
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return logic.AuthEventReasonSecurityFailed
	}
	return reason
}
