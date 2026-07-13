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
	"api/internal/svc"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

// testOpsNonce 是测试使用的固定 16 字节十六进制 nonce。
const testOpsNonce = "00112233445566778899aabbccddeeff"

// validateConfigReloadOps 复用生产验签函数，测试纯鉴权逻辑时不写 Redis。
func validateConfigReloadOps(r *http.Request, cfg config.OpsConfig) error {
	_, err := authenticateOpsWithClientIP(r, cfg, requestClientIP(nil, r), time.Now())
	return err
}

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
	req.Header.Set(HeaderOpsSignature, signOpsRequest("other-token", req.Method, req.URL.RequestURI(), req.Header.Get(HeaderOpsTimestamp), req.Header.Get(HeaderOpsNonce), req.Header.Get(HeaderOpsBodySHA256)))
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

// TestAuthenticateOpsNonceTTLCoversFutureTimestamp 确保未来容差内的签名也会被防重放缓存覆盖到失效。
func TestAuthenticateOpsNonceTTLCoversFutureTimestamp(t *testing.T) {
	now := time.Now()
	signedAt := now.Add(4 * time.Minute)
	req := newSignedOpsTestRequestAt("GET", "/internal/system/config-reload/status", "ops-token", nil, signedAt)
	auth, err := authenticateOpsWithClientIP(req, config.OpsConfig{ConfigReloadToken: "ops-token"}, "10.0.0.10", now)
	if err != nil {
		t.Fatalf("authenticateOpsWithClientIP() error = %v", err)
	}
	want := time.Unix(signedAt.Unix(), 0).Add(opsSignatureWindow).Sub(now)
	if auth.TTL != want {
		t.Fatalf("nonce TTL=%s，期望=%s", auth.TTL, want)
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

// TestValidateConfigReloadOpsRejectsMissingNonce 确保签名协议缺少 nonce 时拒绝访问。
func TestValidateConfigReloadOpsRejectsMissingNonce(t *testing.T) {
	req := newSignedOpsTestRequest("GET", "/internal/system/config-reload/status", "ops-token", nil)
	req.RemoteAddr = "10.0.0.10:12345"
	req.Header.Del(HeaderOpsNonce)
	if err := validateConfigReloadOps(req, config.OpsConfig{ConfigReloadToken: "ops-token"}); err == nil {
		t.Fatal("期望缺少 nonce 返回错误，实际为 nil")
	}
}

// TestValidateConfigReloadOpsRejectsTamperedNonce 确保 nonce 被篡改后原签名失效。
func TestValidateConfigReloadOpsRejectsTamperedNonce(t *testing.T) {
	req := newSignedOpsTestRequest("GET", "/internal/system/config-reload/status", "ops-token", nil)
	req.RemoteAddr = "10.0.0.10:12345"
	req.Header.Set(HeaderOpsNonce, "ffeeddccbbaa99887766554433221100")
	if err := validateConfigReloadOps(req, config.OpsConfig{ConfigReloadToken: "ops-token"}); err == nil {
		t.Fatal("期望 nonce 被篡改后返回错误，实际为 nil")
	}
}

// TestOpsMiddlewareRejectsReplay 确保同一 nonce 只能被 Redis 原子接受一次。
func TestOpsMiddlewareRejectsReplay(t *testing.T) {
	useTestAppID(t, "ops-replay")
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() {
		_ = client.Close()
	})
	cfg := config.Config{
		AppID: "ops-replay",
		Ops:   config.OpsConfig{ConfigReloadToken: "ops-token"},
	}
	middleware := NewOpsMiddleware(svc.NewServiceContext(cfg, "", svc.Dependencies{Rds: client}))
	handler := middleware.Handle(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	first := newSignedOpsTestRequest("POST", "/internal/users/1/runtime-sync", "ops-token", []byte(`{"profile":true}`))
	first.RemoteAddr = "10.0.0.10:12345"
	firstRecorder := httptest.NewRecorder()
	handler(firstRecorder, first)
	if firstRecorder.Code != http.StatusNoContent {
		t.Fatalf("首次请求状态码=%d，期望=%d", firstRecorder.Code, http.StatusNoContent)
	}

	replayed := newSignedOpsTestRequest("POST", "/internal/users/1/runtime-sync", "ops-token", []byte(`{"profile":true}`))
	replayed.RemoteAddr = "10.0.0.10:12345"
	replayRecorder := httptest.NewRecorder()
	handler(replayRecorder, replayed)
	if replayRecorder.Code != http.StatusForbidden {
		t.Fatalf("重放请求状态码=%d，期望=%d", replayRecorder.Code, http.StatusForbidden)
	}
}

// TestOpsMiddlewareFailsClosedWithoutRedis 确保防重放缓存不可用时返回 503，不降级放行。
func TestOpsMiddlewareFailsClosedWithoutRedis(t *testing.T) {
	cfg := config.Config{Ops: config.OpsConfig{ConfigReloadToken: "ops-token"}}
	middleware := NewOpsMiddleware(svc.NewServiceContext(cfg, "", svc.Dependencies{}))
	handler := middleware.Handle(func(http.ResponseWriter, *http.Request) {
		t.Fatal("Redis 不可用时不应进入业务 handler")
	})
	req := newSignedOpsTestRequest("GET", "/internal/system/config-reload/status", "ops-token", nil)
	req.RemoteAddr = "10.0.0.10:12345"
	recorder := httptest.NewRecorder()
	handler(recorder, req)
	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("Redis 不可用状态码=%d，期望=%d", recorder.Code, http.StatusServiceUnavailable)
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
	req.Header.Set(HeaderOpsNonce, testOpsNonce)
	req.Header.Set(HeaderOpsBodySHA256, bodyHash)
	req.Header.Set(HeaderOpsSignature, signOpsRequest(token, req.Method, req.URL.RequestURI(), timestamp, testOpsNonce, bodyHash))
	return req
}
