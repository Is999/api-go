package collectorx

import (
	"context"
	"encoding/json"
	"strings"
)

// 认证风控事件指标标签兜底值。
const (
	authSecurityLabelUnknown = "unknown" // 指标标签缺省值
	authSecurityLabelOther   = "other"   // 未知枚举统一归并，避免指标维度失控
)

// 认证风控事件指标分类，保持低基数便于告警聚合。
const (
	authSecurityCategoryAuth                 = "auth"                   // 账号登录与用户状态类
	authSecurityCategoryToken                = "token"                  // token 和 session 鉴权类
	authSecurityCategoryRateLimit            = "rate_limit"             // 认证限流类
	authSecurityCategorySecurityClient       = "security_client"        // 客户端签名、AppID 或加密声明异常
	authSecurityCategorySecurityConfig       = "security_config"        // 服务端秘钥或配置异常
	authSecurityCategorySecurityPayloadLimit = "security_payload_limit" // 安全字段体积或数量超限
	authSecurityCategorySecurityResponse     = "security_response"      // 响应签名或加密处理异常
	authSecurityCategorySessionLifecycle     = "session_lifecycle"      // session 创建、轮换或批量失效
)

// 认证风控事件动作枚举。
const (
	authSecurityActionRegisterSuccess      = "register_success"       // 注册成功
	authSecurityActionLoginSuccess         = "login_success"          // 登录成功
	authSecurityActionLoginFailed          = "login_failed"           // 登录失败
	authSecurityActionRateLimited          = "rate_limited"           // 认证入口触发限流
	authSecurityActionAuthFailed           = "auth_failed"            // 登录态鉴权失败
	authSecurityActionSecurityFailed       = "security_failed"        // 签名或加密链路失败
	authSecurityActionRefreshSuccess       = "refresh_success"        // 刷新 token 成功
	authSecurityActionLogoutSuccess        = "logout_success"         // 退出登录成功
	authSecurityActionSessionInvalidateAll = "session_invalidate_all" // 用户全部 session 失效
)

// 认证风控事件原因枚举。
const (
	authSecurityReasonInvalidPassword          = "invalid_password"            // 账号或密码错误
	authSecurityReasonUserDisabled             = "user_disabled"               // 用户被禁用
	authSecurityReasonUserNotFound             = "user_not_found"              // 用户不存在
	authSecurityReasonMissingBearer            = "missing_bearer"              // 缺少 Bearer token
	authSecurityReasonTokenExpired             = "token_expired"               // token 已过期
	authSecurityReasonSessionExpired           = "session_expired"             // Redis session 已失效
	authSecurityReasonTokenInvalid             = "token_invalid"               // token 无效
	authSecurityReasonSecurityFailed           = "security_failed"             // 签名或加密链路失败
	authSecurityReasonSecurityAppIDInvalid     = "security_app_id_invalid"     // 安全链路 AppID 无效
	authSecurityReasonSecurityKeyUnavailable   = "security_key_unavailable"    // 安全链路秘钥不可用
	authSecurityReasonSignatureFailed          = "signature_failed"            // 请求验签失败
	authSecurityReasonSecurityPayloadTooLarge  = "security_payload_too_large"  // 安全字段或请求体超过上限
	authSecurityReasonResponseSignFailed       = "response_sign_failed"        // 响应回签失败
	authSecurityReasonCryptoDisabled           = "crypto_disabled"             // 加解密链路关闭
	authSecurityReasonRequestDecryptFailed     = "request_decrypt_failed"      // 请求解密失败
	authSecurityReasonResponseEncryptFailed    = "response_encrypt_failed"     // 响应加密失败
	authSecurityReasonLoginIPRateLimited       = "login_ip_rate_limited"       // 登录 IP 限流
	authSecurityReasonLoginUsernameRateLimited = "login_username_rate_limited" // 登录用户名限流
	authSecurityReasonRegisterIPRateLimited    = "register_ip_rate_limited"    // 注册 IP 限流
	authSecurityReasonSessionCreated           = "session_created"             // 新会话已创建
	authSecurityReasonSessionRotated           = "session_rotated"             // 会话已轮换
	authSecurityReasonCurrentSessionDeleted    = "current_session_deleted"     // 当前会话已删除
	authSecurityReasonUserSessionsInvalidated  = "user_sessions_invalidated"   // 用户会话已全部失效
)

// 认证风控事件指标标签白名单，未知值会归并为 other。
var (
	// authSecurityActions 是认证事件动作指标标签白名单。
	authSecurityActions = map[string]struct{}{
		authSecurityActionRegisterSuccess:      {},
		authSecurityActionLoginSuccess:         {},
		authSecurityActionLoginFailed:          {},
		authSecurityActionRateLimited:          {},
		authSecurityActionAuthFailed:           {},
		authSecurityActionSecurityFailed:       {},
		authSecurityActionRefreshSuccess:       {},
		authSecurityActionLogoutSuccess:        {},
		authSecurityActionSessionInvalidateAll: {},
	}
	// authSecurityReasons 是认证事件原因指标标签白名单。
	authSecurityReasons = map[string]struct{}{
		authSecurityReasonInvalidPassword:          {},
		authSecurityReasonUserDisabled:             {},
		authSecurityReasonUserNotFound:             {},
		authSecurityReasonMissingBearer:            {},
		authSecurityReasonTokenExpired:             {},
		authSecurityReasonSessionExpired:           {},
		authSecurityReasonTokenInvalid:             {},
		authSecurityReasonSecurityFailed:           {},
		authSecurityReasonSecurityAppIDInvalid:     {},
		authSecurityReasonSecurityKeyUnavailable:   {},
		authSecurityReasonSignatureFailed:          {},
		authSecurityReasonSecurityPayloadTooLarge:  {},
		authSecurityReasonResponseSignFailed:       {},
		authSecurityReasonCryptoDisabled:           {},
		authSecurityReasonRequestDecryptFailed:     {},
		authSecurityReasonResponseEncryptFailed:    {},
		authSecurityReasonLoginIPRateLimited:       {},
		authSecurityReasonLoginUsernameRateLimited: {},
		authSecurityReasonRegisterIPRateLimited:    {},
		authSecurityReasonSessionCreated:           {},
		authSecurityReasonSessionRotated:           {},
		authSecurityReasonCurrentSessionDeleted:    {},
		authSecurityReasonUserSessionsInvalidated:  {},
	}
)

