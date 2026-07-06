package config

import "github.com/zeromicro/go-zero/rest"

// MySQLConfig 定义关系数据库连接与连接池参数。
type MySQLConfig struct {
	WriteDataSource string   `json:"write_data_source"`          // 写库 DSN（必填）
	ReadDataSources []string `json:"read_data_sources,optional"` // 读库 DSN 列表（启用读写分离）
	MaxOpenConns    int      `json:"max_open_conns"`             // 最大打开连接数
	MaxIdleConns    int      `json:"max_idle_conns"`             // 最大空闲连接数
	ConnMaxLifetime int      `json:"conn_max_lifetime"`          // 连接最大生命周期，单位：秒
	Debug           bool     `json:"debug"`                      // 是否开启 GORM 调试模式
}

// SiteMySQLConfig 定义可选命名扩展库配置。
type SiteMySQLConfig map[string]MySQLConfig

// RedisConfig 定义 Redis 连接与连接池参数。
type RedisConfig struct {
	Type                  string            `json:"type,optional"`                     // Redis 模式：single 或 cluster
	Addrs                 []string          `json:"addrs"`                             // Redis 地址列表
	AddrMap               map[string]string `json:"addr_map,optional"`                 // 集群地址改写表
	Password              string            `json:"password"`                          // 密码
	DB                    int               `json:"db"`                                // 数据库编号（仅单机有效）
	PoolSize              int               `json:"pool_size"`                         // 连接池大小
	TLS                   bool              `json:"tls,optional"`                      // 是否启用 TLS
	TLSInsecureSkipVerify bool              `json:"tls_insecure_skip_verify,optional"` // 是否跳过 TLS 证书校验
}

// SnowflakeRedisConfig 定义雪花 node_id 的 Redis 租约分配配置。
type SnowflakeRedisConfig struct {
	Enabled              bool                                     `json:"enabled,optional"`                // 是否使用 Redis 租约自动分配 node_id
	Scope                string                                   `json:"scope,optional"`                  // node_id 池部署级作用域，同一套 api/admin 部署必须一致
	LeaseSeconds         int                                      `json:"lease_seconds,optional"`          // node_id 租约 TTL，单位秒
	RenewIntervalSeconds int                                      `json:"renew_interval_seconds,optional"` // node_id 续约间隔，单位秒
	Namespaces           map[string]SnowflakeRedisNamespaceConfig `json:"namespaces,optional"`             // 按业务 namespace 覆盖 node_id 池大小
}

// SnowflakeRedisNamespaceConfig 定义单个业务命名空间的雪花 node_id 池策略。
type SnowflakeRedisNamespaceConfig struct {
	NodeCount int `json:"node_count,optional"` // 当前 namespace 可竞争的 node_id 数量；0 表示使用完整 0-1023 池
}

// IDSegmentNamespaceConfig 定义单个业务命名空间的 Redis 号段取号策略。
type IDSegmentNamespaceConfig struct {
	Enabled           bool  `json:"enabled,optional"`            // 是否让该 namespace 使用 Segment 策略
	Step              int64 `json:"step,optional"`               // 每次从 Redis 申请的号段大小
	PrefetchThreshold int64 `json:"prefetch_threshold,optional"` // 当前号段剩余小于等于该值时预取下一段
	Start             int64 `json:"start,optional"`              // Redis key 首次初始化的高水位，默认 0
}

// IDSegmentConfig 定义高吞吐业务使用的 Redis 号段本地缓存策略。
type IDSegmentConfig struct {
	Enabled                bool                                `json:"enabled,optional"`                  // 是否启用 Segment 策略
	Scope                  string                              `json:"scope,optional"`                    // 号段高水位部署级作用域，同一套 api/admin 部署必须一致
	AllocateTimeoutSeconds int                                 `json:"allocate_timeout_seconds,optional"` // 单次 Redis 号段申请超时，单位秒
	Namespaces             map[string]IDSegmentNamespaceConfig `json:"namespaces,optional"`               // 按业务 namespace 配置 Segment 策略
}

