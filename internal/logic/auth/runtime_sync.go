package auth

import (
	codes "api/common/codes"
	i18n "api/common/i18n"
	userlogic "api/internal/logic/user"
	"api/internal/model"
	"api/internal/svc"
	"api/internal/types"

	"github.com/Is999/go-utils/errors"
)

// SyncUserRuntime 同步后台直改业务用户表后必须由 API 自己维护的运行态缓存。
func (l *AuthLogic) SyncUserRuntime(req *types.UserRuntimeSyncReq) *types.BizResult {
	if err := req.Validate(); err != nil {
		return types.ParamErrorResult(err).
			WithError(err)
	}

	resp := &types.UserRuntimeSyncResp{
		UserID:                  req.ID,
		AuthVersion:             req.AuthVersion,
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
		db := l.Svc.WriteDB(svc.DatabaseMain)
		if db == nil {
			return types.ServerError(i18n.MsgKeyInternalError, errors.New("业务用户主库未初始化"), "AuthLogic.SyncUserRuntime 校验用户认证版本失败 user_id=%d", req.ID).ToBizResult()
		}
		user, err := model.FindUserByID(db, req.ID, l.Svc.CurrentConfig().User.RouteShardCount)
		if err != nil {
			return types.ServerError(i18n.MsgKeyInternalError, err, "AuthLogic.SyncUserRuntime 校验用户认证版本失败 user_id=%d", req.ID).ToBizResult()
		}
		if user == nil || user.AuthVersion != req.AuthVersion {
			err = errors.Errorf("用户认证版本未提交或不一致 user_id=%d expected=%d actual=%d", req.ID, req.AuthVersion, authVersionOf(user))
			return types.ServerError(i18n.MsgKeyInternalError, err, "AuthLogic.SyncUserRuntime 拒绝未提交的用户认证版本 user_id=%d", req.ID).ToBizResult()
		}
		if err := l.InvalidateUserSessions(req.ID, req.AuthVersion); err != nil {
			return types.ServerError(i18n.MsgKeyInternalError, err, "AuthLogic.SyncUserRuntime 失效用户登录态失败 user_id=%d", req.ID).ToBizResult()
		}
		resp.SessionsInvalidated = true
	}
	return types.NewBizResult(codes.UpdateSuccess).
		SetI18nMessage(i18n.MsgKeyUpdateSuccess).
		WithData(resp)
}

// authVersionOf 安全返回用户认证版本，用户不存在时返回零值。
func authVersionOf(user *model.User) uint64 {
	if user == nil {
		return 0
	}
	return user.AuthVersion
}
