package i18n

import codes "api/common/codes"

// codeToMessageKey 维护业务响应码到多语言 key 的映射。
var codeToMessageKey = map[int]string{
	codes.Undefined:    MsgKeyUndefined,
	codes.Success:      MsgKeySuccess,
	codes.Fail:         MsgKeyFail,
	codes.OK:           MsgKeyOK,
	codes.BadRequest:   MsgKeyBadRequest,
	codes.Unauthorized: MsgKeyUnauthorized,
	codes.Forbidden:    MsgKeyForbidden,
	codes.NotFound:     MsgKeyNotFound,
	codes.ServerError:  MsgKeyServerError,
	codes.ServiceBusy:  MsgKeyServiceBusy,
	codes.Timeout:      MsgKeyTimeout,

	codes.ParamError:    MsgKeyParamError,
	codes.AuthFailed:    MsgKeyAuthFailed,
	codes.RateLimit:     MsgKeyRateLimit,
	codes.InternalError: MsgKeyInternalError,
	codes.DBError:       MsgKeyDBError,

	codes.CreateSuccess: MsgKeyCreateSuccess,
	codes.SaveSuccess:   MsgKeySaveSuccess,
	codes.UpdateSuccess: MsgKeyUpdateSuccess,
	codes.DeleteSuccess: MsgKeyDeleteSuccess,
	codes.FetchSuccess:  MsgKeyFetchSuccess,

	codes.InvalidPassword:               MsgKeyInvalidPassword,
	codes.TokenExpired:                  MsgKeyTokenExpired,
	codes.TokenInvalid:                  MsgKeyTokenInvalid,
	codes.SessionExpired:                MsgKeySessionExpired,
	codes.RegisterDisabled:              MsgKeyRegisterDisabled,
	codes.SecurityAppIDInvalid:          MsgKeySecurityAppIDInvalid,
	codes.SecurityKeyUnavailable:        MsgKeySecurityKeyUnavailable,
	codes.SecuritySignatureFailed:       MsgKeySecuritySignatureFailed,
	codes.SecurityPayloadTooLarge:       MsgKeySecurityPayloadTooLarge,
	codes.SecurityCryptoDisabled:        MsgKeySecurityCryptoDisabled,
	codes.SecurityRequestDecryptFailed:  MsgKeySecurityRequestDecryptFailed,
	codes.SecurityResponseSignFailed:    MsgKeySecurityResponseSignFailed,
	codes.SecurityResponseEncryptFailed: MsgKeySecurityResponseEncryptFailed,
	codes.UserNotFound:                  MsgKeyUserNotFound,
	codes.UserAlreadyExists:             MsgKeyUserAlreadyExists,
	codes.UserDisabled:                  MsgKeyUserDisabled,

	codes.DependencyUnavailable: MsgKeyDependencyUnavailable,
	codes.MySQLUnavailable:      MsgKeyMySQLUnavailable,
	codes.RedisUnavailable:      MsgKeyRedisUnavailable,
}
