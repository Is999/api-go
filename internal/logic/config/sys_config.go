package config

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	keys "api/common/rediskeys"
	corelogic "api/internal/logic"
	cachelogic "api/internal/logic/cache"
	"api/internal/model"
	"api/internal/svc"

	"github.com/Is999/go-utils/errors"
	tablecache "github.com/Is999/table-cache"
)

// 系统配置 table-cache Hash 字段。
const (
	sysConfigCacheFieldID        = "id"        // 配置 ID 字段
	sysConfigCacheFieldUUID      = "uuid"      // 配置 uuid 字段
	sysConfigCacheFieldTitle     = "title"     // 配置标题字段
	sysConfigCacheFieldType      = "type"      // 配置类型字段
	sysConfigCacheFieldValue     = "value"     // 配置值字段
	sysConfigCacheFieldExample   = "example"   // 配置示例字段
	sysConfigCacheFieldRemark    = "remark"    // 配置备注字段
	sysConfigCacheFieldPage      = "page"      // 配置页面字段
	sysConfigCacheFieldPid       = "pid"       // 配置上级 ID 字段
	sysConfigCacheFieldPids      = "pids"      // 配置族谱字段
	sysConfigCacheFieldVersion   = "version"   // 配置版本字段
	sysConfigCacheFieldUpdatedAt = "updatedAt" // 配置更新时间字段
)

// ErrSysConfigNotFound 表示指定 uuid 的系统配置不存在。
var ErrSysConfigNotFound = errors.New("系统配置不存在")

// SysConfigLogic 承载系统配置缓存读取与刷新能力。
type SysConfigLogic struct {
	*corelogic.BaseLogic // 复用上下文、数据库、Redis 和日志能力
}

// NewSysConfigLogic 创建系统配置业务逻辑对象。
func NewSysConfigLogic(ctx context.Context, svcCtx *svc.ServiceContext) *SysConfigLogic {
	return &SysConfigLogic{BaseLogic: corelogic.NewBaseLogicWithContext(ctx, svcCtx)}
}

// GetCachedValue 读取指定配置值，优先使用 Redis 缓存，缺失时回源主库重建缓存。
func (l *SysConfigLogic) GetCachedValue(uuid string) (any, error) {
	cache, err := l.getCachedEntry(uuid)
	if err != nil {
		return nil, errors.Tag(err)
	}
	typ, err := sysConfigCacheType(uuid, cache)
	if err != nil {
		return nil, errors.Tag(err)
	}
	return decodeSysConfigValue(typ, cache[sysConfigCacheFieldValue])
}

// getCachedEntry 读取指定配置缓存快照，缺失时回源主库重建缓存。
func (l *SysConfigLogic) getCachedEntry(uuid string) (map[string]string, error) {
	uuid = strings.TrimSpace(uuid)
	if uuid == "" {
		return nil, errors.Errorf("系统配置 uuid 不能为空")
	}
	manager, err := cachelogic.TableCacheManager(l.BaseLogic)
	if err != nil {
		return nil, errors.Tag(err)
	}
	var cache map[string]string
	result, err := manager.LoadThrough(l.Ctx, l.sysConfigCacheKey(uuid), &cache, nil)
	if err != nil {
		return nil, errors.Tag(err)
	}
	if result.State == tablecache.LookupStateEmpty || len(cache) == 0 {
		return nil, ErrSysConfigNotFound
	}
	if corelogic.CacheIsEmptyMarker(cache[sysConfigCacheFieldValue]) {
		return nil, ErrSysConfigNotFound
	}
	return cache, nil
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
	manager, err := cachelogic.TableCacheManager(l.BaseLogic)
	if err != nil {
		return errors.Tag(err)
	}
	return manager.RefreshByKey(l.Ctx, l.sysConfigCacheKey(uuid))
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
	return l.Redis().HGetAll(l.Ctx, l.sysConfigCacheKey(uuid)).Result()
}

// sysConfigCacheKey 生成当前站点下的系统配置缓存 Key。
func (l *SysConfigLogic) sysConfigCacheKey(uuid string) string {
	return cachelogic.TableCachePhysicalKey(l.BaseLogic, fmt.Sprintf(keys.SysConfigUUID, strings.TrimSpace(uuid)))
}

// sysConfigCacheType 解析系统配置缓存声明类型。
func sysConfigCacheType(uuid string, cache map[string]string) (int, error) {
	raw := strings.TrimSpace(cache[sysConfigCacheFieldType])
	if raw == "" {
		return 0, errors.Errorf("系统配置缓存类型为空 uuid=%s", uuid)
	}
	typ, err := strconv.Atoi(raw)
	if err != nil {
		return 0, errors.Wrapf(err, "系统配置缓存类型非法 uuid=%s", uuid)
	}
	return typ, nil
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
