package auth

import (
	"net/http"

	"api/internal/handler/shared"
	"api/internal/middleware"
	"api/internal/svc"

	"github.com/zeromicro/go-zero/rest"
)

// RegisterRoutes 注册前台认证路由。
func RegisterRoutes(server *rest.Server, serverCtx *svc.ServiceContext, authMw *middleware.AuthMiddleware) {
	// 注册和登录是登录态创建入口，仅补 route alias，不做 token 校验。
	server.AddRoute(rest.Route{
		Method:  http.MethodPost,
		Path:    "/api/auth/register",
		Handler: authMw.PublicHandle(RegisterHandler(serverCtx), shared.AuthRegister.Alias),
	})
	server.AddRoute(rest.Route{
		Method:  http.MethodPost,
		Path:    "/api/auth/login",
		Handler: authMw.PublicHandle(LoginHandler(serverCtx), shared.AuthLogin.Alias),
	})
	// 刷新和退出必须校验 JWT 与 Redis session，避免失效 token 继续换取登录态。
	server.AddRoute(rest.Route{
		Method:  http.MethodPost,
		Path:    "/api/auth/refresh",
		Handler: authMw.Handle(RefreshHandler(serverCtx), shared.AuthRefresh.Alias),
	})
	server.AddRoute(rest.Route{
		Method:  http.MethodPost,
		Path:    "/api/auth/logout",
		Handler: authMw.Handle(LogoutHandler(serverCtx), shared.AuthLogout.Alias),
	})
}
