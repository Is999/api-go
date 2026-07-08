package middleware

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"io"
	"net"
	"net/http"
	"net/netip"
	"strconv"
	"strings"
	"time"

	codes "api/common/codes"
	i18n "api/common/i18n"
	"api/helper"
	"api/internal/config"
	"api/internal/svc"

	utils "github.com/Is999/go-utils"
	"github.com/Is999/go-utils/errors"
)

// 运维接口鉴权请求头常量。
const (
	// HeaderOpsToken 表示运维级接口保护令牌请求头。
	HeaderOpsToken = "X-Ops-Token"
	// HeaderOpsTimestamp 表示运维请求签名时间戳，取 Unix 秒。
	HeaderOpsTimestamp = "X-Ops-Timestamp"
	// HeaderOpsBodySHA256 表示运维请求体 SHA256 摘要。
	HeaderOpsBodySHA256 = "X-Ops-Body-SHA256"
	// HeaderOpsSignature 表示运维请求 HMAC-SHA256 签名。
	HeaderOpsSignature = "X-Ops-Signature"
	// opsSignatureBodyMaxBytes 限制参与验签的请求体大小，避免内网接口被异常大包拖垮。
	opsSignatureBodyMaxBytes = 1 << 20
	// opsSignatureWindow 表示运维请求签名允许的时间偏差。
	opsSignatureWindow = 5 * time.Minute
)

// OpsMiddleware 保护配置热加载、运行态同步等运维级接口。
type OpsMiddleware struct {
	svc *svc.ServiceContext // 运维保护依赖的服务上下文
}

// NewOpsMiddleware 创建运维保护中间件实例。
func NewOpsMiddleware(svcCtx *svc.ServiceContext) *OpsMiddleware {
	return &OpsMiddleware{svc: svcCtx}
}

// Handle 校验运维令牌、内网来源和请求签名。
func (m *OpsMiddleware) Handle(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cfg := config.OpsConfig{}
		if m != nil && m.svc != nil {
			cfg = m.svc.CurrentConfig().Ops
		}
		if err := validateConfigReloadOps(r, cfg); err != nil {
			helper.NewJSONResp(r.Context(), w).
				SetHTTPStatus(http.StatusForbidden).
				SetCode(codes.Forbidden).
				SetError(err).
				Fail(i18n.MsgKeyForbidden)
			return
		}
		next(w, r)
	}
}

// validateConfigReloadOps 校验运维接口的访问边界。
func validateConfigReloadOps(r *http.Request, cfg config.OpsConfig) error {
	token := strings.TrimSpace(cfg.ConfigReloadToken)
	if token == "" {
		return errors.Errorf("运维接口令牌未配置")
	}
	got := strings.TrimSpace(r.Header.Get(HeaderOpsToken))
	if got == "" {
		return errors.Errorf("缺少请求头%s", HeaderOpsToken)
	}
	if subtle.ConstantTimeCompare([]byte(got), []byte(token)) != 1 {
		return errors.Errorf("运维接口令牌不匹配")
	}
	if forwardedHeaderHasPublicAddr(r) {
		return errors.Errorf("运维接口转发来源包含公网IP")
	}
	if !clientIPAllowed(utils.ClientIP(r), cfg.ConfigReloadAllowedIPs) {
		return errors.Errorf("运维接口来源IP非内网或未命中白名单")
	}
	if err := validateOpsSignature(r, token); err != nil {
		return errors.Tag(err)
	}
	return nil
}

// validateOpsSignature 校验运维请求签名，并恢复请求体给后续 handler 使用。
func validateOpsSignature(r *http.Request, token string) error {
	timestamp := strings.TrimSpace(r.Header.Get(HeaderOpsTimestamp))
	if timestamp == "" {
		return errors.Errorf("缺少请求头%s", HeaderOpsTimestamp)
	}
	if err := validateOpsTimestamp(timestamp, time.Now()); err != nil {
		return errors.Tag(err)
	}
	body, err := readOpsSignedBody(r)
	if err != nil {
		return errors.Tag(err)
	}
	bodyHash := opsBodySHA256(body)
	gotBodyHash := strings.ToLower(strings.TrimSpace(r.Header.Get(HeaderOpsBodySHA256)))
	if gotBodyHash == "" {
		return errors.Errorf("缺少请求头%s", HeaderOpsBodySHA256)
	}
	if subtle.ConstantTimeCompare([]byte(gotBodyHash), []byte(bodyHash)) != 1 {
		return errors.Errorf("运维接口请求体摘要不匹配")
	}
	gotSignature := strings.ToLower(strings.TrimSpace(r.Header.Get(HeaderOpsSignature)))
	if gotSignature == "" {
		return errors.Errorf("缺少请求头%s", HeaderOpsSignature)
	}
	expectedSignature := signOpsRequest(token, r.Method, r.URL.RequestURI(), timestamp, bodyHash)
	if !opsSignatureEqual(gotSignature, expectedSignature) {
		return errors.Errorf("运维接口签名不匹配")
	}
	return nil
}

