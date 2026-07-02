package runtimefile

import "api/internal/config"

// 运行期外部配置支持的顶层配置段。
const (
	runtimeConfigSectionAuth      = "auth"       // 前台认证运行参数
	runtimeConfigSectionHotReload = "hot_reload" // 配置热加载运行参数
	runtimeConfigSectionSecurity  = "security"   // 签名验签和加解密配置
	runtimeConfigSectionCollector = "collector"  // 通用收集器配置
	runtimeConfigSectionOps       = "ops"        // 运维级接口保护配置
)

// file 描述外部运行期配置文件。
type file struct {
	Auth      config.AuthConfig      `json:"auth,optional"`       // 前台认证运行参数
	HotReload config.HotReloadConfig `json:"hot_reload,optional"` // 配置热加载运行参数
	Security  config.SecurityConfig  `json:"security,optional"`   // 签名验签和加解密配置
	Collector config.CollectorConfig `json:"collector,optional"`  // 通用收集器配置
	Ops       config.OpsConfig       `json:"ops,optional"`        // 运维级接口保护配置
}

// sectionSpec 描述一个允许运行期外置的配置段。
type sectionSpec struct {
	Key   string                             // 外部运行期配置文件中的顶层键
	apply func(cfg *config.Config, ext file) // 将该配置段合并到主配置
}

// sectionSpecs 返回运行期外部配置段规格。
func sectionSpecs() []sectionSpec {
	return []sectionSpec{
		{
			Key: runtimeConfigSectionAuth,
			apply: func(cfg *config.Config, ext file) {
				cfg.Auth = ext.Auth
			},
		},
		{
			Key: runtimeConfigSectionHotReload,
			apply: func(cfg *config.Config, ext file) {
				cfg.HotReload = ext.HotReload
			},
		},
		{
			Key: runtimeConfigSectionSecurity,
			apply: func(cfg *config.Config, ext file) {
				cfg.Security = ext.Security
			},
		},
		{
			Key: runtimeConfigSectionCollector,
			apply: func(cfg *config.Config, ext file) {
				cfg.Collector = ext.Collector
			},
		},
		{
			Key: runtimeConfigSectionOps,
			apply: func(cfg *config.Config, ext file) {
				cfg.Ops = ext.Ops
			},
		},
	}
}

// sectionKeys 返回运行期外部配置段白名单。
func sectionKeys() map[string]struct{} {
	specs := sectionSpecs()
	keys := make(map[string]struct{}, len(specs))
	for _, spec := range specs {
		keys[spec.Key] = struct{}{}
	}
	return keys
}
