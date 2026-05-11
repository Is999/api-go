package config

import (
	"net/http"

	"api/internal/handler/shared"
	"api/internal/middleware"
	"api/internal/svc"

	"github.com/zeromicro/go-zero/rest"
)

// 内网配置热加载路由路径常量。
const (
	// InternalConfigReloadStatusPath 表示内网查询配置热加载状态路由。
	InternalConfigReloadStatusPath = "/internal/system/config-reload/status"
	// InternalConfigReloadRunPath 表示内网手动触发配置热加载路由。
	InternalConfigReloadRunPath = "/internal/system/config-reload/run"
)

// RouteSpecs 返回内网配置热加载路由规格。
func RouteSpecs() []shared.RouteSpec {
	return []shared.RouteSpec{
		// GET /internal/system/config-reload/status：内网查询配置热加载状态。
		{
			Method:       http.MethodGet,
			Path:         InternalConfigReloadStatusPath,
			Meta:         shared.SystemConfigReloadStatus,
			DocumentPath: shared.RouteDocSystem,
			Chain:        shared.RouteSecurityInternal,
			Handler: func(svcCtx *svc.ServiceContext, authMw *middleware.AuthMiddleware) http.HandlerFunc {
				opsMw := middleware.NewOpsMiddleware(svcCtx)
				return authMw.Handle(opsMw.Handle(ConfigReloadStatusHandler(svcCtx)), shared.SystemConfigReloadStatus.Alias)
			},
		},
		// POST /internal/system/config-reload/run：内网手动触发配置热加载。
		{
			Method:       http.MethodPost,
			Path:         InternalConfigReloadRunPath,
			Meta:         shared.SystemConfigReloadRun,
			DocumentPath: shared.RouteDocSystem,
			Chain:        shared.RouteSecurityInternal,
			Handler: func(svcCtx *svc.ServiceContext, authMw *middleware.AuthMiddleware) http.HandlerFunc {
				opsMw := middleware.NewOpsMiddleware(svcCtx)
				return authMw.Handle(opsMw.Handle(RunConfigReloadHandler(svcCtx)), shared.SystemConfigReloadRun.Alias)
			},
		},
	}
}

// RegisterRoutes 注册运行期配置管理路由。
func RegisterRoutes(server *rest.Server, serverCtx *svc.ServiceContext, authMw *middleware.AuthMiddleware) {
	shared.AddRouteSpecs(server, serverCtx, authMw, RouteSpecs())
}
