package keys

import "testing"

func TestNormalizeAppID(t *testing.T) {
	if got := NormalizeAppID(" site-a "); got != "site-a" {
		t.Fatalf("NormalizeAppID() = %q, want site-a", got)
	}
	if got := NormalizeAppID(""); got != AppScopedDefaultAppID {
		t.Fatalf("NormalizeAppID(empty) = %q, want %q", got, AppScopedDefaultAppID)
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
			name: "uses default app id",
			key:  "user:session:42:jti",
			want: "app:default:user:session:42:jti",
		},
		{
			name:  "keeps scoped key unchanged",
			appID: "site-b",
			key:   "app:site-a:user:session:42:jti",
			want:  "app:site-a:user:session:42:jti",
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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := TrimAppScopedPrefix(tt.key); got != tt.want {
				t.Fatalf("TrimAppScopedPrefix() = %q, want %q", got, tt.want)
			}
		})
	}
}
