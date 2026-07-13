package middleware

import (
	"encoding/base64"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	codes "api/common/codes"
	"api/common/runtimecfg"
	"api/internal/config"
	authlogic "api/internal/logic/auth"
	"api/internal/requestctx"
	"api/internal/routealias"
	"api/internal/security"
	"api/internal/svc"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

// TestSignatureMiddlewareSkipsRouteWithoutSignPolicy 验证对应场景符合预期。
func TestSignatureMiddlewareSkipsRouteWithoutSignPolicy(t *testing.T) {
	svcCtx := svc.NewServiceContext(securityEnabledConfig(), "test-version", svc.Dependencies{})
	middleware := NewSignatureMiddleware(svcCtx)
	handler := middleware.Handle(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}, routealias.UserRuntimeSync)

	req := httptest.NewRequest(http.MethodPost, "/internal/users/1/runtime-sync", nil)
	rec := httptest.NewRecorder()
	handler(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNoContent)
	}
}

// TestCryptoMiddlewareSkipsRouteWithoutCipherPolicy 验证对应场景符合预期。
func TestCryptoMiddlewareSkipsRouteWithoutCipherPolicy(t *testing.T) {
	svcCtx := svc.NewServiceContext(securityEnabledConfig(), "test-version", svc.Dependencies{})
	middleware := NewCryptoMiddleware(svcCtx)
	handler := middleware.Handle(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	req := httptest.NewRequest(http.MethodPost, "/api/auth/logout", nil)
	req = bindRequestMeta(req, routealias.AuthLogout, nil)
	rec := httptest.NewRecorder()
	handler(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNoContent)
	}
}

// TestSignatureMiddlewareDisabledSkipsTraceHeaders 确保仅开启加密时，签名中间件不要求 trace_id 和 timestamp。
func TestSignatureMiddlewareDisabledSkipsTraceHeaders(t *testing.T) {
	cfg := securityEnabledConfig()
	cfg.Security.SecretKey.SignStatus = 0
	svcCtx := svc.NewServiceContext(cfg, "test-version", svc.Dependencies{})
	called := false
	handler := NewSignatureMiddleware(svcCtx).Handle(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.WriteHeader(http.StatusNoContent)
	}, routealias.AuthLogin)
	request := httptest.NewRequest(http.MethodPost, "/api/auth/login", nil)
	request.Header.Set("X-App-Id", base64.StdEncoding.EncodeToString([]byte("demo-app")))
	recorder := httptest.NewRecorder()

	handler(recorder, request)

	if !called {
		t.Fatal("签名关闭时请求应进入业务处理器")
	}
	if recorder.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusNoContent)
	}
}

// TestCryptoMiddlewareDisabledSkipsCipherPolicy 确保仅开启签名时，加密中间件不加工响应。
func TestCryptoMiddlewareDisabledSkipsCipherPolicy(t *testing.T) {
	cfg := securityEnabledConfig()
	cfg.Security.SecretKey.CryptoStatus = 0
	svcCtx := svc.NewServiceContext(cfg, "test-version", svc.Dependencies{})
	called := false
	handler := NewCryptoMiddleware(svcCtx).Handle(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.WriteHeader(http.StatusNoContent)
	})
	request := httptest.NewRequest(http.MethodPost, "/api/auth/login", nil)
	request.Header.Set("X-App-Id", base64.StdEncoding.EncodeToString([]byte("demo-app")))
	request = bindRequestMeta(request, routealias.AuthLogin, svcCtx)
	recorder := httptest.NewRecorder()

	handler(recorder, request)

	if !called {
		t.Fatal("加密关闭时请求应进入业务处理器")
	}
	if recorder.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusNoContent)
	}
	if recorder.Header().Get("X-Cipher") != "" || recorder.Header().Get("X-Crypto") != "" {
		t.Fatal("加密关闭时响应不应包含加密协议头")
	}
}

