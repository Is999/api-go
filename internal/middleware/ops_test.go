package middleware

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

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
	req := newSignedOpsTestRequest("GET", "/internal/system/config-reload/status", "ops-token", nil)
	req.RemoteAddr = "172.16.1.10:12345"
	err := validateConfigReloadOps(req, config.OpsConfig{
		ConfigReloadToken: "ops-token",
	})
	if err != nil {
		t.Fatalf("validateConfigReloadOps() error = %v", err)
	}
}

// TestValidateConfigReloadOpsAcceptsTokenAndCIDR 确保运维令牌和 CIDR 白名单同时命中时允许访问。
func TestValidateConfigReloadOpsAcceptsTokenAndCIDR(t *testing.T) {
	req := newSignedOpsTestRequest("GET", "/internal/system/config-reload/status", "ops-token", nil)
	req.RemoteAddr = "10.1.2.3:12345"
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
	req := newSignedOpsTestRequest("GET", "/internal/system/config-reload/status", "ops-token", nil)
	req.RemoteAddr = "192.168.1.10:12345"
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
	req := newSignedOpsTestRequest("GET", "/internal/system/config-reload/status", "ops-token", nil)
	req.RemoteAddr = "8.8.8.8:12345"
	err := validateConfigReloadOps(req, config.OpsConfig{
		ConfigReloadToken: "ops-token",
	})
	if err == nil {
		t.Fatal("期望公网来源返回错误，实际为 nil")
	}
}

// TestValidateConfigReloadOpsRejectsPublicAllowedIP 确保公网白名单误配不会绕过内网边界。
func TestValidateConfigReloadOpsRejectsPublicAllowedIP(t *testing.T) {
	req := newSignedOpsTestRequest("GET", "/internal/system/config-reload/status", "ops-token", nil)
	req.RemoteAddr = "8.8.8.8:12345"
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
	req := newSignedOpsTestRequest("GET", "/internal/system/config-reload/status", "ops-token", nil)
	req.RemoteAddr = "10.0.0.10:12345"
	req.Header.Set("X-Forwarded-For", "8.8.8.8, 10.0.0.10")
	err := validateConfigReloadOps(req, config.OpsConfig{
		ConfigReloadToken: "ops-token",
	})
	if err == nil {
		t.Fatal("期望转发头包含公网来源返回错误，实际为 nil")
	}
}

// TestValidateConfigReloadOpsRejectsRealPublicIP 确保 X-Real-IP 中的公网来源不能访问内网接口。
func TestValidateConfigReloadOpsRejectsRealPublicIP(t *testing.T) {
	req := newSignedOpsTestRequest("GET", "/internal/system/config-reload/status", "ops-token", nil)
	req.RemoteAddr = "10.0.0.10:12345"
	req.Header.Set("X-Real-IP", "8.8.8.8")
	err := validateConfigReloadOps(req, config.OpsConfig{
		ConfigReloadToken: "ops-token",
	})
	if err == nil {
		t.Fatal("期望 X-Real-IP 公网来源返回错误，实际为 nil")
	}
}

// TestValidateConfigReloadOpsRejectsSpoofedPrivateForwardedIP 确保公网直连时不能伪造私网转发头绕过来源校验。
func TestValidateConfigReloadOpsRejectsSpoofedPrivateForwardedIP(t *testing.T) {
	req := newSignedOpsTestRequest("GET", "/internal/system/config-reload/status", "ops-token", nil)
	req.RemoteAddr = "8.8.8.8:12345"
	req.Header.Set("X-Forwarded-For", "10.1.2.3")
	err := validateConfigReloadOps(req, config.OpsConfig{
		ConfigReloadToken: "ops-token",
	})
	if err == nil {
		t.Fatal("期望公网来源伪造私网 X-Forwarded-For 返回错误，实际为 nil")
	}
}

// TestValidateConfigReloadOpsAcceptsForwardedPrivateIP 确保反代转发内网来源时仍可访问。
func TestValidateConfigReloadOpsAcceptsForwardedPrivateIP(t *testing.T) {
	req := newSignedOpsTestRequest("GET", "/internal/system/config-reload/status", "ops-token", nil)
	req.RemoteAddr = "10.0.0.10:12345"
	req.Header.Set("X-Forwarded-For", "10.1.2.3, 10.0.0.10")
	err := validateConfigReloadOps(req, config.OpsConfig{
		ConfigReloadToken: "ops-token",
	})
	if err != nil {
		t.Fatalf("validateConfigReloadOps() error = %v", err)
	}
}

// TestValidateConfigReloadOpsRejectsMissingSignature 确保正确令牌也必须携带 HMAC 签名。
func TestValidateConfigReloadOpsRejectsMissingSignature(t *testing.T) {
	req := httptest.NewRequest("GET", "/internal/system/config-reload/status", nil)
	req.RemoteAddr = "10.0.0.10:12345"
	req.Header.Set(HeaderOpsToken, "ops-token")
	err := validateConfigReloadOps(req, config.OpsConfig{
		ConfigReloadToken: "ops-token",
	})
	if err == nil {
		t.Fatal("期望缺少 HMAC 签名返回错误，实际为 nil")
	}
}

