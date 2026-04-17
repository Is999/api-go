package logic

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	keys "gozero_api/common/rediskeys"
	redislock "gozero_api/internal/infra/redsync"
	"gozero_api/internal/model"
	"gozero_api/internal/svc"

	"github.com/Is999/go-utils/errors"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

// 系统配置缓存字段和保护参数。
const (
	sysConfigCacheFieldID        = "id"         // 配置 ID 字段
	sysConfigCacheFieldUUID      = "uuid"       // 配置 uuid 字段
	sysConfigCacheFieldTitle     = "title"      // 配置标题字段
	sysConfigCacheFieldType      = "type"       // 配置类型字段
	sysConfigCacheFieldValue     = "value"      // 配置值字段
	sysConfigCacheFieldExample   = "example"    // 配置示例字段
	sysConfigCacheFieldRemark    = "remark"     // 配置备注字段
	sysConfigCacheFieldPage      = "page"       // 配置页面字段
	sysConfigCacheFieldPid       = "pid"        // 配置上级 ID 字段
	sysConfigCacheFieldPids      = "pids"       // 配置族谱字段
	sysConfigCacheFieldVersion   = "version"    // 配置版本字段
	sysConfigCacheFieldUpdatedAt = "updated_at" // 配置更新时间字段
	sysConfigCacheFieldEmpty     = "empty"      // 空值占位字段

	sysConfigEmptyValue       = "__EMPTY__"           // 空值缓存占位符
	sysConfigCacheTTL         = 10 * time.Minute      // 正常配置缓存 TTL
	sysConfigEmptyCacheTTL    = 2 * time.Minute       // 不存在配置的空值缓存 TTL
	sysConfigCacheWaitStep    = 50 * time.Millisecond // 锁竞争时等待缓存写入的单次间隔
	sysConfigCacheWaitRetries = 5                     // 锁竞争时等待缓存写入的重试次数
)

// ErrSysConfigNotFound 表示指定 uuid 的系统配置不存在。
var ErrSysConfigNotFound = errors.New("系统配置不存在")

// sysConfigCacheEntry 表示 Redis Hash 中保存的系统配置快照。
type sysConfigCacheEntry struct {
	ID        int    // 配置 ID
	UUID      string // 配置 uuid
	Title     string // 配置标题
	Type      int    // 配置类型
	Value     string // 配置值
	Example   string // 配置示例
	Remark    string // 配置备注
	Page      string // 配置页面
	Pid       int    // 上级 ID
	Pids      string // 上级 ID 族谱
	Version   int    // 版本号
	UpdatedAt string // 更新时间
}

// SysConfigLogic 承载系统配置缓存读取与刷新能力。
type SysConfigLogic struct {
	*BaseLogic // 复用上下文、数据库、Redis 和日志能力
}

// NewSysConfigLogic 创建系统配置业务逻辑对象。
func NewSysConfigLogic(ctx context.Context, svcCtx *svc.ServiceContext) *SysConfigLogic {
	return &SysConfigLogic{BaseLogic: NewBaseLogicWithContext(ctx, svcCtx)}
}

// GetCachedValue 读取指定配置值，优先使用 Redis 缓存，缺失时回源主库重建缓存。
func (l *SysConfigLogic) GetCachedValue(uuid string) (any, error) {
	entry, err := l.getCachedEntry(uuid)
	if err != nil {
		return nil, errors.Tag(err)
	}
	return decodeSysConfigValue(entry.Type, entry.Value)
}

// getCachedEntry 读取指定配置缓存快照，缺失时回源主库重建缓存。
func (l *SysConfigLogic) getCachedEntry(uuid string) (*sysConfigCacheEntry, error) {
	uuid = strings.TrimSpace(uuid)
	if uuid == "" {
		return nil, errors.Errorf("系统配置 uuid 不能为空")
	}
	entry, err := l.readSysConfigCache(uuid)
	if err == nil {
		return entry, nil
	}
	if errors.Is(err, errCacheEmptyMarker) {
		return nil, ErrSysConfigNotFound
	}
	if err != nil && !errors.Is(err, redis.Nil) {
		return nil, errors.Wrapf(err, "读取系统配置缓存失败 uuid=%s", uuid)
	}

	entry, err = l.loadSysConfigThroughCache(uuid)
	if err != nil {
		return nil, errors.Tag(err)
	}
	return entry, nil
}

