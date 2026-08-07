package bootstrap

import (
	"context"
	"fmt"
	"net/http"
	"time"

	i18n "api/common/i18n"
	"api/common/idgen"
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

const (
	// httpDrainTimeout 限制发布停机时等待在途 HTTP 请求的最长时间。
	httpDrainTimeout = 5 * time.Second
)

// App 聚合服务运行所需的配置、HTTP Server 和关闭钩子。
type App struct {
	Server         *rest.Server                // HTTP 服务实例
	InternalServer *rest.Server                // 只注册内网路由的独立 HTTP 服务
	ServiceContext *svc.ServiceContext         // 全局服务上下文
	ConfigFile     string                      // 当前应用对应的配置文件路径
	shutdown       func(context.Context) error // tracing 等基础设施关闭钩子
	hotReload      hotreload.State             // 配置热加载运行态资源
	runtimeAlerts  *runtimeAlertSink           // API 运行异常 Lark 告警发送器
	publicHTTP     *httpServerRun              // 公网监听器运行态，用于启动探测和局部失败关闭
	internalHTTP   *httpServerRun              // 内网监听器运行态，用于启动探测和局部失败关闭
}

// New 负责把依赖装配与 HTTP 服务注册串起来。
func New(ctx context.Context, c config.Config, version string) (*App, error) {
	if err := i18n.ValidateCatalog(); err != nil {
		return nil, errors.Wrap(err, "校验内嵌多语言资产失败")
	}
	runtimeAlerts, err := newRuntimeAlertSink(c)
	if err != nil {
		return nil, errors.Tag(err)
	}
	if err := idgen.RegisterMetrics(); err != nil {
		runtimeAlerts.notify(context.Background(), appalert.LifecycleFailure("start", "metrics_registry", err))
		return nil, errors.Wrap(err, "注册 ID 生成指标失败")
	}
	svcCtx, shutdown, err := BuildServiceContext(ctx, c, version)
	if err != nil {
		runtimeAlerts.notify(context.Background(), appalert.LifecycleFailure("start", "build_service_context", err))
		return nil, errors.Tag(err)
	}
	routeModules := handler.BuiltinRouteModules()
	if err := register.ValidateNamesUnique(register.KindRoute, register.RouteModuleNames(routeModules)); err != nil {
		runtimeAlerts.notify(context.Background(), appalert.LifecycleFailure("start", "route_registry", err))
		_ = bootstrapresources.CloseServiceContextResources(ctx, svcCtx)
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
		_ = bootstrapresources.CloseServiceContextResources(ctx, svcCtx)
		if shutdown != nil {
			_ = shutdown(context.Background())
		}
		return nil, errors.Wrapf(err, "创建 HTTP 服务失败 host=%s port=%d", restConf.Host, restConf.Port)
	}
	internalServer, err := newInternalServer(c)
	if err != nil {
		_ = bootstrapresources.CloseServiceContextResources(ctx, svcCtx)
		if shutdown != nil {
			_ = shutdown(context.Background())
		}
		return nil, errors.Tag(err)
	}
	app := &App{
		Server:         server,
		InternalServer: internalServer,
		ServiceContext: svcCtx,
		shutdown:       shutdown,
		runtimeAlerts:  runtimeAlerts,
		publicHTTP:     newHTTPServerRun("公网", c.Host, c.Port, server),
		internalHTTP:   newHTTPServerRun("内网", c.InternalServer.Host, c.InternalServer.Port, internalServer),
	}
	svcCtx.ConfigReload = app
	app.bindCollectorRuntimeAlerts()
	if err := handler.RegisterPublicHandlersWithModules(server, svcCtx, routeModules...); err != nil {
		runtimeAlerts.notify(context.Background(), appalert.LifecycleFailure("start", "route_registry", err))
		_ = bootstrapresources.CloseServiceContextResources(ctx, svcCtx)
		if shutdown != nil {
			_ = shutdown(context.Background())
		}
		return nil, errors.Wrap(err, "注册公网 HTTP 路由失败")
	}
	if err := handler.RegisterInternalHandlersWithModules(internalServer, svcCtx, routeModules...); err != nil {
		runtimeAlerts.notify(context.Background(), appalert.LifecycleFailure("start", "route_registry", err))
		_ = bootstrapresources.CloseServiceContextResources(ctx, svcCtx)
		if shutdown != nil {
			_ = shutdown(context.Background())
		}
		return nil, errors.Wrap(err, "注册内网 HTTP 路由失败")
	}
	return app, nil
}

// Start 启动 HTTP 服务。
func (a *App) Start() error {
	if a == nil || a.Server == nil || a.InternalServer == nil {
		err := errors.Errorf("HTTP 服务未初始化")
		if a != nil {
			a.notifyLifecycleFailure(context.Background(), "start", "http_server", err)
		}
		return err
	}
	a.startConfigHotReload()
	cfg := a.ServiceContext.CurrentConfig()
	loggerx.Infow(context.Background(), "应用 HTTP 服务开始监听",
		logx.Field("service", cfg.Name),
		logx.Field("host", fmt.Sprintf("%s:%d", cfg.Host, cfg.Port)),
		logx.Field("internal_host", fmt.Sprintf("%s:%d", cfg.InternalServer.Host, cfg.InternalServer.Port)),
		logx.Field("mode", cfg.Mode),
		logx.Field("version", a.ServiceContext.CurrentVersion()),
	)
	err := runHTTPServers([]*httpServerRun{a.internalHTTP, a.publicHTTP}, httpDrainTimeout)
	if err != nil {
		a.notifyLifecycleFailure(context.Background(), "start", "http_server", err)
	}
	return errors.Tag(err)
}

// limitHTTPDrain 到期后关闭仍未结束的连接，确保后续资源关闭可以继续执行。
func limitHTTPDrain(server *http.Server, timeout time.Duration, done <-chan struct{}) {
	if server == nil || timeout <= 0 {
		return
	}
	server.RegisterOnShutdown(func() {
		timer := time.NewTimer(timeout)
		defer timer.Stop()
		select {
		case <-timer.C:
			_ = server.Close()
		case <-done:
		}
	})
}

// Stop 释放服务资源。
func (a *App) Stop(ctx context.Context) error {
	if a == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	var firstErr error
	recordErr := func(err error) {
		if err != nil && firstErr == nil {
			firstErr = errors.Tag(err)
		}
	}
	recordErr(shutdownHTTPServers(ctx, a.publicHTTP, a.internalHTTP))
	if a.Server != nil {
		a.Server.Stop()
	}
	if a.InternalServer != nil {
		a.InternalServer.Stop()
	}
	recordErr(a.stopConfigHotReload(ctx))
	recordErr(bootstrapresources.CloseServiceContextResources(ctx, a.ServiceContext))
	if a.shutdown != nil {
		recordErr(a.shutdown(ctx))
	}
	if firstErr != nil {
		a.notifyLifecycleFailure(ctx, "stop", "resources", firstErr)
	}
	return firstErr
}
