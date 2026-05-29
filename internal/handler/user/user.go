package user

import (
	"net/http"

	"api/internal/handler/shared"
	authlogic "api/internal/logic/auth"
	userlogic "api/internal/logic/user"
	"api/internal/svc"
	"api/internal/types"
)

// UserProfileHandler 获取当前用户资料。
func UserProfileHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		l := userlogic.NewUserLogic(r.Context(), svcCtx)
		shared.WriteBizResponse(w, r, l.Profile())
	}
}

// UserRuntimeSyncHandler 同步后台直改用户表后的 API 运行态缓存。
func UserRuntimeSyncHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return shared.RespHandler[types.UserRuntimeSyncReq](
		func(r *http.Request, svcCtx *svc.ServiceContext, req *types.UserRuntimeSyncReq) *types.BizResult {
			return authlogic.NewAuthLogic(r.Context(), svcCtx).SyncUserRuntime(req)
		},
	)(svcCtx)
}
