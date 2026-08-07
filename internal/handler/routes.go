package handler

import (
	"strings"

	authhandler "api/internal/handler/auth"
	confighandler "api/internal/handler/config"
	docshandler "api/internal/handler/docs"
	healthhandler "api/internal/handler/health"
	"api/internal/handler/shared"
	userhandler "api/internal/handler/user"
	"api/internal/middleware"
	"api/internal/svc"

	"github.com/Is999/go-utils/errors"
	"github.com/zeromicro/go-zero/rest"
)

// RouteModule 描述一个可插拔 HTTP 路由模块。
type RouteModule interface {
	Name() string               // Name 返回路由模块名称
	Routes() []shared.RouteSpec // Routes 返回模块路由规格
}

// RouteModuleFunc 允许通过函数快速声明路由模块。
type RouteModuleFunc struct {
	name   string                    // 路由模块名称
	routes func() []shared.RouteSpec // 路由规格函数
}

// RouteModuleSpec 描述内置 HTTP 路由模块的注册、文档和路由规格来源。
type RouteModuleSpec struct {
	Name        string                    // 模块名称，必须在内置模块中唯一
	File        string                    // 模块路由规格所在文件
	Method      string                    // 模块路由规格入口
	Description string                    // 模块中文说明
	Routes      func() []shared.RouteSpec // 模块内置路由规格
}

// NewRouteModuleFunc 创建函数式路由模块。
func NewRouteModuleFunc(name string, routes func() []shared.RouteSpec) RouteModule {
	return RouteModuleFunc{name: strings.TrimSpace(name), routes: routes}
}

// Name 返回路由模块名称。
func (m RouteModuleFunc) Name() string {
	return m.name
}

// Routes 返回当前模块的路由规格。
func (m RouteModuleFunc) Routes() []shared.RouteSpec {
	if m.routes == nil {
		return nil
	}
	return m.routes()
}

// ComposeRouteModules 合并多组路由模块，保持注册顺序。
func ComposeRouteModules(groups ...[]RouteModule) []RouteModule {
	total := 0
	for _, group := range groups {
		total += len(group)
	}
	modules := make([]RouteModule, 0, total)
	for _, group := range groups {
		modules = append(modules, group...)
	}
	return modules
}

// BuiltinRouteModules 返回当前进程默认启用的路由模块集合。
func BuiltinRouteModules() []RouteModule {
	specs := BuiltinRouteModuleSpecs()
	modules := make([]RouteModule, 0, len(specs))
	for _, spec := range specs {
		modules = append(modules, newRouteModule(spec))
	}
	return modules
}

// BuiltinRouteModuleSpecs 返回内置 HTTP 路由模块规格，供启动装配和注册清单复用。
func BuiltinRouteModuleSpecs() []RouteModuleSpec {
	out := make([]RouteModuleSpec, len(builtinRouteModuleSpecs))
	copy(out, builtinRouteModuleSpecs)
	return out
}

// DefaultRouteSpecs 返回内置 HTTP 路由规格，顺序与内置模块注册顺序保持一致。
func DefaultRouteSpecs() []shared.RouteSpec {
	moduleSpecs := BuiltinRouteModuleSpecs()
	groups := make([][]shared.RouteSpec, 0, len(moduleSpecs))
	total := 0
	for _, moduleSpec := range moduleSpecs {
		if moduleSpec.Routes == nil {
			continue
		}
		group := moduleSpec.Routes()
		total += len(group)
		groups = append(groups, group)
	}
	specs := make([]shared.RouteSpec, 0, total)
	for _, group := range groups {
		specs = append(specs, group...)
	}
	return specs
}

// newRouteModule 根据模块规格构造可注册模块。
func newRouteModule(spec RouteModuleSpec) RouteModule {
	return NewRouteModuleFunc(spec.Name, func() []shared.RouteSpec {
		if spec.Routes == nil {
			return nil
		}
		return spec.Routes()
	})
}

// routeSpecsForServer 按监听器边界筛选路由，公网与内网路由不在同一 Server 注册。
func routeSpecsForServer(specs []shared.RouteSpec, internal bool) []shared.RouteSpec {
	filtered := make([]shared.RouteSpec, 0, len(specs))
	for _, spec := range specs {
		if (spec.Chain == shared.RouteSecurityInternal) != internal {
			continue
		}
		filtered = append(filtered, spec)
	}
	return filtered
}

