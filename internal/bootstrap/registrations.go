package bootstrap

import (
	"api/internal/handler"
	"api/internal/infra/collectorx"
	corelogic "api/internal/logic"
	authlogic "api/internal/logic/auth"
	configlogic "api/internal/logic/config"

	"github.com/Is999/go-utils/errors"
)

const (
	// registrationKindComponent 表示启动期组件来源注册项。
	registrationKindComponent = "component"
	// registrationKindRoute 表示 HTTP 路由模块注册项。
	registrationKindRoute = "route"
	// registrationKindRuntimeRegistry 表示轻量运行时扩展入口注册项。
	registrationKindRuntimeRegistry = "runtime_registry"
)

// RegistrationManifestItem 描述一个默认注册项，供文档、测试和启动装配核对。
type RegistrationManifestItem struct {
	Kind        string // 注册类型，如 component / route / runtime_registry
	Name        string // 注册名称，必须在同类型内保持唯一
	File        string // 注册实现所在文件
	Method      string // 注册入口方法或构造方法
	Description string // 注册项中文说明
}

// registrationSpec 描述默认注册项的稳定说明字段。
type registrationSpec struct {
	Name        string // 注册名称，必须在同类型内保持唯一
	File        string // 注册实现所在文件
	Method      string // 注册入口方法或构造方法
	Description string // 注册项中文说明
}

// runtimeRegistrySpec 描述一个内置轻量运行时扩展入口。
type runtimeRegistrySpec struct {
	registrationSpec // 运行时扩展在默认注册清单中的说明字段
}

// DefaultRegistrationManifest 返回项目前台 API 默认注册清单。
// 该清单只描述内置注册项，不包含业务方后续注册的 Collector Processor。
func DefaultRegistrationManifest() []RegistrationManifestItem {
	items := componentRegistrationManifestItems()
	items = append(items, routeRegistrationManifestItems()...)
	items = append(items, runtimeRegistrationManifestItems()...)
	return items
}

// registrationManifestItem 将注册规格转换为统一清单项。
func registrationManifestItem(kind string, spec registrationSpec) RegistrationManifestItem {
	return RegistrationManifestItem{
		Kind:        kind,
		Name:        spec.Name,
		File:        spec.File,
		Method:      spec.Method,
		Description: spec.Description,
	}
}

// componentRegistrationManifestItems 从启动组件规格派生清单项。
func componentRegistrationManifestItems() []RegistrationManifestItem {
	specs := defaultComponentSpecs()
	items := make([]RegistrationManifestItem, 0, len(specs))
	for _, spec := range specs {
		items = append(items, registrationManifestItem(registrationKindComponent, spec.registrationSpec))
	}
	return items
}

// routeRegistrationManifestItems 从内置路由模块规格派生路由注册清单。
func routeRegistrationManifestItems() []RegistrationManifestItem {
	specs := handler.BuiltinRouteModuleSpecs()
	items := make([]RegistrationManifestItem, 0, len(specs))
	for _, spec := range specs {
		items = append(items, RegistrationManifestItem{
			Kind:        registrationKindRoute,
			Name:        spec.Name,
			File:        spec.File,
			Method:      spec.Method,
			Description: spec.Description,
		})
	}
	return items
}

// defaultRuntimeRegistrySpecs 返回轻量运行时扩展入口规格。
func defaultRuntimeRegistrySpecs() []runtimeRegistrySpec {
	collectorSpecs := collectorRuntimeRegistrySpecs()
	authSpecs := authRuntimeRegistrySpecs()
	processorSpecs := collectorDefaultProcessorRuntimeRegistrySpecs()
	configSpecs := configRuntimeRegistrySpecs()
	cacheSpecs := cacheRuntimeRegistrySpecs()
	specs := make([]runtimeRegistrySpec, 0, len(collectorSpecs)+len(authSpecs)+len(processorSpecs)+len(configSpecs)+len(cacheSpecs))
	specs = append(specs, collectorSpecs...)
	specs = append(specs, authSpecs...)
	specs = append(specs, processorSpecs...)
	specs = append(specs, configSpecs...)
	specs = append(specs, cacheSpecs...)
	return specs
}