// RenewByUUID 删除并重新加载指定配置缓存。
func (l *SysConfigLogic) RenewByUUID(uuid string) error {
	uuid = strings.TrimSpace(uuid)
	if uuid == "" {
		return errors.Errorf("系统配置 uuid 不能为空")
	}
	if l.Redis() == nil {
		return errors.Errorf("Redis 未初始化")
	}
	if err := l.RdsDelKeys(l.sysConfigCacheKey(uuid)); err != nil {
		return errors.Wrapf(err, "删除系统配置缓存失败 uuid=%s", uuid)
	}
	_, err := l.loadSysConfigThroughCache(uuid)
	return errors.Tag(err)
}

// GetCacheHash 读取指定系统配置缓存原始 Hash 数据。
func (l *SysConfigLogic) GetCacheHash(uuid string) (map[string]string, error) {
	uuid = strings.TrimSpace(uuid)
	if uuid == "" {
		return nil, errors.Errorf("系统配置 uuid 不能为空")
	}
	if l.Redis() == nil {
		return nil, errors.Errorf("Redis 未初始化")
	}
	return l.Redis().HGetAll(l.Context(), l.sysConfigCacheKey(uuid)).Result()
}

// loadSysConfigThroughCache 在 redsync 锁保护下回源数据库并刷新缓存。
func (l *SysConfigLogic) loadSysConfigThroughCache(uuid string) (*sysConfigCacheEntry, error) {
	if l.Redis() == nil {
		return nil, errors.Errorf("Redis 未初始化")
	}
	cacheKey := l.sysConfigCacheKey(uuid)
	var entry *sysConfigCacheEntry
	err := redislock.WithLock(l.Context(), l.Redis(), l.cacheLockKey(cacheKey), cacheRebuildLockTTL, func(ctx context.Context) error {
		// 拿到锁后再读一次缓存，避免等待锁期间其它实例已经完成回源。
		cached, err := l.readSysConfigCache(uuid)
		if err == nil {
			entry = cached
			return nil
		}
		if err != nil && !errors.Is(err, redis.Nil) && !errors.Is(err, errCacheEmptyMarker) {
			return errors.Tag(err)
		}
		cfg, err := l.loadSysConfigFromDB(ctx, uuid)
		if err != nil {
			if errors.Is(err, ErrSysConfigNotFound) {
				return errors.Tag(l.writeEmptySysConfigCache(ctx, uuid))
			}
			return errors.Tag(err)
		}
		entry = sysConfigModelToCacheEntry(cfg)
		return errors.Tag(l.writeSysConfigCache(ctx, cacheKey, entry))
	})
	if redislock.IsLockTaken(err) {
		entry, err = l.waitSysConfigCache(uuid)
		if err != nil {
			return nil, errors.Tag(err)
		}
		return entry, nil
	}
	if err != nil {
		return nil, errors.Tag(err)
	}
	if entry == nil {
		return nil, ErrSysConfigNotFound
	}
	return entry, nil
}

// waitSysConfigCache 等待其它实例完成同一配置的缓存重建。
func (l *SysConfigLogic) waitSysConfigCache(uuid string) (*sysConfigCacheEntry, error) {
	var lastErr error
	for i := 0; i < sysConfigCacheWaitRetries; i++ {
		time.Sleep(sysConfigCacheWaitStep)
		entry, err := l.readSysConfigCache(uuid)
		if err == nil {
			return entry, nil
		}
		lastErr = err
		if errors.Is(err, errCacheEmptyMarker) {
			return nil, ErrSysConfigNotFound
		}
		if err != nil && !errors.Is(err, redis.Nil) {
			return nil, errors.Tag(err)
		}
	}
	if lastErr == nil || errors.Is(lastErr, redis.Nil) {
		return nil, redislock.ErrLockTaken
	}
	return nil, errors.Tag(lastErr)
}