// SnowflakeConfig 定义业务 ID 生成配置，默认使用雪花，指定 namespace 可切换 Redis Segment。
type SnowflakeConfig struct {
	WorkerID *int64               `json:"worker_id,optional"` // 手动指定 node_id，范围 0-1023；多实例优先使用 Redis 租约
	Redis    SnowflakeRedisConfig `json:"redis,optional"`     // Redis 租约 node_id 分配配置
	Segment  IDSegmentConfig      `json:"segment,optional"`   // 高吞吐业务号段配置；启用的 namespace 不再使用雪花位格式
}

// SecuritySecretKeyVersionConfig 定义配置文件中的单个秘钥版本材料。
type SecuritySecretKeyVersionConfig struct {
	KeyVersion             string `json:"key_version"`                         // 秘钥版本号
	AESKey                 string `json:"aes_key,optional"`                    // AES KEY 明文；为空时读取 aes_key_ref
	AESKeyRef              string `json:"aes_key_ref,optional"`                // AES KEY 文件路径
	AESIV                  string `json:"aes_iv,optional"`                     // AES IV 明文；为空时读取 aes_iv_ref
	AESIVRef               string `json:"aes_iv_ref,optional"`                 // AES IV 文件路径
	RSAPublicKeyUser       string `json:"rsa_public_key_user,optional"`        // 用户 RSA 公钥 PEM 文本
	RSAPublicKeyUserRef    string `json:"rsa_public_key_user_ref,optional"`    // 用户 RSA 公钥 PEM 文件路径
	RSAPublicKeyServer     string `json:"rsa_public_key_server,optional"`      // 服务端 RSA 公钥 PEM 文本
	RSAPublicKeyServerRef  string `json:"rsa_public_key_server_ref,optional"`  // 服务端 RSA 公钥 PEM 文件路径
	RSAPrivateKeyServer    string `json:"rsa_private_key_server,optional"`     // 服务端 RSA 私钥 PEM 文本
	RSAPrivateKeyServerRef string `json:"rsa_private_key_server_ref,optional"` // 服务端 RSA 私钥 PEM 文件路径
	Remark                 string `json:"remark,optional"`                     // 版本备注
}

// SecuritySecretKeyConfig 定义当前 app_id 的签名验签和加解密秘钥配置。
type SecuritySecretKeyConfig struct {
	KeyVersion             string                           `json:"key_version,optional"`                // 单版本秘钥版本号
	AESKey                 string                           `json:"aes_key,optional"`                    // 单版本 AES KEY 明文
	AESKeyRef              string                           `json:"aes_key_ref,optional"`                // 单版本 AES KEY 文件路径
	AESIV                  string                           `json:"aes_iv,optional"`                     // 单版本 AES IV 明文
	AESIVRef               string                           `json:"aes_iv_ref,optional"`                 // 单版本 AES IV 文件路径
	RSAPublicKeyUser       string                           `json:"rsa_public_key_user,optional"`        // 单版本用户 RSA 公钥 PEM 文本
	RSAPublicKeyUserRef    string                           `json:"rsa_public_key_user_ref,optional"`    // 单版本用户 RSA 公钥 PEM 文件路径
	RSAPublicKeyServer     string                           `json:"rsa_public_key_server,optional"`      // 单版本服务端 RSA 公钥 PEM 文本
	RSAPublicKeyServerRef  string                           `json:"rsa_public_key_server_ref,optional"`  // 单版本服务端 RSA 公钥 PEM 文件路径
	RSAPrivateKeyServer    string                           `json:"rsa_private_key_server,optional"`     // 单版本服务端 RSA 私钥 PEM 文本
	RSAPrivateKeyServerRef string                           `json:"rsa_private_key_server_ref,optional"` // 单版本服务端 RSA 私钥 PEM 文件路径
	SignStatus             int                              `json:"sign_status,optional,default=1"`      // 签名验签状态：1启用，0停用
	CryptoStatus           int                              `json:"crypto_status,optional,default=1"`    // 加密解密状态：1启用，0停用
	StableVersion          string                           `json:"stable_version,optional"`             // 稳定版本；为空时回退 key_version
	GrayVersion            string                           `json:"gray_version,optional"`               // 灰度版本
	GrayPercent            int                              `json:"gray_percent,optional"`               // 灰度比例，0-100
	GraySalt               string                           `json:"gray_salt,optional"`                  // 灰度哈希盐值
	Versions               []SecuritySecretKeyVersionConfig `json:"versions,optional"`                   // 多版本材料列表
}

