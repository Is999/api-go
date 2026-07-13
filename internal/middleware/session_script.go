package middleware

import (
	_ "embed"

	"api/common/embedasset"

	"github.com/redis/go-redis/v9"
)

// userSessionVerifyScript 原子校验会话、过期索引和认证版本。
var userSessionVerifyScript = redis.NewScript(embedasset.StripLeadingLineComments(userSessionVerifyScriptText, "--"))

// userSessionVerifyScriptText 保存会话校验 Lua 资产。
//
//go:embed assets/user_session_verify.lua
var userSessionVerifyScriptText string
