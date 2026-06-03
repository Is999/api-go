package auth

import (
	codes "api/common/codes"
	i18n "api/common/i18n"
	userlogic "api/internal/logic/user"
	"api/internal/types"
)

// SyncUserRuntime 同步后台直改业务用户表后必须由 API 自己维护的运行态缓存。
func (l *AuthLogic) SyncUserRuntime(req *types.UserRuntimeSyncReq) *types.BizResult {
	if err := req.Validate(); err != nil {
		return types.ParamErrorResult(err).
			WithError(err)
	}

	resp := &types.UserRuntimeSyncResp{
		UserID:                  req.ID,
		Reason:                  req.Reason,
		ProfileCacheInvalidated: false,
		SessionsInvalidated:     false,
	}
	if req.Profile {
		if err := userlogic.NewUserLogic(l.Ctx, l.Svc).DeleteUserProfileCache(req.ID); err != nil {
			return types.ServerError(i18n.MsgKeyInternalError, err, "AuthLogic.SyncUserRuntime 删除用户资料缓存失败 user_id=%d", req.ID).ToBizResult()
		}
		resp.ProfileCacheInvalidated = true
	}
	if req.Sessions {
		if err := l.InvalidateUserSessions(req.ID); err != nil {
			return types.ServerError(i18n.MsgKeyInternalError, err, "AuthLogic.SyncUserRuntime 失效用户登录态失败 user_id=%d", req.ID).ToBizResult()
		}
		resp.SessionsInvalidated = true
	}
	return types.NewBizResult(codes.UpdateSuccess).
		SetI18nMessage(i18n.MsgKeyUpdateSuccess).
		WithData(resp)
}
