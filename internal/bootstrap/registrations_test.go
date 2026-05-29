package bootstrap

import "testing"

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