// TestSignatureTraceIDRequiresCanonicalActiveTrace 确保签名标识格式固定且与实际链路一致。
func TestSignatureTraceIDRequiresCanonicalActiveTrace(t *testing.T) {
	const traceID = "0123456789abcdef0123456789abcdef"
	req := httptest.NewRequest(http.MethodPost, "/api/demo", nil)
	req.Header.Set(requestctx.HeaderTraceID, traceID)
	ctx, _ := requestctx.New(req.Context())
	requestctx.SetTrace(ctx, traceID, "0123456789abcdef")
	req = req.WithContext(ctx)

	got, err := signatureTraceID(req)
	if err != nil || got != traceID {
		t.Fatalf("signatureTraceID() = %q, %v, want %q", got, err, traceID)
	}

	req.Header.Set(requestctx.HeaderTraceID, "01234567-89ab-cdef-0123-456789abcdef")
	if _, err = signatureTraceID(req); err == nil || !strings.Contains(err.Error(), "32位小写十六进制") {
		t.Fatalf("UUID trace error = %v, want canonical format rejection", err)
	}

	req.Header.Set(requestctx.HeaderTraceID, "fedcba9876543210fedcba9876543210")
	if _, err = signatureTraceID(req); err == nil || !strings.Contains(err.Error(), "实际链路不一致") {
		t.Fatalf("mismatched trace error = %v, want active trace rejection", err)
	}
}

// TestSecurityConfigConfiguredRequiresConcreteVersion 验证对应场景符合预期。
func TestSecurityConfigConfiguredRequiresConcreteVersion(t *testing.T) {
	cfg := config.Config{
		AppID: "demo-app",
		Security: config.SecurityConfig{
			SecretKey: config.SecuritySecretKeyConfig{
				StableVersion: "v1",
			},
		},
	}
	svcCtx := svc.NewServiceContext(cfg, "test-version", svc.Dependencies{})

	if securityConfigConfigured(svcCtx) {
		t.Fatal("securityConfigConfigured() should ignore stable_version without key material")
	}
}

// TestSignatureMiddlewareRejectsUnsupportedSignatureTypes 确保未知签名算法失败关闭。
func TestSignatureMiddlewareRejectsUnsupportedSignatureTypes(t *testing.T) {
	middleware := NewSignatureMiddleware(svc.NewServiceContext(securityEnabledConfig(), "test-version", svc.Dependencies{}))
	request := httptest.NewRequest(http.MethodPost, "/api/demo", nil)
	for _, signatureType := range []string{"UNKNOWN", "HMAC", "M", "MD5"} {
		if _, _, err := middleware.signer(request, "demo-app", security.NormalizeSignatureType(signatureType), true); err == nil || !strings.Contains(err.Error(), "签名方式不合法") {
			t.Fatalf("signer(%q) error = %v, want invalid signature type", signatureType, err)
		}
	}
}

// TestSignatureMiddlewareRejectsMissingSignatureKey 确保签名材料缺失时不会降级到其它算法。
func TestSignatureMiddlewareRejectsMissingSignatureKey(t *testing.T) {
	cfg := config.Config{
		AppID: "demo-app",
		Security: config.SecurityConfig{
			SecretKey: config.SecuritySecretKeyConfig{
				KeyVersion: "v1",
				SignStatus: 1,
			},
		},
	}
	middleware := NewSignatureMiddleware(svc.NewServiceContext(cfg, "test-version", svc.Dependencies{}))
	request := httptest.NewRequest(http.MethodPost, "/api/demo", nil)
	if _, _, err := middleware.signer(request, "demo-app", security.SignatureTypeAES, true); err == nil {
		t.Fatal("signer() error = nil, want missing key failure")
	}
}

