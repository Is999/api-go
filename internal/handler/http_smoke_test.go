package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	codes "gozero_api/common/codes"
	i18n "gozero_api/common/i18n"
	"gozero_api/helper"
	"gozero_api/internal/config"
	"gozero_api/internal/middleware"
	"gozero_api/internal/requestctx"
	"gozero_api/internal/svc"
	"gozero_api/internal/types"
)

// httpSmokeEnvelope 是 handler 冒烟测试使用的统一响应包。
type httpSmokeEnvelope struct {
	Status  bool           `json:"status"`  // 业务成功标记
	Code    int            `json:"code"`    // 业务响应码
	Message string         `json:"message"` // 多语言响应文案
	Data    map[string]any `json:"data"`    // 响应数据首层对象
	TraceID string         `json:"traceId"` // 请求链路追踪 ID
	SpanID  string         `json:"spanId"`  // 当前服务 span ID
}

func TestWriteBizResponseUsesUnifiedEnvelope(t *testing.T) {
	req := newSmokeRequest(http.MethodGet, "/api/user/profile", nil)
	rec := httptest.NewRecorder()

	writeBizResponse(rec, req, types.NewBizResult(codes.FetchSuccess).
		SetI18nMessage(i18n.MsgKeyFetchSuccess).
		WithData(map[string]any{"ok": true}))

	if rec.Code != http.StatusOK {
		t.Fatalf("http status = %d, want %d", rec.Code, http.StatusOK)
	}
	envelope := decodeSmokeEnvelope(t, rec)
	if !envelope.Status {
		t.Fatal("status = false, want true")
	}
	if envelope.Code != codes.FetchSuccess {
		t.Fatalf("code = %d, want %d", envelope.Code, codes.FetchSuccess)
	}
	if envelope.Message == "" {
		t.Fatal("message should not be empty")
	}
	if envelope.TraceID != "trace-smoke" || envelope.SpanID != "span-smoke" {
		t.Fatalf("trace/span = %s/%s, want trace-smoke/span-smoke", envelope.TraceID, envelope.SpanID)
	}
	if got, ok := envelope.Data["ok"].(bool); !ok || !got {
		t.Fatalf("data.ok = %#v, want true", envelope.Data["ok"])
	}
}

func TestAuthMiddlewareMissingBearerUsesUnifiedEnvelope(t *testing.T) {
	svcCtx := svc.NewServiceContext(smokeConfig(), "test-version", svc.Dependencies{})
	authMw := middleware.NewAuthMiddleware(svcCtx)
	nextCalled := false
	handler := authMw.Handle(func(w http.ResponseWriter, r *http.Request) {
		nextCalled = true
	}, middleware.RouteAlias("user.profile"))

	req := newSmokeRequest(http.MethodGet, "/api/user/profile", nil)
	rec := httptest.NewRecorder()
	handler(rec, req)

	if nextCalled {
		t.Fatal("protected handler should not be called without bearer token")
	}
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("http status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
	envelope := decodeSmokeEnvelope(t, rec)
	if envelope.Status {
		t.Fatal("status = true, want false")
	}
	if envelope.Code != codes.Unauthorized {
		t.Fatalf("code = %d, want %d", envelope.Code, codes.Unauthorized)
	}
	if envelope.Message == "" {
		t.Fatal("message should not be empty")
	}
	if envelope.TraceID != "trace-smoke" || envelope.SpanID != "span-smoke" {
		t.Fatalf("trace/span = %s/%s, want trace-smoke/span-smoke", envelope.TraceID, envelope.SpanID)
	}
}

func TestPublicSecurityChainAllowsPlainJSONWithoutSecret(t *testing.T) {
	svcCtx := svc.NewServiceContext(smokeConfig(), "test-version", svc.Dependencies{})
	authMw := middleware.NewAuthMiddleware(svcCtx)
	nextCalled := false
	handler := authMw.PublicHandle(func(w http.ResponseWriter, r *http.Request) {
		nextCalled = true
		helper.NewJsonResp(r.Context(), w).SetCode(codes.Success).Success(map[string]any{"ok": true})
	}, middleware.RouteAlias("auth.login"))

	req := newSmokeRequest(http.MethodPost, "/api/auth/login", bytes.NewBufferString(`{"username":"demo","password":"secret123"}`))
	rec := httptest.NewRecorder()
	handler(rec, req)

	if !nextCalled {
		t.Fatal("public handler should be called for plain JSON when secret key is not configured")
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("http status = %d, want %d", rec.Code, http.StatusOK)
	}
	if rec.Header().Get("X-Signature") != "" || rec.Header().Get("X-Cipher") != "" {
		t.Fatalf("security headers should be empty, got signature=%q cipher=%q", rec.Header().Get("X-Signature"), rec.Header().Get("X-Cipher"))
	}
	envelope := decodeSmokeEnvelope(t, rec)
	if !envelope.Status {
		t.Fatal("status = false, want true")
	}
	if envelope.Code != codes.Success {
		t.Fatalf("code = %d, want %d", envelope.Code, codes.Success)
	}
	if got, ok := envelope.Data["ok"].(bool); !ok || !got {
		t.Fatalf("data.ok = %#v, want true", envelope.Data["ok"])
	}
}

func smokeConfig() config.Config {
	return config.Config{
		AppID:     "site-smoke",
		JwtSecret: "test-secret-please-change",
	}
}

func newSmokeRequest(method string, path string, body *bytes.Buffer) *http.Request {
	if body == nil {
		body = bytes.NewBuffer(nil)
	}
	ctx, _ := requestctx.New(context.Background())
	requestctx.SetTrace(ctx, "trace-smoke", "span-smoke")
	requestctx.SetRequest(ctx, method, path, "127.0.0.1")
	req := httptest.NewRequest(method, path, body).WithContext(ctx)
	req.Header.Set("Content-Type", "application/json")
	return req
}

func decodeSmokeEnvelope(t *testing.T, rec *httptest.ResponseRecorder) httpSmokeEnvelope {
	t.Helper()
	var envelope httpSmokeEnvelope
	if err := json.Unmarshal(rec.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("decode response JSON: %v, body=%s", err, rec.Body.String())
	}
	return envelope
}
