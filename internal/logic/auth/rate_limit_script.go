package auth

import (
	_ "embed"

	"api/common/embedasset"

	"github.com/redis/go-redis/v9"
)

// authRateLimitScript 原子维护认证窗口计数、TTL 和超限锁。
var authRateLimitScript = redis.NewScript(embedasset.StripLeadingLineComments(authRateLimitScriptText, "--"))

// authRateLimitScriptText 保存认证限流 Lua 资产。
//
//go:embed assets/auth_rate_limit.lua
var authRateLimitScriptText string
