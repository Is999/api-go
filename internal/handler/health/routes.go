package health

import (
	"net/http"

	"api/internal/svc"

	"github.com/zeromicro/go-zero/rest"
)

// RegisterRoutes 注册基础健康检查路由。
func RegisterRoutes(server *rest.Server, serverCtx *svc.ServiceContext) {
	// 健康检查和指标入口供负载均衡、容器探针和监控抓取使用，不校验前台 token。
	server.AddRoutes([]rest.Route{
		{
			Method:  http.MethodGet,
			Path:    "/api/live", // 存活检查，不访问外部依赖
			Handler: LiveHandler(serverCtx),
		},
		{
			Method:  http.MethodGet,
			Path:    "/api/ready", // 就绪检查，探测 MySQL/Redis 等关键依赖
			Handler: ReadyHandler(serverCtx),
		},
		{
			Method:  http.MethodGet,
			Path:    "/api/metrics", // Prometheus 指标抓取入口
			Handler: MetricsHandler(),
		},
	})
}
