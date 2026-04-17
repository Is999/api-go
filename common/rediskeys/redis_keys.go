package keys

// Redis Key 模板集中维护，业务代码只能按模板精确读写。
const (
	// UserSession 表示前台用户登录会话缓存键模板。
	// Redis 类型：String(Token)。
	// 参数依次为用户 ID、JWT jti；实际 Redis key 通过 AppScopedKey 追加 app_id 前缀。
	UserSession = "user:session:%d:%s"

	// UserSessionIndex 表示前台用户登录会话 jti 索引键模板。
	// Redis 类型：ZSet，member 为 JWT jti，score 为会话过期时间戳。
	// 参数为用户 ID；实际 Redis key 通过 AppScopedKey 追加 app_id 前缀。
	UserSessionIndex = "user:session:index:%d"

	// AuthRateLimitCount 表示认证入口限流计数键模板。
	// Redis 类型：String。
	// 参数依次为动作、主体哈希；实际 Redis key 通过 AppScopedKey 追加 app_id 前缀。
	AuthRateLimitCount = "auth:rate_limit:count:%s:%s"

	// AuthRateLimitLock 表示认证入口超限锁定键模板。
	// Redis 类型：String。
	// 参数依次为动作、主体哈希；实际 Redis key 通过 AppScopedKey 追加 app_id 前缀。
	AuthRateLimitLock = "auth:rate_limit:lock:%s:%s"

	// UserProfile 表示前台用户公开资料缓存键模板。
	// Redis 类型：String(JSON)。
	// 参数为用户 ID；实际 Redis key 通过 AppScopedKey 追加 app_id 前缀。
	UserProfile = "user:profile:%d"

	// SysConfigUUID 表示系统配置缓存键模板。
	// Redis 类型：Hash。
	// 参数为配置 uuid；实际 Redis key 通过 AppScopedKey 追加 app_id 前缀。
	SysConfigUUID = "config_uuid:%s"

	// SignatureReplayRequest 表示签名防重放缓存键模板。
	// Redis 类型：String。
	// 参数为 trace_id；实际 Redis key 通过 AppScopedKey 追加 app_id 前缀。
	SignatureReplayRequest = "signature:replay:%s"

	// CacheRebuildLock 表示缓存回源重建互斥锁 key 模板。
	// Redis 类型：String（由 redsync 管理）。
	// `%s` 位置填充真实缓存 key 的业务段；实际 Redis key 通过 AppScopedKey 追加 app_id 前缀。
	CacheRebuildLock = "cache:rebuild:lock:%s"
)
