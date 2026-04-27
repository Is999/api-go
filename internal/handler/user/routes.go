package user

import (
	"net/http"

	"api/internal/handler/shared"
	"api/internal/middleware"
	"api/internal/svc"

	"github.com/zeromicro/go-zero/rest"
)

// RegisterRoutes 注册前台用户路由。
func RegisterRoutes(server *rest.Server, serverCtx *svc.ServiceContext, authMw *middleware.AuthMiddleware) {
	server.AddRoute(rest.Route{
		Method:  http.MethodGet,
		Path:    "/api/user/profile",
		Handler: authMw.Handle(UserProfileHandler(serverCtx), shared.UserProfile.Alias),
	})
}
