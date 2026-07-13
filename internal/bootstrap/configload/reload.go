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
			Reason: "HTTP服务配置变更",
			Changed: func(oldCfg, newCfg config.Config) bool {
				return !reflect.DeepEqual(oldCfg.RestConf, newCfg.RestConf) ||
					!reflect.DeepEqual(oldCfg.InternalServer, newCfg.InternalServer)
			},
			Preserve: func(effective *config.Config, oldCfg config.Config, newCfg config.Config) {
				effective.RestConf = oldCfg.RestConf
				effective.InternalServer = oldCfg.InternalServer
				if oldCfg.Mode != newCfg.Mode {
					effective.Observability.Environment = oldCfg.Observability.Environment
				}
			},
		},
		{
			Reason: "应用ID变更",
			Changed: func(oldCfg, newCfg config.Config) bool {
				return oldCfg.AppID != newCfg.AppID
			},
			Preserve: func(effective *config.Config, oldCfg config.Config, _ config.Config) {
				effective.AppID = oldCfg.AppID
			},
		},
		{
			Reason: "应用密钥变更",
			Changed: func(oldCfg, newCfg config.Config) bool {
				return oldCfg.AppKey != newCfg.AppKey
			},
			Preserve: func(effective *config.Config, oldCfg config.Config, _ config.Config) {
				effective.AppKey = oldCfg.AppKey
			},
		},
		{
			Reason: "实例标识变更",
			Changed: func(oldCfg, newCfg config.Config) bool {
				return oldCfg.InstanceID != newCfg.InstanceID
			},
			Preserve: func(effective *config.Config, oldCfg config.Config, _ config.Config) {
				effective.InstanceID = oldCfg.InstanceID
			},
		},
		{
			Reason: "可信代理配置变更",
			Changed: func(oldCfg, newCfg config.Config) bool {
				return !reflect.DeepEqual(oldCfg.TrustedProxies, newCfg.TrustedProxies)
			},
			Preserve: func(effective *config.Config, oldCfg config.Config, _ config.Config) {
				effective.TrustedProxies = oldCfg.TrustedProxies
			},
		},
		{
			Reason: "JWT认证配置变更",
			Changed: func(oldCfg, newCfg config.Config) bool {
				return oldCfg.JwtSecret != newCfg.JwtSecret ||
					oldCfg.JwtExpiresIn != newCfg.JwtExpiresIn ||
					oldCfg.Auth.Issuer != newCfg.Auth.Issuer
			},
			Preserve: func(effective *config.Config, oldCfg config.Config, _ config.Config) {
				effective.JwtSecret = oldCfg.JwtSecret
				effective.JwtExpiresIn = oldCfg.JwtExpiresIn
				effective.Auth.Issuer = oldCfg.Auth.Issuer
			},
		},
		{
			Reason: "安全链路配置变更",
			Changed: func(oldCfg, newCfg config.Config) bool {
				return !reflect.DeepEqual(oldCfg.Security, newCfg.Security)
			},
			Preserve: func(effective *config.Config, oldCfg config.Config, _ config.Config) {
				effective.Security = oldCfg.Security
			},
		},
		{
			Reason: "Collector配置变更",
			Changed: func(oldCfg, newCfg config.Config) bool {
				return !reflect.DeepEqual(oldCfg.Collector, newCfg.Collector)
			},
			Preserve: func(effective *config.Config, oldCfg config.Config, _ config.Config) {
				effective.Collector = oldCfg.Collector
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
			Reason: "可观测性配置变更",
			Changed: func(oldCfg, newCfg config.Config) bool {
				return !reflect.DeepEqual(oldCfg.Observability, newCfg.Observability)
			},
			Preserve: func(effective *config.Config, oldCfg config.Config, _ config.Config) {
				effective.Observability = oldCfg.Observability
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
