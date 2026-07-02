package configload

import (
	"reflect"
	"strings"

	"api/internal/config"
)

// hotReloadRestartChanged 判断一个配置边界是否发生变化。
type hotReloadRestartChanged func(oldCfg, newCfg config.Config) bool

// hotReloadRestartPreserve 保留当前进程仍在使用的上一版配置。
type hotReloadRestartPreserve func(effective *config.Config, oldCfg config.Config, newCfg config.Config)

// hotReloadRestartSpec 描述一个热加载后仍需重启才能完全生效的配置边界。
type hotReloadRestartSpec struct {
	Reason   string                   // 展示给接口和日志的重启原因
	Changed  hotReloadRestartChanged  // 判断该边界是否发生变化
	Preserve hotReloadRestartPreserve // 保留当前进程仍在使用的上一版配置
}

// hotReloadRestartSpecs 返回热加载不可在线重建的配置边界，顺序即 restartReason 展示顺序。
func hotReloadRestartSpecs() []hotReloadRestartSpec {
	return []hotReloadRestartSpec{
		{
			Reason: "HTTP监听地址变更",
			Changed: func(oldCfg, newCfg config.Config) bool {
				return oldCfg.Host != newCfg.Host || oldCfg.Port != newCfg.Port
			},
			Preserve: func(effective *config.Config, oldCfg config.Config, _ config.Config) {
				effective.RestConf = oldCfg.RestConf
			},
		},
		{
			Reason: "雪花ID worker 配置变更",
			Changed: func(oldCfg, newCfg config.Config) bool {
				return !reflect.DeepEqual(oldCfg.Snowflake, newCfg.Snowflake)
			},
			Preserve: func(effective *config.Config, oldCfg config.Config, _ config.Config) {
				effective.Snowflake = oldCfg.Snowflake
			},
		},
		{
			Reason: "用户写入分表路由配置变更",
			Changed: func(oldCfg, newCfg config.Config) bool {
				return oldCfg.User.RouteShardCount != newCfg.User.RouteShardCount
			},
			Preserve: func(effective *config.Config, oldCfg config.Config, _ config.Config) {
				effective.User = oldCfg.User
			},
		},
		{
			Reason: "MySQL连接配置变更",
			Changed: func(oldCfg, newCfg config.Config) bool {
				return !reflect.DeepEqual(oldCfg.MySQL, newCfg.MySQL) || !reflect.DeepEqual(oldCfg.SiteMySQL, newCfg.SiteMySQL)
			},
			Preserve: func(effective *config.Config, oldCfg config.Config, _ config.Config) {
				effective.MySQL = oldCfg.MySQL
				effective.SiteMySQL = oldCfg.SiteMySQL
			},
		},
		{
			Reason: "Redis连接配置变更",
			Changed: func(oldCfg, newCfg config.Config) bool {
				return !reflect.DeepEqual(oldCfg.Redis, newCfg.Redis)
			},
			Preserve: func(effective *config.Config, oldCfg config.Config, _ config.Config) {
				effective.Redis = oldCfg.Redis
			},
		},
		{
			Reason: "OTLP导出配置变更",
			Changed: func(oldCfg, newCfg config.Config) bool {
				return oldCfg.Observability.OTLPEndpoint != newCfg.Observability.OTLPEndpoint ||
					oldCfg.Observability.OTLPProtocol != newCfg.Observability.OTLPProtocol
			},
			Preserve: func(effective *config.Config, oldCfg config.Config, _ config.Config) {
				effective.Observability.OTLPEndpoint = oldCfg.Observability.OTLPEndpoint
				effective.Observability.OTLPProtocol = oldCfg.Observability.OTLPProtocol
			},
		},
		{
			Reason: "Lark告警配置变更",
			Changed: func(oldCfg, newCfg config.Config) bool {
				return !reflect.DeepEqual(oldCfg.Alert.Lark, newCfg.Alert.Lark)
			},
			Preserve: func(effective *config.Config, oldCfg config.Config, _ config.Config) {
				effective.Alert.Lark = oldCfg.Alert.Lark
			},
		},
	}
}

// changedHotReloadRestartSpecs 返回本次热加载实际命中的重启边界。
func changedHotReloadRestartSpecs(oldCfg, newCfg config.Config) []hotReloadRestartSpec {
	specs := hotReloadRestartSpecs()
	changed := make([]hotReloadRestartSpec, 0, len(specs))
	for _, spec := range specs {
		if spec.Changed == nil || !spec.Changed(oldCfg, newCfg) {
			continue
		}
		changed = append(changed, spec)
	}
	return changed
}

// DetectReloadRestartImpact 判断新配置是否包含启动期依赖变化。
func DetectReloadRestartImpact(oldCfg config.Config, newCfg config.Config) (bool, string) {
	changedSpecs := changedHotReloadRestartSpecs(oldCfg, newCfg)
	reasons := make([]string, 0, len(changedSpecs))
	for _, spec := range changedSpecs {
		reasons = append(reasons, spec.Reason)
	}
	if len(reasons) == 0 {
		return false, ""
	}
	return true, strings.Join(reasons, "；")
}

// BuildReloadEffectiveConfig 保留启动期依赖配置，运行期配置仍按新文件刷新。
func BuildReloadEffectiveConfig(oldCfg config.Config, newCfg config.Config) config.Config {
	effective := newCfg
	for _, spec := range changedHotReloadRestartSpecs(oldCfg, newCfg) {
		if spec.Preserve == nil {
			continue
		}
		spec.Preserve(&effective, oldCfg, newCfg)
	}
	return effective
}
