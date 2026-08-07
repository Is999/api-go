package configload

import (
	"net/netip"
	"strings"

	"api/common/idgen"
	"api/internal/bootstrap/configload/validators"
	"api/internal/config"
	"api/internal/sharding"

	utils "github.com/Is999/go-utils"
	"github.com/Is999/go-utils/errors"
)

// 启动配置校验边界常量。
const (
	minJWTSecretLength            = 16    // JWT 密钥最小长度，避免明显弱配置启动
	minOpsTokenLength             = 16    // 运维令牌生产环境最小长度
	minPasswordLength             = 6     // 前台密码最小允许长度下限
	maxPasswordBytes              = 72    // bcrypt 接受的密码最大字节数
	maxConfigKeySegmentBytes      = 64    // Redis key 配置片段最大长度
	maxHotReloadIntervalSeconds   = 3600  // 配置文件轮询最大间隔
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
	if !validConfigKeySegment(c.AppID) {
		return errors.Errorf("app_id 只能包含字母、数字、点、下划线或短横线，且长度不能超过 %d", maxConfigKeySegmentBytes)
	}
	if _, err := utils.NewTrustedProxies(c.TrustedProxies...); err != nil {
		return errors.Wrap(err, "trusted_proxies 配置非法")
	}
	if err := validateRedisConfig(c.Redis); err != nil {
		return errors.Tag(err)
	}
	if c.Auth.PasswordMinLength < minPasswordLength || c.Auth.PasswordMinLength > maxPasswordBytes {
		return errors.Errorf("auth.password_min_length 必须在 %d-%d 之间", minPasswordLength, maxPasswordBytes)
	}
	if interval := c.HotReload.CheckIntervalSeconds; interval < 0 || interval > maxHotReloadIntervalSeconds {
		return errors.Errorf("hot_reload.check_interval_seconds 必须为 0 或 1-%d", maxHotReloadIntervalSeconds)
	}
	if ratio := c.Observability.SampleRatio; ratio < 0 || ratio > 1 {
		return errors.Errorf("observability.sample_ratio 必须在 0-1 之间")
	}
	if err := validateSiteMySQLNames(c.SiteMySQL); err != nil {
		return errors.Tag(err)
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
	if err := validateInternalServer(c.RestConf, c.InternalServer, c.Mode); err != nil {
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

// validateRedisConfig 校验 Redis 模式与地址语义，禁止拼写错误时静默切换客户端类型。
func validateRedisConfig(cfg config.RedisConfig) error {
	addresses := make([]string, 0, len(cfg.Addrs))
	for _, rawAddress := range cfg.Addrs {
		address := strings.TrimSpace(rawAddress)
		if address == "" || address != rawAddress {
			return errors.Errorf("redis.addrs 不能包含空地址或首尾空白")
		}
		addresses = append(addresses, address)
	}
	if len(addresses) == 0 {
		return errors.Errorf("redis.addrs 不能为空")
	}
	if cfg.PoolSize <= 0 {
		return errors.Errorf("redis.pool_size 必须大于 0")
	}
	redisType := strings.ToLower(strings.TrimSpace(cfg.Type))
	if strings.TrimSpace(cfg.Type) != redisType {
		return errors.Errorf("redis.type 必须使用小写规范值")
	}
	switch redisType {
	case "", "single", "standalone":
		if len(addresses) != 1 {
			return errors.Errorf("redis.type=single 时 addrs 必须且只能配置一个地址")
		}
		if len(cfg.AddrMap) > 0 {
			return errors.Errorf("redis.addr_map 仅支持 cluster 模式")
		}
	case "cluster":
		if cfg.DB != 0 {
			return errors.Errorf("redis.type=cluster 时 db 必须为 0")
		}
	default:
		return errors.Errorf("redis.type 仅支持 single/standalone/cluster")
	}
	if cfg.DB < 0 {
		return errors.Errorf("redis.db 不能小于 0")
	}
	return nil
}

// validateSiteMySQLNames 校验命名扩展库名称，避免 trim 或大小写碰撞覆盖已打开连接池。
func validateSiteMySQLNames(items config.SiteMySQLConfig) error {
	seen := make(map[string]struct{}, len(items))
	for rawName := range items {
		name := strings.TrimSpace(rawName)
		if name == "" || name != rawName || name != strings.ToLower(name) || strings.EqualFold(name, "main") || !validConfigKeySegment(name) {
			return errors.Errorf("site_mysql 名称[%s]非法", rawName)
		}
		normalized := strings.ToLower(name)
		if _, exists := seen[normalized]; exists {
			return errors.Errorf("site_mysql 名称规范化后重复: %s", name)
		}
		seen[normalized] = struct{}{}
	}
	return nil
}

// validConfigKeySegment 校验会参与 Redis key 或运行时注册表的短标识。
func validConfigKeySegment(value string) bool {
	trimmed := strings.TrimSpace(value)
	if value != trimmed || trimmed == "" || len(trimmed) > maxConfigKeySegmentBytes {
		return false
	}
	for _, char := range trimmed {
		if (char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') ||
			(char >= '0' && char <= '9') || char == '.' || char == '_' || char == '-' {
			continue
		}
		return false
	}
	return true
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
	if cfg.Redis.Enabled {
		if err := validateSnowflakeRedisConfig(cfg); err != nil {
			return errors.Tag(err)
		}
	} else if _, err := resolveSnowflakeWorkerID(cfg); err != nil {
		return errors.Tag(err)
	}
	if err := validateIDSegmentConfig(cfg); err != nil {
		return errors.Tag(err)
	}
	return nil
}

// validateSnowflakeRedisConfig 校验 Redis 租约 node_id 分配参数。
func validateSnowflakeRedisConfig(cfg config.SnowflakeConfig) error {
	if cfg.WorkerID != nil {
		return errors.Errorf("snowflake.redis.enabled=true 时不能同时配置 snowflake.worker_id")
	}
	redisCfg := normalizeSnowflakeRedisConfig(cfg.Redis)
	if !validConfigKeySegment(redisCfg.Scope) {
		return errors.Errorf("snowflake.redis.scope 只能包含安全 key 字符且长度不能超过 %d", maxConfigKeySegmentBytes)
	}
	if redisCfg.LeaseSeconds < minSnowflakeRedisLeaseSeconds || redisCfg.LeaseSeconds > maxSnowflakeRedisLeaseSeconds {
		return errors.Errorf("snowflake.redis.lease_seconds 必须在 %d-%d 之间", minSnowflakeRedisLeaseSeconds, maxSnowflakeRedisLeaseSeconds)
	}
	if redisCfg.RenewIntervalSeconds <= 0 || redisCfg.RenewIntervalSeconds > maxSnowflakeRedisLeaseSeconds {
		return errors.Errorf("snowflake.redis.renew_interval_seconds 必须在 1-%d 之间", maxSnowflakeRedisLeaseSeconds)
	}
	if redisCfg.RenewIntervalSeconds >= redisCfg.LeaseSeconds-redisCfg.RenewIntervalSeconds {
		return errors.Errorf("snowflake.redis.renew_interval_seconds 必须小于 lease_seconds 的一半")
	}
	if err := validateSnowflakeRedisNamespaces(cfg.Redis.Namespaces); err != nil {
		return errors.Tag(err)
	}
	return nil
}

// validateSnowflakeRedisNamespaces 校验业务 namespace 的 node_id 池覆盖配置。
func validateSnowflakeRedisNamespaces(items map[string]config.SnowflakeRedisNamespaceConfig) error {
	for rawNamespace, item := range items {
		namespace := idgen.NormalizeNamespace(rawNamespace)
		if namespace == "" {
			return errors.Errorf("snowflake.redis.namespaces 包含空 namespace")
		}
		if rawNamespace != namespace || namespace != strings.ToLower(namespace) {
			return errors.Errorf("snowflake.redis.namespaces.%s 必须使用无首尾空白的小写名称", rawNamespace)
		}
		if !validConfigKeySegment(namespace) {
			return errors.Errorf("snowflake.redis.namespaces.%s 包含非法 key 字符", rawNamespace)
		}
		if item.NodeCount < 0 || item.NodeCount > int(idgen.SnowflakeMaxWorkerID+1) {
			return errors.Errorf("snowflake.redis.namespaces.%s.node_count 必须在 0-%d 之间", namespace, idgen.SnowflakeMaxWorkerID+1)
		}
	}
	return nil
}

// validateIDSegmentConfig 校验高吞吐业务 Redis Segment 号段配置。
func validateIDSegmentConfig(cfg config.SnowflakeConfig) error {
	if !cfg.Segment.Enabled {
		return nil
	}
	segmentCfg := normalizeIDSegmentConfig(cfg.Segment, cfg.Redis)
	if !validConfigKeySegment(segmentCfg.Scope) {
		return errors.Errorf("snowflake.segment.scope 只能包含安全 key 字符且长度不能超过 %d", maxConfigKeySegmentBytes)
	}
	if segmentCfg.AllocateTimeoutSeconds <= 0 || segmentCfg.AllocateTimeoutSeconds > maxIDSegmentAllocateTimeoutSeconds {
		return errors.Errorf("snowflake.segment.allocate_timeout_seconds 必须在 1-%d 之间", maxIDSegmentAllocateTimeoutSeconds)
	}
	enabledNamespaces := 0
	for rawNamespace, rawItem := range cfg.Segment.Namespaces {
		namespace := idgen.NormalizeNamespace(rawNamespace)
		if namespace == "" {
			return errors.Errorf("snowflake.segment.namespaces 包含空 namespace")
		}
		if rawNamespace != namespace || namespace != strings.ToLower(namespace) {
			return errors.Errorf("snowflake.segment.namespaces.%s 必须使用无首尾空白的小写名称", rawNamespace)
		}
		if !validConfigKeySegment(namespace) {
			return errors.Errorf("snowflake.segment.namespaces.%s 包含非法 key 字符", rawNamespace)
		}
		item := normalizeIDSegmentNamespaceConfig(rawItem)
		if !item.Enabled {
			continue
		}
		enabledNamespaces++
		if item.Step <= 0 || item.Step > maxIDSegmentStep {
			return errors.Errorf("snowflake.segment.namespaces.%s.step 必须在 1-%d 之间", namespace, maxIDSegmentStep)
		}
		if item.PrefetchThreshold < 0 || item.PrefetchThreshold >= item.Step {
			return errors.Errorf("snowflake.segment.namespaces.%s.prefetch_threshold 必须小于 step 且不能为负数", namespace)
		}
		if item.Start < 0 {
			return errors.Errorf("snowflake.segment.namespaces.%s.start 不能小于 0", namespace)
		}
	}
	if enabledNamespaces == 0 {
		return errors.Errorf("snowflake.segment.enabled=true 时必须至少启用一个 namespace")
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
	if cfg.Redis.Enabled {
		return errors.New("snowflake.redis.enabled=true 时必须通过 Redis 租约配置雪花 node_id")
	}
	workerID, err := resolveSnowflakeWorkerID(cfg)
	if err != nil {
		return errors.Tag(err)
	}
	return idgen.ConfigureWorkerID(workerID)
}

// validateUserConfig 校验用户物理分片数是否支持平滑二分。
func validateUserConfig(cfg config.UserConfig) error {
	if cfg.RouteShardCount == 0 || sharding.ValidCount(cfg.RouteShardCount) {
		return nil
	}
	return errors.Errorf("user.route_shard_count 仅支持 1/2/4/8/16/32/64/128/256/512/1024")
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
	if len(strings.TrimSpace(cfg.ConfigReloadToken)) < minOpsTokenLength {
		return errors.Errorf("ops.config_reload_token 长度不能小于 %d", minOpsTokenLength)
	}
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
			if !isInternalConfigPrefix(prefix) {
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

// isInternalConfigAddr 判断配置中的地址是否属于内网或本机。
func isInternalConfigAddr(addr netip.Addr) bool {
	addr = addr.Unmap()
	return addr.IsLoopback() || addr.IsPrivate()
}

// internalConfigPrefixes 收口允许的回环和私网网段；使用类型安全地址构造，避免在运行期配置校验中调用可能 panic 的 MustParsePrefix。
var internalConfigPrefixes = []netip.Prefix{
	netip.PrefixFrom(netip.AddrFrom4([4]byte{127, 0, 0, 0}), 8),
	netip.PrefixFrom(netip.AddrFrom4([4]byte{10, 0, 0, 0}), 8),
	netip.PrefixFrom(netip.AddrFrom4([4]byte{172, 16, 0, 0}), 12),
	netip.PrefixFrom(netip.AddrFrom4([4]byte{192, 168, 0, 0}), 16),
	netip.PrefixFrom(netip.AddrFrom16([16]byte{15: 1}), 128),
	netip.PrefixFrom(netip.AddrFrom16([16]byte{0xfc}), 7),
}

// isInternalConfigPrefix 确保 CIDR 的完整地址范围都落在回环或私网网段。
func isInternalConfigPrefix(prefix netip.Prefix) bool {
	prefix = prefix.Masked()
	for _, allowed := range internalConfigPrefixes {
		if prefix.Addr().BitLen() == allowed.Addr().BitLen() &&
			prefix.Bits() >= allowed.Bits() &&
			allowed.Contains(prefix.Addr()) {
			return true
		}
	}
	return false
}
