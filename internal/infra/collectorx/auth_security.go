package collectorx

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/Is999/go-utils/errors"
)

// 默认 Processor 注册名称。
const (
	defaultProcessorAuthSecurity = "auth_security_processor" // 认证风控事件默认 Processor
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
	AuthSecurityReasonLoginUsernameRateLimited = "login_username_rate_limited" // 登录用户名限流
	AuthSecurityReasonRegisterIPRateLimited    = "register_ip_rate_limited"    // 注册 IP 限流
	AuthSecurityReasonSessionCreated           = "session_created"             // 新会话已创建
	AuthSecurityReasonSessionRotated           = "session_rotated"             // 会话已轮换
	AuthSecurityReasonCurrentSessionDeleted    = "current_session_deleted"     // 当前会话已删除
	AuthSecurityReasonUserSessionsInvalidated  = "user_sessions_invalidated"   // 用户会话已全部失效
)

// AuthSecurityReasonContract 描述认证风控原因到低基数告警分类的契约。
type AuthSecurityReasonContract struct {
	Reason   string // Reason 是允许作为指标标签暴露的细分原因。
	Category string // Category 是该原因归并后的低基数告警分类。
}

// defaultAuthSecurityActions 是认证事件动作指标标签契约源，元素为允许暴露的 action 值。
var defaultAuthSecurityActions = []string{
	AuthSecurityActionRegisterSuccess,      // 注册成功允许作为动作指标标签。
	AuthSecurityActionLoginSuccess,         // 登录成功允许作为动作指标标签。
	AuthSecurityActionLoginFailed,          // 登录失败允许作为动作指标标签。
	AuthSecurityActionRateLimited,          // 认证入口触发限流允许作为动作指标标签。
	AuthSecurityActionAuthFailed,           // 登录态鉴权失败允许作为动作指标标签。
	AuthSecurityActionSecurityFailed,       // 签名或加密链路失败允许作为动作指标标签。
	AuthSecurityActionRefreshSuccess,       // 刷新 token 成功允许作为动作指标标签。
	AuthSecurityActionLogoutSuccess,        // 退出登录成功允许作为动作指标标签。
	AuthSecurityActionSessionInvalidateAll, // 用户全部 session 失效允许作为动作指标标签。
}

// defaultAuthSecurityReasonContracts 是认证事件原因指标标签契约源，元素声明 reason 与告警分类。
var defaultAuthSecurityReasonContracts = []AuthSecurityReasonContract{
	{Reason: AuthSecurityReasonInvalidPassword, Category: authSecurityCategoryAuth},                         // 账号或密码错误归并到账号认证类。
	{Reason: AuthSecurityReasonUserDisabled, Category: authSecurityCategoryAuth},                            // 用户被禁用归并到账号认证类。
	{Reason: AuthSecurityReasonUserNotFound, Category: authSecurityCategoryAuth},                            // 用户不存在归并到账号认证类。
	{Reason: AuthSecurityReasonMissingBearer, Category: authSecurityCategoryToken},                          // 缺少 Bearer token 归并到登录态类。
	{Reason: AuthSecurityReasonTokenExpired, Category: authSecurityCategoryToken},                           // token 过期归并到登录态类。
	{Reason: AuthSecurityReasonSessionExpired, Category: authSecurityCategoryToken},                         // Redis session 失效归并到登录态类。
	{Reason: AuthSecurityReasonTokenInvalid, Category: authSecurityCategoryToken},                           // token 无效归并到登录态类。
	{Reason: AuthSecurityReasonSecurityFailed, Category: authSecurityCategorySecurityClient},                // 通用安全链路失败归并到客户端安全类。
	{Reason: AuthSecurityReasonSecurityAppIDInvalid, Category: authSecurityCategorySecurityClient},          // AppID 无效归并到客户端安全类。
	{Reason: AuthSecurityReasonSecurityKeyUnavailable, Category: authSecurityCategorySecurityConfig},        // 秘钥不可用归并到服务端配置类。
	{Reason: AuthSecurityReasonSignatureFailed, Category: authSecurityCategorySecurityClient},               // 请求验签失败归并到客户端安全类。
	{Reason: AuthSecurityReasonSecurityPayloadTooLarge, Category: authSecurityCategorySecurityPayloadLimit}, // 安全字段超限归并到体积限制类。
	{Reason: AuthSecurityReasonResponseSignFailed, Category: authSecurityCategorySecurityResponse},          // 响应签名失败归并到响应安全类。
	{Reason: AuthSecurityReasonCryptoDisabled, Category: authSecurityCategorySecurityClient},                // 加解密关闭归并到客户端安全类。
	{Reason: AuthSecurityReasonRequestDecryptFailed, Category: authSecurityCategorySecurityClient},          // 请求解密失败归并到客户端安全类。
	{Reason: AuthSecurityReasonResponseEncryptFailed, Category: authSecurityCategorySecurityResponse},       // 响应加密失败归并到响应安全类。
	{Reason: AuthSecurityReasonLoginIPRateLimited, Category: authSecurityCategoryRateLimit},                 // 登录 IP 限流归并到限流类。
	{Reason: AuthSecurityReasonLoginUsernameRateLimited, Category: authSecurityCategoryRateLimit},           // 登录用户名限流归并到限流类。
	{Reason: AuthSecurityReasonRegisterIPRateLimited, Category: authSecurityCategoryRateLimit},              // 注册 IP 限流归并到限流类。
	{Reason: AuthSecurityReasonSessionCreated, Category: authSecurityCategorySessionLifecycle},              // 新会话创建归并到 session 生命周期类。
	{Reason: AuthSecurityReasonSessionRotated, Category: authSecurityCategorySessionLifecycle},              // 会话轮换归并到 session 生命周期类。
	{Reason: AuthSecurityReasonCurrentSessionDeleted, Category: authSecurityCategorySessionLifecycle},       // 当前会话删除归并到 session 生命周期类。
	{Reason: AuthSecurityReasonUserSessionsInvalidated, Category: authSecurityCategorySessionLifecycle},     // 用户会话批量失效归并到 session 生命周期类。
}

