package security

import (
	"strings"
	"testing"

	"api/internal/routealias"
)

// TestPolicyByRouteUnknownKeepsEmptyPolicy 验证对应场景符合预期。
func TestPolicyByRouteUnknownKeepsEmptyPolicy(t *testing.T) {
	policy := PolicyByRoute("unknown.route")
	if policy.RequestSign != nil || policy.ResponseSign != nil || len(policy.RequestCipher) != 0 || len(policy.ResponseCipher) != 0 {
		t.Fatalf("PolicyByRoute unknown = %+v, want empty policy", policy)
	}
}

// TestPolicyByRouteForLogoutUsesHeaderOnlySign 验证无业务字段的退出接口仍启用基础头验签。
func TestPolicyByRouteForLogoutUsesHeaderOnlySign(t *testing.T) {
	policy := PolicyByRoute(string(routealias.AuthLogout))
	if policy.RequestSign == nil || len(policy.RequestSign) != 0 {
		t.Fatalf("PolicyByRoute(auth.logout) request sign = %#v, want enabled empty fields", policy.RequestSign)
	}
}

// TestSignPoliciesExcludeLargeDisplayFields 校验描述和备注等展示性长文本不进入轻量签名。
func TestSignPoliciesExcludeLargeDisplayFields(t *testing.T) {
	for alias, policy := range RouteSecurityPolicies {
		for _, fields := range [][]string{policy.RequestSign, policy.ResponseSign} {
			for _, field := range fields {
				switch strings.ToLower(strings.TrimSpace(field)) {
				case "description", "reason", "remark":
					t.Fatalf("route %s signs large display field %q", alias, field)
				}
			}
		}
	}
}
