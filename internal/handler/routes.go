package handler

import (
	"net/http"
	"strings"

	"gozero_api/internal/middleware"
	"gozero_api/internal/svc"

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
	return []RouteModule{
		NewHealthRouteModule(),
		NewAuthRouteModule(),
		NewUserRouteModule(),
		NewSystemRouteModule(),
	}
}

// NewHealthRouteModule 创建健康检查路由模块。
func NewHealthRouteModule() RouteModule {
	return NewRouteModuleFunc("health", func(scope *RouteScope) {
		registerHealthRoutes(scope.Server, scope.ServiceContext)
	})
}

// NewAuthRouteModule 创建前台认证路由模块。
func NewAuthRouteModule() RouteModule {
	return NewRouteModuleFunc("auth", func(scope *RouteScope) {
		registerAuthRoutes(scope.Server, scope.ServiceContext, scope.AuthMiddleware)
	})
}

// NewUserRouteModule 创建前台用户路由模块。
func NewUserRouteModule() RouteModule {
	return NewRouteModuleFunc("user", func(scope *RouteScope) {
		registerUserRoutes(scope.Server, scope.ServiceContext, scope.AuthMiddleware)
	})
}

// NewSystemRouteModule 创建框架运行态管理路由模块。
func NewSystemRouteModule() RouteModule {
	return NewRouteModuleFunc("system", func(scope *RouteScope) {
		registerSystemRoutes(scope.Server, scope.ServiceContext, scope.AuthMiddleware)
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

// addRoute 轻量包装 AddRoute，保持路由元数据和 Handler 绑定在同一处。
func addRoute(server *rest.Server, method, path string, handler http.HandlerFunc) {
	server.AddRoute(rest.Route{
		Method:  method,
		Path:    path,
		Handler: handler,
	})
}
