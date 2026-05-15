package health

import (
	"net/http"

	"api/internal/handler/shared"
	"api/internal/middleware"
	"api/internal/svc"

	"github.com/zeromicro/go-zero/rest"
)

// RouteSpecs 返回基础健康检查路由规格。
func RouteSpecs() []shared.RouteSpec {
	return []shared.RouteSpec{
		// GET /api/live：存活检查，不访问外部依赖。
		{
			Method:        http.MethodGet,
			Path:          "/api/live",
			Meta:          shared.HealthLive,
			DocumentPath:  shared.RouteDocHealth,
			Chain:         shared.RouteSecurityNone,
			SkipAccessLog: true,
			Handler: func(svcCtx *svc.ServiceContext, _ *middleware.AuthMiddleware) http.HandlerFunc {
				return LiveHandler(svcCtx)
			},
		},
		// GET /api/ready：就绪检查，探测 MySQL/Redis 等关键依赖。
		{
			Method:        http.MethodGet,
			Path:          "/api/ready",
			Meta:          shared.HealthReady,
			DocumentPath:  shared.RouteDocHealth,
			Chain:         shared.RouteSecurityNone,
			SkipAccessLog: true,
			Handler: func(svcCtx *svc.ServiceContext, _ *middleware.AuthMiddleware) http.HandlerFunc {
				return ReadyHandler(svcCtx)
			},
		},
		// GET /api/metrics：Prometheus 指标抓取入口。
		{
			Method:        http.MethodGet,
			Path:          "/api/metrics",
			Meta:          shared.HealthMetrics,
			DocumentPath:  shared.RouteDocHealth,
			Chain:         shared.RouteSecurityNone,
			SkipAccessLog: true,
			Handler: func(_ *svc.ServiceContext, _ *middleware.AuthMiddleware) http.HandlerFunc {
				return MetricsHandler()
			},
		},
	}
}

// RegisterRoutes 注册基础健康检查路由。
func RegisterRoutes(server *rest.Server, serverCtx *svc.ServiceContext) {
	// 健康检查和指标入口供负载均衡、容器探针和监控抓取使用，不校验前台 token。
	shared.AddRouteSpecs(server, serverCtx, nil, RouteSpecs())
}
