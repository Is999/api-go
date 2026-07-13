package docs

import (
	"strings"
	"testing"

	"api/internal/handler/shared"
)

// TestDocsRoutesStayInternal 确保 API 只提供内网文档资源，不复制 Admin 角色权限模型。
func TestDocsRoutesStayInternal(t *testing.T) {
	specs := RouteSpecs()
	if len(specs) != 3 {
		t.Fatalf("docs route count = %d, want 3", len(specs))
	}
	for _, spec := range specs {
		if !strings.HasPrefix(spec.Path, "/internal/docs/") {
			t.Fatalf("docs route must use internal prefix: %s", spec.Path)
		}
		if spec.Chain != shared.RouteSecurityInternal {
			t.Fatalf("docs route %s security chain = %s, want internal", spec.Path, spec.Chain)
		}
	}
}