// TestSignatureMiddlewareVerifiesHeaderOnlyPolicy 校验空字段策略能验签并写入防重放缓存。
func TestSignatureMiddlewareVerifiesHeaderOnlyPolicy(t *testing.T) {
	const traceID = "0123456789abcdef0123456789abcdef"
	cfg := securityEnabledConfig()
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() {
		_ = client.Close()
	})
	previous := runtimecfg.Get()
	runtimecfg.Set(cfg)
	t.Cleanup(func() {
		runtimecfg.Restore(previous)
	})

	timestamp := strconv.FormatInt(time.Now().Unix(), 10)
	signer, err := security.NewAESCipher(cfg.Security.SecretKey.AESKey, cfg.Security.SecretKey.AESIV)
	if err != nil {
		t.Fatalf("NewAESCipher() error = %v", err)
	}
	sign, err := signer.Sign(security.BuildSignString(nil, []string{}, traceID, timestamp, cfg.AppID))
	if err != nil {
		t.Fatalf("Sign() error = %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/auth/logout", strings.NewReader(`{"sign":"`+sign+`"}`))
	middleware := NewSignatureMiddleware(svc.NewServiceContext(cfg, "test-version", svc.Dependencies{Rds: client}))
	policy := security.PolicyByRoute(string(routealias.AuthLogout))
	if err = middleware.verifyRequest(req, policy, cfg.AppID, traceID, timestamp, security.SignatureTypeAES); err != nil {
		t.Fatalf("verifyRequest() error = %v", err)
	}
	if err = middleware.verifyRequest(req, policy, cfg.AppID, traceID, timestamp, security.SignatureTypeAES); err == nil || !strings.Contains(err.Error(), "重复请求") {
		t.Fatalf("second verifyRequest() error = %v, want replay rejection", err)
	}
}

// TestSignatureMiddlewareRejectsRequestSignAll 验证对应场景符合预期。
func TestSignatureMiddlewareRejectsRequestSignAll(t *testing.T) {
	middleware := NewSignatureMiddleware(svc.NewServiceContext(securityEnabledConfig(), "test-version", svc.Dependencies{}))
	err := middleware.verifyRequest(httptest.NewRequest(http.MethodPost, "/api/demo", nil), security.RouteSecurityPolicy{
		RequestSign: []string{security.SignFieldAll},
	}, "demo-app", "trace", "1700000000", security.SignatureTypeAES)
	if err == nil || !strings.Contains(err.Error(), "全量字段") {
		t.Fatalf("verifyRequest() error = %v, want full-field rejection", err)
	}
}

// TestSignatureMiddlewareRejectsOversizeRequestSignField 验证对应场景符合预期。
func TestSignatureMiddlewareRejectsOversizeRequestSignField(t *testing.T) {
	middleware := NewSignatureMiddleware(svc.NewServiceContext(securityEnabledConfig(), "test-version", svc.Dependencies{}))
	body := `{"username":"` + strings.Repeat("x", security.MaxSecurityFieldBytes+1) + `","sign":"demo"}`
	err := middleware.verifyRequest(httptest.NewRequest(http.MethodPost, "/api/demo", strings.NewReader(body)), security.RouteSecurityPolicy{
		RequestSign: []string{"username"},
	}, "demo-app", "trace", "1700000000", security.SignatureTypeAES)
	if err == nil || !strings.Contains(err.Error(), "长度超过上限") {
		t.Fatalf("verifyRequest() error = %v, want oversize field rejection", err)
	}
	if got := resolveSecurityFailureCode(authlogic.AuthEventReasonSignatureFailed, codes.AuthFailed, err); got != codes.SecurityPayloadTooLarge {
		t.Fatalf("resolveSecurityFailureCode() = %d, want %d", got, codes.SecurityPayloadTooLarge)
	}
}

