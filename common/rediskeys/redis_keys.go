package keys

// Redis key 根前缀集中维护。
const (
	// ScopeRoot 表示业务 Redis key 的 app_id 命名空间根前缀。
	// Redis 类型：命名空间前缀，TTL 过期规则：不直接写入 Redis。
	ScopeRoot = "app:"
)

// Redis Key 模板集中维护，业务代码只能按模板精确读写。
const (
	// UserSession 表示前台用户登录会话缓存键模板。
	// Redis 类型：String(Token)，TTL 过期规则：使用 Auth 会话 TTL，且不超过 JWT 过期时间。
	// 参数依次为用户 ID、JWT jti；实际 Redis key 通过 WithPrefix 追加 app_id 前缀。
	UserSession = "user:session:%d:%s"

	// UserSessionIndex 表示前台用户登录会话 jti 索引键模板。
	// Redis 类型：ZSet，TTL 过期规则：使用 Auth 会话 TTL，并在鉴权或失效时清理过期 jti。
	// 参数为用户 ID；实际 Redis key 通过 WithPrefix 追加 app_id 前缀。
	UserSessionIndex = "user:session:index:%d"

	// AuthRateLimitCount 表示认证入口限流计数键模板。
	// Redis 类型：String，TTL 过期规则：按认证限流窗口 TTL 过期，登录成功后精确删除。
	// 参数依次为动作、主体哈希；实际 Redis key 通过 WithPrefix 追加 app_id 前缀。
	AuthRateLimitCount = "auth:rate_limit:count:%s:%s"

	// AuthRateLimitLock 表示认证入口超限锁定键模板。
	// Redis 类型：String，TTL 过期规则：按认证限流锁定 TTL 过期，登录成功后精确删除。
	// 参数依次为动作、主体哈希；实际 Redis key 通过 WithPrefix 追加 app_id 前缀。
	AuthRateLimitLock = "auth:rate_limit:lock:%s:%s"

	// UserProfile 表示前台用户公开资料缓存键模板。
	// Redis 类型：String(JSON)，TTL 过期规则：使用用户资料缓存 TTL，默认 5 分钟。
	// 参数为用户 ID；实际 Redis key 通过 WithPrefix 追加 app_id 前缀。
	UserProfile = "user:profile:%d"

	// SysConfigUUID 表示系统配置缓存键模板。
	// Redis 类型：Hash，TTL 过期规则：无固定 TTL，按业务更新或删除精确失效。
	// 参数为配置 uuid；实际 Redis key 通过 WithPrefix 追加 app_id 前缀。
	SysConfigUUID = "config_uuid:%s"

	// SignatureReplayRequest 表示签名防重放缓存键模板。
	// Redis 类型：String，TTL 过期规则：按签名防重放 TTL 自动过期。
	// 参数为 trace_id；实际 Redis key 通过 WithPrefix 追加 app_id 前缀。
	SignatureReplayRequest = "signature:replay:%s"

	// CacheRebuildLock 表示缓存回源重建互斥锁 key 模板。
	// Redis 类型：String（由 redsync 管理），TTL 过期规则：由 redsync 锁 TTL 控制，到期自动释放。
	// `%s` 位置填充真实缓存 key 的业务段；实际 Redis key 通过 WithPrefix 追加 app_id 前缀。
	CacheRebuildLock = "cache:rebuild:lock:%s"
)
