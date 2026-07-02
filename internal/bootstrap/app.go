package bootstrap

import (
	"context"
	"fmt"

	"api/internal/bootstrap/appalert"
	"api/internal/bootstrap/hotreload"
	"api/internal/bootstrap/register"
	bootstrapresources "api/internal/bootstrap/resources"
	"api/internal/config"
	"api/internal/handler"
	"api/internal/infra/loggerx"
	"api/internal/svc"

	"github.com/Is999/go-utils/errors"
	"github.com/zeromicro/go-zero/core/logx"
	"github.com/zeromicro/go-zero/rest"
)

// App 聚合服务运行所需的配置、HTTP Server 和关闭钩子。
type App struct {
	Server         *rest.Server                // HTTP 服务实例
	ServiceContext *svc.ServiceContext         // 全局服务上下文
	ConfigFile     string                      // 当前应用对应的配置文件路径
	shutdown       func(context.Context) error // tracing 等基础设施关闭钩子
	hotReload      hotreload.State             // 配置热加载运行态资源
	runtimeAlerts  *runtimeAlertSink           // API 运行异常 Lark 告警发送器
}

// New 负责把依赖装配与 HTTP 服务注册串起来。
func New(ctx context.Context, c config.Config, version string) (*App, error) {
	runtimeAlerts, err := newRuntimeAlertSink(c)
	if err != nil {
		return nil, errors.Tag(err)
	}
	svcCtx, shutdown, err := BuildServiceContext(ctx, c, version)
	if err != nil {
		runtimeAlerts.notify(context.Background(), appalert.LifecycleFailure("start", "build_service_context", err))
		return nil, errors.Tag(err)
	}
	routeModules := handler.BuiltinRouteModules()
	if err := register.ValidateNamesUnique(register.KindRoute, register.RouteModuleNames(routeModules)); err != nil {
		runtimeAlerts.notify(context.Background(), appalert.LifecycleFailure("start", "route_registry", err))
		_ = bootstrapresources.CloseServiceContextResources(svcCtx)
		if shutdown != nil {
			_ = shutdown(context.Background())
		}
		return nil, errors.Tag(err)
	}

	restConf := c.RestConf
	// 项目已接入自定义 access log 中间件，关闭 go-zero 默认 HTTP 日志。
	restConf.Middlewares.Log = false
	server, err := rest.NewServer(restConf)
	if err != nil {
		runtimeAlerts.notify(context.Background(), appalert.LifecycleFailure("start", "http_server", err))
		_ = bootstrapresources.CloseServiceContextResources(svcCtx)
		if shutdown != nil {
			_ = shutdown(context.Background())
		}
		return nil, errors.Wrapf(err, "创建 HTTP 服务失败 host=%s port=%d", restConf.Host, restConf.Port)
	}
	app := &App{
		Server:         server,
		ServiceContext: svcCtx,
		shutdown:       shutdown,
		runtimeAlerts:  runtimeAlerts,
	}
	svcCtx.ConfigReload = app
	app.bindCollectorRuntimeAlerts()
	handler.RegisterHandlersWithModules(server, svcCtx, routeModules...)
	return app, nil
}

// Start 启动 HTTP 服务。
func (a *App) Start() error {
	if a == nil || a.Server == nil {
		err := errors.Errorf("HTTP 服务未初始化")
		if a != nil {
			a.notifyLifecycleFailure(context.Background(), "start", "http_server", err)
		}
		return err
	}
	a.startConfigHotReload()
	cfg := a.ServiceContext.CurrentConfig()
	loggerx.Infow(context.Background(), "应用 服务已启动",
		logx.Field("service", cfg.Name),
		logx.Field("host", fmt.Sprintf("%s:%d", cfg.Host, cfg.Port)),
		logx.Field("mode", cfg.Mode),
		logx.Field("version", a.ServiceContext.CurrentVersion()),
	)
	a.Server.Start()
	return nil
}

// Stop 释放服务资源。
func (a *App) Stop(ctx context.Context) error {
	if a == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if a.Server != nil {
		a.Server.Stop()
	}
	a.stopConfigHotReload()
	var firstErr error
	recordErr := func(err error) {
		if err != nil && firstErr == nil {
			firstErr = errors.Tag(err)
		}
	}
	recordErr(bootstrapresources.CloseServiceContextResources(a.ServiceContext))
	if a.shutdown != nil {
		recordErr(a.shutdown(ctx))
	}
	if firstErr != nil {
		a.notifyLifecycleFailure(ctx, "stop", "resources", firstErr)
	}
	return firstErr
}
