package handler

import (
	"net/http"

	"gozero_api/internal/middleware"
	"gozero_api/internal/svc"

	"github.com/zeromicro/go-zero/rest"
)

const (
	// InternalConfigReloadStatusPath 表示内网查询配置热加载状态路由。
	InternalConfigReloadStatusPath = "/internal/system/config-reload/status"
	// InternalConfigReloadRunPath 表示内网手动触发配置热加载路由。
	InternalConfigReloadRunPath = "/internal/system/config-reload/run"
)

// registerSystemRoutes 注册前台 API 框架运行态管理路由。
func registerSystemRoutes(server *rest.Server, serverCtx *svc.ServiceContext, authMw *middleware.AuthMiddleware) {
	opsMw := middleware.NewOpsMiddleware(serverCtx)
	addRoute(server, http.MethodGet, InternalConfigReloadStatusPath, authMw.Handle(opsMw.Handle(ConfigReloadStatusHandler(serverCtx)), SystemConfigReloadStatus.Alias))
	addRoute(server, http.MethodPost, InternalConfigReloadRunPath, authMw.Handle(opsMw.Handle(RunConfigReloadHandler(serverCtx)), SystemConfigReloadRun.Alias))
}
