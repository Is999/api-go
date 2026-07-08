package collectorx

// 认证风控事件动作枚举。
const (
	AuthSecurityActionRegisterSuccess      = "register_success"       // 注册成功
	AuthSecurityActionLoginSuccess         = "login_success"          // 登录成功
	AuthSecurityActionLoginFailed          = "login_failed"           // 登录失败
	AuthSecurityActionRateLimited          = "rate_limited"           // 认证入口触发限流
	AuthSecurityActionAuthFailed           = "auth_failed"            // 登录态鉴权失败
	AuthSecurityActionSecurityFailed       = "security_failed"        // 签名或加密链路失败
	AuthSecurityActionRefreshSuccess       = "refresh_success"        // 刷新 token 成功
	AuthSecurityActionLogoutSuccess        = "logout_success"         // 退出登录成功
	AuthSecurityActionSessionInvalidateAll = "session_invalidate_all" // 用户全部 session 失效
)

// 认证风控事件原因枚举。
const (
	AuthSecurityReasonInvalidPassword          = "invalid_password"            // 账号或密码错误
	AuthSecurityReasonUserDisabled             = "user_disabled"               // 用户被禁用
	AuthSecurityReasonUserNotFound             = "user_not_found"              // 用户不存在
	AuthSecurityReasonMissingBearer            = "missing_bearer"              // 缺少 Bearer token
	AuthSecurityReasonTokenExpired             = "token_expired"               // token 已过期
	AuthSecurityReasonSessionExpired           = "session_expired"             // Redis session 已失效
	AuthSecurityReasonTokenInvalid             = "token_invalid"               // token 无效
	AuthSecurityReasonSecurityFailed           = "security_failed"             // 签名或加密链路失败
	AuthSecurityReasonSecurityAppIDInvalid     = "security_app_id_invalid"     // 安全链路 AppID 无效
	AuthSecurityReasonSecurityKeyUnavailable   = "security_key_unavailable"    // 安全链路秘钥不可用
	AuthSecurityReasonSignatureFailed          = "signature_failed"            // 请求验签失败
	AuthSecurityReasonSecurityPayloadTooLarge  = "security_payload_too_large"  // 安全字段或请求体超过上限
	AuthSecurityReasonResponseSignFailed       = "response_sign_failed"        // 响应回签失败
	AuthSecurityReasonCryptoDisabled           = "crypto_disabled"             // 加解密链路关闭
	AuthSecurityReasonRequestDecryptFailed     = "request_decrypt_failed"      // 请求解密失败
	AuthSecurityReasonResponseEncryptFailed    = "response_encrypt_failed"     // 响应加密失败
	AuthSecurityReasonLoginIPRateLimited       = "login_ip_rate_limited"       // 登录 IP 限流
	AuthSecurityReasonLoginIdentityRateLimited = "login_identity_rate_limited" // 登录身份限流
	AuthSecurityReasonRegisterIPRateLimited    = "register_ip_rate_limited"    // 注册 IP 限流
	AuthSecurityReasonSessionCreated           = "session_created"             // 新会话已创建
	AuthSecurityReasonSessionRotated           = "session_rotated"             // 会话已轮换
	AuthSecurityReasonCurrentSessionDeleted    = "current_session_deleted"     // 当前会话已删除
	AuthSecurityReasonUserSessionsInvalidated  = "user_sessions_invalidated"   // 用户会话已全部失效
)
