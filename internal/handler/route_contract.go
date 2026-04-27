package handler

import (
	"net/http"

	confighandler "api/internal/handler/config"
	"api/internal/handler/shared"
)

// 接口文档路径常量，用于路由契约和文档测试防漂移。
const (
	// docHealth 表示前台健康检查接口文档路径。
	docHealth = "docs/site/接口文档/前台系统/健康检查接口.md"
	// docAuth 表示前台认证接口文档路径。
	docAuth = "docs/site/接口文档/前台系统/认证接口.md"
	// docUser 表示前台用户接口文档路径。
	docUser = "docs/site/接口文档/前台系统/用户接口.md"
	// docSystem 表示前台系统接口文档路径。
	docSystem = "docs/site/接口文档/前台系统/系统接口.md"
)

// RouteContract 描述一条内置 HTTP 路由契约。
type RouteContract struct {
	Method       string           // HTTP 方法
	Path         string           // HTTP 路径
	Meta         shared.RouteMeta // 路由元数据
	DocumentPath string           // 仓库根目录下的接口文档路径
}

// DefaultRouteContracts 返回内置 HTTP 路由契约集合。
func DefaultRouteContracts() []RouteContract {
	return []RouteContract{
		{Method: http.MethodGet, Path: "/api/live", Meta: shared.HealthLive, DocumentPath: docHealth},
		{Method: http.MethodGet, Path: "/api/ready", Meta: shared.HealthReady, DocumentPath: docHealth},
		{Method: http.MethodGet, Path: "/api/metrics", Meta: shared.HealthMetrics, DocumentPath: docHealth},
		{Method: http.MethodPost, Path: "/api/auth/register", Meta: shared.AuthRegister, DocumentPath: docAuth},
		{Method: http.MethodPost, Path: "/api/auth/login", Meta: shared.AuthLogin, DocumentPath: docAuth},
		{Method: http.MethodPost, Path: "/api/auth/refresh", Meta: shared.AuthRefresh, DocumentPath: docAuth},
		{Method: http.MethodPost, Path: "/api/auth/logout", Meta: shared.AuthLogout, DocumentPath: docAuth},
		{Method: http.MethodGet, Path: "/api/user/profile", Meta: shared.UserProfile, DocumentPath: docUser},
		{Method: http.MethodGet, Path: confighandler.InternalConfigReloadStatusPath, Meta: shared.SystemConfigReloadStatus, DocumentPath: docSystem},
		{Method: http.MethodPost, Path: confighandler.InternalConfigReloadRunPath, Meta: shared.SystemConfigReloadRun, DocumentPath: docSystem},
	}
}
