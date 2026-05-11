package bootstrap

import (
	"testing"

	"api/internal/handler"
)

// TestValidateDefaultRegistrationManifest 确保默认注册清单与真实内置注册集合保持一致。
func TestValidateDefaultRegistrationManifest(t *testing.T) {
	if err := ValidateDefaultRegistrationManifest(); err != nil {
		t.Fatalf("校验默认注册清单失败: %v", err)
	}
}

// TestDefaultRegistrationManifestHasBuiltinEntries 确保清单覆盖路由和轻量运行时扩展两类内置注册。
func TestDefaultRegistrationManifestHasBuiltinEntries(t *testing.T) {
	items := DefaultRegistrationManifest()
	if len(items) == 0 {
		t.Fatal("默认注册清单不能为空")
	}

	kindSet := make(map[string]struct{}, len(items))
	for _, item := range items {
		kindSet[item.Kind] = struct{}{}
	}
	for _, kind := range []string{registrationKindRoute, registrationKindRuntimeRegistry} {
		if _, ok := kindSet[kind]; !ok {
			t.Fatalf("默认注册清单缺少 kind=%s", kind)
		}
	}
}

// TestDefaultRuntimeRegistryNamesDeriveFromSpecs 确保运行时扩展名称从规格派生。
func TestDefaultRuntimeRegistryNamesDeriveFromSpecs(t *testing.T) {
	names := defaultRuntimeRegistryNames()
	specs := defaultRuntimeRegistrySpecs()
	if len(names) != len(specs) {
		t.Fatalf("runtime names count = %d, spec count = %d", len(names), len(specs))
	}
	for index, spec := range specs {
		if names[index] != spec.Name {
			t.Fatalf("runtime name mismatch index=%d names=%v specs=%+v", index, names, specs)
		}
	}
}

// TestRouteRegistrationManifestMatchesModuleSpecs 确保路由注册清单由内置模块规格派生。
func TestRouteRegistrationManifestMatchesModuleSpecs(t *testing.T) {
	routeItems := manifestItemsByKind(DefaultRegistrationManifest(), registrationKindRoute)
	specs := handler.BuiltinRouteModuleSpecs()
	if len(routeItems) != len(specs) {
		t.Fatalf("route manifest count = %d, spec count = %d", len(routeItems), len(specs))
	}
	for index, spec := range specs {
		item := routeItems[index]
		if item.Name != spec.Name || item.File != spec.File || item.Method != spec.Method || item.Description != spec.Description {
			t.Fatalf("route manifest mismatch index=%d item=%+v spec=%+v", index, item, spec)
		}
	}
}

// TestRuntimeRegistrationManifestMatchesSpecs 确保运行时注册清单由扩展规格派生。
func TestRuntimeRegistrationManifestMatchesSpecs(t *testing.T) {
	runtimeItems := manifestItemsByKind(DefaultRegistrationManifest(), registrationKindRuntimeRegistry)
	specs := defaultRuntimeRegistrySpecs()
	if len(runtimeItems) != len(specs) {
		t.Fatalf("runtime manifest count = %d, spec count = %d", len(runtimeItems), len(specs))
	}
	for index, spec := range specs {
		item := runtimeItems[index]
		if item.Name != spec.Name || item.File != spec.File || item.Method != spec.Method || item.Description != spec.Description {
			t.Fatalf("runtime manifest mismatch index=%d item=%+v spec=%+v", index, item, spec)
		}
	}
}

// TestValidateManifestItemsRejectsIncomplete 确保清单字段缺失会被校验拦截。
func TestValidateManifestItemsRejectsIncomplete(t *testing.T) {
	err := validateManifestItems([]RegistrationManifestItem{
		{
			Kind:        registrationKindRoute,
			Name:        "health",
			Method:      "handler.NewHealthRouteModule",
			Description: "注册健康检查路由",
		},
	})
	if err == nil {
		t.Fatal("期望清单字段缺失返回错误，实际为 nil")
	}
}

// manifestItemsByKind 按注册类型筛选默认注册清单。
func manifestItemsByKind(items []RegistrationManifestItem, kind string) []RegistrationManifestItem {
	result := make([]RegistrationManifestItem, 0, len(items))
	for _, item := range items {
		if item.Kind == kind {
			result = append(result, item)
		}
	}
	return result
}

// TestValidateManifestItemsRejectsDuplicate 确保清单重复项会被校验拦截。
func TestValidateManifestItemsRejectsDuplicate(t *testing.T) {
	items := []RegistrationManifestItem{
		{
			Kind:        registrationKindRoute,
			Name:        "health",
			File:        "internal/handler/routes.go",
			Method:      "handler.NewHealthRouteModule",
			Description: "注册健康检查路由",
		},
		{
			Kind:        registrationKindRoute,
			Name:        "health",
			File:        "internal/handler/routes.go",
			Method:      "handler.NewHealthRouteModule",
			Description: "注册健康检查路由",
		},
	}
	if err := validateManifestItems(items); err == nil {
		t.Fatal("期望清单重复项返回错误，实际为 nil")
	}
}

// TestValidateNameListUniqueRejectsDuplicate 确保真实注册集合出现重复名称时会被启动校验拦截。
func TestValidateNameListUniqueRejectsDuplicate(t *testing.T) {
	if err := validateNameListUnique(registrationKindRoute, []string{"user", "user"}); err == nil {
		t.Fatal("期望重复注册名称返回错误，实际为 nil")
	}
}

// TestDefaultRouteModulesPreservesOrder 确保 bootstrap 统一清单维护内置路由顺序。
func TestDefaultRouteModulesPreservesOrder(t *testing.T) {
	got := routeModuleNames(defaultRouteModules())
	want := []string{"health", "auth", "user", "config"}
	if len(got) != len(want) {
		t.Fatalf("内置路由数量不符合预期: got=%v want=%v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("内置路由顺序不符合预期: got=%v want=%v", got, want)
		}
	}

	fallback := routeModuleNames(handler.BuiltinRouteModules())
	if len(fallback) != len(got) {
		t.Fatalf("handler 兜底路由数量与 bootstrap 不一致: fallback=%v bootstrap=%v", fallback, got)
	}
	for i := range got {
		if fallback[i] != got[i] {
			t.Fatalf("handler 兜底路由与 bootstrap 不一致: fallback=%v bootstrap=%v", fallback, got)
		}
	}
}