// readSysConfigCache 从 Redis Hash 读取配置快照。
func (l *SysConfigLogic) readSysConfigCache(uuid string) (*sysConfigCacheEntry, error) {
	if l.Redis() == nil {
		return nil, errors.Errorf("Redis 未初始化")
	}
	values, err := l.Redis().HGetAll(l.Context(), l.sysConfigCacheKey(uuid)).Result()
	if err != nil {
		return nil, errors.Tag(err)
	}
	if len(values) == 0 {
		return nil, redis.Nil
	}
	if values[sysConfigCacheFieldEmpty] == "1" || values[sysConfigCacheFieldValue] == sysConfigEmptyValue {
		return nil, errCacheEmptyMarker
	}
	entry, err := sysConfigCacheEntryFromHash(values)
	if err != nil {
		return nil, errors.Tag(err)
	}
	return entry, nil
}

// loadSysConfigFromDB 从主库读取配置，避免运行期配置刷新后的读延迟。
func (l *SysConfigLogic) loadSysConfigFromDB(ctx context.Context, uuid string) (*model.SysConfig, error) {
	if l == nil || l.svc == nil {
		return nil, errors.Errorf("服务上下文未初始化")
	}
	writeDB := l.svc.WriteDB(svc.DatabaseMain)
	if writeDB == nil {
		return nil, errors.Errorf("main 主库未初始化")
	}
	var cfg model.SysConfig
	if err := writeDB.WithContext(ctx).Where("uuid = ?", uuid).First(&cfg).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrSysConfigNotFound
		}
		return nil, errors.Wrapf(err, "查询系统配置失败 uuid=%s", uuid)
	}
	return &cfg, nil
}

// writeSysConfigCache 写入系统配置 Hash 缓存。
func (l *SysConfigLogic) writeSysConfigCache(ctx context.Context, cacheKey string, entry *sysConfigCacheEntry) error {
	if entry == nil {
		return errors.Errorf("系统配置缓存快照为空")
	}
	values := map[string]any{
		sysConfigCacheFieldID:        entry.ID,
		sysConfigCacheFieldUUID:      entry.UUID,
		sysConfigCacheFieldTitle:     entry.Title,
		sysConfigCacheFieldType:      entry.Type,
		sysConfigCacheFieldValue:     entry.Value,
		sysConfigCacheFieldExample:   entry.Example,
		sysConfigCacheFieldRemark:    entry.Remark,
		sysConfigCacheFieldPage:      entry.Page,
		sysConfigCacheFieldPid:       entry.Pid,
		sysConfigCacheFieldPids:      entry.Pids,
		sysConfigCacheFieldVersion:   entry.Version,
		sysConfigCacheFieldUpdatedAt: entry.UpdatedAt,
	}
	return errors.Tag(l.writeSysConfigHash(ctx, cacheKey, values, sysConfigCacheTTL))
}

// writeEmptySysConfigCache 写入不存在配置的短 TTL 占位。
func (l *SysConfigLogic) writeEmptySysConfigCache(ctx context.Context, uuid string) error {
	values := map[string]any{
		sysConfigCacheFieldUUID:      uuid,
		sysConfigCacheFieldValue:     sysConfigEmptyValue,
		sysConfigCacheFieldEmpty:     "1",
		sysConfigCacheFieldUpdatedAt: time.Now().Format(time.DateTime),
	}
	return errors.Tag(l.writeSysConfigHash(ctx, l.sysConfigCacheKey(uuid), values, sysConfigEmptyCacheTTL))
}

// writeSysConfigHash 原子提交 Hash 字段和 TTL。
func (l *SysConfigLogic) writeSysConfigHash(ctx context.Context, key string, values map[string]any, ttl time.Duration) error {
	pipe := l.Redis().TxPipeline()
	pipe.HSet(ctx, key, values)
	pipe.Expire(ctx, key, jitterTTL(ttl))
	_, err := pipe.Exec(ctx)
	return errors.Tag(err)
}

