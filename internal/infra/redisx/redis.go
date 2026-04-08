package redisx

import (
	"context"
	"crypto/tls"
	"net"
	"strings"
	"time"

	"gozero_api/internal/config"
	"gozero_api/internal/infra/loggerx"

	"github.com/Is999/go-utils/errors"
	"github.com/redis/go-redis/v9"
	"github.com/redis/go-redis/v9/maintnotifications"
	"github.com/zeromicro/go-zero/core/logx"
)

// Redis 命令和脚本缓存错误常量。
const (
	redisCommandEvalSHA   = "evalsha"    // EVALSHA 命令名，用于识别脚本缓存未命中
	redisCommandEvalSHARO = "evalsha_ro" // 只读 EVALSHA 命令名
	redisNoScriptPrefix   = "NOSCRIPT"   // Redis 脚本缓存未命中错误前缀
)

// New 创建 Redis 客户端，并注册统一的命令耗时/错误日志 hook。
func New(ctx context.Context, cfg config.RedisConfig, obs config.ObservabilityConfig) (redis.UniversalClient, error) {
	addrs, err := resolveAddrs(cfg.Addrs)
	if err != nil {
		return nil, errors.Tag(err)
	}
	addrMap := resolveAddrMap(cfg.AddrMap)
	if err := pingConfiguredAddrs(ctx, cfg, addrs, addrMap, obs); err != nil {
		return nil, errors.Tag(err)
	}

	poolSize := cfg.PoolSize
	if poolSize <= 0 {
		poolSize = 100
	}

	var rdb redis.UniversalClient
	if !isClusterMode(cfg, addrs) {
		option := &redis.Options{
			Addr:            addrs[0],
			Password:        cfg.Password,
			DB:              cfg.DB,
			PoolSize:        poolSize,
			MinIdleConns:    poolSize / 5,
			DisableIdentity: true,
			Protocol:        2,
			MaintNotificationsConfig: &maintnotifications.Config{
				Mode: maintnotifications.ModeDisabled,
			},
		}
		applyTLSConfig(option, cfg)
		rdb = redis.NewClient(option)
	} else {
		clusterOpts := &redis.ClusterOptions{
			Addrs:           addrs,
			Password:        cfg.Password,
			PoolSize:        poolSize,
			MinIdleConns:    poolSize / 5,
			DisableIdentity: true,
			Protocol:        2,
			MaintNotificationsConfig: &maintnotifications.Config{
				Mode: maintnotifications.ModeDisabled,
			},
		}
		applyClusterTLSConfig(clusterOpts, cfg, obs)
		applyDevClusterProxySlots(clusterOpts, addrMap, obs)
		if len(addrMap) > 0 {
			clusterOpts.NewClient = func(opt *redis.Options) *redis.Client {
				cloned := *opt
				cloned.Addr = rewriteClusterAddr(opt.Addr, addrMap)
				return redis.NewClient(&cloned)
			}
		}
		rdb = redis.NewClusterClient(clusterOpts)
	}

	rdb.AddHook(newHook(time.Duration(obs.RedisSlowMs) * time.Millisecond))
	if err := rdb.Ping(ctx).Err(); err != nil {
		_ = rdb.Close()
		return nil, errors.Tag(err)
	}
	return rdb, nil
}

