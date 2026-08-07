package manifest

import (
	"api/internal/bootstrap/components"
	"api/internal/bootstrap/register"
	"api/internal/handler"
	corelogic "api/internal/logic"
	authlogic "api/internal/logic/auth"
	configlogic "api/internal/logic/config"
)

// Default 返回项目前台 API 默认注册清单。
// 该清单只描述 API 内置注册项。
func Default() []register.Item {
	items := componentItems()
	items = append(items, routeItems()...)
	items = append(items, runtimeItems()...)
	return items
}

// componentItems 从启动组件规格派生清单项。
func componentItems() []register.Item {
	return itemsFromSpecs(register.KindComponent, components.DefaultSpecs(), func(spec components.Spec) register.Spec {
		return spec.Spec
	})
}

// routeItems 从内置路由模块规格派生路由注册清单。
func routeItems() []register.Item {
	return itemsFromSpecs(register.KindRoute, handler.BuiltinRouteModuleSpecs(), func(spec handler.RouteModuleSpec) register.Spec {
		return specFromFields(spec.Name, spec.File, spec.Method, spec.Description)
	})
}

// runtimeItems 从运行时扩展规格派生注册清单。
func runtimeItems() []register.Item {
	authSpecs := authlogic.RuntimeRegistrySpecs()
	configSpecs := configlogic.RuntimeRegistrySpecs()
	items := make([]register.Item, 0, len(authSpecs)+len(configSpecs))
	items = append(items, itemsFromSpecs(register.KindRuntimeRegistry, authSpecs, func(spec corelogic.RuntimeRegistrySpec) register.Spec {
		return specFromFields(spec.Name, spec.File, spec.Method, spec.Description)
	})...)
	items = append(items, itemsFromSpecs(register.KindRuntimeRegistry, configSpecs, func(spec corelogic.RuntimeRegistrySpec) register.Spec {
		return specFromFields(spec.Name, spec.File, spec.Method, spec.Description)
	})...)
	return items
}

// itemsFromSpecs 把不同领域的注册规格转换成统一默认清单项。
func itemsFromSpecs[T any](kind string, specs []T, toSpec func(T) register.Spec) []register.Item {
	items := make([]register.Item, 0, len(specs))
	for _, spec := range specs {
		items = append(items, register.NewItem(kind, toSpec(spec)))
	}
	return items
}

// specFromFields 把领域注册规格字段收敛为统一注册规格。
func specFromFields(name, file, method, description string) register.Spec {
	return register.Spec{
		Name:        name,
		File:        file,
		Method:      method,
		Description: description,
	}
}
