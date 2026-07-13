package security

import (
	"strings"

	"api/internal/routealias"
)

// RouteSecurityPolicy 定义单个路由的请求验签、响应回签与响应加密策略。
type RouteSecurityPolicy struct {
	RequestSign    []string // RequestSign 表示请求验签关键字段；nil 关闭验签，空切片只签基础头，禁止使用 *
	RequestCipher  []string // RequestCipher 表示请求允许解密的字段；禁止使用 cipher 整包加密
	ResponseSign   []string // ResponseSign 表示响应回签关键字段；nil 关闭回签，空切片只签基础头，禁止使用 *
	ResponseCipher []string // ResponseCipher 表示响应需要加密的字段路径；禁止使用 cipher 整包加密
}

// RouteSecurityPolicies 定义前台 API 的推荐安全策略，key 来自统一路由别名常量。
var RouteSecurityPolicies = map[routealias.Alias]RouteSecurityPolicy{
	// auth.register 保护注册账号、密码、联系方式和新会话 token。
	routealias.AuthRegister: {
		RequestSign:    []string{"username", "password", "nickname", "email", "phone"},
		RequestCipher:  []string{"password", "email", "phone"},
		ResponseSign:   []string{"token", "expiresAt", "user.email", "user.phone"},
		ResponseCipher: []string{"token", "user.email", "user.phone"},
	},
	// auth.login 保护登录身份、密码和响应 token。
	routealias.AuthLogin: {
		RequestSign:    []string{"identityType", "identityValue", "password"},
		RequestCipher:  []string{"identityValue", "password"},
		ResponseSign:   []string{"token", "expiresAt", "user.email", "user.phone"},
		ResponseCipher: []string{"token", "user.email", "user.phone"},
	},
	// auth.refresh 保护刷新后的访问 token。
	routealias.AuthRefresh: {
		ResponseSign:   []string{"token", "expiresAt"},
		ResponseCipher: []string{"token"},
	},
	// auth.logout 没有业务请求字段，使用 AppID、TraceID 与时间戳完成轻量验签。
	routealias.AuthLogout: {
		RequestSign: []string{},
	},
	// user.profile 对当前用户联系方式先回签再加密。
	routealias.UserProfile: {
		ResponseSign:   []string{"email", "phone"},
		ResponseCipher: []string{"email", "phone"},
	},
	// user.runtime.sync 走内网运维链路，不参与前台签名加密。
	routealias.UserRuntimeSync: {},
	// system.config_reload.status 走内网运维链路，不参与前台签名加密。
	routealias.SystemConfigReloadStatus: {},
	// system.config_reload.items 走内网运维链路，不参与前台签名加密。
	routealias.SystemConfigReloadItems: {},
	// system.config_reload.run 走内网运维链路，不参与前台签名加密。
	routealias.SystemConfigReloadRun: {},
}

// PolicyByRoute 根据路由别名读取统一安全策略。
func PolicyByRoute(route string) RouteSecurityPolicy {
	alias := routealias.Alias(strings.TrimSpace(route))
	if alias == "" || strings.EqualFold(string(alias), string(routealias.Ignore)) {
		return RouteSecurityPolicy{}
	}
	if policy, ok := RouteSecurityPolicies[alias]; ok {
		return policy
	}
	return RouteSecurityPolicy{}
}
