package types

// UserProfile 表示前台用户公开资料。
type UserProfile struct {
	ID          int64  `json:"id"`          // 用户 ID
	ShardNo     int    `json:"shardNo"`     // 取模分片
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

// UserRuntimeSyncReq 表示内网同步单个前台用户运行态缓存的请求。
type UserRuntimeSyncReq struct {
	ID       int64  `path:"id" json:"id,optional" form:"id,optional"` // 用户 ID
	Profile  bool   `json:"profile,optional"`                         // 是否删除用户资料缓存
	Sessions bool   `json:"sessions,optional"`                        // 是否失效该用户全部登录态
	Reason   string `json:"reason,optional"`                          // 触发同步的后台操作原因
}

// UserRuntimeSyncResp 表示前台用户运行态同步结果。
type UserRuntimeSyncResp struct {
	UserID                  int64  `json:"userId"`                  // 用户 ID
	ProfileCacheInvalidated bool   `json:"profileCacheInvalidated"` // 是否已处理资料缓存
	SessionsInvalidated     bool   `json:"sessionsInvalidated"`     // 是否已处理全部登录态
	Reason                  string `json:"reason"`                  // 后台传入的同步原因
}
