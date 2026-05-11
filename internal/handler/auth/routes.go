package auth

import (
	"net/http"

	"api/internal/handler/shared"
	"api/internal/middleware"
	"api/internal/svc"

	"github.com/zeromicro/go-zero/rest"
)

// RouteSpecs 返回前台认证路由规格。
func RouteSpecs() []shared.RouteSpec {
	return []shared.RouteSpec{
		// POST /api/auth/register：前台用户注册，创建登录态前不校验 token。
		{
			Method:       http.MethodPost,
			Path:         "/api/auth/register",
			Meta:         shared.AuthRegister,
			DocumentPath: shared.RouteDocAuth,
			Chain:        shared.RouteSecurityPublic,
			Handler: func(svcCtx *svc.ServiceContext, authMw *middleware.AuthMiddleware) http.HandlerFunc {
				return authMw.PublicHandle(RegisterHandler(svcCtx), shared.AuthRegister.Alias)
			},
		},
		// POST /api/auth/login：前台用户登录，创建登录态前不校验 token。
		{
			Method:       http.MethodPost,
			Path:         "/api/auth/login",
			Meta:         shared.AuthLogin,
			DocumentPath: shared.RouteDocAuth,
			Chain:        shared.RouteSecurityPublic,
			Handler: func(svcCtx *svc.ServiceContext, authMw *middleware.AuthMiddleware) http.HandlerFunc {
				return authMw.PublicHandle(LoginHandler(svcCtx), shared.AuthLogin.Alias)
			},
		},
		// POST /api/auth/refresh：刷新访问令牌，必须校验当前 JWT 与 Redis session。
		{
			Method:       http.MethodPost,
			Path:         "/api/auth/refresh",
			Meta:         shared.AuthRefresh,
			DocumentPath: shared.RouteDocAuth,
			Chain:        shared.RouteSecurityAuth,
			Handler: func(svcCtx *svc.ServiceContext, authMw *middleware.AuthMiddleware) http.HandlerFunc {
				return authMw.Handle(RefreshHandler(svcCtx), shared.AuthRefresh.Alias)
			},
		},
		// POST /api/auth/logout：退出当前登录态，必须校验当前 JWT 与 Redis session。
		{
			Method:       http.MethodPost,
			Path:         "/api/auth/logout",
			Meta:         shared.AuthLogout,
			DocumentPath: shared.RouteDocAuth,
			Chain:        shared.RouteSecurityAuth,
			Handler: func(svcCtx *svc.ServiceContext, authMw *middleware.AuthMiddleware) http.HandlerFunc {
				return authMw.Handle(LogoutHandler(svcCtx), shared.AuthLogout.Alias)
			},
		},
	}
}

// RegisterRoutes 注册前台认证路由。
func RegisterRoutes(server *rest.Server, serverCtx *svc.ServiceContext, authMw *middleware.AuthMiddleware) {
	shared.AddRouteSpecs(server, serverCtx, authMw, RouteSpecs())
}
