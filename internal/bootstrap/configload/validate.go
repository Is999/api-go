package configload

import (
	"net/netip"
	"strings"

	"api/common/idgen"
	"api/internal/bootstrap/configload/validators"
	"api/internal/config"

	"github.com/Is999/go-utils/errors"
)

// 启动配置校验边界常量。
const (
	minJWTSecretLength            = 16    // JWT 密钥最小长度，避免明显弱配置启动
	minOpsTokenLength             = 16    // 运维令牌生产环境最小长度
	minPasswordLength             = 6     // 前台密码最小允许长度下限
	defaultUserRouteShardCount    = 1     // 业务用户默认保持单张物理表
	maxAuthRateLimitWindowSeconds = 3600  // 认证限流最大统计窗口
	maxAuthRateLimitLockSeconds   = 86400 // 认证限流最大锁定时长
	maxAuthRateLimitAttempts      = 1000  // 认证限流最大尝试次数
)

// Validate 校验启动必填配置，避免服务以明显错误状态启动。
func Validate(c config.Config) error {
	if len(strings.TrimSpace(c.JwtSecret)) < minJWTSecretLength {
		return errors.Errorf("jwt_secret 长度不能小于 %d", minJWTSecretLength)
	}
	if strings.TrimSpace(c.AppID) == "" {
		return errors.Errorf("app_id 不能为空")
	}
	if len(c.Redis.Addrs) == 0 {
		return errors.Errorf("redis.addrs 不能为空")
	}
	if c.Redis.PoolSize <= 0 {
		return errors.Errorf("redis.pool_size 必须大于 0")
	}
	if c.Auth.PasswordMinLength < minPasswordLength {
		return errors.Errorf("auth.password_min_length 不能小于 %d", minPasswordLength)
	}
	if err := validateSnowflakeConfig(c.Snowflake); err != nil {
		return errors.Tag(err)
	}
	if err := validateUserConfig(c.User); err != nil {
		return errors.Tag(err)
	}
	if err := validateAuthRateLimitConfig("auth.login_rate_limit", c.Auth.LoginRateLimit); err != nil {
		return errors.Tag(err)
	}
	if err := validateAuthRateLimitConfig("auth.register_rate_limit", c.Auth.RegisterRateLimit); err != nil {
		return errors.Tag(err)
	}
	if err := validators.ValidateCollector(c); err != nil {
		return errors.Tag(err)
	}
	if err := validateOpsConfig(c.Ops); err != nil {
		return errors.Tag(err)
	}
	if err := validateAlertConfig(c.Alert); err != nil {
		return errors.Tag(err)
	}
	if err := validators.ValidateSecurity(c); err != nil {
		return errors.Tag(err)
	}
	if err := validators.ValidateProduction(c); err != nil {
		return errors.Tag(err)
	}
	return nil
}

// validateAlertConfig 校验外部告警通道的最小可用配置。
func validateAlertConfig(cfg config.AlertConfig) error {
	if !cfg.Lark.Enabled {
		return nil
	}
	if strings.TrimSpace(cfg.Lark.WebhookURL) == "" && strings.TrimSpace(cfg.Lark.WebhookURLRef) == "" {
		return errors.Errorf("alert.lark.enabled=true 时必须配置 webhook_url 或 webhook_url_ref")
	}
	if cfg.Lark.TimeoutSeconds < 0 {
		return errors.Errorf("alert.lark.timeout_seconds 不能小于 0")
	}
	if cfg.Lark.MaxErrorBytes < 0 {
		return errors.Errorf("alert.lark.max_error_bytes 不能小于 0")
	}
	return nil
}

// validateSnowflakeConfig 校验分布式雪花 ID worker 配置。
func validateSnowflakeConfig(cfg config.SnowflakeConfig) error {
	if _, err := resolveSnowflakeWorkerID(cfg); err != nil {
		return errors.Tag(err)
	}
	return nil
}

// resolveSnowflakeWorkerID 解析配置或环境变量中的显式 worker_id。
func resolveSnowflakeWorkerID(cfg config.SnowflakeConfig) (int64, error) {
	if cfg.WorkerID == nil {
		return idgen.ResolveWorkerID(idgen.SnowflakeWorkerIDUnset)
	}
	return idgen.ResolveWorkerID(*cfg.WorkerID)
}

// ConfigureSnowflakeWorkerID 发布当前进程使用的雪花 ID worker 配置。
func ConfigureSnowflakeWorkerID(cfg config.SnowflakeConfig) error {
	workerID, err := resolveSnowflakeWorkerID(cfg)
	if err != nil {
		return errors.Tag(err)
	}
	return idgen.ConfigureWorkerID(workerID)
}

// validateUserConfig 校验业务用户默认物理表数量，防止写入路由不可迁移。
func validateUserConfig(cfg config.UserConfig) error {
	if cfg.RouteShardCount == 0 || validUserRouteShardCount(cfg.RouteShardCount) {
		return nil
	}
	return errors.Errorf("user.route_shard_count 仅支持 1/2/4/8/16/32/64/128/256/512/1024")
}

// validUserRouteShardCount 判断业务用户物理表数量是否能平分 1024 逻辑分片。
func validUserRouteShardCount(routeShardCount int) bool {
	return routeShardCount > 0 && routeShardCount <= idgen.ShardMod && routeShardCount&(routeShardCount-1) == 0
}

// validateAuthRateLimitConfig 校验认证限流参数是否在可控范围内。
func validateAuthRateLimitConfig(name string, cfg config.AuthRateLimitConfig) error {
	if !cfg.Enabled {
		return nil
	}
	if cfg.WindowSeconds > maxAuthRateLimitWindowSeconds {
		return errors.Errorf("%s.window_seconds 不能大于 %d", name, maxAuthRateLimitWindowSeconds)
	}
	if cfg.MaxAttempts > maxAuthRateLimitAttempts {
		return errors.Errorf("%s.max_attempts 不能大于 %d", name, maxAuthRateLimitAttempts)
	}
	if cfg.LockSeconds > maxAuthRateLimitLockSeconds {
		return errors.Errorf("%s.lock_seconds 不能大于 %d", name, maxAuthRateLimitLockSeconds)
	}
	return nil
}

// validateOpsConfig 校验运维白名单，防止配置层误放行公网来源。
func validateOpsConfig(cfg config.OpsConfig) error {
	for _, item := range cfg.ConfigReloadAllowedIPs {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		if strings.Contains(item, "/") {
			prefix, err := netip.ParsePrefix(item)
			if err != nil {
				return errors.Wrapf(err, "ops.config_reload_allowed_ips CIDR 非法: %s", item)
			}
			if !isInternalConfigAddr(prefix.Addr()) {
				return errors.Errorf("ops.config_reload_allowed_ips 不能配置公网 CIDR: %s", item)
			}
			continue
		}
		addr, err := netip.ParseAddr(item)
		if err != nil {
			return errors.Wrapf(err, "ops.config_reload_allowed_ips IP 非法: %s", item)
		}
		if !isInternalConfigAddr(addr) {
			return errors.Errorf("ops.config_reload_allowed_ips 不能配置公网 IP: %s", item)
		}
	}
	return nil
}

// isInternalConfigAddr 判断配置中的地址是否属于内网、本机或链路本地。
func isInternalConfigAddr(addr netip.Addr) bool {
	addr = addr.Unmap()
	return addr.IsLoopback() || addr.IsPrivate() || addr.IsLinkLocalUnicast()
}
