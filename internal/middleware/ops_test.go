package middleware

import (
	"net/http/httptest"
	"testing"

	"api/internal/config"
)

// TestValidateConfigReloadOpsRequiresToken 确保未配置运维令牌时默认拒绝热加载接口。
func TestValidateConfigReloadOpsRequiresToken(t *testing.T) {
	req := httptest.NewRequest("GET", "/internal/system/config-reload/status", nil)
	req.Header.Set(HeaderOpsToken, "token")
	if err := validateConfigReloadOps(req, config.OpsConfig{}); err == nil {
		t.Fatal("期望未配置运维令牌返回错误，实际为 nil")
	}
}

// TestValidateConfigReloadOpsAcceptsPrivateIPWithoutWhitelist 确保空白名单默认只放行内网来源。
func TestValidateConfigReloadOpsAcceptsPrivateIPWithoutWhitelist(t *testing.T) {
	req := httptest.NewRequest("GET", "/internal/system/config-reload/status", nil)
	req.RemoteAddr = "172.16.1.10:12345"
	req.Header.Set(HeaderOpsToken, "ops-token")
	err := validateConfigReloadOps(req, config.OpsConfig{
		ConfigReloadToken: "ops-token",
	})
	if err != nil {
		t.Fatalf("validateConfigReloadOps() error = %v", err)
	}
}

// TestValidateConfigReloadOpsAcceptsTokenAndCIDR 确保运维令牌和 CIDR 白名单同时命中时允许访问。
func TestValidateConfigReloadOpsAcceptsTokenAndCIDR(t *testing.T) {
	req := httptest.NewRequest("GET", "/internal/system/config-reload/status", nil)
	req.RemoteAddr = "10.1.2.3:12345"
	req.Header.Set(HeaderOpsToken, "ops-token")
	err := validateConfigReloadOps(req, config.OpsConfig{
		ConfigReloadToken:      "ops-token",
		ConfigReloadAllowedIPs: []string{"10.1.0.0/16"},
	})
	if err != nil {
		t.Fatalf("validateConfigReloadOps() error = %v", err)
	}
}

// TestValidateConfigReloadOpsRejectsInvalidIP 确保来源 IP 不在白名单时拒绝访问。
func TestValidateConfigReloadOpsRejectsInvalidIP(t *testing.T) {
	req := httptest.NewRequest("GET", "/internal/system/config-reload/status", nil)
	req.RemoteAddr = "192.168.1.10:12345"
	req.Header.Set(HeaderOpsToken, "ops-token")
	err := validateConfigReloadOps(req, config.OpsConfig{
		ConfigReloadToken:      "ops-token",
		ConfigReloadAllowedIPs: []string{"10.1.0.0/16"},
	})
	if err == nil {
		t.Fatal("期望来源 IP 不允许返回错误，实际为 nil")
	}
}

// TestValidateConfigReloadOpsRejectsPublicIP 确保公网来源即使带正确令牌也不允许访问。
func TestValidateConfigReloadOpsRejectsPublicIP(t *testing.T) {
	req := httptest.NewRequest("GET", "/internal/system/config-reload/status", nil)
	req.RemoteAddr = "8.8.8.8:12345"
	req.Header.Set(HeaderOpsToken, "ops-token")
	err := validateConfigReloadOps(req, config.OpsConfig{
		ConfigReloadToken: "ops-token",
	})
	if err == nil {
		t.Fatal("期望公网来源返回错误，实际为 nil")
	}
}

// TestValidateConfigReloadOpsRejectsPublicAllowedIP 确保公网白名单误配不会绕过内网边界。
func TestValidateConfigReloadOpsRejectsPublicAllowedIP(t *testing.T) {
	req := httptest.NewRequest("GET", "/internal/system/config-reload/status", nil)
	req.RemoteAddr = "8.8.8.8:12345"
	req.Header.Set(HeaderOpsToken, "ops-token")
	err := validateConfigReloadOps(req, config.OpsConfig{
		ConfigReloadToken:      "ops-token",
		ConfigReloadAllowedIPs: []string{"8.8.8.8"},
	})
	if err == nil {
		t.Fatal("期望公网白名单误配仍返回错误，实际为 nil")
	}
}

// TestValidateConfigReloadOpsRejectsForwardedPublicIP 确保反代转发公网来源时拒绝访问。
func TestValidateConfigReloadOpsRejectsForwardedPublicIP(t *testing.T) {
	req := httptest.NewRequest("GET", "/internal/system/config-reload/status", nil)
	req.RemoteAddr = "10.0.0.10:12345"
	req.Header.Set("X-Forwarded-For", "8.8.8.8, 10.0.0.10")
	req.Header.Set(HeaderOpsToken, "ops-token")
	err := validateConfigReloadOps(req, config.OpsConfig{
		ConfigReloadToken: "ops-token",
	})
	if err == nil {
		t.Fatal("期望转发头包含公网来源返回错误，实际为 nil")
	}
}

// TestValidateConfigReloadOpsAcceptsForwardedPrivateIP 确保反代转发内网来源时仍可访问。
func TestValidateConfigReloadOpsAcceptsForwardedPrivateIP(t *testing.T) {
	req := httptest.NewRequest("GET", "/internal/system/config-reload/status", nil)
	req.RemoteAddr = "10.0.0.10:12345"
	req.Header.Set("X-Forwarded-For", "10.1.2.3, 10.0.0.10")
	req.Header.Set(HeaderOpsToken, "ops-token")
	err := validateConfigReloadOps(req, config.OpsConfig{
		ConfigReloadToken: "ops-token",
	})
	if err != nil {
		t.Fatalf("validateConfigReloadOps() error = %v", err)
	}
}
