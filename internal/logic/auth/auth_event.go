package auth

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"api/internal/config"
	"api/internal/infra/collectorx"
	"api/internal/requestctx"
	"api/internal/svc"
)

const (
	// AuthCollectorBizType 表示认证风控事件的 Collector bizType。
	AuthCollectorBizType = collectorx.BizTypeAuthSecurity
)

// 认证风控事件动作。
const (
	AuthEventActionRegisterSuccess      = "register_success"       // 注册成功
	AuthEventActionLoginSuccess         = "login_success"          // 登录成功
	AuthEventActionLoginFailed          = "login_failed"           // 登录失败
	AuthEventActionRateLimited          = "rate_limited"           // 认证入口触发限流
	AuthEventActionAuthFailed           = "auth_failed"            // 登录态鉴权失败
	AuthEventActionSecurityFailed       = "security_failed"        // 签名或加密链路失败
	AuthEventActionRefreshSuccess       = "refresh_success"        // 刷新 token 成功
	AuthEventActionLogoutSuccess        = "logout_success"         // 退出登录成功
	AuthEventActionSessionInvalidateAll = "session_invalidate_all" // 用户全部 session 失效
)

// 认证风控事件原因。
const (
	AuthEventReasonInvalidPassword          = "invalid_password"            // 账号或密码错误
	AuthEventReasonUserDisabled             = "user_disabled"               // 用户被禁用
	AuthEventReasonUserNotFound             = "user_not_found"              // 用户不存在
	AuthEventReasonMissingBearer            = "missing_bearer"              // 缺少 Bearer token
	AuthEventReasonTokenExpired             = "token_expired"               // token 已过期
	AuthEventReasonSessionExpired           = "session_expired"             // Redis session 已失效
	AuthEventReasonTokenInvalid             = "token_invalid"               // token 无效
	AuthEventReasonSecurityFailed           = "security_failed"             // 签名或加密链路失败
	AuthEventReasonSecurityAppIDInvalid     = "security_app_id_invalid"     // 安全链路 AppID 无效
	AuthEventReasonSecurityKeyUnavailable   = "security_key_unavailable"    // 安全链路秘钥不可用
	AuthEventReasonSignatureFailed          = "signature_failed"            // 请求验签失败
	AuthEventReasonSecurityPayloadTooLarge  = "security_payload_too_large"  // 安全字段或请求体超过上限
	AuthEventReasonResponseSignFailed       = "response_sign_failed"        // 响应回签失败
	AuthEventReasonCryptoDisabled           = "crypto_disabled"             // 加解密链路关闭
	AuthEventReasonRequestDecryptFailed     = "request_decrypt_failed"      // 请求解密失败
	AuthEventReasonResponseEncryptFailed    = "response_encrypt_failed"     // 响应加密失败
	AuthEventReasonLoginIPRateLimited       = "login_ip_rate_limited"       // 登录 IP 限流
	AuthEventReasonLoginUsernameRateLimited = "login_username_rate_limited" // 登录用户名限流
	AuthEventReasonRegisterIPRateLimited    = "register_ip_rate_limited"    // 注册 IP 限流
	AuthEventReasonSessionCreated           = "session_created"             // 新会话已创建
	AuthEventReasonSessionRotated           = "session_rotated"             // 会话已轮换
	AuthEventReasonCurrentSessionDeleted    = "current_session_deleted"     // 当前会话已删除
	AuthEventReasonUserSessionsInvalidated  = "user_sessions_invalidated"   // 用户会话已全部失效
)

// AuthEventInput 表示认证流程内待投递的轻量风控事件。
type AuthEventInput struct {
	Action   string // 事件动作
	UserID   int64  // 用户 ID，未知时为 0
	Username string // 用户名，仅用于生成脱敏哈希
	ClientIP string // 客户端 IP，仅用于生成脱敏哈希
	JTI      string // JWT ID，仅用于生成脱敏哈希
	Reason   string // 事件原因
	Count    int    // 批量操作影响数量
}

