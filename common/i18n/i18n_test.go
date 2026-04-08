package i18n

import (
	"strings"
	"testing"

	codes "gozero_api/common/codes"
)

// TestMessageByCodeUsesLocale 验证业务码按语言返回对应文案。
func TestMessageByCodeUsesLocale(t *testing.T) {
	if got := MessageByCode(codes.TokenExpired, LocaleENUS); !strings.Contains(strings.ToLower(got), "expired") {
		t.Fatalf("MessageByCode(TokenExpired,en-US)=%q, want expired text", got)
	}
	if got := MessageByCode(codes.TokenExpired, LocaleZHCN); !strings.Contains(got, "过期") {
		t.Fatalf("MessageByCode(TokenExpired,zh-CN)=%q, want 中文过期文案", got)
	}
	if got := MessageByCode(codes.SecuritySignatureFailed, LocaleENUS); !strings.Contains(strings.ToLower(got), "signature") {
		t.Fatalf("MessageByCode(SecuritySignatureFailed,en-US)=%q, want signature text", got)
	}
	if got := MessageByCode(codes.SecurityPayloadTooLarge, LocaleZHCN); !strings.Contains(got, "限制") {
		t.Fatalf("MessageByCode(SecurityPayloadTooLarge,zh-CN)=%q, want 中文限制文案", got)
	}
}

// TestNormalizeLocale 验证语言标签归一化和默认回退。
func TestNormalizeLocale(t *testing.T) {
	if got := NormalizeLocale("en;q=0.8"); got != LocaleENUS {
		t.Fatalf("NormalizeLocale(en)= %q, want %q", got, LocaleENUS)
	}
	if got := NormalizeLocale("fr-FR, en-US;q=0.8, zh-CN;q=0.6"); got != LocaleENUS {
		t.Fatalf("NormalizeLocale(fr,en-US,zh-CN)= %q, want %q", got, LocaleENUS)
	}
	if got := NormalizeLocale("zh-HK, zh-CN;q=0.8"); got != LocaleZHCN {
		t.Fatalf("NormalizeLocale(zh-HK)= %q, want %q", got, LocaleZHCN)
	}
	if got := NormalizeLocale("fr"); got != LocaleZHCN {
		t.Fatalf("NormalizeLocale(fr)= %q, want %q", got, LocaleZHCN)
	}
}

// TestCatalogLocalesHaveSameKeys 确保所有语言包维护同一批消息 key。
func TestCatalogLocalesHaveSameKeys(t *testing.T) {
	zh := messageCatalog[LocaleZHCN]
	en := messageCatalog[LocaleENUS]
	for key := range zh {
		if en[key] == "" {
			t.Fatalf("en-US catalog missing key=%s", key)
		}
	}
	for key := range en {
		if zh[key] == "" {
			t.Fatalf("zh-CN catalog missing key=%s", key)
		}
	}
}

// TestCodeToMessageKeyCoveredByCatalog 确保业务码映射到的消息 key 都有语言包文案。
func TestCodeToMessageKeyCoveredByCatalog(t *testing.T) {
	for code, key := range codeToMessageKey {
		for locale, catalog := range messageCatalog {
			if catalog[key] == "" {
				t.Fatalf("catalog locale=%s missing code=%d key=%s", locale, code, key)
			}
		}
	}
}
