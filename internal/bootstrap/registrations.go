package bootstrap

import (
	"sort"

	"gozero_api/internal/handler"

	utils "github.com/Is999/go-utils"
	"github.com/Is999/go-utils/errors"
)

const (
	// registrationKindRoute 表示 HTTP 路由模块注册项。
	registrationKindRoute = "route"
	// registrationKindRuntimeRegistry 表示轻量运行时扩展入口注册项。
	registrationKindRuntimeRegistry = "runtime_registry"

	// runtimeRegistryComponentLifecycle 表示启动期组件生命周期注册入口。
	runtimeRegistryComponentLifecycle = "component_lifecycle"
	// runtimeRegistryCollectorProcessor 表示 Collector Processor 注册入口。
	runtimeRegistryCollectorProcessor = "collector_processor"
	// runtimeRegistryAuthSecurityEvent 表示认证风控事件投递入口。
	runtimeRegistryAuthSecurityEvent = "auth_security_event"
	// runtimeRegistryAuthSecurityProcessor 表示认证风控事件默认 Processor。
	runtimeRegistryAuthSecurityProcessor = "auth_security_processor"
	// runtimeRegistrySysConfigCache 表示 sys_config 缓存读取入口。
	runtimeRegistrySysConfigCache = "sys_config_cache"
	// runtimeRegistrySysConfigKeyRegistry 表示 sys_config key 注册入口。
	runtimeRegistrySysConfigKeyRegistry = "sys_config_key_registry"
	// runtimeRegistryCacheRebuildLock 表示缓存重建分布式锁入口。
	runtimeRegistryCacheRebuildLock = "cache_rebuild_lock"
)

// RegistrationManifestItem 描述一个默认注册项，供文档、测试和启动装配核对。
type RegistrationManifestItem struct {
	Kind        string // 注册类型，如 route / runtime_registry
	Name        string // 注册名称，必须在同类型内保持唯一
	File        string // 注册实现所在文件
	Method      string // 注册入口方法或构造方法
	Description string // 注册项中文说明
}

// DefaultRegistrationManifest 返回项目前台 API 默认注册清单。
// 该清单只描述内置注册项，不包含业务方后续注册的 Collector Processor。
func DefaultRegistrationManifest() []RegistrationManifestItem {
	return []RegistrationManifestItem{
		{
			Kind:        registrationKindRoute,
			Name:        "health",
			File:        "internal/handler/routes.go + internal/handler/routes_health.go",
			Method:      "handler.NewHealthRouteModule / registerHealthRoutes",
			Description: "注册健康检查路由",
		},
		{
			Kind:        registrationKindRoute,
			Name:        "auth",
			File:        "internal/handler/routes.go + internal/handler/routes_auth.go",
			Method:      "handler.NewAuthRouteModule / registerAuthRoutes",
			Description: "注册前台认证路由",
		},
		{
			Kind:        registrationKindRoute,
			Name:        "user",
			File:        "internal/handler/routes.go + internal/handler/routes_user.go",
			Method:      "handler.NewUserRouteModule / registerUserRoutes",
			Description: "注册前台用户路由",
		},
		{
			Kind:        registrationKindRoute,
			Name:        "system",
			File:        "internal/handler/routes.go + internal/handler/routes_system.go",
			Method:      "handler.NewSystemRouteModule / registerSystemRoutes",
			Description: "注册内网运行态系统管理路由",
		},
		{
			Kind:        registrationKindRuntimeRegistry,
			Name:        runtimeRegistryComponentLifecycle,
			File:        "internal/bootstrap/components.go + internal/svc/component.go",
			Method:      "buildDefaultComponentRegistry / svc.NewComponentRegistry",
			Description: "统一注册核心组件健康探测和关闭入口",
		},
		{
			Kind:        registrationKindRuntimeRegistry,
			Name:        runtimeRegistryCollectorProcessor,
			File:        "internal/collector/manager.go",
			Method:      "collector.Manager.RegisterProcessor / RegisterProcessorFunc",
			Description: "按 bizType 注册轻量 Collector Processor",
		},
		{
			Kind:        registrationKindRuntimeRegistry,
			Name:        runtimeRegistryAuthSecurityEvent,
			File:        "internal/logic/auth_event.go",
			Method:      "logic.AuthCollectorBizType / RecordAuthEvent",
			Description: "投递脱敏认证风控事件到轻量 Collector",
		},
		{
			Kind:        registrationKindRuntimeRegistry,
			Name:        runtimeRegistryAuthSecurityProcessor,
			File:        "internal/collector/auth_security.go",
			Method:      "collector.RegisterDefaultProcessors / NewAuthSecurityProcessor",
			Description: "默认汇总 auth.security 认证风控事件指标",
		},
		{
			Kind:        registrationKindRuntimeRegistry,
			Name:        runtimeRegistrySysConfigCache,
			File:        "internal/logic/sys_config.go",
			Method:      "logic.NewSysConfigLogic / GetCachedValue",
			Description: "读取 sys_config 运行期配置缓存",
		},
		{
			Kind:        registrationKindRuntimeRegistry,
			Name:        runtimeRegistrySysConfigKeyRegistry,
			File:        "internal/logic/sys_config_key.go",
			Method:      "logic.NewSysConfigKeyRegistry / SysConfigLogic.GetBool",
			Description: "按 key 声明类型化读取 sys_config 配置",
		},
		{
			Kind:        registrationKindRuntimeRegistry,
			Name:        runtimeRegistryCacheRebuildLock,
			File:        "internal/infra/redsync/lock.go + internal/logic/cache_guard.go",
			Method:      "redsync.WithLock / BaseLogic.tryRebuildCacheWithLock",
			Description: "使用 redsync 保护缓存重建",
		},
	}
}

