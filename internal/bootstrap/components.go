package bootstrap

import (
	"context"
	"database/sql"
	"sort"

	codes "api/common/codes"
	"api/internal/svc"

	"github.com/Is999/go-utils/errors"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

// 默认组件生命周期名称。
const (
	componentNameServiceContext = "service_context" // ServiceContext 为空时的兜底组件
	componentNameMySQL          = "mysql"           // 默认主库组件
	componentNameRedis          = "redis"           // 默认 Redis 组件
	componentSourceSiteMySQL    = "site_mysql"      // 命名扩展库组件来源
)

// gormDBCloseGuard 避免同一 GORM 连接池被重复关闭。
type gormDBCloseGuard struct {
	closed map[*gorm.DB]struct{} // 已关闭的连接池
}

// componentBuildContext 表示构造默认组件时需要的共享依赖。
type componentBuildContext struct {
	ServiceContext *svc.ServiceContext // 全局服务上下文
	CloseGuard     *gormDBCloseGuard   // MySQL 连接池关闭去重器
}

// componentSpec 描述一个默认组件来源及其构造逻辑。
type componentSpec struct {
	registrationSpec                                             // 组件来源在默认注册清单中的说明字段
	Build            func(componentBuildContext) []svc.Component // 构造该来源下的实际组件
}

// buildDefaultComponentRegistry 构造启动期核心组件注册表。
func buildDefaultComponentRegistry(svcCtx *svc.ServiceContext) (*svc.ComponentRegistry, error) {
	if svcCtx == nil {
		return svc.NewComponentRegistry(serviceContextComponent())
	}

	closeGuard := &gormDBCloseGuard{closed: make(map[*gorm.DB]struct{}, 4)}
	components := defaultComponents(componentBuildContext{
		ServiceContext: svcCtx,
		CloseGuard:     closeGuard,
	})
	return svc.NewComponentRegistry(components...)
}

// defaultComponentSpecs 返回默认组件生命周期来源，顺序即注册和关闭顺序。
func defaultComponentSpecs() []componentSpec {
	return []componentSpec{
		{
			registrationSpec: registrationSpec{
				Name:        componentNameMySQL,
				File:        "internal/bootstrap/components.go",
				Method:      "mysqlComponent / buildDefaultComponentRegistry",
				Description: "注册默认主库健康探测和关闭入口",
			},
			Build: func(ctx componentBuildContext) []svc.Component {
				if ctx.ServiceContext == nil {
					return nil
				}
				return []svc.Component{mysqlComponent(componentNameMySQL, ctx.ServiceContext.SiteDBs.MainDB, ctx.CloseGuard)}
			},
		},
		{
			registrationSpec: registrationSpec{
				Name:        componentSourceSiteMySQL,
				File:        "internal/bootstrap/components.go",
				Method:      "siteMySQLComponents / buildDefaultComponentRegistry",
				Description: "按名称注册扩展库健康探测和关闭入口",
			},
			Build: siteMySQLComponents,
		},
		{
			registrationSpec: registrationSpec{
				Name:        componentNameRedis,
				File:        "internal/bootstrap/components.go",
				Method:      "redisComponent / buildDefaultComponentRegistry",
				Description: "注册默认 Redis 健康探测和关闭入口",
			},
			Build: func(ctx componentBuildContext) []svc.Component {
				if ctx.ServiceContext == nil {
					return nil
				}
				return []svc.Component{redisComponent(ctx.ServiceContext.Rds)}
			},
		},
	}
}

// defaultComponents 从默认组件规格派生组件生命周期清单。
func defaultComponents(ctx componentBuildContext) []svc.Component {
	specs := defaultComponentSpecs()
	components := make([]svc.Component, 0, len(specs))
	for _, spec := range specs {
		if spec.Build == nil {
			continue
		}
		components = append(components, spec.Build(ctx)...)
	}
	return components
}

// siteMySQLComponents 按名称排序生成扩展库组件，保证启动与关闭顺序稳定。
func siteMySQLComponents(ctx componentBuildContext) []svc.Component {
	if ctx.ServiceContext == nil {
		return nil
	}
	names := make([]string, 0, len(ctx.ServiceContext.SiteDBs.NamedDBs))
	for name := range ctx.ServiceContext.SiteDBs.NamedDBs {
		names = append(names, string(name))
	}
	sort.Strings(names)
	components := make([]svc.Component, 0, len(names))
	for _, name := range names {
		db := ctx.ServiceContext.SiteDBs.NamedDBs[svc.DbName(name)]
		components = append(components, mysqlComponent("mysql_"+name, db, ctx.CloseGuard))
	}
	return components
}

// serviceContextComponent 返回 ServiceContext 缺失时的启动健康兜底组件。
func serviceContextComponent() svc.Component {
	return svc.Component{
		Name:      componentNameServiceContext,
		ErrorCode: codes.DependencyUnavailable,
		Check: func(context.Context) error {
			return errors.Errorf("ServiceContext未初始化")
		},
	}
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
		Name:      componentNameRedis,
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