// AuthSecurityProcessor 汇总认证风控事件的轻量指标。
type AuthSecurityProcessor struct{}

// NewAuthSecurityProcessor 创建认证风控事件 Processor。
func NewAuthSecurityProcessor() *AuthSecurityProcessor {
	return &AuthSecurityProcessor{}
}

// RegisterDefaultProcessors 注册项目前台 API 内置轻量 Processor。
func RegisterDefaultProcessors(manager *Manager) error {
	if manager == nil {
		return nil
	}
	return manager.RegisterProcessor(BizTypeAuthSecurity, NewAuthSecurityProcessor())
}

// authSecurityPayload 是认证风控事件 Processor 关心的字段子集。
type authSecurityPayload struct {
	Action string `json:"action"` // 事件动作
	Reason string `json:"reason"` // 事件原因
	AppID  string `json:"app_id"` // 站点命名空间
}

// ProcessBatch 解析认证风控事件并记录聚合指标。
func (p *AuthSecurityProcessor) ProcessBatch(ctx context.Context, events []Event) ([]ProcessResult, error) {
	results := make([]ProcessResult, 0, len(events))
	for _, event := range events {
		result := ProcessResult{EventID: event.EventID}
		if strings.TrimSpace(event.BizType) != BizTypeAuthSecurity {
			result.Error = "biz_type 不匹配"
			results = append(results, result)
			continue
		}
		payload := authSecurityPayload{}
		if err := json.Unmarshal(event.Payload, &payload); err != nil {
			result.Error = "payload 解析失败"
			results = append(results, result)
			continue
		}
		recordAuthSecurityEvent(payload.AppID, payload.Action, payload.Reason)
		result.Success = true
		results = append(results, result)
	}
	return results, nil
}

// normalizeAuthSecurityAction 归一化认证事件动作标签。
func normalizeAuthSecurityAction(action string) string {
	action = strings.TrimSpace(action)
	if action == "" {
		return authSecurityLabelUnknown
	}
	if _, ok := authSecurityActions[action]; ok {
		return action
	}
	return authSecurityLabelOther
}

// normalizeAuthSecurityReason 归一化认证事件原因标签。
func normalizeAuthSecurityReason(reason string) string {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return authSecurityLabelUnknown
	}
	if _, ok := authSecurityReasons[reason]; ok {
		return reason
	}
	return authSecurityLabelOther
}

// normalizeAuthSecurityCategory 将细分原因归并为低基数告警分类。
func normalizeAuthSecurityCategory(reason string) string {
	switch normalizeAuthSecurityReason(reason) {
	case authSecurityReasonInvalidPassword,
		authSecurityReasonUserDisabled,
		authSecurityReasonUserNotFound:
		return authSecurityCategoryAuth
	case authSecurityReasonMissingBearer,
		authSecurityReasonTokenExpired,
		authSecurityReasonSessionExpired,
		authSecurityReasonTokenInvalid:
		return authSecurityCategoryToken
	case authSecurityReasonLoginIPRateLimited,
		authSecurityReasonLoginUsernameRateLimited,
		authSecurityReasonRegisterIPRateLimited:
		return authSecurityCategoryRateLimit
	case authSecurityReasonSecurityFailed,
		authSecurityReasonSecurityAppIDInvalid,
		authSecurityReasonSignatureFailed,
		authSecurityReasonCryptoDisabled,
		authSecurityReasonRequestDecryptFailed:
		return authSecurityCategorySecurityClient
	case authSecurityReasonSecurityKeyUnavailable:
		return authSecurityCategorySecurityConfig
	case authSecurityReasonSecurityPayloadTooLarge:
		return authSecurityCategorySecurityPayloadLimit
	case authSecurityReasonResponseSignFailed,
		authSecurityReasonResponseEncryptFailed:
		return authSecurityCategorySecurityResponse
	case authSecurityReasonSessionCreated,
		authSecurityReasonSessionRotated,
		authSecurityReasonCurrentSessionDeleted,
		authSecurityReasonUserSessionsInvalidated:
		return authSecurityCategorySessionLifecycle
	case authSecurityLabelUnknown:
		return authSecurityLabelUnknown
	default:
		return authSecurityLabelOther
	}
}

// normalizeAuthSecurityAppID 归一化站点标签，避免异常值扩大指标维度。
func normalizeAuthSecurityAppID(appID string) string {
	appID = strings.TrimSpace(appID)
	if appID == "" {
		return authSecurityLabelUnknown
	}
	if len(appID) > 64 {
		return authSecurityLabelOther
	}
	for _, r := range appID {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' || r == '-' || r == '.' {
			continue
		}
		return authSecurityLabelOther
	}
	return appID
}
