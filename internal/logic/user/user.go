package user

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	codes "api/common/codes"
	i18n "api/common/i18n"
	keys "api/common/rediskeys"
	redislock "api/internal/infra/redsync"
	corelogic "api/internal/logic"
	"api/internal/model"
	"api/internal/svc"
	"api/internal/types"

	"github.com/Is999/go-utils/errors"
	"github.com/redis/go-redis/v9"
	"golang.org/x/sync/singleflight"
	"gorm.io/gorm"
)

// 前台用户逻辑使用的哨兵错误。
var (
	// ErrUserNotFound 表示前台用户不存在。
	ErrUserNotFound = errors.New("前台用户不存在")
	// ErrUserDisabled 表示前台用户已禁用。
	ErrUserDisabled = errors.New("前台用户已禁用")
)

// userProfileLoadGroup 合并当前进程内同一用户资料缓存的并发回源。
var userProfileLoadGroup singleflight.Group

const (
	// userProfileRebuildLockTTL 是用户资料缓存跨进程重建锁租约。
	userProfileRebuildLockTTL = 10 * time.Second
	// userProfileRebuildWaitStep 是锁竞争时轮询缓存的间隔。
	userProfileRebuildWaitStep = 50 * time.Millisecond
	// userProfileRebuildWaitAttempts 限制等待其它实例重建的次数。
	userProfileRebuildWaitAttempts = 20
)

// UserLogic 承载前台用户资料查询与缓存能力。
type UserLogic struct {
	*corelogic.BaseLogic // 复用上下文、日志、数据库和缓存等公共能力
}

// NewUserLogic 创建前台用户逻辑对象。
func NewUserLogic(ctx context.Context, svcCtx *svc.ServiceContext) *UserLogic {
	return &UserLogic{BaseLogic: corelogic.NewBaseLogicWithContext(ctx, svcCtx)}
}

// GetActiveUser 获取启用状态的用户实体。
func (l *UserLogic) GetActiveUser(userID int64) (*model.User, error) {
	return l.getActiveUserByDB(l.Svc.ReadDB(svc.DatabaseMain), userID)
}

// GetActiveUserForAuth 获取鉴权链路用户，使用主库避免账号状态读延迟。
func (l *UserLogic) GetActiveUserForAuth(userID int64) (*model.User, error) {
	return l.getActiveUserByDB(l.Svc.WriteDB(svc.DatabaseMain), userID)
}

// getActiveUserByDB 使用指定数据库连接查询启用用户，调用方决定读写一致性。
func (l *UserLogic) getActiveUserByDB(db *gorm.DB, userID int64) (*model.User, error) {
	user, err := l.getUserByID(db, userID)
	if err != nil {
		return nil, errors.Tag(err)
	}
	if user == nil {
		return nil, ErrUserNotFound
	}
	if user.Status != model.UserStatusEnabled {
		return nil, ErrUserDisabled
	}
	return user, nil
}

// getUserByID 使用指定数据库连接查询用户，便于鉴权和资料读取分离压力。
func (l *UserLogic) getUserByID(db *gorm.DB, userID int64) (*model.User, error) {
	if userID <= 0 {
		return nil, nil
	}
	user, err := model.FindUserByID(db, userID, l.Svc.CurrentConfig().User.RouteShardCount)
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
		return types.DBError(i18n.MsgKeyDBError, err, "UserLogic.Profile 用户 ID[%d]", ctxUser.ID).ToBizResult()
	}
	return types.NewBizResult(codes.FetchSuccess).
		SetI18nMessage(i18n.MsgKeyFetchSuccess).
		WithData(profile)
}

// GetUserProfile 获取用户公开资料，优先读 Redis 缓存。
func (l *UserLogic) GetUserProfile(userID int64) (*types.UserProfile, error) {
	if userID <= 0 {
		return nil, errors.Errorf("用户 ID不能为空")
	}
	cacheKey := l.userProfileKey(userID)
	profile, found, err := l.cachedUserProfile(cacheKey, userID)
	if err != nil || found {
		return profile, errors.Tag(err)
	}

	// loadKey 在 Redis 未配置时仍按用户隔离并发回源。
	loadKey := cacheKey
	if loadKey == "" {
		loadKey = fmt.Sprintf(keys.UserProfile, userID)
	}
	value, err, _ := userProfileLoadGroup.Do(loadKey, func() (any, error) {
		return l.loadUserProfile(cacheKey, userID)
	})
	if err != nil {
		return nil, errors.Tag(err)
	}
	profile, ok := value.(*types.UserProfile)
	if !ok {
		return nil, errors.Errorf("用户资料回源结果类型错误 user_id=%d", userID)
	}
	return profile, nil
}

// loadUserProfile 通过 Redis 分布式锁保护跨进程缓存回源。
func (l *UserLogic) loadUserProfile(cacheKey string, userID int64) (*types.UserProfile, error) {
	if l.Redis() == nil {
		return l.loadUserProfileFromDB(userID)
	}
	var profile *types.UserProfile
	err := redislock.WithLock(l.Ctx, l.Redis(), l.userProfileRebuildLockKey(userID), userProfileRebuildLockTTL, func(lockCtx context.Context) error {
		lockedLogic := NewUserLogic(lockCtx, l.Svc)
		cached, found, err := lockedLogic.cachedUserProfile(cacheKey, userID)
		if err != nil || found {
			profile = cached
			return errors.Tag(err)
		}
		profile, err = lockedLogic.loadUserProfileFromDB(userID)
		return errors.Tag(err)
	})
	if err == nil {
		return profile, nil
	}
	if !redislock.IsLockTaken(err) {
		return nil, errors.Tag(err)
	}
	return l.waitCachedUserProfile(cacheKey, userID)
}