// validateOpsTimestamp 校验签名时间戳是否在允许窗口内。
func validateOpsTimestamp(raw string, now time.Time) error {
	seconds, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return errors.Wrap(err, "运维接口签名时间戳非法")
	}
	delta := now.Sub(time.Unix(seconds, 0))
	if delta < 0 {
		delta = -delta
	}
	if delta > opsSignatureWindow {
		return errors.Errorf("运维接口签名已过期")
	}
	return nil
}

// readOpsSignedBody 读取参与签名的请求体并重置 Body，保证后续业务仍可读取。
func readOpsSignedBody(r *http.Request) ([]byte, error) {
	if r == nil || r.Body == nil {
		return nil, nil
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, opsSignatureBodyMaxBytes+1))
	if err != nil {
		return nil, errors.Wrap(err, "读取运维接口请求体失败")
	}
	if len(body) > opsSignatureBodyMaxBytes {
		return nil, errors.Errorf("运维接口请求体超过大小限制")
	}
	r.Body = io.NopCloser(bytes.NewReader(body))
	return body, nil
}

// opsBodySHA256 返回请求体 SHA256 十六进制摘要。
func opsBodySHA256(body []byte) string {
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:])
}

// signOpsRequest 按稳定串生成运维请求 HMAC-SHA256 签名。
func signOpsRequest(secret string, method string, requestURI string, timestamp string, bodyHash string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(strings.Join([]string{
		strings.ToUpper(strings.TrimSpace(method)),
		requestURI,
		timestamp,
		bodyHash,
	}, "\n")))
	return hex.EncodeToString(mac.Sum(nil))
}

// opsSignatureEqual 以常量时间比较十六进制签名。
func opsSignatureEqual(got string, expected string) bool {
	gotBytes, err := hex.DecodeString(got)
	if err != nil {
		return false
	}
	expectedBytes, err := hex.DecodeString(expected)
	if err != nil {
		return false
	}
	return hmac.Equal(gotBytes, expectedBytes)
}

// clientIPAllowed 校验客户端 IP 是否来自内网，并按白名单进一步收窄。
func clientIPAllowed(clientIP string, allowed []string) bool {
	addr, ok := parseAddrValue(clientIP)
	if !ok {
		return false
	}
	if !isInternalClientAddr(addr) {
		return false
	}

	allowed = normalizeAllowedIPs(allowed)
	if len(allowed) == 0 {
		return true
	}
	for _, item := range allowed {
		if strings.Contains(item, "/") {
			prefix, err := netip.ParsePrefix(item)
			if err == nil && prefix.Contains(addr) {
				return true
			}
			continue
		}
		if allowedAddr, ok := parseAddrValue(item); ok && allowedAddr == addr {
			return true
		}
		if ip := net.ParseIP(item); ip != nil && ip.String() == addr.String() {
			return true
		}
	}
	return false
}

// forwardedHeaderHasPublicAddr 拦截反代转发头中的公网真实来源。
func forwardedHeaderHasPublicAddr(r *http.Request) bool {
	if r == nil {
		return false
	}
	for _, header := range []string{"X-Forwarded-For", "X-Real-IP"} {
		for _, item := range strings.Split(r.Header.Get(header), ",") {
			addr, ok := parseAddrValue(item)
			if ok && !isInternalClientAddr(addr) {
				return true
			}
		}
	}
	return false
}

// isInternalClientAddr 判断来源地址是否属于本机、私有地址或链路本地地址。
func isInternalClientAddr(addr netip.Addr) bool {
	addr = addr.Unmap()
	return addr.IsLoopback() || addr.IsPrivate() || addr.IsLinkLocalUnicast()
}

// parseAddrValue 解析 IP、host:port 或 [IPv6]:port 形式的地址。
func parseAddrValue(raw string) (netip.Addr, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return netip.Addr{}, false
	}
	if host, _, err := net.SplitHostPort(raw); err == nil {
		raw = host
	} else {
		raw = strings.Trim(raw, "[]")
	}
	addr, err := netip.ParseAddr(raw)
	if err != nil {
		return netip.Addr{}, false
	}
	return addr.Unmap(), true
}

// normalizeAllowedIPs 清洗配置中的空白 IP 或 CIDR。
func normalizeAllowedIPs(items []string) []string {
	result := make([]string, 0, len(items))
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item != "" {
			result = append(result, item)
		}
	}
	return result
}
