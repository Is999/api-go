package keys

// Redis Key 模板集中维护，业务代码只能按模板精确读写。
const (
	// UserSession 表示前台用户登录会话缓存键模板。
	// Redis 类型：String(Token)。
	// 参数依次为 app_id、用户 ID、JWT jti。
	UserSession = "api:user:session:%s:%d:%s"

	// UserSessionIndex 表示前台用户登录会话 jti 索引键模板。
	// Redis 类型：ZSet，member 为 JWT jti，score 为会话过期时间戳。
	// 参数依次为 app_id、用户 ID。
	UserSessionIndex = "api:user:session:index:%s:%d"

	// AuthRateLimitCount 表示认证入口限流计数键模板。
	// Redis 类型：String。
	// 参数依次为 app_id、动作、主体哈希。
	AuthRateLimitCount = "api:auth:rate_limit:count:%s:%s:%s"

	// AuthRateLimitLock 表示认证入口超限锁定键模板。
	// Redis 类型：String。
	// 参数依次为 app_id、动作、主体哈希。
	AuthRateLimitLock = "api:auth:rate_limit:lock:%s:%s:%s"

	// UserProfile 表示前台用户公开资料缓存键模板。
	// Redis 类型：String(JSON)。
	// 参数依次为 app_id、用户 ID。
	UserProfile = "api:user:profile:%s:%d"

	// SysConfigUUID 表示系统配置缓存键模板。
	// Redis 类型：Hash。
	// 参数依次为 app_id、配置 uuid。
	SysConfigUUID = "api:sys_config:%s:%s"

	// SignatureReplayRequest 表示签名防重放缓存键模板。
	// Redis 类型：String。
	// 参数依次为 app_id、trace_id。
	SignatureReplayRequest = "api:signature:replay:%s:%s"

	// CacheRebuildLock 表示缓存回源重建互斥锁 key 模板。
	// Redis 类型：String（由 redsync 管理）。
	// `%s` 位置填充真实缓存 key。
	CacheRebuildLock = "api:cache:rebuild:lock:%s"
)
