package config

import (
	"net/http"

	"api/internal/handler/shared"
	"api/internal/middleware"
	"api/internal/svc"
)

// RouteSpecs 返回内网配置热加载路由规格。
func RouteSpecs() []shared.RouteSpec {
	return []shared.RouteSpec{
		{
			Method:       http.MethodGet,
			Path:         "/internal/system/config-reload/status", // 内网查询配置热加载状态。
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
			Path:         "/internal/system/config-reload/items", // 内网查询运行态配置项。
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
			Path:         "/internal/system/config-reload/run", // 内网手动触发配置热加载。
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
