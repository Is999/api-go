package config

import corelogic "api/internal/logic"

const (
	// RuntimeRegistrySysConfigCache 表示 sys_config 缓存读取入口。
	RuntimeRegistrySysConfigCache = "sys_config_cache"
	// RuntimeRegistrySysConfigKeyRegistry 表示 sys_config key 注册入口。
	RuntimeRegistrySysConfigKeyRegistry = "sys_config_key_registry"
)

// runtimeRegistrySpecs 是运行期配置运行时注册入口的清单源。
var runtimeRegistrySpecs = []corelogic.RuntimeRegistrySpec{
	{
		Name:        RuntimeRegistrySysConfigCache,
		File:        "internal/logic/config/sys_config.go",
		Method:      "config.NewSysConfigLogic / GetCachedValue",
		Description: "读取 sys_config 运行期配置缓存",
	},
	{
		Name:        RuntimeRegistrySysConfigKeyRegistry,
		File:        "internal/logic/config/sys_config_key.go",
		Method:      "config.NewSysConfigKeyRegistry / SysConfigLogic.GetBool",
		Description: "按 key 声明类型化读取 sys_config 配置",
	},
}

// RuntimeRegistrySpecs 返回运行期配置运行时注册入口规格快照。
func RuntimeRegistrySpecs() []corelogic.RuntimeRegistrySpec {
	return append([]corelogic.RuntimeRegistrySpec(nil), runtimeRegistrySpecs...)
}