// resolveAddrs 清洗 Redis 地址列表并去重。
func resolveAddrs(addrs []string) ([]string, error) {
	if len(addrs) == 0 {
		return nil, errors.Errorf("缺少 Redis 地址配置")
	}
	result := make([]string, 0, len(addrs))
	seen := make(map[string]struct{}, len(addrs))
	for _, addr := range addrs {
		trimmed := strings.TrimSpace(addr)
		if trimmed == "" {
			continue
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		result = append(result, trimmed)
	}
	if len(result) == 0 {
		return nil, errors.Errorf("缺少 Redis 地址配置")
	}
	return result, nil
}

// pingConfiguredAddrs 在启动期探测声明的 Redis 地址，提前暴露不可达配置。
func pingConfiguredAddrs(ctx context.Context, cfg config.RedisConfig, addrs []string, addrMap map[string]string, obs config.ObservabilityConfig) error {
	db := cfg.DB
	if isClusterMode(cfg, addrs) {
		db = 0
	}
	if isClusterMode(cfg, addrs) && shouldUseDevClusterProxySlots(addrs, obs) {
		return nil
	}
	for idx, addr := range addrs {
		pingAddr := addr
		if isClusterMode(cfg, addrs) {
			pingAddr = rewriteClusterAddr(addr, addrMap)
		}
		option := &redis.Options{
			Addr:            pingAddr,
			Password:        cfg.Password,
			DB:              db,
			PoolSize:        1,
			DisableIdentity: true,
			Protocol:        2,
			MaintNotificationsConfig: &maintnotifications.Config{
				Mode: maintnotifications.ModeDisabled,
			},
		}
		applyTLSConfig(option, cfg)
		client := redis.NewClient(option)
		err := client.Ping(ctx).Err()
		_ = client.Close()
		if err != nil {
			return errors.Wrapf(err, "探测 Redis 地址[%d]=%s 失败", idx, pingAddr)
		}
	}
	return nil
}

// resolveAddrMap 清洗 Redis Cluster 地址改写表。
func resolveAddrMap(raw map[string]string) map[string]string {
	if len(raw) == 0 {
		return nil
	}
	result := make(map[string]string, len(raw))
	for key, value := range raw {
		trimmedKey := strings.TrimSpace(key)
		trimmedValue := strings.TrimSpace(value)
		if trimmedKey == "" || trimmedValue == "" {
			continue
		}
		result[trimmedKey] = trimmedValue
	}
	if len(result) == 0 {
		return nil
	}
	return result
}

// isClusterMode 根据显式 type 和地址数量判断 Redis 模式。
func isClusterMode(cfg config.RedisConfig, addrs []string) bool {
	switch strings.ToLower(strings.TrimSpace(cfg.Type)) {
	case "cluster":
		return true
	case "single", "standalone":
		return false
	default:
		return len(addrs) > 1
	}
}

// applyTLSConfig 为单机 Redis 客户端应用 TLS 配置。
func applyTLSConfig(option *redis.Options, cfg config.RedisConfig) {
	if option == nil || !cfg.TLS {
		return
	}
	option.TLSConfig = &tls.Config{
		MinVersion:         tls.VersionTLS12,
		InsecureSkipVerify: cfg.TLSInsecureSkipVerify,
	}
}

// applyClusterTLSConfig 为 Redis Cluster 客户端应用 TLS 配置。
func applyClusterTLSConfig(option *redis.ClusterOptions, cfg config.RedisConfig, obs config.ObservabilityConfig) {
	if option == nil || !cfg.TLS {
		return
	}
	insecureSkipVerify := cfg.TLSInsecureSkipVerify
	if isDevEnvironment(obs) {
		insecureSkipVerify = true
	}
	option.TLSConfig = &tls.Config{
		MinVersion:         tls.VersionTLS12,
		InsecureSkipVerify: insecureSkipVerify,
	}
}

// applyDevClusterProxySlots 兼容本地代理单入口访问 Redis Cluster 的开发场景。
func applyDevClusterProxySlots(option *redis.ClusterOptions, addrMap map[string]string, obs config.ObservabilityConfig) {
	if option == nil || !shouldUseDevClusterProxySlots(option.Addrs, obs) {
		return
	}
	entryAddr := rewriteClusterAddr(option.Addrs[0], addrMap)
	option.ClusterSlots = func(context.Context) ([]redis.ClusterSlot, error) {
		return []redis.ClusterSlot{{
			Start: 0,
			End:   16383,
			Nodes: []redis.ClusterNode{{Addr: entryAddr}},
		}}, nil
	}
}

// shouldUseDevClusterProxySlots 判断是否启用开发环境单入口 slots 覆盖。
func shouldUseDevClusterProxySlots(addrs []string, obs config.ObservabilityConfig) bool {
	return isDevEnvironment(obs) && len(addrs) == 1 && strings.TrimSpace(addrs[0]) != ""
}

// isDevEnvironment 判断当前观测配置是否声明为开发环境。
func isDevEnvironment(obs config.ObservabilityConfig) bool {
	return strings.EqualFold(strings.TrimSpace(obs.Environment), "dev")
}

// rewriteClusterAddr 按地址改写表替换 Redis Cluster 节点地址。
func rewriteClusterAddr(addr string, addrMap map[string]string) string {
	if len(addrMap) == 0 {
		return addr
	}
	if mapped, ok := addrMap[addr]; ok && mapped != "" {
		return mapped
	}
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return addr
	}
	mappedHost, ok := addrMap[host]
	if !ok || mappedHost == "" {
		return addr
	}
	if strings.Contains(mappedHost, ":") {
		return mappedHost
	}
	return net.JoinHostPort(mappedHost, port)
}

