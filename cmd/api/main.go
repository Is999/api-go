package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"api/internal/bootstrap"
	"api/internal/infra/loggerx"
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
	app, err := bootstrap.Wire(ctx, configFile)
	if err != nil {
		loggerx.Errorw(nil, "应用启动装配失败", err)
		return 1
	}
	defer func() {
		// 退出时统一关闭 server、tracer provider、连接池等资源。
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := app.Stop(ctx); err != nil {
			loggerx.Errorw(nil, "应用停止失败", err)
		}
	}()

	if err = app.Start(); err != nil {
		loggerx.Errorw(nil, "应用启动失败", err)
		return 1
	}
	return 0
}
