package bootstrap

import (
	"context"

	"api/common/runtimecfg"
	"api/internal/bootstrap/components"
	bootstrapresources "api/internal/bootstrap/resources"
	"api/internal/config"
	"api/internal/infra/collectorx"
	"api/internal/svc"

	"github.com/Is999/go-utils/errors"
)

// BuildServiceContext 统一完成基础设施初始化，并发布当前进程运行配置快照。
func BuildServiceContext(ctx context.Context, c config.Config, version string) (*svc.ServiceContext, func(context.Context) error, error) {
	svcCtx, shutdown, err := bootstrapresources.BuildServiceContext(ctx, c, version)
	if err != nil {
		return nil, nil, errors.Tag(err)
	}
	previousRuntime := publishRuntimeConfig(c)
	collectorManager, err := collectorx.New(c.Collector)
	if err != nil {
		runtimecfg.Restore(previousRuntime)
		_ = bootstrapresources.CloseServiceContextResources(svcCtx)
		if shutdown != nil {
			_ = shutdown(context.Background())
		}
		return nil, nil, errors.Tag(err)
	}
	svcCtx.Collector = collectorManager
	componentRegistry, err := components.NewRegistry(svcCtx)
	if err != nil {
		runtimecfg.Restore(previousRuntime)
		_ = bootstrapresources.CloseServiceContextResources(svcCtx)
		if shutdown != nil {
			_ = shutdown(context.Background())
		}
		return nil, nil, errors.Wrapf(err, "构建组件生命周期注册表失败")
	}
	svcCtx.SetComponentRegistry(componentRegistry)
	return svcCtx, shutdown, nil
}
