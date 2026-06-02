package logic

import (
	"time"

	keys "api/common/rediskeys"
)

// CacheIsEmptyMarker 判断缓存值是否为空值占位符。
func CacheIsEmptyMarker(value string) bool {
	return value == keys.EmptyValueMarker || value == "__EMPTY__"
}

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
