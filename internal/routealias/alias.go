package routealias

// Alias 是日志、鉴权、RouteMeta 和安全策略共用的稳定路由别名。
type Alias string

const (
	// Ignore 表示该路由跳过业务路由别名写入。
	Ignore Alias = "ignore"
)

const (
	// HealthLive 表示存活检查路由别名。
	HealthLive Alias = "health.live"
	// HealthReady 表示就绪检查路由别名。
	HealthReady Alias = "health.ready"
	// HealthMetrics 表示指标抓取路由别名。
	HealthMetrics Alias = "health.metrics"
)

const (
	// AuthRegister 表示前台用户注册路由别名。
	AuthRegister Alias = "auth.register"
	// AuthLogin 表示前台用户登录路由别名。
	AuthLogin Alias = "auth.login"
	// AuthRefresh 表示刷新访问令牌路由别名。
	AuthRefresh Alias = "auth.refresh"
	// AuthLogout 表示前台用户退出登录路由别名。
	AuthLogout Alias = "auth.logout"
)

const (
	// UserProfile 表示当前用户资料路由别名。
	UserProfile Alias = "user.profile"
	// UserRuntimeSync 表示内网同步前台用户运行态缓存路由别名。
	UserRuntimeSync Alias = "user.runtime.sync"
)

const (
	// SystemConfigReloadStatus 表示内网查询配置热加载状态路由别名。
	SystemConfigReloadStatus Alias = "system.config_reload.status"
	// SystemConfigReloadRun 表示内网手动触发配置热加载路由别名。
	SystemConfigReloadRun Alias = "system.config_reload.run"
	// SystemDocsFile 表示内网读取 API 接口文档二级资源路由别名。
	SystemDocsFile Alias = "system.docs.file"
	// SystemDocsNestedFile 表示内网读取 API 接口文档三级资源路由别名。
	SystemDocsNestedFile Alias = "system.docs.nested_file"
)
