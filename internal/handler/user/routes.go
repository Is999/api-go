package user

import (
	"net/http"

	"api/internal/handler/shared"
	"api/internal/middleware"
	"api/internal/svc"
)

// 内网用户运行态同步路由路径常量。
const (
	// InternalUserRuntimeSyncPath 表示内网同步单个前台用户缓存和会话的路由。
	InternalUserRuntimeSyncPath = "/internal/users/:id/runtime-sync"
)

// RouteSpecs 返回前台用户路由规格。
func RouteSpecs() []shared.RouteSpec {
	return []shared.RouteSpec{
		{
			Method:       http.MethodGet,
			Path:         "/api/user/profile", // 获取当前用户资料，必须校验前台登录态。
			Meta:         shared.UserProfile,
			DocumentPath: shared.RouteDocUser,
			Chain:        shared.RouteSecurityAuth,
			Handler: func(svcCtx *svc.ServiceContext, authMw *middleware.AuthMiddleware) http.HandlerFunc {
				return authMw.Handle(UserProfileHandler(svcCtx), shared.UserProfile.Alias)
			},
		},
		{
			Method:       http.MethodPost,
			Path:         InternalUserRuntimeSyncPath, // 内网同步后台直改用户表后的 API 运行态缓存。
			Meta:         shared.UserRuntimeSync,
			DocumentPath: shared.RouteDocUser,
			Chain:        shared.RouteSecurityInternal,
			Handler: func(svcCtx *svc.ServiceContext, _ *middleware.AuthMiddleware) http.HandlerFunc {
				opsMw := middleware.NewOpsMiddleware(svcCtx)
				return opsMw.Handle(UserRuntimeSyncHandler(svcCtx))
			},
		},
	}
}
