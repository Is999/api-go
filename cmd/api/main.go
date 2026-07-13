package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"api/internal/bootstrap"
	"api/internal/infra/loggerx"

	"github.com/zeromicro/go-zero/core/proc"
)

const (
	// shutdownTimeout 为 HTTP 排空后的基础设施关闭预留时间。
	shutdownTimeout = 20 * time.Second
	// forceQuitTimeout 必须晚于应用停止期限，并早于容器默认 30 秒终止宽限期。
	forceQuitTimeout = 29 * time.Second
)

// configFile 支持通过 -f 指定配置文件，便于区分本地、测试和线上环境。
var configFile = flag.String("f", "./etc/config.yaml", "the config file")

// buildVersion 由构建阶段通过 -ldflags 注入，用于发布排查。
var buildVersion = "dev"

// showVersion 控制是否只输出二进制版本并退出。
var showVersion = flag.Bool("version", false, "print build version and exit")

// main 解析启动参数并按 runApp 退出码结束进程。
func main() {
	flag.Parse()
	if *showVersion {
		fmt.Println(buildVersion)
		return
	}
	os.Exit(runApp(context.Background(), *configFile))
}

// runApp 执行应用装配、启动和停止，并返回进程退出码。
func runApp(ctx context.Context, configFile string) int {
	if ctx == nil {
		ctx = context.Background()
	}
	proc.SetTimeToForceQuit(forceQuitTimeout)
	app, err := bootstrap.Wire(ctx, configFile)
	if err != nil {
		loggerx.Errorw(ctx, "应用启动装配失败", err)
		return 1
	}
	defer func() {
		// 退出时统一关闭 server、tracer provider、连接池等资源。
		stopCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()
		if err := app.Stop(stopCtx); err != nil {
			loggerx.Errorw(stopCtx, "应用停止失败", err)
		}
	}()

	if err = app.Start(); err != nil {
		loggerx.Errorw(ctx, "应用启动失败", err)
		return 1
	}
	return 0
}
