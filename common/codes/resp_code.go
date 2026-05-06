package codes

// 通用响应码，0-1999 保留给跨模块基础语义。
const (
	// 通用响应码 0 - 99 为通用状态码。
	Undefined = 0 // 未定义
	Success   = 1 // 成功
	Fail      = 2 // 失败

	// 通用响应码 200 - 599 对齐 HTTP 语义。
	OK           = 200 // 请求成功
	BadRequest   = 400 // 错误请求
	Unauthorized = 401 // 未授权
	Forbidden    = 403 // 禁止访问
	NotFound     = 404 // 未找到
	ServerError  = 500 // 服务器错误
	ServiceBusy  = 503 // 服务繁忙
	Timeout      = 504 // 请求超时

	// 通用业务响应码 1000 - 1999。
	ParamError    = 1001 // 参数错误
	AuthFailed    = 1002 // 验证失败
	RateLimit     = 1003 // 请求过多
	InternalError = 1004 // 内部错误
	DBError       = 1005 // 数据库错误

	CreateSuccess = 1100 // 创建成功
	CreateFail    = 1101 // 创建失败
	SaveSuccess   = 1104 // 保存成功
	SaveFail      = 1105 // 保存失败
	UpdateSuccess = 1106 // 更新成功
	UpdateFail    = 1107 // 更新失败
	DeleteSuccess = 1108 // 删除成功
	DeleteFail    = 1109 // 删除失败
	FetchSuccess  = 1110 // 获取成功
	FetchFail     = 1111 // 获取失败
)

// 业务码号段常量，按前台领域和外部依赖分段。
const (
	// CodeAuthBase 表示前台认证业务码号段起点，范围 20000-20999。
	CodeAuthBase = 20000
	// CodeUserBase 表示前台用户业务码号段起点，范围 21000-21999。
	CodeUserBase = 21000
	// CodeDependencyBase 表示外部依赖业务码号段起点，范围 50000-50999。
	CodeDependencyBase = 50000
)

// 前台认证和用户领域业务码。
const (
	// InvalidPassword 表示账号或密码错误。
	InvalidPassword = CodeAuthBase + 1
	// TokenExpired 表示访问令牌已过期。
	TokenExpired = CodeAuthBase + 2
	// TokenInvalid 表示访问令牌无效。
	TokenInvalid = CodeAuthBase + 3
	// SessionExpired 表示服务端会话已失效。
	SessionExpired = CodeAuthBase + 4
	// RegisterDisabled 表示当前站点未开放注册。
	RegisterDisabled = CodeAuthBase + 5
	// SecurityAppIDInvalid 表示安全链路 AppID 无效。
	SecurityAppIDInvalid = CodeAuthBase + 6
	// SecurityKeyUnavailable 表示安全链路秘钥不可用。
	SecurityKeyUnavailable = CodeAuthBase + 7
	// SecuritySignatureFailed 表示请求签名校验失败。
	SecuritySignatureFailed = CodeAuthBase + 8
	// SecurityPayloadTooLarge 表示签名或加密字段超过安全链路上限。
	SecurityPayloadTooLarge = CodeAuthBase + 9
	// SecurityCryptoDisabled 表示加解密链路未启用。
	SecurityCryptoDisabled = CodeAuthBase + 10
	// SecurityRequestDecryptFailed 表示请求解密失败。
	SecurityRequestDecryptFailed = CodeAuthBase + 11
	// SecurityResponseSignFailed 表示响应回签失败。
	SecurityResponseSignFailed = CodeAuthBase + 12
	// SecurityResponseEncryptFailed 表示响应加密失败。
	SecurityResponseEncryptFailed = CodeAuthBase + 13

	// UserNotFound 表示用户不存在。
	UserNotFound = CodeUserBase + 1
	// UserAlreadyExists 表示用户名已存在。
	UserAlreadyExists = CodeUserBase + 2
	// UserDisabled 表示账号被禁用。
	UserDisabled = CodeUserBase + 3
)

// 健康检查依赖业务码。
const (
	// DependencyUnavailable 表示 ready 检查发现外部依赖不可用。
	DependencyUnavailable = CodeDependencyBase + 1
	// MySQLUnavailable 表示 MySQL 连接不可用。
	MySQLUnavailable = CodeDependencyBase + 2
	// RedisUnavailable 表示 Redis 连接不可用。
	RedisUnavailable = CodeDependencyBase + 3
)