// hook 负责把 go-redis 命令执行结果转成结构化日志。
type hook struct {
	slowThreshold time.Duration // 慢 Redis 命令阈值
}

// newHook 创建 Redis 命令日志 hook。
func newHook(slowThreshold time.Duration) hook {
	return hook{slowThreshold: slowThreshold}
}

// DialHook 保持默认拨号流程，仅满足 go-redis Hook 接口。
func (h hook) DialHook(next redis.DialHook) redis.DialHook {
	return next
}

// ProcessHook 记录单条 Redis 命令耗时和错误。
func (h hook) ProcessHook(next redis.ProcessHook) redis.ProcessHook {
	return func(ctx context.Context, cmd redis.Cmder) error {
		begin := time.Now()
		err := next(ctx, cmd)
		h.logProcess(ctx, time.Since(begin), err, cmd)
		return err
	}
}

// ProcessPipelineHook 记录 Redis Pipeline 耗时和错误。
func (h hook) ProcessPipelineHook(next redis.ProcessPipelineHook) redis.ProcessPipelineHook {
	return func(ctx context.Context, cmds []redis.Cmder) error {
		begin := time.Now()
		err := next(ctx, cmds)
		if err != nil {
			fields := []logx.LogField{
				logx.Field("latency_ms", time.Since(begin).Milliseconds()),
				logx.Field("commands", pipelineNames(cmds)),
			}
			loggerx.Errorw(ctx, "缓存 管道执行失败", err, fields...)
			return err
		}
		if h.slowThreshold > 0 && time.Since(begin) > h.slowThreshold {
			fields := []logx.LogField{
				logx.Field("latency_ms", time.Since(begin).Milliseconds()),
				logx.Field("commands", pipelineNames(cmds)),
			}
			loggerx.Sloww(ctx, "缓存 管道耗时较高", fields...)
		}
		return nil
	}
}

// logProcess 按错误和慢阈值输出单条 Redis 命令日志。
func (h hook) logProcess(ctx context.Context, duration time.Duration, err error, cmd redis.Cmder) {
	fields := []logx.LogField{
		logx.Field("latency_ms", duration.Milliseconds()),
		logx.Field("cmd", cmd.FullName()),
		logx.Field("arg_count", max(len(cmd.Args())-1, 0)),
	}
	switch {
	case isRedisScriptCacheMiss(err, cmd):
		return
	case err != nil && !errors.Is(err, redis.Nil):
		loggerx.Errorw(ctx, "缓存 命令执行失败", err, fields...)
	case h.slowThreshold > 0 && duration > h.slowThreshold:
		loggerx.Sloww(ctx, "缓存 命令耗时较高", fields...)
	}
}

// isRedisScriptCacheMiss 判断 EVALSHA 脚本缓存未命中，避免误记为异常。
func isRedisScriptCacheMiss(err error, cmd redis.Cmder) bool {
	if err == nil || cmd == nil {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(cmd.FullName())) {
	case redisCommandEvalSHA, redisCommandEvalSHARO:
	default:
		return false
	}
	if errors.Is(err, redis.ErrNoScript) || redis.HasErrorPrefix(err, redisNoScriptPrefix) {
		return true
	}
	return strings.HasPrefix(err.Error(), redisNoScriptPrefix)
}

// pipelineNames 提取 Pipeline 命令名称，避免日志记录完整参数。
func pipelineNames(cmds []redis.Cmder) []string {
	names := make([]string, 0, len(cmds))
	for _, cmd := range cmds {
		names = append(names, cmd.FullName())
	}
	return names
}
