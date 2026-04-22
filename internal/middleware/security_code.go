package middleware

import (
	"strings"

	codes "api/common/codes"
	authlogic "api/internal/logic/auth"
	"api/internal/security"

	"github.com/Is999/go-utils/errors"
)

// resolveSecurityFailureCode 将安全链路失败原因转换为可观测的业务码。
func resolveSecurityFailureCode(reason string, fallback int, err error) int {
	reason = resolveSecurityFailureReason(reason, err)
	if errors.Is(err, security.ErrSecurityPayloadTooLarge) {
		return codes.SecurityPayloadTooLarge
	}
	switch strings.TrimSpace(reason) {
	case authlogic.AuthEventReasonSecurityAppIDInvalid:
		return codes.SecurityAppIDInvalid
	case authlogic.AuthEventReasonSecurityKeyUnavailable:
		return codes.SecurityKeyUnavailable
	case authlogic.AuthEventReasonSignatureFailed:
		return codes.SecuritySignatureFailed
	case authlogic.AuthEventReasonSecurityPayloadTooLarge:
		return codes.SecurityPayloadTooLarge
	case authlogic.AuthEventReasonResponseSignFailed:
		return codes.SecurityResponseSignFailed
	case authlogic.AuthEventReasonCryptoDisabled:
		return codes.SecurityCryptoDisabled
	case authlogic.AuthEventReasonRequestDecryptFailed:
		return codes.SecurityRequestDecryptFailed
	case authlogic.AuthEventReasonResponseEncryptFailed:
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
		return authlogic.AuthEventReasonSecurityPayloadTooLarge
	}
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return authlogic.AuthEventReasonSecurityFailed
	}
	return reason
}