// RegisterPublicHandlers 注册公网监听器的全局中间件、内置路由和扩展模块。
func RegisterPublicHandlers(server *rest.Server, serverCtx *svc.ServiceContext, modules ...RouteModule) error {
	moduleGroups := [][]RouteModule{BuiltinRouteModules(), modules}
	return RegisterPublicHandlersWithModules(server, serverCtx, ComposeRouteModules(moduleGroups...)...)
}

// RegisterInternalHandlers 注册内网监听器的全局中间件、内置路由和扩展模块。
func RegisterInternalHandlers(server *rest.Server, serverCtx *svc.ServiceContext, modules ...RouteModule) error {
	moduleGroups := [][]RouteModule{BuiltinRouteModules(), modules}
	return RegisterInternalHandlersWithModules(server, serverCtx, ComposeRouteModules(moduleGroups...)...)
}

// RegisterPublicHandlersWithModules 按完整模块清单注册公网路由。
func RegisterPublicHandlersWithModules(server *rest.Server, serverCtx *svc.ServiceContext, modules ...RouteModule) error {
	return registerHandlersWithModules(server, serverCtx, false, modules...)
}

// RegisterInternalHandlersWithModules 按完整模块清单注册内网路由。
func RegisterInternalHandlersWithModules(server *rest.Server, serverCtx *svc.ServiceContext, modules ...RouteModule) error {
	return registerHandlersWithModules(server, serverCtx, true, modules...)
}

// registerHandlersWithModules 注册公共中间件，并按监听器边界筛选所有模块路由。
func registerHandlersWithModules(server *rest.Server, serverCtx *svc.ServiceContext, internal bool, modules ...RouteModule) error {
	if server == nil {
		return errors.Errorf("注册 HTTP 路由时 Server 为空 internal=%t", internal)
	}
	if serverCtx == nil {
		return errors.Errorf("注册 HTTP 路由时 ServiceContext 为空 internal=%t", internal)
	}
	// 中间件顺序固定为 outer recover -> trace -> access log -> inner recover：
	// 1. outer recover 兜底保护入口中间件自身异常；
	// 2. trace 创建上下文和 span；
	// 3. access log 使用 defer 在请求结束时统一收口；
	// 4. inner recover 最靠近业务 handler，把 panic 转成标准响应后交回上层记录。
	server.Use(middleware.NewRecoverMiddleware().Handle)
	server.Use(middleware.NewTraceMiddleware(serverCtx).Handle)
	server.Use(middleware.NewAccessLogMiddleware().Handle)
	server.Use(middleware.NewRecoverMiddleware().Handle)

	authMw := middleware.NewAuthMiddleware(serverCtx)
	opsMw := middleware.NewOpsMiddleware(serverCtx)
	for _, module := range modules {
		if module == nil {
			continue
		}
		routes := routeSpecsForServer(module.Routes(), internal)
		if err := shared.AddRouteSpecs(server, serverCtx, authMw, opsMw, routes); err != nil {
			return errors.Wrapf(err, "注册路由模块失败 module=%s internal=%t", module.Name(), internal)
		}
	}
	return nil
}

// builtinRouteModuleSpecs 是前台 API 内置 HTTP 路由模块的单一装配规格。
var builtinRouteModuleSpecs = []RouteModuleSpec{
	{
		Name:        "health",
		File:        "internal/handler/health/routes.go",
		Method:      "health.RouteSpecs",
		Description: "注册健康检查路由",
		Routes:      healthhandler.RouteSpecs,
	},
	{
		Name:        "auth",
		File:        "internal/handler/auth/routes.go",
		Method:      "auth.RouteSpecs",
		Description: "注册前台认证路由",
		Routes:      authhandler.RouteSpecs,
	},
	{
		Name:        "user",
		File:        "internal/handler/user/routes.go",
		Method:      "user.RouteSpecs",
		Description: "注册前台用户路由",
		Routes:      userhandler.RouteSpecs,
	},
	{
		Name:        "config",
		File:        "internal/handler/config/routes.go",
		Method:      "config.RouteSpecs",
		Description: "注册内网运行期配置管理路由",
		Routes:      confighandler.RouteSpecs,
	},
	{
		Name:        "docs",
		File:        "internal/handler/docs/routes.go",
		Method:      "docs.RouteSpecs",
		Description: "注册内网 API 文档资源路由",
		Routes:      docshandler.RouteSpecs,
	},
}
