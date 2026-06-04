package docs

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestHandlerServesInterfaceDocs 校验内网文档入口可输出接口文档资源。
func TestHandlerServesInterfaceDocs(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/internal/docs/%E6%8E%A5%E5%8F%A3%E6%96%87%E6%A1%A3/%E5%89%8D%E5%8F%B0%E7%B3%BB%E7%BB%9F/%E8%AE%A4%E8%AF%81%E6%8E%A5%E5%8F%A3.md", nil)
	recorder := httptest.NewRecorder()

	Handler()(recorder, req)
	if recorder.Code != http.StatusOK {
		t.Fatalf("http status = %d, want %d", recorder.Code, http.StatusOK)
	}
	if !strings.Contains(recorder.Body.String(), "认证接口") {
		t.Fatal("response should contain auth document content")
	}
}

// TestHandlerServesSidebar 校验文档站侧边栏由 API 项目自身提供。
func TestHandlerServesSidebar(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/internal/docs/_sidebar.md", nil)
	recorder := httptest.NewRecorder()

	Handler()(recorder, req)
	if recorder.Code != http.StatusOK {
		t.Fatalf("http status = %d, want %d", recorder.Code, http.StatusOK)
	}
	body := recorder.Body.String()
	for _, text := range []string{"前台 API 文档", "接口文档", "角色文档", "AI开发规范"} {
		if !strings.Contains(body, text) {
			t.Fatalf("sidebar missing %q", text)
		}
	}
}

// TestHandlerServesRoleDocs 校验 API 角色文档可通过后台文档站阅读。
func TestHandlerServesRoleDocs(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/internal/docs/角色文档/后端开发/AI开发规范.md", nil)
	recorder := httptest.NewRecorder()

	Handler()(recorder, req)
	if recorder.Code != http.StatusOK {
		t.Fatalf("http status = %d, want %d", recorder.Code, http.StatusOK)
	}
	if !strings.Contains(recorder.Body.String(), "AI开发规范") {
		t.Fatal("response should contain role document content")
	}
}

// TestHandlerRejectsManifest 校验安全清单不会作为 Markdown 页面资源输出。
func TestHandlerRejectsManifest(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/internal/docs/route_security_manifest.json", nil)
	recorder := httptest.NewRecorder()

	Handler()(recorder, req)
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("http status = %d, want %d", recorder.Code, http.StatusNotFound)
	}
}

// TestInterfaceSpecLinksVisibleRoleDocs 校验接口规范可链接到同站角色文档。
func TestInterfaceSpecLinksVisibleRoleDocs(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/internal/docs/接口文档/接口文档统一规范.md", nil)
	recorder := httptest.NewRecorder()

	Handler()(recorder, req)
	if recorder.Code != http.StatusOK {
		t.Fatalf("http status = %d, want %d", recorder.Code, http.StatusOK)
	}
	if !strings.Contains(recorder.Body.String(), "](角色文档/") {
		t.Fatal("interface docs should link to visible role documents")
	}
}

// TestInternalDocsAssetPathCleansTraversal 校验路径穿越不会绕过文档资源范围。
func TestInternalDocsAssetPathCleansTraversal(t *testing.T) {
	if _, ok := internalDocsAssetPath("/internal/docs/接口文档/前台系统/../../角色文档/后端开发/AI开发规范.md"); ok {
		t.Fatal("path traversal should be rejected")
	}
}
