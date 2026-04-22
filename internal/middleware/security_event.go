package middleware

import (
	"context"
	"strings"

	authlogic "api/internal/logic/auth"
	"api/internal/svc"
)

// emitSecurityFailureEvent 投递签名或加密链路失败事件。
func emitSecurityFailureEvent(ctx context.Context, svcCtx *svc.ServiceContext, reason string) {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		reason = authlogic.AuthEventReasonSecurityFailed
	}
	authlogic.RecordAuthEvent(ctx, svcCtx, authlogic.AuthEventInput{
		Action: authlogic.AuthEventActionSecurityFailed,
		Reason: reason,
	})
}
