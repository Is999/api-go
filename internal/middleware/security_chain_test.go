package middleware

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	codes "api/common/codes"
	"api/common/runtimecfg"
	"api/internal/config"
	authlogic "api/internal/logic/auth"
	"api/internal/security"
	"api/internal/svc"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func TestSignatureMiddlewareSkipsRouteWithoutSignPolicy(t *testing.T) {
	svcCtx := svc.NewServiceContext(securityEnabledConfig(), "test-version", svc.Dependencies{})
	middleware := NewSignatureMiddleware(svcCtx)
	handler := middleware.Handle(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}, RouteAlias("user.profile"))

	req := httptest.NewRequest(http.MethodGet, "/api/user/profile", nil)
	rec := httptest.NewRecorder()
	handler(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNoContent)
	}
}

func TestCryptoMiddlewareSkipsRouteWithoutCipherPolicy(t *testing.T) {
	svcCtx := svc.NewServiceContext(securityEnabledConfig(), "test-version", svc.Dependencies{})
	middleware := NewCryptoMiddleware(svcCtx)
	handler := middleware.Handle(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	req := httptest.NewRequest(http.MethodPost, "/api/auth/logout", nil)
	req = bindRequestMeta(req, RouteAlias("auth.logout"))
	rec := httptest.NewRecorder()
	handler(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNoContent)
	}
}

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

func TestSignatureMiddlewareRejectsRequestSignAll(t *testing.T) {
	middleware := NewSignatureMiddleware(svc.NewServiceContext(securityEnabledConfig(), "test-version", svc.Dependencies{}))
	err := middleware.verifyRequest(httptest.NewRequest(http.MethodPost, "/api/demo", nil), security.RouteSecurityPolicy{
		RequestSign: []string{security.SignFieldAll},
	}, "demo-app", "trace", "1700000000", security.SignatureTypeMD5)
	if err == nil || !strings.Contains(err.Error(), "全量字段") {
		t.Fatalf("verifyRequest() error = %v, want full-field rejection", err)
	}
}

func TestSignatureMiddlewareRejectsOversizeRequestSignField(t *testing.T) {
	middleware := NewSignatureMiddleware(svc.NewServiceContext(securityEnabledConfig(), "test-version", svc.Dependencies{}))
	body := `{"username":"` + strings.Repeat("x", security.MaxSecurityFieldBytes+1) + `","sign":"demo"}`
	err := middleware.verifyRequest(httptest.NewRequest(http.MethodPost, "/api/demo", strings.NewReader(body)), security.RouteSecurityPolicy{
		RequestSign: []string{"username"},
	}, "demo-app", "trace", "1700000000", security.SignatureTypeMD5)
	if err == nil || !strings.Contains(err.Error(), "长度超过上限") {
		t.Fatalf("verifyRequest() error = %v, want oversize field rejection", err)
	}
	if got := resolveSecurityFailureCode(authlogic.AuthEventReasonSignatureFailed, codes.AuthFailed, err); got != codes.SecurityPayloadTooLarge {
		t.Fatalf("resolveSecurityFailureCode() = %d, want %d", got, codes.SecurityPayloadTooLarge)
	}
}

func TestSignatureMiddlewareRejectsOversizeSignValue(t *testing.T) {
	middleware := NewSignatureMiddleware(svc.NewServiceContext(securityEnabledConfig(), "test-version", svc.Dependencies{}))
	body := `{"username":"demo","sign":"` + strings.Repeat("x", security.MaxSecurityFieldBytes+1) + `"}`
	err := middleware.verifyRequest(httptest.NewRequest(http.MethodPost, "/api/demo", strings.NewReader(body)), security.RouteSecurityPolicy{
		RequestSign: []string{"username"},
	}, "demo-app", "trace", "1700000000", security.SignatureTypeMD5)
	if err == nil || !strings.Contains(err.Error(), "长度超过上限") {
		t.Fatalf("verifyRequest() error = %v, want oversize sign rejection", err)
	}
}

func TestSignatureMiddlewareRejectsResponseSignAll(t *testing.T) {
	middleware := NewSignatureMiddleware(svc.NewServiceContext(securityEnabledConfig(), "test-version", svc.Dependencies{}))
	recorder := newBodyRecorder()
	_, _ = recorder.body.WriteString(`{"status":true,"data":{"token":"t","items":[1,2,3]}}`)
	_, err := middleware.signResponse(recorder, security.RouteSecurityPolicy{
		ResponseSign: []string{security.SignFieldAll},
	}, "demo-app", "trace", "1700000000", security.SignatureTypeMD5, httptest.NewRequest(http.MethodPost, "/api/demo", nil))
	if err == nil || !strings.Contains(err.Error(), "全量字段") {
		t.Fatalf("signResponse() error = %v, want full-field rejection", err)
	}
}

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

