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

	"github.com/zeromicro/go-zero/rest"
)

// RouteModule 描述一个可插拔 HTTP 路由模块。
type RouteModule interface {
	Name() string         // Name 返回路由模块名称
	Register(*RouteScope) // Register 注册当前模块路由
}

// RouteScope 表示路由模块注册时共享的上下文。
type RouteScope struct {
	Server         *rest.Server               // HTTP 服务实例
	ServiceContext *svc.ServiceContext        // 全局服务上下文
	AuthMiddleware *middleware.AuthMiddleware // 前台鉴权中间件
}

// RouteModuleFunc 允许通过函数快速声明路由模块。
type RouteModuleFunc struct {
	name     string            // 路由模块名称
	register func(*RouteScope) // 路由注册逻辑
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
func NewRouteModuleFunc(name string, register func(*RouteScope)) RouteModule {
	return RouteModuleFunc{name: strings.TrimSpace(name), register: register}
}

// Name 返回路由模块名称。
func (m RouteModuleFunc) Name() string {
	return m.name
}

// Register 执行路由注册逻辑。
func (m RouteModuleFunc) Register(scope *RouteScope) {
	if m.register != nil {
		m.register(scope)
	}
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
	return NewRouteModuleFunc(spec.Name, func(scope *RouteScope) {
		if spec.Routes == nil {
			return
		}
		shared.AddRouteSpecs(scope.Server, scope.ServiceContext, scope.AuthMiddleware, spec.Routes())
	})
}

// RegisterHandlers 统一注册全局中间件和各领域路由模块。
func RegisterHandlers(server *rest.Server, serverCtx *svc.ServiceContext, modules ...RouteModule) {
	// 中间件顺序固定为 outer recover -> trace -> access log -> inner recover：
	// 1. outer recover 兜底保护入口中间件自身异常；
	// 2. trace 创建上下文和 span；
	// 3. access log 使用 defer 在请求结束时统一收口；
	// 4. inner recover 最靠近业务 handler，把 panic 转成标准响应后交回上层记录。
	server.Use(middleware.NewRecoverMiddleware().Handle)
	server.Use(middleware.NewTraceMiddleware().Handle)
	server.Use(middleware.NewAccessLogMiddleware().Handle)
	server.Use(middleware.NewRecoverMiddleware().Handle)

	if len(modules) == 0 {
		modules = BuiltinRouteModules()
	}
	authMw := middleware.NewAuthMiddleware(serverCtx)
	scope := &RouteScope{
		Server:         server,
		ServiceContext: serverCtx,
		AuthMiddleware: authMw,
	}
	for _, module := range modules {
		if module == nil {
			continue
		}
		module.Register(scope)
	}
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
