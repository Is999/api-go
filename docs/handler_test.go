package docs

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestHandlerServesInterfaceDocs 校验内网文档入口只输出接口文档资源。
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

// TestHandlerRejectsRoleDocs 校验 API 角色文档不会通过后台代理入口泄露。
func TestHandlerRejectsRoleDocs(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/internal/docs/角色文档/后端开发/AI开发规范.md", nil)
	recorder := httptest.NewRecorder()

	Handler()(recorder, req)
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("http status = %d, want %d", recorder.Code, http.StatusNotFound)
	}
}

// TestInternalDocsAssetPathCleansTraversal 校验路径穿越不会绕过接口文档范围。
func TestInternalDocsAssetPathCleansTraversal(t *testing.T) {
	if _, ok := internalDocsAssetPath("/internal/docs/接口文档/前台系统/../../角色文档/后端开发/AI开发规范.md"); ok {
		t.Fatal("path traversal should be rejected")
	}
}
