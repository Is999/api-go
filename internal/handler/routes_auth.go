package handler

import (
	"net/http"

	"gozero_api/internal/middleware"
	"gozero_api/internal/svc"

	"github.com/zeromicro/go-zero/rest"
)

// registerAuthRoutes 注册前台认证路由。
func registerAuthRoutes(server *rest.Server, serverCtx *svc.ServiceContext, authMw *middleware.AuthMiddleware) {
	// 注册和登录是登录态创建入口，仅补 route alias，不做 token 校验。
	addRoute(server, http.MethodPost, "/api/auth/register", authMw.PublicHandle(RegisterHandler(serverCtx), AuthRegister.Alias))
	addRoute(server, http.MethodPost, "/api/auth/login", authMw.PublicHandle(LoginHandler(serverCtx), AuthLogin.Alias))
	// 刷新和退出必须校验 JWT 与 Redis session，避免失效 token 继续换取登录态。
	addRoute(server, http.MethodPost, "/api/auth/refresh", authMw.Handle(RefreshHandler(serverCtx), AuthRefresh.Alias))
	addRoute(server, http.MethodPost, "/api/auth/logout", authMw.Handle(LogoutHandler(serverCtx), AuthLogout.Alias))
}
