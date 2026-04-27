package logic

import (
	"context"
	"fmt"
	"time"

	keys "api/common/rediskeys"
	redislock "api/internal/infra/redsync"

	"github.com/Is999/go-utils/errors"
)

// 缓存空值标记错误，供跨包识别缓存穿透保护命中。
var (
	// errCacheEmptyMarker 表示命中了空值缓存占位，用于避免缓存穿透时持续回源数据库。
	errCacheEmptyMarker = errors.New("cache empty marker")
	// ErrCacheEmptyMarker 表示命中了空值缓存占位，用于跨领域包判断缓存空标记。
	ErrCacheEmptyMarker = errCacheEmptyMarker
)

// 缓存重建锁默认参数。
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

// JitterTTL 为基础过期时间添加抖动。
func JitterTTL(base time.Duration) time.Duration {
	return jitterTTL(base)
}

// emptyCacheTTL 返回空值缓存的过期时间。
func emptyCacheTTL() time.Duration {
	return jitterTTL(2 * time.Minute)
}

// EmptyCacheTTL 返回空值缓存的过期时间。
func EmptyCacheTTL() time.Duration {
	return emptyCacheTTL()
}

// tryRebuildCacheWithLock 使用 redsync 保护缓存重建，避免热点 key 并发击穿数据库。
func (l *BaseLogic) tryRebuildCacheWithLock(cacheKey string, rebuild func() error) error {
	if rebuild == nil {
		return nil
	}
	err := l.rebuildCacheWithLock(cacheKey, func(context.Context) error {
		return rebuild()
	})
	if redislock.IsLockTaken(err) {
		return nil
	}
	return errors.Tag(err)
}

// rebuildCacheWithLock 使用 redsync 保护缓存重建，并保留锁竞争结果供调用方决定是否等待。
func (l *BaseLogic) rebuildCacheWithLock(cacheKey string, rebuild func(context.Context) error) error {
	if rebuild == nil {
		return nil
	}
	if l == nil || l.Redis() == nil || cacheKey == "" {
		return rebuild(context.Background())
	}
	lockKey := l.cacheLockKey(cacheKey)
	err := redislock.WithLock(l.Ctx, l.Redis(), lockKey, cacheRebuildLockTTL, rebuild)
	return errors.Tag(err)
}

// TryRebuildCacheWithLock 使用分布式锁保护缓存回源重建。
func (l *BaseLogic) TryRebuildCacheWithLock(cacheKey string, rebuild func() error) error {
	return l.tryRebuildCacheWithLock(cacheKey, rebuild)
}

// RebuildCacheWithLock 使用分布式锁保护缓存重建，并把锁竞争错误交给调用方处理。
func (l *BaseLogic) RebuildCacheWithLock(cacheKey string, rebuild func(context.Context) error) error {
	return l.rebuildCacheWithLock(cacheKey, rebuild)
}

// IsCacheRebuildBusy 判断缓存重建锁是否已被其它实例持有。
func IsCacheRebuildBusy(err error) bool {
	return redislock.IsLockTaken(err)
}

// CacheLockKey 返回当前 app_id 作用域下的缓存重建锁 Redis 键。
func (l *BaseLogic) CacheLockKey(cacheKey string) string {
	return l.cacheLockKey(cacheKey)
}

// cacheLockKey 返回当前 app_id 作用域下的缓存重建锁 Redis 键。
func (l *BaseLogic) cacheLockKey(cacheKey string) string {
	return l.AppRedisKey(fmt.Sprintf(keys.CacheRebuildLock, keys.TrimAppScopedPrefix(cacheKey)))
}
