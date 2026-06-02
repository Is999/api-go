package config

import (
	"net/http"

	"api/internal/handler/shared"
	"api/internal/middleware"
	"api/internal/svc"
)

// 内网配置热加载路由路径常量。
const (
	// InternalConfigReloadStatusPath 表示内网查询配置热加载状态路由。
	InternalConfigReloadStatusPath = "/internal/system/config-reload/status"
	// InternalConfigReloadItemsPath 表示内网查询运行态配置项路由。
	InternalConfigReloadItemsPath = "/internal/system/config-reload/items"
	// InternalConfigReloadRunPath 表示内网手动触发配置热加载路由。
	InternalConfigReloadRunPath = "/internal/system/config-reload/run"
)

// RouteSpecs 返回内网配置热加载路由规格。
func RouteSpecs() []shared.RouteSpec {
	return []shared.RouteSpec{
		{
			Method:       http.MethodGet,
			Path:         InternalConfigReloadStatusPath, // 内网查询配置热加载状态。
			Meta:         shared.SystemConfigReloadStatus,
			DocumentPath: shared.RouteDocSystem,
			Chain:        shared.RouteSecurityInternal,
			Handler: func(svcCtx *svc.ServiceContext, _ *middleware.AuthMiddleware) http.HandlerFunc {
				opsMw := middleware.NewOpsMiddleware(svcCtx)
				return opsMw.Handle(ConfigReloadStatusHandler(svcCtx))
			},
		},
		{
			Method:       http.MethodGet,
			Path:         InternalConfigReloadItemsPath, // 内网查询运行态配置项。
			Meta:         shared.SystemConfigReloadItems,
			DocumentPath: shared.RouteDocSystem,
			Chain:        shared.RouteSecurityInternal,
			Handler: func(svcCtx *svc.ServiceContext, _ *middleware.AuthMiddleware) http.HandlerFunc {
				opsMw := middleware.NewOpsMiddleware(svcCtx)
				return opsMw.Handle(ConfigReloadItemsHandler(svcCtx))
			},
		},
		{
			Method:       http.MethodPost,
			Path:         InternalConfigReloadRunPath, // 内网手动触发配置热加载。
			Meta:         shared.SystemConfigReloadRun,
			DocumentPath: shared.RouteDocSystem,
			Chain:        shared.RouteSecurityInternal,
			Handler: func(svcCtx *svc.ServiceContext, _ *middleware.AuthMiddleware) http.HandlerFunc {
				opsMw := middleware.NewOpsMiddleware(svcCtx)
				return opsMw.Handle(RunConfigReloadHandler(svcCtx))
			},
		},
	}
}
