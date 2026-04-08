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
	Success:       {},
	OK:            {},
	CreateSuccess: {},
	SaveSuccess:   {},
	UpdateSuccess: {},
	DeleteSuccess: {},
	FetchSuccess:  {},
}

// codeHTTPStatusMap 定义业务码到 HTTP 状态码的建议映射。
var codeHTTPStatusMap = map[int]int{
	BadRequest:                    BadRequest,
	Unauthorized:                  Unauthorized,
	Forbidden:                     Forbidden,
	NotFound:                      NotFound,
	ServerError:                   ServerError,
	ServiceBusy:                   ServiceBusy,
	Timeout:                       Timeout,
	ParamError:                    BadRequest,
	AuthFailed:                    Unauthorized,
	RateLimit:                     429,
	InternalError:                 ServerError,
	DBError:                       ServerError,
	InvalidPassword:               BadRequest,
	TokenExpired:                  Unauthorized,
	TokenInvalid:                  Unauthorized,
	SessionExpired:                Unauthorized,
	RegisterDisabled:              Forbidden,
	SecurityAppIDInvalid:          BadRequest,
	SecurityKeyUnavailable:        ServerError,
	SecuritySignatureFailed:       Unauthorized,
	SecurityPayloadTooLarge:       413,
	SecurityCryptoDisabled:        Forbidden,
	SecurityRequestDecryptFailed:  Unauthorized,
	SecurityResponseSignFailed:    ServerError,
	SecurityResponseEncryptFailed: ServerError,
	UserNotFound:                  NotFound,
	UserAlreadyExists:             BadRequest,
	UserDisabled:                  Unauthorized,
	DependencyUnavailable:         ServiceBusy,
	MySQLUnavailable:              ServiceBusy,
	RedisUnavailable:              ServiceBusy,
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