// loadUserProfileFromDB 回源用户资料，并写入正值或空值缓存。
func (l *UserLogic) loadUserProfileFromDB(userID int64) (*types.UserProfile, error) {
	user, err := l.GetActiveUser(userID)
	if err != nil {
		if !errors.Is(err, ErrUserNotFound) && !errors.Is(err, model.ErrUserIdentityMissing) {
			return nil, errors.Tag(err)
		}
		if l.Redis() != nil {
			if err := l.cacheMissingUserProfile(userID); err != nil {
				return nil, errors.Wrapf(err, "写入用户资料空值缓存失败 user_id=%d", userID)
			}
		}
		return nil, ErrUserNotFound
	}
	profile := BuildUserProfile(user)
	if l.Redis() != nil {
		if err := l.CacheUserProfile(userID, profile); err != nil {
			return nil, errors.Wrapf(err, "写入用户资料缓存失败 user_id=%d", userID)
		}
	}
	return profile, nil
}

// waitCachedUserProfile 在锁竞争时有界等待其它实例写回缓存。
func (l *UserLogic) waitCachedUserProfile(cacheKey string, userID int64) (*types.UserProfile, error) {
	for attempt := 0; attempt <= userProfileRebuildWaitAttempts; attempt++ {
		profile, found, err := l.cachedUserProfile(cacheKey, userID)
		if err != nil || found {
			return profile, errors.Tag(err)
		}
		if attempt == userProfileRebuildWaitAttempts {
			break
		}
		timer := time.NewTimer(userProfileRebuildWaitStep)
		select {
		case <-l.Ctx.Done():
			if !timer.Stop() {
				<-timer.C
			}
			return nil, errors.Tag(l.Ctx.Err())
		case <-timer.C:
		}
	}
	return nil, errors.Errorf("等待用户资料缓存重建超时 user_id=%d", userID)
}

// cachedUserProfile 读取用户资料缓存，并区分命中与未命中。
func (l *UserLogic) cachedUserProfile(cacheKey string, userID int64) (*types.UserProfile, bool, error) {
	if l.Redis() == nil {
		return nil, false, nil
	}
	if cacheKey == "" {
		return nil, false, errors.Errorf("用户资料缓存 Key 为空 user_id=%d", userID)
	}
	value, err := l.Redis().Get(l.Ctx, cacheKey).Result()
	if errors.Is(err, redis.Nil) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, errors.Wrapf(err, "读取用户资料缓存失败 user_id=%d", userID)
	}
	if corelogic.CacheIsEmptyMarker(value) {
		return nil, true, ErrUserNotFound
	}
	profile := &types.UserProfile{}
	if err := json.Unmarshal([]byte(value), profile); err != nil {
		return nil, false, errors.Wrapf(err, "解析用户资料缓存失败 user_id=%d", userID)
	}
	return profile, profile.ID > 0, nil
}

// cacheMissingUserProfile 短时缓存用户不存在结果，避免重复穿透数据库。
func (l *UserLogic) cacheMissingUserProfile(userID int64) error {
	if l.Redis() == nil {
		return nil
	}
	cacheKey := l.userProfileKey(userID)
	if cacheKey == "" {
		return errors.Errorf("用户资料缓存 Key 为空 user_id=%d", userID)
	}
	return errors.Tag(l.Redis().Set(l.Ctx, cacheKey, keys.EmptyValueMarker, corelogic.EmptyCacheTTL()).Err())
}

// CacheUserProfile 写入用户资料缓存，调用方不需要了解具体 Redis key。
func (l *UserLogic) CacheUserProfile(userID int64, profile *types.UserProfile) error {
	if userID <= 0 || profile == nil || l.Redis() == nil {
		return nil
	}
	return l.RdsSetJSONValue(l.userProfileKey(userID), profile, l.profileCacheTTL())
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

// userProfileRebuildLockKey 生成当前站点下的用户资料缓存重建锁 Key。
func (l *UserLogic) userProfileRebuildLockKey(userID int64) string {
	return l.AppRedisKey(fmt.Sprintf(keys.UserProfileRebuildLock, userID))
}

// profileCacheTTL 返回用户资料缓存 TTL，未配置时使用 5 分钟。
func (l *UserLogic) profileCacheTTL() int64 {
	cfg := l.Svc.CurrentConfig()
	if cfg.Auth.ProfileCacheTTLSeconds > 0 {
		return cfg.Auth.ProfileCacheTTLSeconds
	}
	return 300
}

// BuildUserProfile 将用户实体转换为前台可展示资料。
func BuildUserProfile(user *model.User) *types.UserProfile {
	if user == nil {
		return &types.UserProfile{}
	}
	return &types.UserProfile{
		ID:          user.ID,
		ShardNo:     user.ShardNo,
		Username:    user.Username,
		Nickname:    user.Nickname,
		Email:       user.EmailMasked,
		Phone:       user.PhoneMasked,
		Avatar:      user.Avatar,
		Status:      user.Status,
		LastLoginAt: corelogic.FormatDateTime(user.LastLoginAt),
		LastLoginIP: user.LastLoginIP,
		CreatedAt:   corelogic.FormatDateTime(user.CreatedAt),
		UpdatedAt:   corelogic.FormatDateTime(user.UpdatedAt),
	}
}
