package helper

import (
	"context"

	"gozero_api/internal/requestctx"
)

// CtxUser 存储上下文中的前台用户信息。
type CtxUser struct {
	ID   int64  // 用户 ID
	Name string // 用户名
	IP   string // 当前请求 IP
}

// GetCtxUser 从请求元数据中提取前台用户信息。
func GetCtxUser(ctx context.Context) *CtxUser {
	if ctx == nil {
		return nil
	}
	meta := requestctx.FromContext(ctx)
	if meta == nil || meta.UserID == 0 {
		return nil
	}
	user := &CtxUser{
		ID:   meta.UserID,
		Name: meta.UserName,
		IP:   meta.ClientIP,
	}
	if user.Name == "" {
		return nil
	}
	return user
}
