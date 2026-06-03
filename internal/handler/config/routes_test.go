package config

import (
	"strings"
	"testing"
)

// TestInternalConfigReloadPaths 确保配置热加载只挂载内网路由前缀。
func TestInternalConfigReloadPaths(t *testing.T) {
	for _, spec := range RouteSpecs() {
		if !strings.HasPrefix(spec.Path, "/internal/") {
			t.Fatalf("%s path must use /internal/ prefix: %s", spec.Meta.Alias, spec.Path)
		}
		if strings.HasPrefix(spec.Path, "/api/") {
			t.Fatalf("%s path must not use public /api/ prefix: %s", spec.Meta.Alias, spec.Path)
		}
	}
}
