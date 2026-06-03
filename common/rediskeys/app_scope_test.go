package keys

import (
	"testing"
)

// TestWithPrefix 验证对应场景符合预期。
func TestWithPrefix(t *testing.T) {
	tests := []struct {
		name  string // name 表示测试场景名称。
		appID string // appID 表示测试应用 ID。
		key   string // key 表示待验证 key。
		want  string // want 表示期望结果。
	}{
		{
			name:  "scopes logical key",
			appID: "site-a",
			key:   "config_uuid:featureFlag",
			want:  "app:site-a:config_uuid:featureFlag",
		},
		{
			name:  "keeps current app scoped key unchanged",
			appID: "site-a",
			key:   "app:site-a:user:session:42:jti",
			want:  "app:site-a:user:session:42:jti",
		},
		{
			name:  "rejects other app scoped key",
			appID: "site-b",
			key:   "app:site-a:user:session:42:jti",
			want:  "",
		},
		{
			name:  "scopes incomplete app prefix as logical key",
			appID: "site-b",
			key:   "app:site-a",
			want:  "app:site-b:app:site-a",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			useAppID(t, tt.appID)
			if got := WithPrefix(tt.key); got != tt.want {
				t.Fatalf("WithPrefix() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestHasPrefix 验证对应场景符合预期。
func TestHasPrefix(t *testing.T) {
	tests := []struct {
		name string // name 表示测试场景名称。
		key  string // key 表示待验证 key。
		want bool   // want 表示期望结果。
	}{
		{name: "scoped key", key: "app:site-a:user:session:42:jti", want: true},
		{name: "empty logical key", key: "app:site-a:", want: false},
		{name: "missing logical separator", key: "app:site-a", want: false},
		{name: "missing app id", key: "app::user:session:42:jti", want: false},
		{name: "logical key", key: "user:session:42:jti", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := HasPrefix(tt.key); got != tt.want {
				t.Fatalf("HasPrefix() = %t, want %t", got, tt.want)
			}
		})
	}
}

// TestOwner 验证对应场景符合预期。
func TestOwner(t *testing.T) {
	tests := []struct {
		name   string // name 表示测试场景名称。
		key    string // key 表示待验证 key。
		want   string // want 表示期望结果。
		wantOK bool   // wantOK 表示期望是否成功。
	}{
		{name: "scoped key", key: "app:site-a:user:session:42:jti", want: "site-a", wantOK: true},
		{name: "empty logical key", key: "app:site-a:", want: "", wantOK: false},
		{name: "missing logical separator", key: "app:site-a", want: "", wantOK: false},
		{name: "missing app id", key: "app::user:session:42:jti", want: "", wantOK: false},
		{name: "logical key", key: "user:session:42:jti", want: "", wantOK: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := Owner(tt.key)
			if got != tt.want || ok != tt.wantOK {
				t.Fatalf("Owner() = %q, %t, want %q, %t", got, ok, tt.want, tt.wantOK)
			}
		})
	}
}

// TestIsForeignKey 验证对应场景符合预期。
func TestIsForeignKey(t *testing.T) {
	tests := []struct {
		name  string // name 表示测试场景名称。
		appID string // appID 表示测试应用 ID。
		key   string // key 表示待验证 key。
		want  bool   // want 表示期望结果。
	}{
		{name: "current app key", appID: "site-a", key: "app:site-a:user:session:42:jti", want: false},
		{name: "other app key", appID: "site-a", key: "app:site-b:user:session:42:jti", want: true},
		{name: "logical key", appID: "site-a", key: "user:session:42:jti", want: false},
		{name: "empty app id", appID: "", key: "app:site-a:user:session:42:jti", want: true},
		{name: "incomplete prefix", appID: "site-a", key: "app:site-b", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			useAppID(t, tt.appID)
			if got := IsForeignKey(tt.key); got != tt.want {
				t.Fatalf("IsForeignKey() = %t, want %t", got, tt.want)
			}
		})
	}
}

// TestWithPrefixWithEmptyAppIDFailsClosed 验证对应场景符合预期。
func TestWithPrefixWithEmptyAppIDFailsClosed(t *testing.T) {
	useAppID(t, "")
	if got := Prefix(); got != "" {
		t.Fatalf("Prefix(empty) = %q, want empty", got)
	}
	if got := WithPrefix("user:session:42:jti"); got != "" {
		t.Fatalf("WithPrefix(empty app, logical key) = %q, want empty", got)
	}
	if got := WithPrefix("app:site-a:user:session:42:jti"); got != "" {
		t.Fatalf("WithPrefix(empty app, scoped key) = %q, want empty", got)
	}
}

// TestTrimPrefix 验证对应场景符合预期。
func TestTrimPrefix(t *testing.T) {
	tests := []struct {
		name string // name 表示测试场景名称。
		key  string // key 表示待验证 key。
		want string // want 表示期望结果。
	}{
		{
			name: "trims scoped key",
			key:  "app:site-a:user:session:42:jti",
			want: "user:session:42:jti",
		},
		{
			name: "keeps logical key",
			key:  "user:session:42:jti",
			want: "user:session:42:jti",
		},
		{
			name: "keeps incomplete prefix",
			key:  "app:site-a",
			want: "app:site-a",
		},
		{
			name: "keeps missing app id prefix",
			key:  "app::user:session:42:jti",
			want: "app::user:session:42:jti",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := TrimPrefix(tt.key); got != tt.want {
				t.Fatalf("TrimPrefix() = %q, want %q", got, tt.want)
			}
		})
	}
}
