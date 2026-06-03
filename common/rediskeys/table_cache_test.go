package keys

import "testing"

// TestTableCachePrefix 验证对应场景符合预期。
func TestTableCachePrefix(t *testing.T) {
	useAppID(t, "site-a")
	if got, want := TableCachePrefix(), "app:site-a:table:"; got != want {
		t.Fatalf("TableCachePrefix() = %q, want %q", got, want)
	}
}

// TestIsTableCacheKey 验证对应场景符合预期。
func TestIsTableCacheKey(t *testing.T) {
	tests := []struct {
		name  string // name 表示测试场景名称。
		appID string // appID 表示测试应用 ID。
		key   string // key 表示待验证 key。
		want  bool   // want 表示期望结果。
	}{
		{name: "current app table key", appID: "site-a", key: "app:site-a:table:config_uuid:featureFlag", want: true},
		{name: "other app table key", appID: "site-a", key: "app:site-b:table:config_uuid:featureFlag", want: false},
		{name: "direct app key", appID: "site-a", key: "app:site-a:config_uuid:featureFlag", want: false},
		{name: "logical table segment", appID: "site-a", key: "table:config_uuid:featureFlag", want: false},
		{name: "incomplete table prefix", appID: "site-a", key: "app:site-a:table", want: false},
		{name: "empty app id", appID: "", key: "app:site-a:table:config_uuid:featureFlag", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			useAppID(t, tt.appID)
			if got := IsTableCacheKey(tt.key); got != tt.want {
				t.Fatalf("IsTableCacheKey() = %t, want %t", got, tt.want)
			}
		})
	}
}

// TestTrimTableCachePrefix 验证对应场景符合预期。
func TestTrimTableCachePrefix(t *testing.T) {
	tests := []struct {
		name  string // name 表示测试场景名称。
		appID string // appID 表示测试应用 ID。
		key   string // key 表示待验证 key。
		want  string // want 表示期望结果。
	}{
		{name: "trims table key", appID: "site-a", key: "app:site-a:table:config_uuid:featureFlag", want: "config_uuid:featureFlag"},
		{name: "keeps other app table key", appID: "site-a", key: "app:site-b:table:config_uuid:featureFlag", want: "app:site-b:table:config_uuid:featureFlag"},
		{name: "keeps direct app key", appID: "site-a", key: "app:site-a:config_uuid:featureFlag", want: "app:site-a:config_uuid:featureFlag"},
		{name: "keeps logical key", appID: "site-a", key: "config_uuid:featureFlag", want: "config_uuid:featureFlag"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			useAppID(t, tt.appID)
			if got := TrimTableCachePrefix(tt.key); got != tt.want {
				t.Fatalf("TrimTableCachePrefix() = %q, want %q", got, tt.want)
			}
		})
	}
}
