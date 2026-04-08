package handler

import (
	"net/http"

	"gozero_api/internal/middleware"
	"gozero_api/internal/svc"

	"github.com/zeromicro/go-zero/rest"
)

// registerUserRoutes 注册前台用户路由。
func registerUserRoutes(server *rest.Server, serverCtx *svc.ServiceContext, authMw *middleware.AuthMiddleware) {
	addRoute(server, http.MethodGet, "/api/user/profile", authMw.Handle(UserProfileHandler(serverCtx), UserProfile.Alias))
}
