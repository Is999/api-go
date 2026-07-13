package keys

import "testing"

// TestUserSessionKeys 验证会话 Key 文本和 Redis Cluster hash tag。
func TestUserSessionKeys(t *testing.T) {
	useAppID(t, "1")
	want := []string{
		"app:1:user:session:{42}",
		"app:1:user:session:index:{42}",
		"app:1:user:session:auth_version:{42}",
	}
	got := UserSessionKeys(42)
	for index := range want {
		if got[index] != want[index] {
			t.Fatalf("UserSessionKeys()[%d] = %q, want %q", index, got[index], want[index])
		}
	}
}