// runtimeRegistrationManifestItems 从运行时扩展规格派生注册清单。
func runtimeRegistrationManifestItems() []RegistrationManifestItem {
	specs := defaultRuntimeRegistrySpecs()
	items := make([]RegistrationManifestItem, 0, len(specs))
	for _, spec := range specs {
		items = append(items, registrationManifestItem(registrationKindRuntimeRegistry, spec.registrationSpec))
	}
	return items
}

// runtimeRegistrySpecFromFields 构造 bootstrap 内部统一的运行时扩展规格。
func runtimeRegistrySpecFromFields(name, file, method, description string) runtimeRegistrySpec {
	return runtimeRegistrySpec{
		registrationSpec: registrationSpec{
			Name:        name,
			File:        file,
			Method:      method,
			Description: description,
		},
	}
}

// collectorRuntimeRegistrySpecs 从 Collector 注册规格派生运行时清单。
func collectorRuntimeRegistrySpecs() []runtimeRegistrySpec {
	items := collectorx.RuntimeRegistrySpecs()
	specs := make([]runtimeRegistrySpec, 0, len(items))
	for _, item := range items {
		specs = append(specs, runtimeRegistrySpecFromFields(item.Name, item.File, item.Method, item.Description))
	}
	return specs
}

// authRuntimeRegistrySpecs 从认证业务注册规格派生运行时清单。
func authRuntimeRegistrySpecs() []runtimeRegistrySpec {
	items := authlogic.RuntimeRegistrySpecs()
	specs := make([]runtimeRegistrySpec, 0, len(items))
	for _, item := range items {
		specs = append(specs, runtimeRegistrySpecFromFields(item.Name, item.File, item.Method, item.Description))
	}
	return specs
}

// collectorDefaultProcessorRuntimeRegistrySpecs 从默认 Processor 注册规格派生运行时清单。
func collectorDefaultProcessorRuntimeRegistrySpecs() []runtimeRegistrySpec {
	items := collectorx.DefaultProcessorRuntimeRegistrySpecs()
	specs := make([]runtimeRegistrySpec, 0, len(items))
	for _, item := range items {
		specs = append(specs, runtimeRegistrySpecFromFields(item.Name, item.File, item.Method, item.Description))
	}
	return specs
}

// configRuntimeRegistrySpecs 从运行期配置注册规格派生运行时清单。
func configRuntimeRegistrySpecs() []runtimeRegistrySpec {
	items := configlogic.RuntimeRegistrySpecs()
	specs := make([]runtimeRegistrySpec, 0, len(items))
	for _, item := range items {
		specs = append(specs, runtimeRegistrySpecFromFields(item.Name, item.File, item.Method, item.Description))
	}
	return specs
}

// cacheRuntimeRegistrySpecs 从通用缓存保护注册规格派生运行时清单。
func cacheRuntimeRegistrySpecs() []runtimeRegistrySpec {
	items := corelogic.RuntimeRegistrySpecs()
	specs := make([]runtimeRegistrySpec, 0, len(items))
	for _, item := range items {
		specs = append(specs, runtimeRegistrySpecFromFields(item.Name, item.File, item.Method, item.Description))
	}
	return specs
}

// defaultRouteModules 返回项目前台 API 内置 HTTP 路由模块集合。
func defaultRouteModules() []handler.RouteModule {
	return handler.BuiltinRouteModules()
}

// validateRegistrationNamesUnique 校验注册列表内部名称唯一，避免同一能力被重复装配。
func validateRegistrationNamesUnique(kind string, names []string) error {
	seen := make(map[string]struct{}, len(names))
	for _, name := range names {
		if name == "" {
			return errors.Errorf("注册集合[%s]存在空名称", kind)
		}
		if _, ok := seen[name]; ok {
			return errors.Errorf("注册集合[%s]存在重复名称: %s", kind, name)
		}
		seen[name] = struct{}{}
	}
	return nil
}

// routeModuleNames 提取路由模块名称列表，供启动装配和测试复用。
func routeModuleNames(items []handler.RouteModule) []string {
	names := make([]string, 0, len(items))
	for _, item := range items {
		if item == nil {
			continue
		}
		names = append(names, item.Name())
	}
	return names
}
