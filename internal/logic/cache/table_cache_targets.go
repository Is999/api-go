package cache

import (
	"context"
	"time"

	keys "api/common/rediskeys"
	corelogic "api/internal/logic"
	"api/internal/model"
	"api/internal/svc"

	"github.com/Is999/go-utils/errors"
	tablecache "github.com/Is999/table-cache"
	"gorm.io/gorm"
)

// tableCacheTargets 返回 API 与 admin 共享的 table-cache 缓存目标。
func tableCacheTargets(base *corelogic.BaseLogic) []tablecache.Target {
	return []tablecache.Target{
		{
			Index:            "config_uuid",
			Title:            "系统常量配置",
			Key:              cacheTemplatePrefix(keys.SysConfigUUIDPattern),
			KeyTitle:         keys.SysConfigUUIDPattern,
			Type:             tablecache.TypeHash,
			Remark:           "系统常量配置缓存",
			TTL:              time.Hour,
			AllowEmptyMarker: true,
			Loader:           loadSysConfigTableCache(base),
		},
	}
}

// loadSysConfigTableCache 加载单个系统配置 Hash 缓存数据。
func loadSysConfigTableCache(base *corelogic.BaseLogic) tablecache.Loader {
	return func(ctx context.Context, params tablecache.LoadParams) ([]tablecache.Entry, error) {
		uuid, err := tableCacheFirstStringPart(params, "配置UUID")
		if err != nil {
			return nil, errors.Tag(err)
		}
		writeDB, err := tableCacheWriteDB(base, svc.DatabaseMain, "main")
		if err != nil {
			return nil, errors.Tag(err)
		}
		var cfg model.SysConfig
		if err := writeDB.WithContext(ctx).Where("uuid = ?", uuid).First(&cfg).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil, nil
			}
			return nil, errors.Tag(err)
		}
		cache := map[string]any{
			"id":        cfg.ID,
			"uuid":      cfg.UUID,
			"title":     cfg.Title,
			"type":      cfg.Type,
			"value":     cfg.Value,
			"example":   cfg.Example,
			"remark":    cfg.Remark,
			"page":      cfg.Page,
			"pid":       cfg.Pid,
			"pids":      cfg.Pids,
			"version":   cfg.Version,
			"updatedAt": corelogic.FormatDateTime(cfg.UpdatedAt),
		}
		return []tablecache.Entry{{
			Key:   params.Key,
			Type:  tablecache.TypeHash,
			Value: cache,
		}}, nil
	}
}