// TestSignatureMiddlewareRejectsOversizeSignValue 验证对应场景符合预期。
func TestSignatureMiddlewareRejectsOversizeSignValue(t *testing.T) {
	middleware := NewSignatureMiddleware(svc.NewServiceContext(securityEnabledConfig(), "test-version", svc.Dependencies{}))
	body := `{"username":"demo","sign":"` + strings.Repeat("x", security.MaxSecurityFieldBytes+1) + `"}`
	err := middleware.verifyRequest(httptest.NewRequest(http.MethodPost, "/api/demo", strings.NewReader(body)), security.RouteSecurityPolicy{
		RequestSign: []string{"username"},
	}, "demo-app", "trace", "1700000000", security.SignatureTypeAES)
	if err == nil || !strings.Contains(err.Error(), "长度超过上限") {
		t.Fatalf("verifyRequest() error = %v, want oversize sign rejection", err)
	}
}

// TestSignatureMiddlewareRejectsResponseSignAll 验证对应场景符合预期。
func TestSignatureMiddlewareRejectsResponseSignAll(t *testing.T) {
	middleware := NewSignatureMiddleware(svc.NewServiceContext(securityEnabledConfig(), "test-version", svc.Dependencies{}))
	recorder := newBodyRecorder()
	_, _ = recorder.body.WriteString(`{"status":true,"data":{"token":"t","items":[1,2,3]}}`)
	_, err := middleware.signResponse(recorder, security.RouteSecurityPolicy{
		ResponseSign: []string{security.SignFieldAll},
	}, "demo-app", "trace", "1700000000", security.SignatureTypeAES, httptest.NewRequest(http.MethodPost, "/api/demo", nil))
	if err == nil || !strings.Contains(err.Error(), "全量字段") {
		t.Fatalf("signResponse() error = %v, want full-field rejection", err)
	}
}

// TestSignatureMiddlewareMarkRequestVerifiedFailsClosedOnAppIDMismatch 验证对应场景符合预期。
func TestSignatureMiddlewareMarkRequestVerifiedFailsClosedOnAppIDMismatch(t *testing.T) {
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() {
		_ = client.Close()
	})
	prev := runtimecfg.Get()
	runtimecfg.Set(config.Config{AppID: "site-b"})
	t.Cleanup(func() {
		runtimecfg.Restore(prev)
	})

	middleware := NewSignatureMiddleware(svc.NewServiceContext(config.Config{AppID: "site-a"}, "test-version", svc.Dependencies{Rds: client}))
	err := middleware.markRequestVerified(httptest.NewRequest(http.MethodPost, "/api/demo", nil), "site-a", "trace-1")
	if err == nil || !strings.Contains(err.Error(), "app_id") {
		t.Fatalf("markRequestVerified() error = %v, want app_id mismatch", err)
	}
	if server.Exists("app:site-a:signature:replay:trace-1") || server.Exists("app:site-b:signature:replay:trace-1") {
		t.Fatal("app_id 不一致时不应写入签名防重放缓存")
	}
}

// TestSignatureMiddlewareRejectsOversizeResponseSignField 验证对应场景符合预期。
func TestSignatureMiddlewareRejectsOversizeResponseSignField(t *testing.T) {
	middleware := NewSignatureMiddleware(svc.NewServiceContext(securityEnabledConfig(), "test-version", svc.Dependencies{}))
	recorder := newBodyRecorder()
	_, _ = recorder.body.WriteString(`{"status":true,"data":{"token":"` + strings.Repeat("x", security.MaxSecurityFieldBytes+1) + `"}}`)
	_, err := middleware.signResponse(recorder, security.RouteSecurityPolicy{
		ResponseSign: []string{"token"},
	}, "demo-app", "trace", "1700000000", security.SignatureTypeAES, httptest.NewRequest(http.MethodPost, "/api/demo", nil))
	if err == nil || !strings.Contains(err.Error(), "长度超过上限") {
		t.Fatalf("signResponse() error = %v, want oversize field rejection", err)
	}
}

