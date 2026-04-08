package bootstrap

import (
	"testing"

	"gozero_api/internal/handler"
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

// TestDefaultRuntimeRegistryNamesCoversBuiltinEntries 确保默认运行时扩展入口都被清单覆盖。
func TestDefaultRuntimeRegistryNamesCoversBuiltinEntries(t *testing.T) {
	names := defaultRuntimeRegistryNames()
	nameSet := make(map[string]struct{}, len(names))
	for _, name := range names {
		nameSet[name] = struct{}{}
	}
	for _, want := range []string{
		runtimeRegistryComponentLifecycle,
		runtimeRegistryCollectorProcessor,
		runtimeRegistryAuthSecurityEvent,
		runtimeRegistryAuthSecurityProcessor,
		runtimeRegistrySysConfigCache,
		runtimeRegistrySysConfigKeyRegistry,
		runtimeRegistryCacheRebuildLock,
	} {
		if _, ok := nameSet[want]; !ok {
			t.Fatalf("默认运行时注册集合缺少 %s: %v", want, names)
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
	want := []string{"health", "auth", "user", "system"}
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
