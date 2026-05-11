package handler

import (
	"api/internal/handler/shared"
	"api/internal/middleware"
)

// RouteSecurityChain 表示路由实际挂载的安全链路。
type RouteSecurityChain = shared.RouteSecurityChain

// 路由安全链路枚举常量。
const (
	// RouteSecurityNone 表示路由不经过前台签名、加密或 JWT 链路。
	RouteSecurityNone = shared.RouteSecurityNone
	// RouteSecurityPublic 表示路由经过签名和加密链路，但不校验 JWT。
	RouteSecurityPublic = shared.RouteSecurityPublic
	// RouteSecurityAuth 表示路由必须校验 JWT 与 Redis session。
	RouteSecurityAuth = shared.RouteSecurityAuth
	// RouteSecurityInternal 表示路由必须校验 JWT、Redis session 和内网 Ops 令牌。
	RouteSecurityInternal = shared.RouteSecurityInternal
)

// RouteSecurityContract 描述内置路由别名对应的安全链路契约。
type RouteSecurityContract struct {
	Alias middleware.RouteAlias // 路由别名
	Chain RouteSecurityChain    // 安全链路
}

// DefaultRouteSecurityContracts 返回内置路由安全链路契约集合。
func DefaultRouteSecurityContracts() []RouteSecurityContract {
	specs := DefaultRouteSpecs()
	contracts := make([]RouteSecurityContract, 0, len(specs))
	for _, spec := range specs {
		contracts = append(contracts, RouteSecurityContract{
			Alias: spec.Meta.Alias,
			Chain: spec.Chain,
		})
	}
	return contracts
}
