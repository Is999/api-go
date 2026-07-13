package security

import (
	"testing"
)

// TestBuildSignStringUsesStableOrder 验证对应场景符合预期。
func TestBuildSignStringUsesStableOrder(t *testing.T) {
	data := map[string]any{
		"b":       2,
		"sign":    "ignored",
		"a":       "1",
		"profile": map[string]any{"name": "tom", "age": 18},
	}

	got := BuildSignString(data, []string{SignFieldAll}, "trace", "1700000000", "app")
	want := `v2|app=3:app|trace=5:trace|timestamp=10:1700000000|field=1:a1:1|field=1:b1:2|field=7:profile23:{"age":18,"name":"tom"}`
	if got != want {
		t.Fatalf("BuildSignString() = %q, want %q", got, want)
	}
}

// TestBuildSignStringWithoutBusinessFields 校验空字段策略只使用 AppID、TraceID 与时间戳。
func TestBuildSignStringWithoutBusinessFields(t *testing.T) {
	got := BuildSignString(map[string]any{"ignored": "value"}, []string{}, "trace-demo-000", "1700000000", "demo-app")
	want := "v2|app=8:demo-app|trace=14:trace-demo-000|timestamp=10:1700000000"
	if got != want {
		t.Fatalf("BuildSignString() = %q, want %q", got, want)
	}
}

// TestBuildSignStringStableArray 校验数组字段使用稳定 JSON 签名格式。
func TestBuildSignStringStableArray(t *testing.T) {
	got := BuildSignString(map[string]any{
		"ids": []int64{2, 3},
	}, []string{"ids"}, "trace-demo-004", "1700000000", "demo-app")
	want := "v2|app=8:demo-app|trace=14:trace-demo-004|timestamp=10:1700000000|field=3:ids5:[2,3]"
	if got != want {
		t.Fatalf("BuildSignString() = %q, want %q", got, want)
	}
}

// TestBuildSignStringSeparatesDelimiterValues 验证长度前缀协议不会因字段值包含旧分隔符而碰撞。
func TestBuildSignStringSeparatesDelimiterValues(t *testing.T) {
	left := BuildSignString(map[string]any{"a": "1&b=2", "b": "3"}, []string{"a", "b"}, "trace", "1700000000", "app")
	right := BuildSignString(map[string]any{"a": "1", "b": "2&b=3"}, []string{"a", "b"}, "trace", "1700000000", "app")
	if left == right {
		t.Fatalf("不同字段值生成了相同签名串: %q", left)
	}
}

// TestBuildSignStringResolvesNestedFieldPaths 确保嵌套密文字段的明文值真实参与回签。
func TestBuildSignStringResolvesNestedFieldPaths(t *testing.T) {
	data := map[string]any{
		"token": "token-value",
		"user": map[string]any{
			"email": "masked@example.test",
			"phone": "138****0000",
		},
	}
	got := BuildSignString(data, []string{"token", "user.email", "user.phone"}, "trace", "1700000000", "app")
	want := "v2|app=3:app|trace=5:trace|timestamp=10:1700000000" +
		"|field=5:token11:token-value" +
		"|field=10:user.email19:masked@example.test" +
		"|field=10:user.phone11:138****0000"
	if got != want {
		t.Fatalf("BuildSignString() = %q, want %q", got, want)
	}
}

// TestNormalizeSecurityHeaderTypes 验证对应场景符合预期。
func TestNormalizeSecurityHeaderTypes(t *testing.T) {
	signCases := map[string]string{
		"":    SignatureTypeRSA,
		"R":   SignatureTypeRSA,
		"rsa": SignatureTypeRSA,
		"AES": SignatureTypeAES,
	}
	for input, want := range signCases {
		if got := NormalizeSignatureType(input); got != want {
			t.Fatalf("NormalizeSignatureType(%q) = %q, want %q", input, got, want)
		}
	}
	for input, want := range map[string]string{"hmac": "HMAC", "md5": "MD5", "M": "M"} {
		if got := NormalizeSignatureType(input); got != want {
			t.Fatalf("NormalizeSignatureType(%q) = %q, want unsupported value %q", input, got, want)
		}
	}

	cryptoCases := map[string]string{
		"":    CryptoTypeAES,
		"A":   CryptoTypeAES,
		"aes": CryptoTypeAES,
		"RSA": CryptoTypeRSA,
	}
	for input, want := range cryptoCases {
		if got := NormalizeCryptoType(input); got != want {
			t.Fatalf("NormalizeCryptoType(%q) = %q, want %q", input, got, want)
		}
	}
}