// ValidateDefaultRegistrationManifest 校验默认注册清单与真实内置注册集合是否一致。
func ValidateDefaultRegistrationManifest() error {
	items := DefaultRegistrationManifest()
	if err := validateManifestItems(items); err != nil {
		return errors.Tag(err)
	}
	manifestNames := groupManifestNames(items)

	actualRouteNames := routeModuleNames(defaultRouteModules())
	if err := validateNameListUnique(registrationKindRoute, actualRouteNames); err != nil {
		return errors.Tag(err)
	}
	if err := validateManifestKindNames(registrationKindRoute, manifestNames[registrationKindRoute], actualRouteNames); err != nil {
		return errors.Tag(err)
	}

	actualRuntimeNames := defaultRuntimeRegistryNames()
	if err := validateNameListUnique(registrationKindRuntimeRegistry, actualRuntimeNames); err != nil {
		return errors.Tag(err)
	}
	if err := validateManifestKindNames(registrationKindRuntimeRegistry, manifestNames[registrationKindRuntimeRegistry], actualRuntimeNames); err != nil {
		return errors.Tag(err)
	}
	return nil
}

// defaultRouteModules 返回项目前台 API 内置 HTTP 路由模块集合。
func defaultRouteModules() []handler.RouteModule {
	return []handler.RouteModule{
		handler.NewHealthRouteModule(), // 基础健康检查
		handler.NewAuthRouteModule(),   // 前台认证模块
		handler.NewUserRouteModule(),   // 前台用户模块
		handler.NewSystemRouteModule(), // 运行态系统模块
	}
}

// defaultRuntimeRegistryNames 返回清单需要覆盖的轻量运行时扩展入口。
func defaultRuntimeRegistryNames() []string {
	return []string{
		runtimeRegistryComponentLifecycle,
		runtimeRegistryCollectorProcessor,
		runtimeRegistryAuthSecurityEvent,
		runtimeRegistryAuthSecurityProcessor,
		runtimeRegistrySysConfigCache,
		runtimeRegistrySysConfigKeyRegistry,
		runtimeRegistryCacheRebuildLock,
	}
}

// validateManifestItems 校验注册清单项本身的结构完整性和唯一性。
func validateManifestItems(items []RegistrationManifestItem) error {
	seen := make(map[string]struct{}, len(items))
	for _, item := range items {
		if item.Kind == "" {
			return errors.Errorf("默认注册清单存在空 kind 项")
		}
		if item.Name == "" {
			return errors.Errorf("默认注册清单[%s]存在空 name 项", item.Kind)
		}
		if item.File == "" {
			return errors.Errorf("默认注册清单[%s:%s]存在空 file 项", item.Kind, item.Name)
		}
		if item.Method == "" {
			return errors.Errorf("默认注册清单[%s:%s]存在空 method 项", item.Kind, item.Name)
		}
		if item.Description == "" {
			return errors.Errorf("默认注册清单[%s:%s]存在空 description 项", item.Kind, item.Name)
		}
		key := item.Kind + ":" + item.Name
		if _, ok := seen[key]; ok {
			return errors.Errorf("默认注册清单存在重复项 %s", key)
		}
		seen[key] = struct{}{}
	}
	return nil
}

// groupManifestNames 按 kind 归集注册名称，供后续和真实内置注册集合比对。
func groupManifestNames(items []RegistrationManifestItem) map[string][]string {
	grouped := make(map[string][]string, 2)
	for _, item := range items {
		grouped[item.Kind] = append(grouped[item.Kind], item.Name)
	}
	return grouped
}

// validateManifestKindNames 校验某一类注册清单名称与真实内置注册名称完全一致。
func validateManifestKindNames(kind string, manifestNames, actualNames []string) error {
	missing := utils.Diff(actualNames, manifestNames)
	extra := utils.Diff(manifestNames, actualNames)
	if len(missing) == 0 && len(extra) == 0 {
		return nil
	}
	sort.Strings(missing)
	sort.Strings(extra)
	return errors.Errorf("默认注册清单[%s]与真实内置注册不一致 missing=%v extra=%v", kind, missing, extra)
}

// validateNameListUnique 校验真实注册列表内部名称唯一，避免同一模块被重复装配。
func validateNameListUnique(kind string, names []string) error {
	seen := make(map[string]struct{}, len(names))
	for _, name := range names {
		if name == "" {
			return errors.Errorf("默认注册清单[%s]存在空真实注册名称", kind)
		}
		if _, ok := seen[name]; ok {
			return errors.Errorf("默认注册清单[%s]存在重复真实注册名称: %s", kind, name)
		}
		seen[name] = struct{}{}
	}
	return nil
}

// routeModuleNames 提取路由模块名称列表，供注册清单校验复用。
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
