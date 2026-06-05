package types

import (
	"strings"

	"github.com/Is999/go-utils/errors"
)

// userRuntimeSyncReasonMaxLength 表示内网同步原因最大字符数。
const userRuntimeSyncReasonMaxLength = 128

// UserProfile 表示业务用户公开资料。
type UserProfile struct {
	ID          int64  `json:"id,string"`   // 用户 ID，JSON 以字符串返回，避免前端丢失精度
	ShardNo     int    `json:"shardNo"`     // ID 哈希分片，来源 CRC32(id字符串)%1000，便于分表和分片游标查询
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

// UserRuntimeSyncReq 表示内网同步单个业务用户运行态缓存的请求。
type UserRuntimeSyncReq struct {
	ID       int64  `path:"id" json:"id,optional" form:"id,optional"` // 用户 ID
	Profile  bool   `json:"profile,optional"`                         // 是否删除用户资料缓存
	Sessions bool   `json:"sessions,optional"`                        // 是否失效该用户全部登录态
	Reason   string `json:"reason,optional"`                          // 触发同步的后台操作原因
}

// Validate 校验并归一化内网用户运行态同步请求。
func (r *UserRuntimeSyncReq) Validate() error {
	if r == nil {
		return errors.New("请求不能为空")
	}
	if r.ID <= 0 {
		return errors.New("用户 ID 不能为空")
	}
	if !r.Profile && !r.Sessions {
		r.Profile = true
	}
	r.Reason = strings.TrimSpace(r.Reason)
	if len([]rune(r.Reason)) > userRuntimeSyncReasonMaxLength {
		return errors.Errorf("同步原因不能超过 %d 个字符", userRuntimeSyncReasonMaxLength)
	}
	return nil
}

// UserRuntimeSyncResp 表示业务用户运行态同步结果。
type UserRuntimeSyncResp struct {
	UserID                  int64  `json:"userId,string"`           // 用户 ID，JSON 以字符串返回，避免前端丢失精度
	ProfileCacheInvalidated bool   `json:"profileCacheInvalidated"` // 是否已处理资料缓存
	SessionsInvalidated     bool   `json:"sessionsInvalidated"`     // 是否已处理全部登录态
	Reason                  string `json:"reason"`                  // 后台传入的同步原因
}
