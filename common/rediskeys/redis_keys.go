package keys

// Redis key 根前缀集中维护。
const (
	// ScopeRoot 表示业务 Redis key 的 app_id 命名空间根前缀。
	// Redis 类型：命名空间前缀，TTL 过期规则：不直接写入 Redis。
	ScopeRoot = "app:"
)

// table-cache Redis key 二级前缀集中维护。
const (
	// tableCacheSegment 表示 table-cache 托管缓存的二级前缀。
	// Redis 类型：Key 片段，TTL 过期规则：不直接写入 Redis，由具体 table-cache 目标控制。
	tableCacheSegment = "table"

	// EmptyValueMarker 表示空值缓存占位符，避免缓存穿透时重复回源。
	// Redis 类型：占位值，TTL 过期规则：由具体缓存目标控制。
	EmptyValueMarker = "__empty__"
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
	// Redis 类型：Hash，TTL 过期规则：由 table-cache 目标配置控制。
	// 参数为配置 uuid；实际 Redis key 通过 TableCachePrefix 追加 app_id 与 table 前缀。
	SysConfigUUID = "config_uuid:%s"

	// SysConfigUUIDPattern 表示系统配置缓存键展示模板。
	// Redis 类型：Hash 模板，TTL 过期规则：不直接写入 Redis，仅用于展示或匹配真实 key。
	SysConfigUUIDPattern = "config_uuid:{uuid}"

	// SignatureReplayRequest 表示签名防重放缓存键模板。
	// Redis 类型：String，TTL 过期规则：按签名防重放 TTL 自动过期。
	// 参数为 trace_id；实际 Redis key 通过 WithPrefix 追加 app_id 前缀。
	SignatureReplayRequest = "signature:replay:%s"
)