// 认证风控事件指标标签索引，未知值会归并为 other。
var (
	// authSecurityActions 由 defaultAuthSecurityActions 派生，key 是允许暴露的 action 标签值。
	authSecurityActions = buildStringSet(defaultAuthSecurityActions)
	// authSecurityReasons 由 defaultAuthSecurityReasonContracts 派生，key 是允许暴露的 reason 标签值。
	authSecurityReasons = buildAuthSecurityReasonSet(defaultAuthSecurityReasonContracts)
	// authSecurityReasonCategories 由 defaultAuthSecurityReasonContracts 派生，key 为 reason，value 为低基数 category。
	authSecurityReasonCategories = buildAuthSecurityReasonCategoryMap(defaultAuthSecurityReasonContracts)
)

// defaultProcessorSpecs 是内置 Processor 规格源，元素按启动注册顺序排列。
var defaultProcessorSpecs = []DefaultProcessorSpec{
	{
		Name:        defaultProcessorAuthSecurity,
		BizType:     BizTypeAuthSecurity,
		File:        "internal/infra/collectorx/auth_security.go",
		Method:      "collectorx.DefaultProcessorSpecs / RegisterDefaultProcessors / NewAuthSecurityProcessor",
		Description: "默认汇总 auth.security 认证风控事件指标",
		Build: func() Processor {
			return NewAuthSecurityProcessor()
		},
	},
}

// DefaultProcessorSpecs 返回项目前台 API 内置 Processor 规格快照。
func DefaultProcessorSpecs() []DefaultProcessorSpec {
	return append([]DefaultProcessorSpec(nil), defaultProcessorSpecs...)
}

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
	for _, spec := range defaultProcessorSpecs {
		if spec.Build == nil {
			return errors.Errorf("默认 Collector Processor 构造器为空 name=%s biz_type=%s", spec.Name, spec.BizType)
		}
		if err := manager.RegisterProcessor(spec.BizType, spec.Build()); err != nil {
			return errors.Wrapf(err, "注册默认 Collector Processor 失败 name=%s biz_type=%s", spec.Name, spec.BizType)
		}
	}
	return nil
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
	normalizedReason := normalizeAuthSecurityReason(reason)
	if normalizedReason == authSecurityLabelUnknown {
		return authSecurityLabelUnknown
	}
	if category, ok := authSecurityReasonCategories[normalizedReason]; ok {
		return category
	}
	return authSecurityLabelOther
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

// buildStringSet 构造字符串白名单集合，空字符串会被忽略。
func buildStringSet(values []string) map[string]struct{} {
	result := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		result[value] = struct{}{}
	}
	return result
}

// buildAuthSecurityReasonSet 从原因契约派生原因白名单集合。
func buildAuthSecurityReasonSet(contracts []AuthSecurityReasonContract) map[string]struct{} {
	result := make(map[string]struct{}, len(contracts))
	for _, contract := range contracts {
		reason := strings.TrimSpace(contract.Reason)
		if reason == "" {
			continue
		}
		result[reason] = struct{}{}
	}
	return result
}

// buildAuthSecurityReasonCategoryMap 从原因契约派生 reason 到 category 的映射。
func buildAuthSecurityReasonCategoryMap(contracts []AuthSecurityReasonContract) map[string]string {
	result := make(map[string]string, len(contracts))
	for _, contract := range contracts {
		reason := strings.TrimSpace(contract.Reason)
		category := strings.TrimSpace(contract.Category)
		if reason == "" || category == "" {
			continue
		}
		result[reason] = category
	}
	return result
}
