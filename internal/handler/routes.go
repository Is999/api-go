package handler

import (
	"strings"

	authhandler "api/internal/handler/auth"
	confighandler "api/internal/handler/config"
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
	File        string                    // 模块注册和路由规格所在文件
	Method      string                    // 模块构造和路由规格入口
	Description string                    // 模块中文说明
	Routes      func() []shared.RouteSpec // 模块内置路由规格
}

// 内置路由模块名称常量，供规格表和显式构造入口复用。
const (
	// routeModuleHealth 表示健康检查路由模块。
	routeModuleHealth = "health"
	// routeModuleAuth 表示前台认证路由模块。
	routeModuleAuth = "auth"
	// routeModuleUser 表示前台用户路由模块。
	routeModuleUser = "user"
	// routeModuleConfig 表示运行期配置管理路由模块。
	routeModuleConfig = "config"
)

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

// NewHealthRouteModule 创建健康检查路由模块。
func NewHealthRouteModule() RouteModule {
	return newBuiltinRouteModule(routeModuleHealth)
}

// NewAuthRouteModule 创建前台认证路由模块。
func NewAuthRouteModule() RouteModule {
	return newBuiltinRouteModule(routeModuleAuth)
}

// NewUserRouteModule 创建前台用户路由模块。
func NewUserRouteModule() RouteModule {
	return newBuiltinRouteModule(routeModuleUser)
}

// NewConfigRouteModule 创建运行期配置管理路由模块。
func NewConfigRouteModule() RouteModule {
	return newBuiltinRouteModule(routeModuleConfig)
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

// builtinRouteModuleSpecs 是前台 API 内置 HTTP 路由模块的单一装配规格。
var builtinRouteModuleSpecs = []RouteModuleSpec{
	{
		Name:        routeModuleHealth,
		File:        "internal/handler/routes.go + internal/handler/health/routes.go",
		Method:      "handler.NewHealthRouteModule / health.RouteSpecs",
		Description: "注册健康检查路由",
		Routes:      healthhandler.RouteSpecs,
	},
	{
		Name:        routeModuleAuth,
		File:        "internal/handler/routes.go + internal/handler/auth/routes.go",
		Method:      "handler.NewAuthRouteModule / auth.RouteSpecs",
		Description: "注册前台认证路由",
		Routes:      authhandler.RouteSpecs,
	},
	{
		Name:        routeModuleUser,
		File:        "internal/handler/routes.go + internal/handler/user/routes.go",
		Method:      "handler.NewUserRouteModule / user.RouteSpecs",
		Description: "注册前台用户路由",
		Routes:      userhandler.RouteSpecs,
	},
	{
		Name:        routeModuleConfig,
		File:        "internal/handler/routes.go + internal/handler/config/routes.go",
		Method:      "handler.NewConfigRouteModule / config.RouteSpecs",
		Description: "注册内网运行期配置管理路由",
		Routes:      confighandler.RouteSpecs,
	},
}

// newBuiltinRouteModule 按内置模块名称构造路由模块，供显式 NewXxx 入口复用同一份规格。
func newBuiltinRouteModule(name string) RouteModule {
	spec, ok := builtinRouteModuleSpec(name)
	if !ok {
		panic("unknown builtin route module: " + name)
	}
	return newRouteModule(spec)
}

// builtinRouteModuleSpec 根据模块名称读取内置模块规格。
func builtinRouteModuleSpec(name string) (RouteModuleSpec, bool) {
	name = strings.TrimSpace(name)
	for _, spec := range builtinRouteModuleSpecs {
		if spec.Name == name {
			return spec, true
		}
	}
	return RouteModuleSpec{}, false
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