// TestRequestTimestampWindow 验证对应场景符合预期。
func TestRequestTimestampWindow(t *testing.T) {
	now := time.Now().Unix()
	req := httptest.NewRequest(http.MethodPost, "/api/demo", nil)
	req.Header.Set("X-Timestamp", " "+fmt.Sprint(now)+" ")
	got, err := requestTimestamp(req)
	if err != nil {
		t.Fatalf("requestTimestamp() error = %v", err)
	}
	if got != fmt.Sprint(now) {
		t.Fatalf("requestTimestamp() = %q, want %d", got, now)
	}

	expired := httptest.NewRequest(http.MethodPost, "/api/demo", nil)
	expired.Header.Set("X-Timestamp", fmt.Sprint(now-int64(signatureReplayTTL.Seconds())-1))
	if _, err := requestTimestamp(expired); err == nil || !strings.Contains(err.Error(), "已过期") {
		t.Fatalf("requestTimestamp(expired) error = %v, want expired", err)
	}
}

// TestCryptoMiddlewareRejectsWholeBodyRequestCipher 验证对应场景符合预期。
func TestCryptoMiddlewareRejectsWholeBodyRequestCipher(t *testing.T) {
	middleware := NewCryptoMiddleware(svc.NewServiceContext(securityEnabledConfig(), "test-version", svc.Dependencies{}))
	err := middleware.decryptRequest(httptest.NewRequest(http.MethodPost, "/api/demo", nil), []string{cipherWholeBody}, noopCryptor{})
	if err == nil || !strings.Contains(err.Error(), "整包") {
		t.Fatalf("decryptRequest() error = %v, want whole-body rejection", err)
	}
}

// TestCryptoMiddlewareRejectsTooManyCipherFields 验证对应场景符合预期。
func TestCryptoMiddlewareRejectsTooManyCipherFields(t *testing.T) {
	fields := []string{"f1", "f2", "f3", "f4", "f5", "f6", "f7", "f8", "f9"}
	raw := security.EncodeCipherParams(fields)
	_, err := decodeAndValidateCipherParams(raw, fields, "请求")
	if err == nil || !strings.Contains(err.Error(), "数量超过上限") {
		t.Fatalf("decodeAndValidateCipherParams() error = %v, want field count rejection", err)
	}
}

// TestCryptoMiddlewareRejectsOversizeCipherHeader 验证对应场景符合预期。
func TestCryptoMiddlewareRejectsOversizeCipherHeader(t *testing.T) {
	raw := strings.Repeat("x", security.MaxSecurityJSONFieldBytes+1)
	_, err := decodeAndValidateCipherParams(raw, []string{"password"}, "请求")
	if err == nil || !strings.Contains(err.Error(), "长度超过上限") {
		t.Fatalf("decodeAndValidateCipherParams() error = %v, want header size rejection", err)
	}
}

// TestCryptoMiddlewareRejectsUndeclaredRequestCipher 验证对应场景符合预期。
func TestCryptoMiddlewareRejectsUndeclaredRequestCipher(t *testing.T) {
	raw := security.EncodeCipherParams([]string{"profile"})
	_, err := decodeAndValidateCipherParams(raw, []string{"password"}, "请求")
	if err == nil || !strings.Contains(err.Error(), "不允许") {
		t.Fatalf("decodeAndValidateCipherParams() error = %v, want undeclared field rejection", err)
	}
}

// TestCryptoMiddlewareRejectsOversizeRequestCipherValue 验证对应场景符合预期。
func TestCryptoMiddlewareRejectsOversizeRequestCipherValue(t *testing.T) {
	middleware := NewCryptoMiddleware(svc.NewServiceContext(securityEnabledConfig(), "test-version", svc.Dependencies{}))
	req := httptest.NewRequest(http.MethodPost, "/api/demo", strings.NewReader(`{"password":"`+strings.Repeat("x", security.MaxSecurityFieldBytes+1)+`"}`))
	err := middleware.decryptRequest(req, []string{"password"}, noopCryptor{})
	if err == nil || !strings.Contains(err.Error(), "长度超过上限") {
		t.Fatalf("decryptRequest() error = %v, want oversize field rejection", err)
	}
}

