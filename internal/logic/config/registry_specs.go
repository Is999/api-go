package config

// RuntimeRegistrySpec 描述运行期配置提供的轻量扩展入口。
type RuntimeRegistrySpec struct {
	Name        string // 注册名称，必须在运行时扩展清单中唯一
	File        string // 注册实现所在文件
	Method      string // 注册入口方法
	Description string // 注册项中文说明
}

const (
	// RuntimeRegistrySysConfigCache 表示 sys_config 缓存读取入口。
	RuntimeRegistrySysConfigCache = "sys_config_cache"
	// RuntimeRegistrySysConfigKeyRegistry 表示 sys_config key 注册入口。
	RuntimeRegistrySysConfigKeyRegistry = "sys_config_key_registry"
)

// runtimeRegistrySpecs 是运行期配置运行时注册入口的清单源。
var runtimeRegistrySpecs = []RuntimeRegistrySpec{
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
func RuntimeRegistrySpecs() []RuntimeRegistrySpec {
	return append([]RuntimeRegistrySpec(nil), runtimeRegistrySpecs...)
}
