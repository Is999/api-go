package security

import (
	"encoding/base64"
	"encoding/json"
	"testing"

	utils "github.com/Is999/go-utils"
)

func TestBuildSignStringUsesStableOrder(t *testing.T) {
	data := map[string]any{
		"b":       2,
		"sign":    "ignored",
		"a":       "1",
		"profile": map[string]any{"name": "tom", "age": 18},
	}

	got := BuildSignString(data, []string{SignFieldAll}, "trace", "app")
	want := `a=1&b=2&profile={"age":18,"name":"tom"}&key=` + utils.Md5("app-trace")
	if got != want {
		t.Fatalf("BuildSignString() = %q, want %q", got, want)
	}
}

func TestEncodeCipherParams(t *testing.T) {
	if got := EncodeCipherParams([]string{CipherWholeBody}); got != CipherWholeBody {
		t.Fatalf("EncodeCipherParams whole body = %q", got)
	}

	got := EncodeCipherParams([]string{"token", " token ", "", "user.email"})
	body, err := base64.StdEncoding.DecodeString(got)
	if err != nil {
		t.Fatalf("DecodeString() error = %v", err)
	}
	var params []string
	if err := json.Unmarshal(body, &params); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	want := []string{"token", "user.email"}
	if len(params) != len(want) {
		t.Fatalf("params len = %d, want %d", len(params), len(want))
	}
	for i := range want {
		if params[i] != want[i] {
			t.Fatalf("params[%d] = %q, want %q", i, params[i], want[i])
		}
	}
}

func TestPolicyByRouteUnknownKeepsEmptyPolicy(t *testing.T) {
	policy := PolicyByRoute("unknown.route")
	if len(policy.RequestSign) != 0 || len(policy.RequestCipher) != 0 || len(policy.ResponseSign) != 0 || len(policy.ResponseCipher) != 0 {
		t.Fatalf("PolicyByRoute unknown = %+v, want empty policy", policy)
	}
}

func TestNormalizeSecurityHeaderTypes(t *testing.T) {
	signCases := map[string]string{
		"":    SignatureTypeRSA,
		"R":   SignatureTypeRSA,
		"rsa": SignatureTypeRSA,
		"AES": SignatureTypeAES,
		"md5": SignatureTypeMD5,
	}
	for input, want := range signCases {
		if got := NormalizeSignatureType(input); got != want {
			t.Fatalf("NormalizeSignatureType(%q) = %q, want %q", input, got, want)
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
