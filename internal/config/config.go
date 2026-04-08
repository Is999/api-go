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

// CollectorRedisConfig 定义通用收集器 Redis Stream 载体配置。
type CollectorRedisConfig struct {
	Enabled  bool   `json:"enabled,optional"`  // 是否启用 Redis Stream 载体
	Stream   string `json:"stream,optional"`   // Redis Stream 名称
	Consumer string `json:"consumer,optional"` // Redis Stream 消费者名前缀
	MaxLen   int64  `json:"max_len,optional"`  // Stream 最大长度近似值，<=0 不裁剪
}

// CollectorConfig 定义通用收集器配置。
type CollectorConfig struct {
	Enabled   bool                 `json:"enabled,optional"`   // 是否启用通用收集器
	Transport string               `json:"transport,optional"` // 载体：sync/redis/auto
	Redis     CollectorRedisConfig `json:"redis,optional"`     // Redis Stream 载体配置
}

// ObservabilityConfig 聚合日志、链路追踪相关配置。
type ObservabilityConfig struct {
	ServiceName     string  `json:"service_name,optional"`       // 服务名
	Environment     string  `json:"environment,optional"`        // 环境名
	TraceEnabled    bool    `json:"trace_enabled,optional"`      // 是否启用 trace 采样/上报
	OTLPProtocol    string  `json:"otlp_protocol,optional"`      // OTLP 协议：grpc/http
	OTLPEndpoint    string  `json:"otlp_endpoint,optional"`      // OTLP endpoint
	OTLPInsecure    bool    `json:"otlp_insecure,optional"`      // OTLP 是否明文
	SampleRatio     float64 `json:"sample_ratio,optional"`       // trace 采样率 0~1
	SlowSQLMs       int64   `json:"slow_sql_ms,optional"`        // 慢 SQL 阈值，毫秒
	RedisSlowMs     int64   `json:"redis_slow_ms,optional"`      // 慢 Redis 阈值，毫秒
	LogBodyMaxBytes int     `json:"log_body_max_bytes,optional"` // 日志负载最大长度
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
	JwtSecret     string              `json:"jwt_secret"`                            // JWT 签名密钥
	JwtExpiresIn  int64               `json:"jwt_expires_in,optional,default=86400"` // JWT 过期时间，单位秒
	Auth          AuthConfig          `json:"auth,optional"`                         // 前台用户认证配置
	HotReload     HotReloadConfig     `json:"hot_reload,optional"`                   // 配置热加载配置
	ConfigFiles   ConfigFilesConfig   `json:"config_files,optional"`                 // 外部配置文件入口
	Security      SecurityConfig      `json:"security,optional"`                     // 签名验签和加解密配置
	Collector     CollectorConfig     `json:"collector,optional"`                    // 通用收集器配置
	Ops           OpsConfig           `json:"ops,optional"`                          // 运维级接口保护配置
	Observability ObservabilityConfig `json:"observability,optional"`                // 日志与链路追踪配置
	MySQL         MySQLConfig         `json:"mysql,optional"`                        // 默认主库 MySQL 配置
	SiteMySQL     SiteMySQLConfig     `json:"site_mysql,optional"`                   // 可选命名扩展库配置
	Redis         RedisConfig         `json:"redis"`                                 // Redis 连接与连接池配置
}
