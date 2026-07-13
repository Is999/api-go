package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"api/internal/requestctx"
)

// TestRecoverMiddlewareKeepsPanicCauseInRequestMeta 验证统一错误响应不会把已捕获的 panic 原因覆盖为空。
func TestRecoverMiddlewareKeepsPanicCauseInRequestMeta(t *testing.T) {
	ctx, _ := requestctx.New(t.Context())
	request := httptest.NewRequest(http.MethodGet, "/panic", nil).WithContext(ctx)
	recorder := httptest.NewRecorder()
	NewRecoverMiddleware().Handle(func(http.ResponseWriter, *http.Request) {
		panic("boom")
	})(recorder, request)

	meta := requestctx.FromContext(ctx)
	if meta == nil || meta.ErrorCause == nil || !strings.Contains(meta.ErrorCause.Error(), "boom") {
		t.Fatalf("panic cause missing from request meta: %+v", meta)
	}
	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", recorder.Code)
	}
}