// SecurityConfig 聚合前台接口安全链路配置。
type SecurityConfig struct {
	SecretKey SecuritySecretKeyConfig `json:"secret_key,optional"` // 当前 app_id 的秘钥版本和材料配置
}

// HotReloadConfig 定义 config.yaml 热加载监听参数。
type HotReloadConfig struct {
	Enabled              bool `json:"enabled,optional"`                // 是否启用配置热加载
	CheckIntervalSeconds int  `json:"check_interval_seconds,optional"` // 配置文件轮询间隔，单位秒
}

// ConfigFilesConfig 定义可选外部配置文件入口。
type ConfigFilesConfig struct {
	Runtime string `json:"runtime,optional"` // 运行期配置文件路径
}

// CollectorKafkaConfig 定义通用收集器 Kafka 投递配置。
type CollectorKafkaConfig struct {
	Brokers                    []string `json:"brokers,optional"`                       // Kafka broker 地址
	WriteBatchSize             int      `json:"write_batch_size,optional"`              // Producer 写入批次大小
	WriteBatchWaitMilliseconds int      `json:"write_batch_wait_milliseconds,optional"` // Producer 写入批次等待时间，单位毫秒
	WriteTimeout               int      `json:"write_timeout,optional"`                 // Producer 写入超时时间，单位秒
}

// CollectorTaskConfig 定义单个 Collector 任务的 Kafka 路由。
type CollectorTaskConfig struct {
	Topic string `json:"topic,optional"` // 当前 bizType 投递的 Kafka Topic
}

// CollectorConfig 定义通用收集器配置。
type CollectorConfig struct {
	Enabled     bool                           `json:"enabled,optional"`      // 是否启用通用收集器
	Kafka       CollectorKafkaConfig           `json:"kafka,optional"`        // Kafka 投递链路配置
	DefaultTask CollectorTaskConfig            `json:"default_task,optional"` // 未单独配置 bizType 时的默认路由
	Tasks       map[string]CollectorTaskConfig `json:"tasks,optional"`        // 按 bizType 覆盖的 Kafka 路由
}

// ObservabilityConfig 聚合日志、链路追踪相关配置。
type ObservabilityConfig struct {
	ServiceName     string  `json:"service_name,optional"`       // 服务名
	Environment     string  `json:"environment,optional"`        // 观测环境，由顶层 Mode 填充
	TraceEnabled    bool    `json:"trace_enabled,optional"`      // 是否启用 trace 采样/上报
	OTLPProtocol    string  `json:"otlp_protocol,optional"`      // OTLP 协议：grpc/http
	OTLPEndpoint    string  `json:"otlp_endpoint,optional"`      // OTLP endpoint
	OTLPInsecure    bool    `json:"otlp_insecure,optional"`      // OTLP 是否明文
	SampleRatio     float64 `json:"sample_ratio,optional"`       // trace 采样率 0~1
	SlowSQLMs       int64   `json:"slow_sql_ms,optional"`        // 慢 SQL 阈值，毫秒
	RedisSlowMs     int64   `json:"redis_slow_ms,optional"`      // 慢 Redis 阈值，毫秒
	LogBodyMaxBytes int     `json:"log_body_max_bytes,optional"` // 日志负载最大长度
}

// LarkAlertConfig 定义 Lark 群机器人告警配置。
type LarkAlertConfig struct {
	Enabled        bool   `json:"enabled,optional"`         // 是否启用 Lark 告警
	WebhookURL     string `json:"webhook_url,optional"`     // Lark 机器人 webhook URL；为空时读取 webhook_url_ref
	WebhookURLRef  string `json:"webhook_url_ref,optional"` // webhook URL 文件路径
	Secret         string `json:"secret,optional"`          // Lark 签名密钥；为空时读取 secret_ref
	SecretRef      string `json:"secret_ref,optional"`      // Lark 签名密钥文件路径
	TimeoutSeconds int    `json:"timeout_seconds,optional"` // HTTP 请求超时，单位秒，默认 5 秒
	AtAll          bool   `json:"at_all,optional"`          // 是否在告警中 @所有人
	MaxErrorBytes  int    `json:"max_error_bytes,optional"` // 错误摘要最大字节数，默认 800
}

