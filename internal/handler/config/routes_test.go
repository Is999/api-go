package config

import (
	"strings"
	"testing"
)

// TestInternalConfigReloadPaths 确保配置热加载只挂载内网路由前缀。
func TestInternalConfigReloadPaths(t *testing.T) {
	items := map[string]string{
		"status": InternalConfigReloadStatusPath,
		"run":    InternalConfigReloadRunPath,
	}
	for name, path := range items {
		if !strings.HasPrefix(path, "/internal/") {
			t.Fatalf("%s path must use /internal/ prefix: %s", name, path)
		}
		if strings.HasPrefix(path, "/api/") {
			t.Fatalf("%s path must not use public /api/ prefix: %s", name, path)
		}
	}
}
