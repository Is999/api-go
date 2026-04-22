package config

import (
	"net/http"

	"api/internal/handler/shared"
	"api/internal/middleware"
	"api/internal/svc"

	"github.com/zeromicro/go-zero/rest"
)

const (
	// InternalConfigReloadStatusPath 表示内网查询配置热加载状态路由。
	InternalConfigReloadStatusPath = "/internal/system/config-reload/status"
	// InternalConfigReloadRunPath 表示内网手动触发配置热加载路由。
	InternalConfigReloadRunPath = "/internal/system/config-reload/run"
)

// RegisterRoutes 注册运行期配置管理路由。
func RegisterRoutes(server *rest.Server, serverCtx *svc.ServiceContext, authMw *middleware.AuthMiddleware) {
	opsMw := middleware.NewOpsMiddleware(serverCtx)
	server.AddRoute(rest.Route{
		Method:  http.MethodGet,
		Path:    InternalConfigReloadStatusPath,
		Handler: authMw.Handle(opsMw.Handle(ConfigReloadStatusHandler(serverCtx)), shared.SystemConfigReloadStatus.Alias),
	})
	server.AddRoute(rest.Route{
		Method:  http.MethodPost,
		Path:    InternalConfigReloadRunPath,
		Handler: authMw.Handle(opsMw.Handle(RunConfigReloadHandler(serverCtx)), shared.SystemConfigReloadRun.Alias),
	})
}
