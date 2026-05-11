package user

import (
	"net/http"

	"api/internal/handler/shared"
	"api/internal/middleware"
	"api/internal/svc"

	"github.com/zeromicro/go-zero/rest"
)

// RouteSpecs 返回前台用户路由规格。
func RouteSpecs() []shared.RouteSpec {
	return []shared.RouteSpec{
		// GET /api/user/profile：获取当前用户资料，必须校验前台登录态。
		{
			Method:       http.MethodGet,
			Path:         "/api/user/profile",
			Meta:         shared.UserProfile,
			DocumentPath: shared.RouteDocUser,
			Chain:        shared.RouteSecurityAuth,
			Handler: func(svcCtx *svc.ServiceContext, authMw *middleware.AuthMiddleware) http.HandlerFunc {
				return authMw.Handle(UserProfileHandler(svcCtx), shared.UserProfile.Alias)
			},
		},
	}
}

// RegisterRoutes 注册前台用户路由。
func RegisterRoutes(server *rest.Server, serverCtx *svc.ServiceContext, authMw *middleware.AuthMiddleware) {
	shared.AddRouteSpecs(server, serverCtx, authMw, RouteSpecs())
}
