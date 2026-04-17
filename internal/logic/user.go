package logic

import (
	"context"
	"fmt"
	"time"

	codes "gozero_api/common/codes"
	i18n "gozero_api/common/i18n"
	keys "gozero_api/common/rediskeys"
	"gozero_api/internal/model"
	"gozero_api/internal/svc"
	"gozero_api/internal/types"

	"github.com/Is999/go-utils/errors"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

// 前台用户逻辑使用的哨兵错误。
var (
	// ErrAPIUserNotFound 表示前台用户不存在。
	ErrAPIUserNotFound = errors.New("前台用户不存在")
	// ErrAPIUserDisabled 表示前台用户已禁用。
	ErrAPIUserDisabled = errors.New("前台用户已禁用")
)

// UserLogic 承载前台用户资料查询与缓存能力。
type UserLogic struct {
	*BaseLogic // 复用上下文、日志、数据库和缓存等公共能力
}

// NewUserLogic 创建前台用户逻辑对象。
func NewUserLogic(ctx context.Context, svcCtx *svc.ServiceContext) *UserLogic {
	return &UserLogic{BaseLogic: NewBaseLogicWithContext(ctx, svcCtx)}
}

// GetActiveUser 获取启用状态的用户实体。
func (l *UserLogic) GetActiveUser(userID int64) (*model.APIUser, error) {
	return l.getActiveUserByDB(l.svc.ReadDB(svc.DatabaseMain), userID)
}

// GetActiveUserForAuth 获取鉴权链路用户，使用主库避免账号状态读延迟。
func (l *UserLogic) GetActiveUserForAuth(userID int64) (*model.APIUser, error) {
	return l.getActiveUserByDB(l.svc.WriteDB(svc.DatabaseMain), userID)
}

// GetUserByID 根据用户 ID 查询用户。
func (l *UserLogic) GetUserByID(userID int64) (*model.APIUser, error) {
	return l.getUserByID(l.svc.ReadDB(svc.DatabaseMain), userID)
}

// getActiveUserByDB 使用指定数据库连接查询启用用户，调用方决定读写一致性。
func (l *UserLogic) getActiveUserByDB(db *gorm.DB, userID int64) (*model.APIUser, error) {
	user, err := l.getUserByID(db, userID)
	if err != nil {
		return nil, errors.Tag(err)
	}
	if user == nil {
		return nil, ErrAPIUserNotFound
	}
	if user.Status != model.APIUserStatusEnabled {
		return nil, ErrAPIUserDisabled
	}
	return user, nil
}

// getUserByID 使用指定数据库连接查询用户，便于鉴权和资料读取分离压力。
func (l *UserLogic) getUserByID(db *gorm.DB, userID int64) (*model.APIUser, error) {
	if userID <= 0 {
		return nil, nil
	}
	user, err := model.FindAPIUserByID(db, userID)
	if err != nil {
		return nil, errors.Tag(err)
	}
	return user, nil
}

// Profile 返回当前用户资料。
func (l *UserLogic) Profile() *types.BizResult {
	ctxUser := l.GetCtxUser()
	if ctxUser == nil || ctxUser.ID <= 0 {
		return types.NewBizResult(codes.Unauthorized).
			SetI18nMessage(i18n.MsgKeyUnauthorizedText).
			WithError(errors.New("UserLogic.Profile 当前请求未登录"))
	}
	profile, err := l.GetUserProfile(ctxUser.ID)
	if err != nil {
		return types.DBError(i18n.MsgKeyDBError, err, "UserLogic.Profile 用户ID[%d]", ctxUser.ID).ToBizResult()
	}
	return types.NewBizResult(codes.FetchSuccess).
		SetI18nMessage(i18n.MsgKeyFetchSuccess).
		WithData(profile)
}

// GetUserProfile 获取用户公开资料，优先读 Redis 缓存。
func (l *UserLogic) GetUserProfile(userID int64) (*types.UserProfile, error) {
	if userID <= 0 {
		return nil, errors.Errorf("用户ID不能为空")
	}
	cacheKey := l.userProfileKey(userID)
	if l.Redis() != nil {
		profile := &types.UserProfile{}
		if err := l.RdsGetJsonObj(cacheKey, profile); err == nil && profile.ID > 0 {
			return profile, nil
		} else if err != nil && !errors.Is(err, redis.Nil) {
			return nil, errors.Wrapf(err, "读取用户资料缓存失败 user_id=%d", userID)
		}
	}
	user, err := l.GetActiveUser(userID)
	if err != nil {
		return nil, errors.Tag(err)
	}
	profile := buildUserProfile(user)
	if l.Redis() != nil {
		if err := l.RdsSetJSONValue(cacheKey, profile, l.profileCacheTTL()); err != nil {
			return nil, errors.Wrapf(err, "写入用户资料缓存失败 user_id=%d", userID)
		}
	}
	return profile, nil
}

// DeleteUserProfileCache 删除用户资料缓存。
func (l *UserLogic) DeleteUserProfileCache(userID int64) error {
	if userID <= 0 || l.Redis() == nil {
		return nil
	}
	return l.RdsDelKeys(l.userProfileKey(userID))
}

// userProfileKey 生成当前站点下的用户资料缓存 Key。
func (l *UserLogic) userProfileKey(userID int64) string {
	return l.AppRedisKey(fmt.Sprintf(keys.UserProfile, userID))
}

// profileCacheTTL 返回用户资料缓存 TTL，未配置时使用 5 分钟。
func (l *UserLogic) profileCacheTTL() int64 {
	cfg := l.svc.CurrentConfig()
	if cfg.Auth.ProfileCacheTTLSeconds > 0 {
		return cfg.Auth.ProfileCacheTTLSeconds
	}
	return 300
}

// buildUserProfile 将用户实体转换为前台可展示资料。
func buildUserProfile(user *model.APIUser) *types.UserProfile {
	if user == nil {
		return &types.UserProfile{}
	}
	return &types.UserProfile{
		ID:          user.ID,
		Username:    user.Username,
		Nickname:    user.Nickname,
		Email:       user.Email,
		Phone:       user.Phone,
		Avatar:      user.Avatar,
		Status:      user.Status,
		LastLoginAt: formatDateTime(user.LastLoginAt),
		LastLoginIP: user.LastLoginIP,
		CreatedAt:   formatDateTime(user.CreatedAt),
		UpdatedAt:   formatDateTime(user.UpdatedAt),
	}
}

// formatDateTime 将时间格式化为前端稳定展示字符串。
func formatDateTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format(time.DateTime)
}