// TestValidateConfigReloadOpsRejectsBodyHashMismatch 确保请求体摘要不匹配时拒绝访问。
func TestValidateConfigReloadOpsRejectsBodyHashMismatch(t *testing.T) {
	req := newSignedOpsTestRequest("POST", "/internal/users/1/runtime-sync", "ops-token", []byte(`{"profile":true}`))
	req.RemoteAddr = "10.0.0.10:12345"
	req.Header.Set(HeaderOpsBodySHA256, opsBodySHA256([]byte(`{"profile":false}`)))
	err := validateConfigReloadOps(req, config.OpsConfig{
		ConfigReloadToken: "ops-token",
	})
	if err == nil {
		t.Fatal("期望请求体摘要不匹配返回错误，实际为 nil")
	}
}

// TestValidateConfigReloadOpsRejectsSignatureMismatch 确保 HMAC 签名不匹配时拒绝访问。
func TestValidateConfigReloadOpsRejectsSignatureMismatch(t *testing.T) {
	req := newSignedOpsTestRequest("POST", "/internal/users/1/runtime-sync", "ops-token", []byte(`{"profile":true}`))
	req.RemoteAddr = "10.0.0.10:12345"
	req.Header.Set(HeaderOpsSignature, signOpsRequest("other-token", req.Method, req.URL.RequestURI(), req.Header.Get(HeaderOpsTimestamp), req.Header.Get(HeaderOpsBodySHA256)))
	err := validateConfigReloadOps(req, config.OpsConfig{
		ConfigReloadToken: "ops-token",
	})
	if err == nil {
		t.Fatal("期望 HMAC 签名不匹配返回错误，实际为 nil")
	}
}

// TestValidateConfigReloadOpsRejectsOversizeBody 确保超出签名读取上限的请求体会被拒绝。
func TestValidateConfigReloadOpsRejectsOversizeBody(t *testing.T) {
	body := bytes.Repeat([]byte("a"), opsSignatureBodyMaxBytes+1)
	req := newSignedOpsTestRequest("POST", "/internal/users/1/runtime-sync", "ops-token", body)
	req.RemoteAddr = "10.0.0.10:12345"
	err := validateConfigReloadOps(req, config.OpsConfig{
		ConfigReloadToken: "ops-token",
	})
	if err == nil {
		t.Fatal("期望超大请求体返回错误，实际为 nil")
	}
}

// TestValidateConfigReloadOpsRejectsExpiredSignature 确保过期签名不能访问运维接口。
func TestValidateConfigReloadOpsRejectsExpiredSignature(t *testing.T) {
	req := newSignedOpsTestRequestAt("GET", "/internal/system/config-reload/status", "ops-token", nil, time.Now().Add(-opsSignatureWindow-time.Second))
	req.RemoteAddr = "10.0.0.10:12345"
	err := validateConfigReloadOps(req, config.OpsConfig{
		ConfigReloadToken: "ops-token",
	})
	if err == nil {
		t.Fatal("期望过期 HMAC 签名返回错误，实际为 nil")
	}
}

// TestValidateConfigReloadOpsRestoresSignedBody 确保验签后请求体仍可被业务 handler 读取。
func TestValidateConfigReloadOpsRestoresSignedBody(t *testing.T) {
	body := []byte(`{"profile":true}`)
	req := newSignedOpsTestRequest("POST", "/internal/users/1/runtime-sync", "ops-token", body)
	req.RemoteAddr = "10.0.0.10:12345"
	err := validateConfigReloadOps(req, config.OpsConfig{
		ConfigReloadToken: "ops-token",
	})
	if err != nil {
		t.Fatalf("validateConfigReloadOps() error = %v", err)
	}
	got, err := io.ReadAll(req.Body)
	if err != nil {
		t.Fatalf("ReadAll restored body error = %v", err)
	}
	if !bytes.Equal(got, body) {
		t.Fatalf("restored body = %s, want %s", got, body)
	}
}

// newSignedOpsTestRequest 构造带当前时间签名的运维接口测试请求。
func newSignedOpsTestRequest(method string, target string, token string, body []byte) *http.Request {
	return newSignedOpsTestRequestAt(method, target, token, body, time.Now())
}

// newSignedOpsTestRequestAt 构造指定时间戳签名的运维接口测试请求。
func newSignedOpsTestRequestAt(method string, target string, token string, body []byte, now time.Time) *http.Request {
	req := httptest.NewRequest(method, target, bytes.NewReader(body))
	timestamp := strconv.FormatInt(now.Unix(), 10)
	bodyHash := opsBodySHA256(body)
	req.Header.Set(HeaderOpsToken, token)
	req.Header.Set(HeaderOpsTimestamp, timestamp)
	req.Header.Set(HeaderOpsBodySHA256, bodyHash)
	req.Header.Set(HeaderOpsSignature, signOpsRequest(token, req.Method, req.URL.RequestURI(), timestamp, bodyHash))
	return req
}
