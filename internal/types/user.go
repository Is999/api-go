package types

// UserProfile 表示前台用户公开资料。
type UserProfile struct {
	ID          int64  `json:"id"`          // 用户 ID
	Username    string `json:"username"`    // 用户名
	Nickname    string `json:"nickname"`    // 昵称
	Email       string `json:"email"`       // 邮箱
	Phone       string `json:"phone"`       // 手机号
	Avatar      string `json:"avatar"`      // 头像
	Status      int    `json:"status"`      // 状态：1 正常，0 禁用
	LastLoginAt string `json:"lastLoginAt"` // 最后登录时间
	LastLoginIP string `json:"lastLoginIp"` // 最后登录 IP
	CreatedAt   string `json:"createdAt"`   // 创建时间
	UpdatedAt   string `json:"updatedAt"`   // 更新时间
}
