package bootstrap

import "testing"

// TestDefaultRegistrationManifestValid 确保默认注册清单可供文档和排障读取。
func TestDefaultRegistrationManifestValid(t *testing.T) {
	items := DefaultRegistrationManifest()
	if len(items) == 0 {
		t.Fatal("默认注册清单不能为空")
	}
	seen := make(map[string]struct{}, len(items))
	knownKinds := map[string]struct{}{
		registrationKindComponent:       {},
		registrationKindRoute:           {},
		registrationKindRuntimeRegistry: {},
	}
	for _, item := range items {
		if _, ok := knownKinds[item.Kind]; !ok {
			t.Fatalf("默认注册清单存在未知 kind: %+v", item)
		}
		if item.Name == "" || item.File == "" || item.Method == "" || item.Description == "" {
			t.Fatalf("默认注册清单字段不完整: %+v", item)
		}
		key := item.Kind + ":" + item.Name
		if _, ok := seen[key]; ok {
			t.Fatalf("默认注册清单存在重复项: %s", key)
		}
		seen[key] = struct{}{}
	}
}

// TestValidateRegistrationNamesUniqueRejectsDuplicate 确保真实注册集合出现重复名称时会被启动校验拦截。
func TestValidateRegistrationNamesUniqueRejectsDuplicate(t *testing.T) {
	if err := validateRegistrationNamesUnique(registrationKindRoute, []string{"user", "user"}); err == nil {
		t.Fatal("期望重复注册名称返回错误，实际为 nil")
	}
}

// TestDefaultRouteModulesNamesUnique 确保默认路由模块注册名不重复。
func TestDefaultRouteModulesNamesUnique(t *testing.T) {
	names := routeModuleNames(defaultRouteModules())
	if err := validateRegistrationNamesUnique(registrationKindRoute, names); err != nil {
		t.Fatalf("默认路由模块名称不合法: %v", err)
	}
}
