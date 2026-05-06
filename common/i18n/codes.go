package i18n

import codes "api/common/codes"

// codeToMessageKey 维护业务响应码到多语言 key 的映射。
var codeToMessageKey = map[int]string{
	codes.Undefined:    MsgKeyUndefined,    // 未定义使用该多语言 key。
	codes.Success:      MsgKeySuccess,      // 成功使用该多语言 key。
	codes.Fail:         MsgKeyFail,         // 失败使用该多语言 key。
	codes.OK:           MsgKeyOK,           // 请求成功使用该多语言 key。
	codes.BadRequest:   MsgKeyBadRequest,   // 错误请求使用该多语言 key。
	codes.Unauthorized: MsgKeyUnauthorized, // 未授权使用该多语言 key。
	codes.Forbidden:    MsgKeyForbidden,    // 禁止访问使用该多语言 key。
	codes.NotFound:     MsgKeyNotFound,     // 未找到使用该多语言 key。
	codes.ServerError:  MsgKeyServerError,  // 服务器错误使用该多语言 key。
	codes.ServiceBusy:  MsgKeyServiceBusy,  // 服务繁忙使用该多语言 key。
	codes.Timeout:      MsgKeyTimeout,      // 请求超时使用该多语言 key。

	codes.ParamError:    MsgKeyParamError,    // 参数错误使用该多语言 key。
	codes.AuthFailed:    MsgKeyAuthFailed,    // 验证失败使用该多语言 key。
	codes.RateLimit:     MsgKeyRateLimit,     // 请求过多使用该多语言 key。
	codes.InternalError: MsgKeyInternalError, // 内部错误使用该多语言 key。
	codes.DBError:       MsgKeyDBError,       // 数据库错误使用该多语言 key。

	codes.CreateSuccess: MsgKeyCreateSuccess, // 创建成功使用该多语言 key。
	codes.SaveSuccess:   MsgKeySaveSuccess,   // 保存成功使用该多语言 key。
	codes.UpdateSuccess: MsgKeyUpdateSuccess, // 更新成功使用该多语言 key。
	codes.DeleteSuccess: MsgKeyDeleteSuccess, // 删除成功使用该多语言 key。
	codes.FetchSuccess:  MsgKeyFetchSuccess,  // 获取成功使用该多语言 key。

	codes.InvalidPassword:               MsgKeyInvalidPassword,               // 账号或密码错误使用该多语言 key。
	codes.TokenExpired:                  MsgKeyTokenExpired,                  // 访问令牌已过期使用该多语言 key。
	codes.TokenInvalid:                  MsgKeyTokenInvalid,                  // 访问令牌无效使用该多语言 key。
	codes.SessionExpired:                MsgKeySessionExpired,                // 服务端会话已失效使用该多语言 key。
	codes.RegisterDisabled:              MsgKeyRegisterDisabled,              // 当前站点未开放注册使用该多语言 key。
	codes.SecurityAppIDInvalid:          MsgKeySecurityAppIDInvalid,          // 安全链路 AppID 无效使用该多语言 key。
	codes.SecurityKeyUnavailable:        MsgKeySecurityKeyUnavailable,        // 安全链路秘钥不可用使用该多语言 key。
	codes.SecuritySignatureFailed:       MsgKeySecuritySignatureFailed,       // 请求签名校验失败使用该多语言 key。
	codes.SecurityPayloadTooLarge:       MsgKeySecurityPayloadTooLarge,       // 签名或加密字段超过安全链路上限使用该多语言 key。
	codes.SecurityCryptoDisabled:        MsgKeySecurityCryptoDisabled,        // 加解密链路未启用使用该多语言 key。
	codes.SecurityRequestDecryptFailed:  MsgKeySecurityRequestDecryptFailed,  // 请求解密失败使用该多语言 key。
	codes.SecurityResponseSignFailed:    MsgKeySecurityResponseSignFailed,    // 响应回签失败使用该多语言 key。
	codes.SecurityResponseEncryptFailed: MsgKeySecurityResponseEncryptFailed, // 响应加密失败使用该多语言 key。
	codes.UserNotFound:                  MsgKeyUserNotFound,                  // 用户不存在使用该多语言 key。
	codes.UserAlreadyExists:             MsgKeyUserAlreadyExists,             // 用户名已存在使用该多语言 key。
	codes.UserDisabled:                  MsgKeyUserDisabled,                  // 账号被禁用使用该多语言 key。

	codes.DependencyUnavailable: MsgKeyDependencyUnavailable, // ready 检查发现外部依赖不可用使用该多语言 key。
	codes.MySQLUnavailable:      MsgKeyMySQLUnavailable,      // MySQL 连接不可用使用该多语言 key。
	codes.RedisUnavailable:      MsgKeyRedisUnavailable,      // Redis 连接不可用使用该多语言 key。
}
