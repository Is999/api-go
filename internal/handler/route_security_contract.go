package handler

import (
	"api/internal/handler/shared"
	"api/internal/middleware"
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

// RouteSecurityContract 描述内置路由别名对应的安全链路契约。
type RouteSecurityContract struct {
	Alias middleware.RouteAlias // 路由别名
	Chain RouteSecurityChain    // 安全链路
}

// DefaultRouteSecurityContracts 返回内置路由安全链路契约集合。
func DefaultRouteSecurityContracts() []RouteSecurityContract {
	return []RouteSecurityContract{
		{Alias: shared.HealthLive.Alias, Chain: RouteSecurityNone},
		{Alias: shared.HealthReady.Alias, Chain: RouteSecurityNone},
		{Alias: shared.HealthMetrics.Alias, Chain: RouteSecurityNone},
		{Alias: shared.AuthRegister.Alias, Chain: RouteSecurityPublic},
		{Alias: shared.AuthLogin.Alias, Chain: RouteSecurityPublic},
		{Alias: shared.AuthRefresh.Alias, Chain: RouteSecurityAuth},
		{Alias: shared.AuthLogout.Alias, Chain: RouteSecurityAuth},
		{Alias: shared.UserProfile.Alias, Chain: RouteSecurityAuth},
		{Alias: shared.SystemConfigReloadStatus.Alias, Chain: RouteSecurityInternal},
		{Alias: shared.SystemConfigReloadRun.Alias, Chain: RouteSecurityInternal},
	}
}
