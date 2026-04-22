package keys

import "testing"

func TestNormalizeAppID(t *testing.T) {
	if got := NormalizeAppID(" site-a "); got != "site-a" {
		t.Fatalf("NormalizeAppID() = %q, want site-a", got)
	}
	if got := NormalizeAppID(""); got != "" {
		t.Fatalf("NormalizeAppID(empty) = %q, want empty", got)
	}
}

func TestAppScopedKey(t *testing.T) {
	tests := []struct {
		name  string
		appID string
		key   string
		want  string
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
			name:  "keeps other app scoped key unchanged",
			appID: "site-b",
			key:   "app:site-a:user:session:42:jti",
			want:  "app:site-a:user:session:42:jti",
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
			if got := AppScopedKey(tt.appID, tt.key); got != tt.want {
				t.Fatalf("AppScopedKey() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestHasAppScopedPrefix(t *testing.T) {
	tests := []struct {
		name string
		key  string
		want bool
	}{
		{name: "scoped key", key: "app:site-a:user:session:42:jti", want: true},
		{name: "empty logical key", key: "app:site-a:", want: false},
		{name: "missing logical separator", key: "app:site-a", want: false},
		{name: "missing app id", key: "app::user:session:42:jti", want: false},
		{name: "logical key", key: "user:session:42:jti", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := HasAppScopedPrefix(tt.key); got != tt.want {
				t.Fatalf("HasAppScopedPrefix() = %t, want %t", got, tt.want)
			}
		})
	}
}

func TestAppScopedAppID(t *testing.T) {
	tests := []struct {
		name   string
		key    string
		want   string
		wantOK bool
	}{
		{name: "scoped key", key: "app:site-a:user:session:42:jti", want: "site-a", wantOK: true},
		{name: "empty logical key", key: "app:site-a:", want: "", wantOK: false},
		{name: "missing logical separator", key: "app:site-a", want: "", wantOK: false},
		{name: "missing app id", key: "app::user:session:42:jti", want: "", wantOK: false},
		{name: "logical key", key: "user:session:42:jti", want: "", wantOK: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := AppScopedAppID(tt.key)
			if got != tt.want || ok != tt.wantOK {
				t.Fatalf("AppScopedAppID() = %q, %t, want %q, %t", got, ok, tt.want, tt.wantOK)
			}
		})
	}
}

func TestIsForeignAppScopedKey(t *testing.T) {
	tests := []struct {
		name  string
		appID string
		key   string
		want  bool
	}{
		{name: "current app key", appID: "site-a", key: "app:site-a:user:session:42:jti", want: false},
		{name: "other app key", appID: "site-a", key: "app:site-b:user:session:42:jti", want: true},
		{name: "logical key", appID: "site-a", key: "user:session:42:jti", want: false},
		{name: "empty app id", appID: "", key: "app:site-a:user:session:42:jti", want: false},
		{name: "incomplete prefix", appID: "site-a", key: "app:site-b", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsForeignAppScopedKey(tt.appID, tt.key); got != tt.want {
				t.Fatalf("IsForeignAppScopedKey() = %t, want %t", got, tt.want)
			}
		})
	}
}

func TestAppScopedKeyWithEmptyAppIDFailsClosed(t *testing.T) {
	if got := AppScopedPrefix(""); got != "" {
		t.Fatalf("AppScopedPrefix(empty) = %q, want empty", got)
	}
	if got := AppScopedKey("", "user:session:42:jti"); got != "" {
		t.Fatalf("AppScopedKey(empty app, logical key) = %q, want empty", got)
	}
	if got := AppScopedKey("", "app:site-a:user:session:42:jti"); got != "app:site-a:user:session:42:jti" {
		t.Fatalf("AppScopedKey(empty app, scoped key) = %q", got)
	}
}

func TestTrimAppScopedPrefix(t *testing.T) {
	tests := []struct {
		name string
		key  string
		want string
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
			if got := TrimAppScopedPrefix(tt.key); got != tt.want {
				t.Fatalf("TrimAppScopedPrefix() = %q, want %q", got, tt.want)
			}
		})
	}
}
