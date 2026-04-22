package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"api/internal/security"

	"github.com/Is999/go-utils/errors"
)

func TestReadRequestBodyRejectsOversizeContentLength(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/api/demo", strings.NewReader("{}"))
	req.ContentLength = security.MaxSecurityRequestBodyBytes + 1

	if _, err := readRequestBody(req); !errors.Is(err, security.ErrSecurityPayloadTooLarge) {
		t.Fatalf("readRequestBody() error = %v, want ErrSecurityPayloadTooLarge", err)
	}
}

func TestReadRequestBodyRejectsOversizeStream(t *testing.T) {
	body := strings.Repeat("x", security.MaxSecurityRequestBodyBytes+1)
	req := httptest.NewRequest(http.MethodPost, "/api/demo", strings.NewReader(body))
	req.ContentLength = -1

	if _, err := readRequestBody(req); !errors.Is(err, security.ErrSecurityPayloadTooLarge) {
		t.Fatalf("readRequestBody() error = %v, want ErrSecurityPayloadTooLarge", err)
	}
}

func TestReadRequestBodyKeepsReadableBody(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/api/demo", strings.NewReader(`{"name":"demo"}`))

	body, err := readRequestBody(req)
	if err != nil {
		t.Fatalf("readRequestBody() error = %v", err)
	}
	if string(body) != `{"name":"demo"}` {
		t.Fatalf("readRequestBody() = %q", string(body))
	}
	bodyAgain, err := readRequestBody(req)
	if err != nil {
		t.Fatalf("readRequestBody() second read error = %v", err)
	}
	if string(bodyAgain) != string(body) {
		t.Fatalf("second body = %q, want %q", string(bodyAgain), string(body))
	}
}
