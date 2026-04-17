package logic

import (
	"context"
	"fmt"
	"time"

	keys "gozero_api/common/rediskeys"
	redislock "gozero_api/internal/infra/redsync"

	"github.com/Is999/go-utils/errors"
)

var (
	// errCacheEmptyMarker 表示命中了空值缓存占位，用于避免缓存穿透时持续回源数据库。
	errCacheEmptyMarker = errors.New("cache empty marker")
)

const (
	cacheRebuildLockTTL = 10 * time.Second // 缓存重建锁默认持有时间
)

// jitterTTL 为基础过期时间添加抖动，降低同类缓存集中失效导致的雪崩风险。
func jitterTTL(base time.Duration) time.Duration {
	if base <= 0 {
		return 0
	}
	jitterRange := base / 10
	if jitterRange <= 0 {
		jitterRange = time.Second
	}
	return base + time.Duration(time.Now().UnixNano()%int64(jitterRange))
}

// emptyCacheTTL 返回空值缓存的过期时间。
func emptyCacheTTL() time.Duration {
	return jitterTTL(2 * time.Minute)
}

// tryRebuildCacheWithLock 使用 redsync 保护缓存重建，避免热点 key 并发击穿数据库。
func (l *BaseLogic) tryRebuildCacheWithLock(cacheKey string, rebuild func() error) error {
	if rebuild == nil {
		return nil
	}
	if l == nil || l.Redis() == nil || cacheKey == "" {
		return rebuild()
	}
	lockKey := l.cacheLockKey(cacheKey)
	err := redislock.WithLock(l.Context(), l.Redis(), lockKey, cacheRebuildLockTTL, func(context.Context) error {
		return rebuild()
	})
	if redislock.IsLockTaken(err) {
		return nil
	}
	return errors.Tag(err)
}

// cacheLockKey 返回当前 app_id 作用域下的缓存重建锁 Redis 键。
func (l *BaseLogic) cacheLockKey(cacheKey string) string {
	return l.AppRedisKey(fmt.Sprintf(keys.CacheRebuildLock, keys.TrimAppScopedPrefix(cacheKey)))
}