// successCodeSet 定义统一响应可识别为成功的业务码集合。
var successCodeSet = map[int]struct{}{
	Success:       {}, // 成功纳入成功码集合。
	OK:            {}, // 请求成功纳入成功码集合。
	CreateSuccess: {}, // 创建成功纳入成功码集合。
	SaveSuccess:   {}, // 保存成功纳入成功码集合。
	UpdateSuccess: {}, // 更新成功纳入成功码集合。
	DeleteSuccess: {}, // 删除成功纳入成功码集合。
	FetchSuccess:  {}, // 获取成功纳入成功码集合。
}

// codeHTTPStatusMap 定义业务码到 HTTP 状态码的建议映射。
var codeHTTPStatusMap = map[int]int{
	BadRequest:                    BadRequest,   // 错误请求返回 HTTP 错误请求。
	Unauthorized:                  Unauthorized, // 未授权返回 HTTP 未授权。
	Forbidden:                     Forbidden,    // 禁止访问返回 HTTP 禁止访问。
	NotFound:                      NotFound,     // 未找到返回 HTTP 未找到。
	ServerError:                   ServerError,  // 服务器错误返回 HTTP 服务器错误。
	ServiceBusy:                   ServiceBusy,  // 服务繁忙返回 HTTP 服务繁忙。
	Timeout:                       Timeout,      // 请求超时返回 HTTP 请求超时。
	ParamError:                    BadRequest,   // 参数错误返回 HTTP 错误请求。
	AuthFailed:                    Unauthorized, // 验证失败返回 HTTP 未授权。
	RateLimit:                     429,          // 请求过多返回 HTTP 429。
	InternalError:                 ServerError,  // 内部错误返回 HTTP 服务器错误。
	DBError:                       ServerError,  // 数据库错误返回 HTTP 服务器错误。
	InvalidPassword:               BadRequest,   // 账号或密码错误返回 HTTP 错误请求。
	TokenExpired:                  Unauthorized, // 访问令牌已过期返回 HTTP 未授权。
	TokenInvalid:                  Unauthorized, // 访问令牌无效返回 HTTP 未授权。
	SessionExpired:                Unauthorized, // 服务端会话已失效返回 HTTP 未授权。
	RegisterDisabled:              Forbidden,    // 当前站点未开放注册返回 HTTP 禁止访问。
	SecurityAppIDInvalid:          BadRequest,   // 安全链路 AppID 无效返回 HTTP 错误请求。
	SecurityKeyUnavailable:        ServerError,  // 安全链路秘钥不可用返回 HTTP 服务器错误。
	SecuritySignatureFailed:       Unauthorized, // 请求签名校验失败返回 HTTP 未授权。
	SecurityPayloadTooLarge:       413,          // 签名或加密字段超过安全链路上限返回 HTTP 413。
	SecurityCryptoDisabled:        Forbidden,    // 加解密链路未启用返回 HTTP 禁止访问。
	SecurityRequestDecryptFailed:  Unauthorized, // 请求解密失败返回 HTTP 未授权。
	SecurityResponseSignFailed:    ServerError,  // 响应回签失败返回 HTTP 服务器错误。
	SecurityResponseEncryptFailed: ServerError,  // 响应加密失败返回 HTTP 服务器错误。
	UserNotFound:                  NotFound,     // 用户不存在返回 HTTP 未找到。
	UserAlreadyExists:             BadRequest,   // 用户名已存在返回 HTTP 错误请求。
	UserDisabled:                  Unauthorized, // 账号被禁用返回 HTTP 未授权。
	DependencyUnavailable:         ServiceBusy,  // ready 检查发现外部依赖不可用返回 HTTP 服务繁忙。
	MySQLUnavailable:              ServiceBusy,  // MySQL 连接不可用返回 HTTP 服务繁忙。
	RedisUnavailable:              ServiceBusy,  // Redis 连接不可用返回 HTTP 服务繁忙。
}

// IsSuccess 判断业务码是否代表成功结果。
func IsSuccess(code int) bool {
	_, ok := successCodeSet[code]
	return ok
}

// HTTPStatus 根据业务码返回建议 HTTP 状态码，未知成功码返回 200，未知失败码返回 500。
func HTTPStatus(code int) int {
	if status, ok := codeHTTPStatusMap[code]; ok {
		return status
	}
	if IsSuccess(code) {
		return OK
	}
	return ServerError
}
