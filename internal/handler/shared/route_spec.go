package shared

import (
	"net/http"

	"api/internal/middleware"
	"api/internal/svc"

	"github.com/zeromicro/go-zero/rest"
)

// 接口文档路径常量，供路由规格、契约和文档漂移测试复用。
const (
	// RouteDocHealth 表示前台健康检查接口文档路径。
	RouteDocHealth = "docs/site/接口文档/前台系统/健康检查接口.md"
	// RouteDocAuth 表示前台认证接口文档路径。
	RouteDocAuth = "docs/site/接口文档/前台系统/认证接口.md"
	// RouteDocUser 表示前台用户接口文档路径。
	RouteDocUser = "docs/site/接口文档/前台系统/用户接口.md"
	// RouteDocSystem 表示前台系统接口文档路径。
	RouteDocSystem = "docs/site/接口文档/前台系统/系统接口.md"
)

// RouteSecurityChain 表示路由实际挂载的安全链路。
type RouteSecurityChain string

// 路由安全链路枚举常量。
const (
	// RouteSecurityNone 表示路由不经过前台签名、加密或 JWT 链路。
	RouteSecurityNone RouteSecurityChain = "none"
	// RouteSecurityPublic 表示路由经过签名和加密链路，但不校验 JWT。
	RouteSecurityPublic RouteSecurityChain = "public"
	// RouteSecurityAuth 表示路由必须校验 JWT 与 Redis session。
	RouteSecurityAuth RouteSecurityChain = "auth"
	// RouteSecurityInternal 表示路由必须校验 JWT、Redis session 和内网 Ops 令牌。
	RouteSecurityInternal RouteSecurityChain = "internal"
)

// RouteHandler 根据服务上下文和鉴权中间件构造真实 HTTP Handler。
type RouteHandler func(*svc.ServiceContext, *middleware.AuthMiddleware) http.HandlerFunc

// RouteSpec 是路由注册、契约、安全链路和文档同步的单一规格。
type RouteSpec struct {
	Method       string             // HTTP 方法
	Path         string             // HTTP 路径
	Meta         RouteMeta          // 路由元数据
	DocumentPath string             // 仓库根目录下的接口文档路径
	Chain        RouteSecurityChain // 实际安全链路
	Handler      RouteHandler       // 真实 Handler 构造函数
}

// RestRoute 将路由规格转换为 go-zero 路由。
func (s RouteSpec) RestRoute(svcCtx *svc.ServiceContext, authMw *middleware.AuthMiddleware) rest.Route {
	if s.Handler == nil {
		panic("路由规格缺少 Handler: " + s.Method + " " + s.Path)
	}
	return rest.Route{
		Method:  s.Method,
		Path:    s.Path,
		Handler: s.Handler(svcCtx, authMw),
	}
}

// AddRouteSpecs 按声明顺序注册一组路由规格。
func AddRouteSpecs(server *rest.Server, svcCtx *svc.ServiceContext, authMw *middleware.AuthMiddleware, specs []RouteSpec) {
	routes := make([]rest.Route, 0, len(specs))
	for _, spec := range specs {
		routes = append(routes, spec.RestRoute(svcCtx, authMw))
	}
	server.AddRoutes(routes)
}
