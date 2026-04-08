package types

// RegisterReq 表示前台用户注册请求。
type RegisterReq struct {
	Username string `json:"username"`          // 用户名，3-32 位
	Password string `json:"password"`          // 登录密码
	Nickname string `json:"nickname,optional"` // 昵称
	Email    string `json:"email,optional"`    // 邮箱
	Phone    string `json:"phone,optional"`    // 手机号
}

// LoginReq 表示前台用户登录请求。
type LoginReq struct {
	Username string `json:"username"` // 用户名
	Password string `json:"password"` // 登录密码
}

// AuthTokenResp 表示登录或刷新后的令牌响应。
type AuthTokenResp struct {
	Token     string       `json:"token"`     // Bearer token
	ExpiresAt int64        `json:"expiresAt"` // 过期时间戳，单位秒
	User      *UserProfile `json:"user"`      // 当前用户资料
}