// authEventPayload 表示写入 Collector 的脱敏认证事件负载。
type authEventPayload struct {
	Action         string `json:"action"`                   // 事件动作
	UserID         int64  `json:"user_id,omitempty"`        // 用户 ID
	UsernameHash   string `json:"username_hash,omitempty"`  // 用户名 HMAC 哈希
	ClientIPHash   string `json:"client_ip_hash,omitempty"` // 客户端 IP HMAC 哈希
	SessionHash    string `json:"session_hash,omitempty"`   // jti HMAC 哈希
	AppID          string `json:"app_id"`                   // 当前站点命名空间
	Route          string `json:"route,omitempty"`          // 路由别名
	TraceID        string `json:"trace_id,omitempty"`       // 链路追踪 ID
	SpanID         string `json:"span_id,omitempty"`        // 当前服务 span ID
	Node           string `json:"node,omitempty"`           // 当前服务节点
	Mode           string `json:"mode,omitempty"`           // 当前运行模式
	Reason         string `json:"reason,omitempty"`         // 事件原因
	Count          int    `json:"count,omitempty"`          // 批量影响数量
	OccurredAtUnix int64  `json:"occurred_at_unix"`         // 事件发生时间，Unix 秒
}

// emitAuthEvent 投递认证风控事件；Collector 不可用时不影响主业务流程。
func (l *AuthLogic) emitAuthEvent(input AuthEventInput) {
	if l == nil {
		return
	}
	RecordAuthEvent(l.Ctx, l.Svc, input)
}

// RecordAuthEvent 将认证风控事件写入轻量 Collector。
func RecordAuthEvent(ctx context.Context, svcCtx *svc.ServiceContext, input AuthEventInput) {
	if svcCtx == nil || svcCtx.Collector == nil || strings.TrimSpace(input.Action) == "" {
		return
	}
	cfg := svcCtx.CurrentConfig()
	if !cfg.Collector.Enabled {
		return
	}
	payload := buildAuthEventPayload(ctx, cfg, input)
	data, err := json.Marshal(payload)
	if err != nil {
		return
	}
	_, _ = svcCtx.Collector.Enqueue(ctx, collectorx.Event{
		BizType:      AuthCollectorBizType,
		PartitionKey: authEventPartitionKey(payload),
		Payload:      json.RawMessage(data),
	})
}

// buildAuthEventPayload 构造脱敏后的认证事件负载。
func buildAuthEventPayload(ctx context.Context, cfg config.Config, input AuthEventInput) authEventPayload {
	meta := requestctx.FromContext(ctx)
	clientIP := strings.TrimSpace(input.ClientIP)
	payload := authEventPayload{
		Action:         strings.TrimSpace(input.Action),
		UserID:         input.UserID,
		AppID:          strings.TrimSpace(cfg.AppID),
		Reason:         strings.TrimSpace(input.Reason),
		Count:          input.Count,
		OccurredAtUnix: time.Now().Unix(),
	}
	if meta != nil {
		if clientIP == "" {
			clientIP = meta.ClientIP
		}
		payload.Route = strings.TrimSpace(meta.Route)
		payload.TraceID = strings.TrimSpace(meta.TraceID)
		payload.SpanID = strings.TrimSpace(meta.SpanID)
		payload.Node = strings.TrimSpace(meta.Node)
		payload.Mode = strings.TrimSpace(meta.Mode)
	}
	if payload.Node == "" {
		payload.Node = strings.TrimSpace(cfg.InstanceID)
	}
	if payload.Mode == "" {
		payload.Mode = strings.TrimSpace(cfg.Mode)
	}
	payload.UsernameHash = authEventHash(cfg, input.Username)
	payload.ClientIPHash = authEventHash(cfg, clientIP)
	payload.SessionHash = authEventHash(cfg, input.JTI)
	return payload
}

// authEventPartitionKey 返回 Collector 分区键，优先按用户聚合。
func authEventPartitionKey(payload authEventPayload) string {
	if payload.UserID > 0 {
		return fmt.Sprintf("%s:%d", payload.AppID, payload.UserID)
	}
	if payload.UsernameHash != "" {
		return payload.AppID + ":" + payload.UsernameHash
	}
	if payload.ClientIPHash != "" {
		return payload.AppID + ":" + payload.ClientIPHash
	}
	return payload.AppID
}

// authEventHash 使用应用密钥对敏感字段做不可逆关联哈希。
func authEventHash(cfg config.Config, value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return ""
	}
	secret := strings.TrimSpace(cfg.AppKey)
	if secret == "" {
		secret = strings.TrimSpace(cfg.JwtSecret)
	}
	if secret == "" {
		sum := sha256.Sum256([]byte(value))
		return hex.EncodeToString(sum[:])
	}
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(value))
	return hex.EncodeToString(mac.Sum(nil))
}
