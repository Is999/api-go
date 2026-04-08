package i18n

// 多语言消息 key，按通用、认证、用户和依赖模块分段维护。
const (
	// MsgKeyUndefined 表示未知业务状态的通用文案 key。
	MsgKeyUndefined = "common.undefined"
	// MsgKeySuccess 表示通用成功响应文案 key。
	MsgKeySuccess = "common.success"
	// MsgKeyFail 表示通用失败响应文案 key。
	MsgKeyFail = "common.fail"
	// MsgKeyOK 表示 HTTP OK 语义的文案 key。
	MsgKeyOK = "http.ok"
	// MsgKeyBadRequest 表示请求参数或格式错误的 HTTP 文案 key。
	MsgKeyBadRequest = "http.bad_request"
	// MsgKeyUnauthorized 表示未授权访问的 HTTP 文案 key。
	MsgKeyUnauthorized = "http.unauthorized"
	// MsgKeyForbidden 表示无权限访问的 HTTP 文案 key。
	MsgKeyForbidden = "http.forbidden"
	// MsgKeyNotFound 表示资源未找到的 HTTP 文案 key。
	MsgKeyNotFound = "http.not_found"
	// MsgKeyServerError 表示服务端异常的 HTTP 文案 key。
	MsgKeyServerError = "http.server_error"
	// MsgKeyServiceBusy 表示服务繁忙或依赖不可用的 HTTP 文案 key。
	MsgKeyServiceBusy = "http.service_busy"
	// MsgKeyTimeout 表示请求超时的 HTTP 文案 key。
	MsgKeyTimeout = "http.timeout"

	// MsgKeyParamError 表示通用参数错误的业务文案 key。
	MsgKeyParamError = "biz.param_error"
	// MsgKeyParamErrorFormat 表示参数错误动态详情模板 key。
	MsgKeyParamErrorFormat = "fmt.param_error"
	// MsgKeyAuthFailed 表示认证失败的业务文案 key。
	MsgKeyAuthFailed = "biz.auth_failed"
	// MsgKeyRateLimit 表示触发限流保护的业务文案 key。
	MsgKeyRateLimit = "biz.rate_limit"
	// MsgKeyInternalError 表示内部错误的业务文案 key。
	MsgKeyInternalError = "biz.internal_error"
	// MsgKeyInternalErrorFormat 表示内部错误动态详情模板 key。
	MsgKeyInternalErrorFormat = "fmt.internal_error"
	// MsgKeyDBError 表示数据库错误的业务文案 key。
	MsgKeyDBError = "biz.db_error"
	// MsgKeyDBErrorFormat 表示数据库错误动态详情模板 key。
	MsgKeyDBErrorFormat = "fmt.db_error"

	// MsgKeyCreateSuccess 表示创建成功的业务文案 key。
	MsgKeyCreateSuccess = "biz.create_success"
	// MsgKeySaveSuccess 表示保存成功的业务文案 key。
	MsgKeySaveSuccess = "biz.save_success"
	// MsgKeyUpdateSuccess 表示更新成功的业务文案 key。
	MsgKeyUpdateSuccess = "biz.update_success"
	// MsgKeyDeleteSuccess 表示删除成功的业务文案 key。
	MsgKeyDeleteSuccess = "biz.delete_success"
	// MsgKeyFetchSuccess 表示获取成功的业务文案 key。
	MsgKeyFetchSuccess = "biz.fetch_success"

	// MsgKeyUnauthorizedText 表示需要登录或重新登录的认证文案 key。
	MsgKeyUnauthorizedText = "auth.unauthorized_text"
	// MsgKeyTokenExpired 表示登录 token 已过期的认证文案 key。
	MsgKeyTokenExpired = "auth.token_expired"
	// MsgKeyTokenInvalid 表示登录 token 无效的认证文案 key。
	MsgKeyTokenInvalid = "auth.token_invalid"
	// MsgKeySessionExpired 表示服务端会话已失效的认证文案 key。
	MsgKeySessionExpired = "auth.session_expired"
	// MsgKeyInvalidPassword 表示账号或密码错误的认证文案 key。
	MsgKeyInvalidPassword = "auth.invalid_password"
	// MsgKeyLogoutSuccess 表示登出成功的认证文案 key。
	MsgKeyLogoutSuccess = "auth.logout_success"
	// MsgKeyRegisterDisabled 表示注册入口已关闭的认证文案 key。
	MsgKeyRegisterDisabled = "auth.register_disabled"
	// MsgKeySecurityAppIDInvalid 表示安全链路 AppID 无效的文案 key。
	MsgKeySecurityAppIDInvalid = "security.app_id_invalid"
	// MsgKeySecurityKeyUnavailable 表示安全链路秘钥不可用的文案 key。
	MsgKeySecurityKeyUnavailable = "security.key_unavailable"
	// MsgKeySecuritySignatureFailed 表示请求签名校验失败的文案 key。
	MsgKeySecuritySignatureFailed = "security.signature_failed"
	// MsgKeySecurityPayloadTooLarge 表示安全字段超过限制的文案 key。
	MsgKeySecurityPayloadTooLarge = "security.payload_too_large"
	// MsgKeySecurityCryptoDisabled 表示加解密链路未启用的文案 key。
	MsgKeySecurityCryptoDisabled = "security.crypto_disabled"
	// MsgKeySecurityRequestDecryptFailed 表示请求解密失败的文案 key。
	MsgKeySecurityRequestDecryptFailed = "security.request_decrypt_failed"
	// MsgKeySecurityResponseSignFailed 表示响应签名处理失败的文案 key。
	MsgKeySecurityResponseSignFailed = "security.response_sign_failed"
	// MsgKeySecurityResponseEncryptFailed 表示响应加密处理失败的文案 key。
	MsgKeySecurityResponseEncryptFailed = "security.response_encrypt_failed"

	// MsgKeyUserNotFound 表示用户不存在的文案 key。
	MsgKeyUserNotFound = "user.not_found"
	// MsgKeyUserAlreadyExists 表示用户已存在的文案 key。
	MsgKeyUserAlreadyExists = "user.already_exists"
	// MsgKeyUserDisabled 表示账号被禁用的文案 key。
	MsgKeyUserDisabled = "user.disabled"

	// MsgKeyDependencyUnavailable 表示核心依赖不可用的文案 key。
	MsgKeyDependencyUnavailable = "dependency.unavailable"
	// MsgKeyMySQLUnavailable 表示 MySQL 不可用的文案 key。
	MsgKeyMySQLUnavailable = "dependency.mysql_unavailable"
	// MsgKeyRedisUnavailable 表示 Redis 不可用的文案 key。
	MsgKeyRedisUnavailable = "dependency.redis_unavailable"
)
