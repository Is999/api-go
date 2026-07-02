package manifest

import (
	"strings"
	"testing"

	"api/internal/bootstrap/components"
	"api/internal/bootstrap/register"
)

// TestDefaultKeepsComponentOrder 确保默认清单以启动组件顺序开头。
func TestDefaultKeepsComponentOrder(t *testing.T) {
	items := Default()
	componentSpecs := components.DefaultSpecs()
	if len(items) < len(componentSpecs) {
		t.Fatalf("默认注册清单长度不足: got=%d want>=%d", len(items), len(componentSpecs))
	}
	for index, spec := range componentSpecs {
		item := items[index]
		if item.Kind != register.KindComponent ||
			item.Name != spec.Name ||
			item.File != spec.File ||
			item.Method != spec.Method ||
			item.Description != spec.Description {
			t.Fatalf("组件清单顺序或字段不符合预期: index=%d item=%+v spec=%+v", index, item, spec)
		}
	}
}

// TestDefaultItemsValid 确保默认注册清单字段完整且同类注册名称唯一。
func TestDefaultItemsValid(t *testing.T) {
	items := Default()
	if len(items) == 0 {
		t.Fatal("默认注册清单不能为空")
	}
	seen := make(map[string]struct{}, len(items))
	for _, item := range items {
		if strings.TrimSpace(item.Kind) == "" ||
			strings.TrimSpace(item.Name) == "" ||
			strings.TrimSpace(item.File) == "" ||
			strings.TrimSpace(item.Method) == "" ||
			strings.TrimSpace(item.Description) == "" {
			t.Fatalf("默认注册清单字段不完整: %+v", item)
		}
		key := item.Kind + ":" + item.Name
		if _, ok := seen[key]; ok {
			t.Fatalf("默认注册清单名称重复: %s", key)
		}
		seen[key] = struct{}{}
	}
}
