package middleware

import (
	"context"
	"strings"

	"gozero_api/internal/logic"
	"gozero_api/internal/svc"
)

// emitSecurityFailureEvent 投递签名或加密链路失败事件。
func emitSecurityFailureEvent(ctx context.Context, svcCtx *svc.ServiceContext, reason string) {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		reason = logic.AuthEventReasonSecurityFailed
	}
	logic.RecordAuthEvent(ctx, svcCtx, logic.AuthEventInput{
		Action: logic.AuthEventActionSecurityFailed,
		Reason: reason,
	})
}