// sysConfigCacheKey 生成当前站点下的系统配置缓存 Key。
func (l *SysConfigLogic) sysConfigCacheKey(uuid string) string {
	return l.AppRedisKey(fmt.Sprintf(keys.SysConfigUUID, strings.TrimSpace(uuid)))
}

// sysConfigModelToCacheEntry 把系统配置模型转换为缓存快照。
func sysConfigModelToCacheEntry(cfg *model.SysConfig) *sysConfigCacheEntry {
	if cfg == nil {
		return nil
	}
	return &sysConfigCacheEntry{
		ID:        cfg.ID,
		UUID:      cfg.UUID,
		Title:     cfg.Title,
		Type:      cfg.Type,
		Value:     cfg.Value,
		Example:   cfg.Example,
		Remark:    cfg.Remark,
		Page:      cfg.Page,
		Pid:       cfg.Pid,
		Pids:      cfg.Pids,
		Version:   cfg.Version,
		UpdatedAt: formatDateTime(cfg.UpdatedAt),
	}
}

// sysConfigCacheEntryFromHash 把 Redis Hash 转换为缓存快照。
func sysConfigCacheEntryFromHash(values map[string]string) (*sysConfigCacheEntry, error) {
	typ, err := parseSysConfigCacheInt(values, sysConfigCacheFieldType)
	if err != nil {
		return nil, errors.Tag(err)
	}
	id, _ := parseSysConfigCacheInt(values, sysConfigCacheFieldID)
	pid, _ := parseSysConfigCacheInt(values, sysConfigCacheFieldPid)
	version, _ := parseSysConfigCacheInt(values, sysConfigCacheFieldVersion)
	return &sysConfigCacheEntry{
		ID:        id,
		UUID:      values[sysConfigCacheFieldUUID],
		Title:     values[sysConfigCacheFieldTitle],
		Type:      typ,
		Value:     values[sysConfigCacheFieldValue],
		Example:   values[sysConfigCacheFieldExample],
		Remark:    values[sysConfigCacheFieldRemark],
		Page:      values[sysConfigCacheFieldPage],
		Pid:       pid,
		Pids:      values[sysConfigCacheFieldPids],
		Version:   version,
		UpdatedAt: values[sysConfigCacheFieldUpdatedAt],
	}, nil
}

// parseSysConfigCacheInt 解析 Redis Hash 中的整数字段。
func parseSysConfigCacheInt(values map[string]string, field string) (int, error) {
	raw := strings.TrimSpace(values[field])
	if raw == "" {
		return 0, errors.Errorf("系统配置缓存字段[%s]为空", field)
	}
	value, err := strconv.Atoi(raw)
	if err != nil {
		return 0, errors.Wrapf(err, "系统配置缓存字段[%s]不是整数", field)
	}
	return value, nil
}

// decodeSysConfigValue 把缓存中的字符串值还原为业务类型。
func decodeSysConfigValue(typ int, raw string) (any, error) {
	switch typ {
	case model.SysConfigTypeGroup:
		return nil, nil
	case model.SysConfigTypeObject, model.SysConfigTypeArray:
		var value any
		if err := json.Unmarshal([]byte(raw), &value); err != nil {
			return nil, errors.Tag(err)
		}
		return value, nil
	case model.SysConfigTypeString:
		var value string
		if err := json.Unmarshal([]byte(raw), &value); err != nil {
			return raw, nil
		}
		return value, nil
	case model.SysConfigTypeInteger:
		value, err := strconv.Atoi(strings.TrimSpace(raw))
		if err != nil {
			return 0, errors.Tag(err)
		}
		return value, nil
	case model.SysConfigTypeFloat:
		value, err := strconv.ParseFloat(strings.TrimSpace(raw), 64)
		if err != nil {
			return 0, errors.Tag(err)
		}
		return value, nil
	case model.SysConfigTypeBoolean:
		return raw == "1" || strings.EqualFold(raw, "true"), nil
	default:
		return raw, nil
	}
}