func TestSignatureMiddlewareRejectsOversizeResponseSignField(t *testing.T) {
	middleware := NewSignatureMiddleware(svc.NewServiceContext(securityEnabledConfig(), "test-version", svc.Dependencies{}))
	recorder := newBodyRecorder()
	_, _ = recorder.body.WriteString(`{"status":true,"data":{"token":"` + strings.Repeat("x", security.MaxSecurityFieldBytes+1) + `"}}`)
	_, err := middleware.signResponse(recorder, security.RouteSecurityPolicy{
		ResponseSign: []string{"token"},
	}, "demo-app", "trace", "1700000000", security.SignatureTypeMD5, httptest.NewRequest(http.MethodPost, "/api/demo", nil))
	if err == nil || !strings.Contains(err.Error(), "长度超过上限") {
		t.Fatalf("signResponse() error = %v, want oversize field rejection", err)
	}
}

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

func TestCryptoMiddlewareRejectsWholeBodyRequestCipher(t *testing.T) {
	middleware := NewCryptoMiddleware(svc.NewServiceContext(securityEnabledConfig(), "test-version", svc.Dependencies{}))
	err := middleware.decryptRequest(httptest.NewRequest(http.MethodPost, "/api/demo", nil), []string{cipherWholeBody}, noopCryptor{})
	if err == nil || !strings.Contains(err.Error(), "整包") {
		t.Fatalf("decryptRequest() error = %v, want whole-body rejection", err)
	}
}

func TestCryptoMiddlewareRejectsTooManyCipherFields(t *testing.T) {
	fields := []string{"f1", "f2", "f3", "f4", "f5", "f6", "f7", "f8", "f9"}
	raw := security.EncodeCipherParams(fields)
	_, err := decodeAndValidateCipherParams(raw, fields, "请求")
	if err == nil || !strings.Contains(err.Error(), "数量超过上限") {
		t.Fatalf("decodeAndValidateCipherParams() error = %v, want field count rejection", err)
	}
}

func TestCryptoMiddlewareRejectsOversizeCipherHeader(t *testing.T) {
	raw := strings.Repeat("x", security.MaxSecurityJSONFieldBytes+1)
	_, err := decodeAndValidateCipherParams(raw, []string{"password"}, "请求")
	if err == nil || !strings.Contains(err.Error(), "长度超过上限") {
		t.Fatalf("decodeAndValidateCipherParams() error = %v, want header size rejection", err)
	}
}

func TestCryptoMiddlewareRejectsUndeclaredRequestCipher(t *testing.T) {
	raw := security.EncodeCipherParams([]string{"profile"})
	_, err := decodeAndValidateCipherParams(raw, []string{"password"}, "请求")
	if err == nil || !strings.Contains(err.Error(), "不允许") {
		t.Fatalf("decodeAndValidateCipherParams() error = %v, want undeclared field rejection", err)
	}
}

func TestCryptoMiddlewareRejectsOversizeRequestCipherValue(t *testing.T) {
	middleware := NewCryptoMiddleware(svc.NewServiceContext(securityEnabledConfig(), "test-version", svc.Dependencies{}))
	req := httptest.NewRequest(http.MethodPost, "/api/demo", strings.NewReader(`{"password":"`+strings.Repeat("x", security.MaxSecurityFieldBytes+1)+`"}`))
	err := middleware.decryptRequest(req, []string{"password"}, noopCryptor{})
	if err == nil || !strings.Contains(err.Error(), "长度超过上限") {
		t.Fatalf("decryptRequest() error = %v, want oversize field rejection", err)
	}
}

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

func TestCryptoMiddlewareRejectsOversizeResponseCipherValue(t *testing.T) {
	middleware := NewCryptoMiddleware(svc.NewServiceContext(securityEnabledConfig(), "test-version", svc.Dependencies{}))
	recorder := newBodyRecorder()
	_, _ = recorder.body.WriteString(`{"status":true,"data":{"token":"` + strings.Repeat("x", security.MaxSecurityFieldBytes+1) + `"}}`)
	err := middleware.encryptResponse(recorder, []string{"token"}, noopCryptor{})
	if err == nil || !strings.Contains(err.Error(), "长度超过上限") {
		t.Fatalf("encryptResponse() error = %v, want oversize field rejection", err)
	}
}

func TestCryptoMiddlewareRejectsWholeBodyResponseCipher(t *testing.T) {
	middleware := NewCryptoMiddleware(svc.NewServiceContext(securityEnabledConfig(), "test-version", svc.Dependencies{}))
	recorder := newBodyRecorder()
	_, _ = recorder.body.WriteString(`{"status":true,"data":{"items":[1,2,3]}}`)
	err := middleware.encryptResponse(recorder, []string{cipherWholeBody}, noopCryptor{})
	if err == nil || !strings.Contains(err.Error(), "整包") {
		t.Fatalf("encryptResponse() error = %v, want whole-body rejection", err)
	}
}

func TestCryptoMiddlewareRejectsUndeclaredResponseCipher(t *testing.T) {
	raw := security.EncodeCipherParams([]string{"items"})
	_, err := decodeAndValidateCipherParams(raw, []string{"token"}, "响应")
	if err == nil || !strings.Contains(err.Error(), "不允许") {
		t.Fatalf("decodeAndValidateCipherParams() error = %v, want undeclared field rejection", err)
	}
}

type noopCryptor struct{}

func (noopCryptor) Encrypt(data string) (string, error) {
	return data, nil
}

func (noopCryptor) Decrypt(data string) (string, error) {
	return data, nil
}

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
