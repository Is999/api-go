package handler

import (
	"strings"

	"gozero_api/internal/middleware"
	"gozero_api/internal/security"

	"github.com/Is999/go-utils/errors"
)

// RouteSecurityManifestItem 描述前后端同步安全策略所需的单路由清单项。
type RouteSecurityManifestItem struct {
	Alias          middleware.RouteAlias `json:"alias"`          // 路由别名
	Method         string                `json:"method"`         // HTTP 方法
	Path           string                `json:"path"`           // HTTP 路径
	Access         RouteAccess           `json:"access"`         // 访问边界
	Chain          RouteSecurityChain    `json:"chain"`          // 实际安全链路
	Describe       string                `json:"describe"`       // 中文业务说明
	RequestSign    []string              `json:"requestSign"`    // 请求签名字段
	RequestCipher  []string              `json:"requestCipher"`  // 请求解密字段
	ResponseSign   []string              `json:"responseSign"`   // 响应回签字段
	ResponseCipher []string              `json:"responseCipher"` // 响应加密字段
	DocumentPath   string                `json:"documentPath"`   // 接口文档路径
}

// DefaultRouteSecurityManifest 返回内置路由的安全策略清单，供测试、文档和前端同步复用。
func DefaultRouteSecurityManifest() []RouteSecurityManifestItem {
	securityByAlias := defaultRouteSecurityContractsByAlias()
	contracts := DefaultRouteContracts()
	items := make([]RouteSecurityManifestItem, 0, len(contracts))
	for _, contract := range contracts {
		securityContract := securityByAlias[string(contract.Meta.Alias)]
		policy := security.PolicyByRoute(string(contract.Meta.Alias))
		items = append(items, RouteSecurityManifestItem{
			Alias:          contract.Meta.Alias,
			Method:         contract.Method,
			Path:           contract.Path,
			Access:         contract.Meta.Access,
			Chain:          securityContract.Chain,
			Describe:       contract.Meta.Describe,
			RequestSign:    cloneSecurityFields(policy.RequestSign),
			RequestCipher:  cloneSecurityFields(policy.RequestCipher),
			ResponseSign:   cloneSecurityFields(policy.ResponseSign),
			ResponseCipher: cloneSecurityFields(policy.ResponseCipher),
			DocumentPath:   contract.DocumentPath,
		})
	}
	return items
}

// ValidateDefaultRouteSecurityManifest 校验路由、链路和字段级安全策略没有漂移。
func ValidateDefaultRouteSecurityManifest() error {
	items := DefaultRouteSecurityManifest()
	if len(items) != len(DefaultRouteContracts()) {
		return errors.Errorf("路由安全清单数量不一致 manifest=%d contracts=%d", len(items), len(DefaultRouteContracts()))
	}
	securityByAlias := defaultRouteSecurityContractsByAlias()
	seenRoutes := make(map[string]struct{}, len(items))
	seenSecurityAliases := make(map[string]struct{}, len(securityByAlias))
	for _, item := range items {
		alias := strings.TrimSpace(string(item.Alias))
		if alias == "" || item.Method == "" || item.Path == "" || item.Access == "" || item.Chain == "" || item.DocumentPath == "" {
			return errors.Errorf("路由安全清单字段不完整: %+v", item)
		}
		if _, ok := securityByAlias[alias]; !ok {
			return errors.Errorf("路由安全清单缺少安全链路契约 alias=%s", alias)
		}
		seenSecurityAliases[alias] = struct{}{}
		routeKey := securityManifestRouteKey(item.Method, item.Path)
		if _, ok := seenRoutes[routeKey]; ok {
			return errors.Errorf("路由安全清单重复声明 route=%s", routeKey)
		}
		seenRoutes[routeKey] = struct{}{}
		if err := validateRouteSecurityManifestItem(item); err != nil {
			return errors.Tag(err)
		}
	}
	for alias := range securityByAlias {
		if _, ok := seenSecurityAliases[alias]; !ok {
			return errors.Errorf("路由安全清单未覆盖安全链路契约 alias=%s", alias)
		}
	}
	for alias := range security.RouteSecurityPolicies {
		if _, ok := seenSecurityAliases[alias]; !ok {
			return errors.Errorf("路由安全策略未进入清单 alias=%s", alias)
		}
	}
	return nil
}

func validateRouteSecurityManifestItem(item RouteSecurityManifestItem) error {
	alias := string(item.Alias)
	hasPolicy := hasExplicitRouteSecurityPolicy(alias)
	policyEmpty := routeSecurityManifestPolicyEmpty(item)
	switch item.Chain {
	case RouteSecurityNone:
		if hasPolicy {
			return errors.Errorf("无安全链路路由不能声明前台安全策略 alias=%s", alias)
		}
	case RouteSecurityInternal:
		if !hasPolicy {
			return errors.Errorf("内网路由必须显式声明空安全策略 alias=%s", alias)
		}
		if !policyEmpty {
			return errors.Errorf("内网路由必须跳过前台签名加密策略 alias=%s", alias)
		}
	case RouteSecurityPublic, RouteSecurityAuth:
		if !hasPolicy {
			return errors.Errorf("前台路由必须显式声明安全策略 alias=%s", alias)
		}
	default:
		return errors.Errorf("未知路由安全链路 alias=%s chain=%s", alias, item.Chain)
	}
	return validateRouteSecurityManifestFields(item)
}

func validateRouteSecurityManifestFields(item RouteSecurityManifestItem) error {
	groups := []struct {
		label     string
		fields    []string
		forbidden string
	}{
		{label: "request sign", fields: item.RequestSign, forbidden: security.SignFieldAll},
		{label: "request cipher", fields: item.RequestCipher, forbidden: security.CipherWholeBody},
		{label: "response sign", fields: item.ResponseSign, forbidden: security.SignFieldAll},
		{label: "response cipher", fields: item.ResponseCipher, forbidden: security.CipherWholeBody},
	}
	for _, group := range groups {
		if hasRouteSecurityManifestField(group.fields, group.forbidden) {
			return errors.Errorf("路由安全清单禁止全量处理 alias=%s group=%s field=%s", item.Alias, group.label, group.forbidden)
		}
		if err := security.ValidateSecurityFieldCount(group.fields, group.label); err != nil {
			return errors.Wrapf(err, "路由安全清单字段数量非法 alias=%s group=%s", item.Alias, group.label)
		}
	}
	return nil
}

func defaultRouteSecurityContractsByAlias() map[string]RouteSecurityContract {
	result := make(map[string]RouteSecurityContract, len(DefaultRouteSecurityContracts()))
	for _, contract := range DefaultRouteSecurityContracts() {
		result[string(contract.Alias)] = contract
	}
	return result
}

func cloneSecurityFields(fields []string) []string {
	if len(fields) == 0 {
		return nil
	}
	result := make([]string, len(fields))
	copy(result, fields)
	return result
}

func hasExplicitRouteSecurityPolicy(alias string) bool {
	_, ok := security.RouteSecurityPolicies[alias]
	return ok
}

func routeSecurityManifestPolicyEmpty(item RouteSecurityManifestItem) bool {
	return len(item.RequestSign) == 0 &&
		len(item.RequestCipher) == 0 &&
		len(item.ResponseSign) == 0 &&
		len(item.ResponseCipher) == 0
}

func hasRouteSecurityManifestField(fields []string, want string) bool {
	for _, field := range fields {
		if strings.EqualFold(strings.TrimSpace(field), want) {
			return true
		}
	}
	return false
}

func securityManifestRouteKey(method string, path string) string {
	return strings.ToUpper(strings.TrimSpace(method)) + " " + strings.TrimSpace(path)
}
