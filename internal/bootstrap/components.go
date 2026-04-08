package bootstrap

import (
	"context"
	"database/sql"
	"sort"

	codes "gozero_api/common/codes"
	"gozero_api/internal/svc"

	"github.com/Is999/go-utils/errors"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

// gormDBCloseGuard 避免同一 GORM 连接池被重复关闭。
type gormDBCloseGuard struct {
	closed map[*gorm.DB]struct{} // 已关闭的连接池
}

// buildDefaultComponentRegistry 构造启动期核心组件注册表。
func buildDefaultComponentRegistry(svcCtx *svc.ServiceContext) (*svc.ComponentRegistry, error) {
	if svcCtx == nil {
		return svc.NewComponentRegistry(svc.Component{
			Name:      "service_context",
			ErrorCode: codes.DependencyUnavailable,
			Check: func(context.Context) error {
				return errors.Errorf("ServiceContext未初始化")
			},
		})
	}

	closeGuard := &gormDBCloseGuard{closed: make(map[*gorm.DB]struct{}, 4)}
	components := make([]svc.Component, 0, len(svcCtx.SiteDBs.NamedDBs)+2)
	components = append(components, mysqlComponent("mysql", svcCtx.SiteDBs.MainDB, closeGuard))

	names := make([]string, 0, len(svcCtx.SiteDBs.NamedDBs))
	for name := range svcCtx.SiteDBs.NamedDBs {
		names = append(names, string(name))
	}
	sort.Strings(names)
	for _, name := range names {
		db := svcCtx.SiteDBs.NamedDBs[svc.DbName(name)]
		components = append(components, mysqlComponent("mysql_"+name, db, closeGuard))
	}
	components = append(components, redisComponent(svcCtx.Rds))
	return svc.NewComponentRegistry(components...)
}

// mysqlComponent 创建 MySQL 组件探测和释放入口。
func mysqlComponent(name string, db *gorm.DB, closeGuard *gormDBCloseGuard) svc.Component {
	return svc.Component{
		Name:      name,
		ErrorCode: codes.MySQLUnavailable,
		Check: func(ctx context.Context) error {
			return errors.Tag(checkGormDB(ctx, db))
		},
		Close: func() error {
			return closeGuard.close(name, db)
		},
	}
}

// redisComponent 创建 Redis 组件探测和释放入口。
func redisComponent(rds redis.UniversalClient) svc.Component {
	return svc.Component{
		Name:      "redis",
		ErrorCode: codes.RedisUnavailable,
		Check: func(ctx context.Context) error {
			if rds == nil {
				return errors.Errorf("Redis客户端未初始化")
			}
			return errors.Tag(rds.Ping(ctx).Err())
		},
		Close: func() error {
			if rds == nil {
				return nil
			}
			return errors.Tag(rds.Close())
		},
	}
}

// checkGormDB 将 GORM 连接转换为底层连接池并执行 PING。
func checkGormDB(ctx context.Context, db *gorm.DB) error {
	if db == nil {
		return errors.Errorf("数据库连接未初始化")
	}
	sqlDB, err := db.DB()
	if err != nil {
		return errors.Wrap(err, "数据库连接池不可用")
	}
	return errors.Tag(checkSQLDB(ctx, sqlDB))
}

// checkSQLDB 探测 SQL 连接池。
func checkSQLDB(ctx context.Context, db *sql.DB) error {
	if db == nil {
		return errors.Errorf("数据库连接池未初始化")
	}
	if err := db.PingContext(ctx); err != nil {
		return errors.Wrap(err, "数据库PING失败")
	}
	return nil
}

// close 去重关闭 GORM 底层连接池。
func (g *gormDBCloseGuard) close(name string, db *gorm.DB) error {
	if g == nil || db == nil {
		return nil
	}
	if _, ok := g.closed[db]; ok {
		return nil
	}
	g.closed[db] = struct{}{}
	sqlDB, err := db.DB()
	if err != nil {
		return errors.Wrapf(err, "获取 MySQL[%s]底层连接池失败", name)
	}
	if sqlDB == nil {
		return nil
	}
	if err = sqlDB.Close(); err != nil {
		return errors.Wrapf(err, "关闭 MySQL[%s]连接池失败", name)
	}
	return nil
}

// componentNames 提取组件名称，供注册清单和测试复用。
func componentNames(registry *svc.ComponentRegistry) []string {
	items := registry.Items()
	names := make([]string, 0, len(items))
	for _, item := range items {
		names = append(names, item.Name)
	}
	sort.Strings(names)
	return names
}