// TestCryptoMiddlewareAcceptsDeclaredRequestCipher 验证对应场景符合预期。
func TestCryptoMiddlewareAcceptsDeclaredRequestCipher(t *testing.T) {
	raw := security.EncodeCipherParams([]string{"password"})
	params, err := decodeAndValidateCipherParams(raw, []string{"password"}, "请求")
	if err != nil {
		t.Fatalf("decodeAndValidateCipherParams() error = %v", err)
	}
	middleware := NewCryptoMiddleware(svc.NewServiceContext(securityEnabledConfig(), "test-version", svc.Dependencies{}))
	req := httptest.NewRequest(http.MethodPost, "/api/demo", strings.NewReader(`{"username":"demo","password":"secret"}`))
	if err := middleware.decryptRequest(req, params, noopCryptor{}); err != nil {
		t.Fatalf("decryptRequest() error = %v", err)
	}
}

// TestCryptoMiddlewareRejectsOversizeResponseCipherValue 验证对应场景符合预期。
func TestCryptoMiddlewareRejectsOversizeResponseCipherValue(t *testing.T) {
	middleware := NewCryptoMiddleware(svc.NewServiceContext(securityEnabledConfig(), "test-version", svc.Dependencies{}))
	recorder := newBodyRecorder()
	_, _ = recorder.body.WriteString(`{"status":true,"data":{"token":"` + strings.Repeat("x", security.MaxSecurityFieldBytes+1) + `"}}`)
	err := middleware.encryptResponse(recorder, []string{"token"}, noopCryptor{})
	if err == nil || !strings.Contains(err.Error(), "长度超过上限") {
		t.Fatalf("encryptResponse() error = %v, want oversize field rejection", err)
	}
}

// TestCryptoMiddlewareRejectsWholeBodyResponseCipher 验证对应场景符合预期。
func TestCryptoMiddlewareRejectsWholeBodyResponseCipher(t *testing.T) {
	middleware := NewCryptoMiddleware(svc.NewServiceContext(securityEnabledConfig(), "test-version", svc.Dependencies{}))
	recorder := newBodyRecorder()
	_, _ = recorder.body.WriteString(`{"status":true,"data":{"items":[1,2,3]}}`)
	err := middleware.encryptResponse(recorder, []string{cipherWholeBody}, noopCryptor{})
	if err == nil || !strings.Contains(err.Error(), "整包") {
		t.Fatalf("encryptResponse() error = %v, want whole-body rejection", err)
	}
}

// TestCryptoMiddlewareRejectsUndeclaredResponseCipher 验证对应场景符合预期。
func TestCryptoMiddlewareRejectsUndeclaredResponseCipher(t *testing.T) {
	raw := security.EncodeCipherParams([]string{"items"})
	_, err := decodeAndValidateCipherParams(raw, []string{"token"}, "响应")
	if err == nil || !strings.Contains(err.Error(), "不允许") {
		t.Fatalf("decodeAndValidateCipherParams() error = %v, want undeclared field rejection", err)
	}
}

// noopCryptor 表示测试使用的辅助结构。
type noopCryptor struct{}

// Encrypt 表示测试辅助逻辑。
func (noopCryptor) Encrypt(data string) (string, error) {
	return data, nil
}

// Decrypt 表示测试辅助逻辑。
func (noopCryptor) Decrypt(data string) (string, error) {
	return data, nil
}

// securityEnabledConfig 返回安全测试辅助数据。
func securityEnabledConfig() config.Config {
	return config.Config{
		AppID: "demo-app",
		Security: config.SecurityConfig{
			SecretKey: config.SecuritySecretKeyConfig{
				KeyVersion:   "v1",
				SignStatus:   1,
				CryptoStatus: 1,
				AESKey:       "1234567890123456",
				AESIV:        "abcdefghijklmnop",
			},
		},
	}
}
