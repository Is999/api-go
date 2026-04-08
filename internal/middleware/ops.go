package middleware

import (
	"crypto/subtle"
	"net"
	"net/http"
	"net/netip"
	"strings"

	codes "gozero_api/common/codes"
	i18n "gozero_api/common/i18n"
	"gozero_api/helper"
	"gozero_api/internal/config"
	"gozero_api/internal/svc"

	utils "github.com/Is999/go-utils"
	"github.com/Is999/go-utils/errors"
)

const (
	// HeaderOpsToken 表示运维级接口保护令牌请求头。
	HeaderOpsToken = "X-Ops-Token"
)

// OpsMiddleware 保护配置热加载等运维级接口。
type OpsMiddleware struct {
	svc *svc.ServiceContext // 运维保护依赖的服务上下文
}

// NewOpsMiddleware 创建运维保护中间件实例。
func NewOpsMiddleware(svcCtx *svc.ServiceContext) *OpsMiddleware {
	return &OpsMiddleware{svc: svcCtx}
}

// Handle 校验运维令牌和内网来源边界。
func (m *OpsMiddleware) Handle(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cfg := config.OpsConfig{}
		if m != nil && m.svc != nil {
			cfg = m.svc.CurrentConfig().Ops
		}
		if err := validateConfigReloadOps(r, cfg); err != nil {
			helper.NewJsonResp(r.Context(), w).
				SetHttpStatus(http.StatusForbidden).
				SetCode(codes.Forbidden).
				SetError(err).
				Fail(i18n.MsgKeyForbidden)
			return
		}
		next(w, r)
	}
}

// validateConfigReloadOps 校验配置热加载接口的运维访问边界。
func validateConfigReloadOps(r *http.Request, cfg config.OpsConfig) error {
	token := strings.TrimSpace(cfg.ConfigReloadToken)
	if token == "" {
		return errors.Errorf("配置热加载运维令牌未配置")
	}
	got := strings.TrimSpace(r.Header.Get(HeaderOpsToken))
	if got == "" {
		return errors.Errorf("缺少请求头%s", HeaderOpsToken)
	}
	if subtle.ConstantTimeCompare([]byte(got), []byte(token)) != 1 {
		return errors.Errorf("配置热加载运维令牌不匹配")
	}
	if forwardedHeaderHasPublicAddr(r) {
		return errors.Errorf("配置热加载转发来源包含公网IP")
	}
	if !clientIPAllowed(utils.ClientIP(r), cfg.ConfigReloadAllowedIPs) {
		return errors.Errorf("配置热加载来源IP非内网或未命中白名单")
	}
	return nil
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
