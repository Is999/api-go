package handler

import "api/internal/handler/shared"

// routeMetaAccessByAlias 返回路由测试辅助数据。
func routeMetaAccessByAlias() map[string]shared.RouteAccess {
	result := make(map[string]shared.RouteAccess, len(shared.DefaultRouteMetas()))
	for _, meta := range shared.DefaultRouteMetas() {
		result[string(meta.Alias)] = meta.Access
	}
	return result
}
