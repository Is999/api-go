package embedasset

import "testing"

// TestStripLeadingLineComments 验证文本资产文件头注释剥离行为。
func TestStripLeadingLineComments(t *testing.T) {
	source := "-- 文件头\n-- 说明\n\nCREATE TABLE `api_user` (`id` bigint);\n"

	got := StripLeadingLineComments(source, "--")

	want := "CREATE TABLE `api_user` (`id` bigint);\n"
	if got != want {
		t.Fatalf("StripLeadingLineComments() = %q, want %q", got, want)
	}
}
