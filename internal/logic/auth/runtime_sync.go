package auth

import (
	"strings"

	codes "api/common/codes"
	i18n "api/common/i18n"
	userlogic "api/internal/logic/user"
	"api/internal/types"

	"github.com/Is999/go-utils/errors"
)

// SyncUserRuntime 同步后台直改前台用户表后必须由 API 自己维护的运行态缓存。
func (l *AuthLogic) SyncUserRuntime(req *types.UserRuntimeSyncReq) *types.BizResult {
	if req == nil || req.ID <= 0 {
		return types.NewBizResult(codes.ParamError).
			SetI18nMessage(i18n.MsgKeyParamError).
			WithError(errors.New("用户 ID 不能为空"))
	}
	if !req.Profile && !req.Sessions {
		req.Profile = true
	}

	resp := &types.UserRuntimeSyncResp{
		UserID:                  req.ID,
		Reason:                  strings.TrimSpace(req.Reason),
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