// AlertConfig 聚合外部告警通道配置。
type AlertConfig struct {
	Lark LarkAlertConfig `json:"lark,optional"` // Lark 群机器人告警配置
}

// AuthConfig 定义前台用户登录态运行参数。
type AuthConfig struct {
	RegisterEnabled        bool                `json:"register_enabled,optional"`              // 是否开放注册接口
	Issuer                 string              `json:"issuer,optional"`                        // JWT issuer
	SessionTTLSeconds      int64               `json:"session_ttl_seconds,optional"`           // Redis 会话 TTL；<=0 或超过 JWT 时使用 jwt_expires_in
	ProfileCacheTTLSeconds int64               `json:"profile_cache_ttl_seconds,optional"`     // 用户资料缓存 TTL
	PasswordMinLength      int                 `json:"password_min_length,optional,default=8"` // 密码最小长度
	LoginRateLimit         AuthRateLimitConfig `json:"login_rate_limit,optional"`              // 登录限流配置
	RegisterRateLimit      AuthRateLimitConfig `json:"register_rate_limit,optional"`           // 注册限流配置
}

// AuthRateLimitConfig 定义前台认证入口的 Redis 限流参数。
type AuthRateLimitConfig struct {
	Enabled       bool `json:"enabled,optional"`        // 是否启用限流
	WindowSeconds int  `json:"window_seconds,optional"` // 统计窗口，单位秒
	MaxAttempts   int  `json:"max_attempts,optional"`   // 窗口内最大尝试次数
	LockSeconds   int  `json:"lock_seconds,optional"`   // 超限后的锁定时间，单位秒
}

// UserConfig 定义业务用户写入和后续拆表路由配置。
type UserConfig struct {
	RouteShardCount int `json:"route_shard_count,optional,default=1"` // 新增用户默认物理表数量：1/2/4/.../1024
}

// OpsConfig 定义运维级接口保护配置。
type OpsConfig struct {
	ConfigReloadToken      string   `json:"config_reload_token,optional"`       // 配置热加载接口运维令牌
	ConfigReloadAllowedIPs []string `json:"config_reload_allowed_ips,optional"` // 配置热加载允许的内网 IP 或 CIDR
}

// Config 是前台 API 服务总配置。
type Config struct {
	rest.RestConf                     // go-zero HTTP 服务配置
	AppID         string              `json:"app_id,optional"`                       // 站点/应用 ID
	AppKey        string              `json:"app_key,optional"`                      // 全局应用密钥，用于安全链路扩展
	InstanceID    string              `json:"instance_id,optional"`                  // 当前实例 ID；为空时使用主机名
	Snowflake     SnowflakeConfig     `json:"snowflake,optional"`                    // 分布式雪花 ID 配置
	JwtSecret     string              `json:"jwt_secret"`                            // JWT 签名密钥
	JwtExpiresIn  int64               `json:"jwt_expires_in,optional,default=86400"` // JWT 过期时间，单位秒
	Auth          AuthConfig          `json:"auth,optional"`                         // 前台用户认证配置
	User          UserConfig          `json:"user,optional"`                         // 业务用户写入路由配置
	HotReload     HotReloadConfig     `json:"hot_reload,optional"`                   // 配置热加载配置
	ConfigFiles   ConfigFilesConfig   `json:"config_files,optional"`                 // 外部配置文件入口
	Security      SecurityConfig      `json:"security,optional"`                     // 签名验签和加解密配置
	Collector     CollectorConfig     `json:"collector,optional"`                    // 通用收集器配置
	Ops           OpsConfig           `json:"ops,optional"`                          // 运维级接口保护配置
	Observability ObservabilityConfig `json:"observability,optional"`                // 日志与链路追踪配置
	Alert         AlertConfig         `json:"alert,optional"`                        // 外部运行异常告警配置
	MySQL         MySQLConfig         `json:"mysql,optional"`                        // 默认主库 MySQL 配置
	SiteMySQL     SiteMySQLConfig     `json:"site_mysql,optional"`                   // 可选命名扩展库配置
	Redis         RedisConfig         `json:"redis"`                                 // Redis 连接与连接池配置
}
