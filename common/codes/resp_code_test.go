package codes

import "testing"

// TestHTTPStatus 验证业务码到 HTTP 状态码的映射。
func TestHTTPStatus(t *testing.T) {
	tests := []struct {
		name string
		code int
		want int
	}{
		{name: "success", code: Success, want: OK},
		{name: "token invalid", code: TokenInvalid, want: Unauthorized},
		{name: "security signature", code: SecuritySignatureFailed, want: Unauthorized},
		{name: "security payload too large", code: SecurityPayloadTooLarge, want: 413},
		{name: "dependency", code: DependencyUnavailable, want: ServiceBusy},
		{name: "unknown failure", code: 999999, want: ServerError},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := HTTPStatus(tt.code); got != tt.want {
				t.Fatalf("HTTPStatus(%d)=%d, want %d", tt.code, got, tt.want)
			}
		})
	}
}
