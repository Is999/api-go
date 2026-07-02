package validators

import (
	"strings"

	keys "api/common/rediskeys"
	"api/internal/config"

	"github.com/Is999/go-utils/errors"
)

// Collector 载体枚举限定配置文件可声明的事件投递通道。
const (
	collectorTransportAuto  = "auto"  // 自动选择可用的 Collector 载体
	collectorTransportRedis = "redis" // 强制使用 Redis Stream 作为 Collector 载体
	collectorTransportSync  = "sync"  // 强制使用进程内同步处理作为 Collector 载体
)

// ValidateCollector 校验 Collector 载体配置是否自洽。
func ValidateCollector(c config.Config) error {
	cfg := c.Collector
	transport := strings.ToLower(strings.TrimSpace(cfg.Transport))
	if cfg.Redis.Enabled && strings.TrimSpace(cfg.Redis.Stream) == "" {
		return errors.Errorf("collector.redis.enabled=true 时必须配置 collector.redis.stream")
	}
	if ownerAppID, ok := keys.Owner(cfg.Redis.Stream); ok && strings.TrimSpace(c.AppID) != ownerAppID {
		return errors.Errorf("collector.redis.stream 属于其它 app_id[%s]", ownerAppID)
	}
	switch transport {
	case "", collectorTransportAuto, collectorTransportSync:
		return nil
	case collectorTransportRedis:
		if !cfg.Redis.Enabled {
			return errors.Errorf("collector.transport=redis 时必须启用 collector.redis.enabled")
		}
		if strings.TrimSpace(cfg.Redis.Stream) == "" {
			return errors.Errorf("collector.transport=redis 时必须配置 collector.redis.stream")
		}
		return nil
	default:
		return errors.Errorf("collector.transport 仅支持 auto/sync/redis")
	}
}
