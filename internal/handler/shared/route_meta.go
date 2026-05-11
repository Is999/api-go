package shared

import "api/internal/middleware"

// RouteAccess 表示路由访问边界。
type RouteAccess string

// 路由访问边界枚举常量。
const (
	// RouteAccessPublic 表示公开路由，不要求登录态。
	RouteAccessPublic RouteAccess = "public"
	// RouteAccessAuth 表示前台登录态路由。
	RouteAccessAuth RouteAccess = "auth"
	// RouteAccessInternal 表示内网运维路由。
	RouteAccessInternal RouteAccess = "internal"
)

// RouteMeta 描述一条业务路由的统一元数据。
type RouteMeta struct {
	Alias    middleware.RouteAlias // 统一路由别名
	Access   RouteAccess           // 访问边界：public/auth/internal
	Describe string                // 中文业务说明
}

// defaultRouteMetas 按声明顺序登记内置路由元数据。
var defaultRouteMetas []RouteMeta

// newRouteMeta 创建并登记路由元数据，避免 RouteMeta 变量和默认清单双份维护。
func newRouteMeta(alias middleware.RouteAlias, access RouteAccess, describe string) RouteMeta {
	meta := RouteMeta{Alias: alias, Access: access, Describe: describe}
	defaultRouteMetas = append(defaultRouteMetas, meta)
	return meta
}

// 内置路由元数据按模块集中声明，新增路由必须同步补充。
var (
	// HealthLive 表示存活检查路由。
	HealthLive = newRouteMeta("health.live", RouteAccessPublic, "存活检查")
	// HealthReady 表示就绪检查路由。
	HealthReady = newRouteMeta("health.ready", RouteAccessPublic, "就绪检查")
	// HealthMetrics 表示 Prometheus 指标抓取路由。
	HealthMetrics = newRouteMeta("health.metrics", RouteAccessPublic, "指标抓取")

	// AuthRegister 表示前台用户注册路由。
	AuthRegister = newRouteMeta("auth.register", RouteAccessPublic, "前台用户注册")
	// AuthLogin 表示前台用户登录路由。
	AuthLogin = newRouteMeta("auth.login", RouteAccessPublic, "前台用户登录")
	// AuthRefresh 表示访问令牌刷新路由。
	AuthRefresh = newRouteMeta("auth.refresh", RouteAccessAuth, "刷新访问令牌")
	// AuthLogout 表示前台用户退出登录路由。
	AuthLogout = newRouteMeta("auth.logout", RouteAccessAuth, "前台用户退出登录")

	// UserProfile 表示当前用户资料路由。
	UserProfile = newRouteMeta("user.profile", RouteAccessAuth, "获取当前用户资料")

	// SystemConfigReloadStatus 表示内网配置热加载状态查询路由。
	SystemConfigReloadStatus = newRouteMeta("system.config_reload.status", RouteAccessInternal, "内网查询配置热加载状态")
	// SystemConfigReloadRun 表示内网手动触发配置热加载路由。
	SystemConfigReloadRun = newRouteMeta("system.config_reload.run", RouteAccessInternal, "内网手动触发配置热加载")
)

// DefaultRouteMetas 返回内置路由元数据集合，供测试和文档防漂移复用。
func DefaultRouteMetas() []RouteMeta {
	out := make([]RouteMeta, len(defaultRouteMetas))
	copy(out, defaultRouteMetas)
	return out
}
