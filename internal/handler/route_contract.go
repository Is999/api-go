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
		// GET /api/live：表示存活检查路由。
		{Method: http.MethodGet, Path: "/api/live", Meta: shared.HealthLive, DocumentPath: docHealth},
		// GET /api/ready：表示就绪检查路由。
		{Method: http.MethodGet, Path: "/api/ready", Meta: shared.HealthReady, DocumentPath: docHealth},
		// GET /api/metrics：表示 Prometheus 指标抓取路由。
		{Method: http.MethodGet, Path: "/api/metrics", Meta: shared.HealthMetrics, DocumentPath: docHealth},
		// POST /api/auth/register：表示前台用户注册路由。
		{Method: http.MethodPost, Path: "/api/auth/register", Meta: shared.AuthRegister, DocumentPath: docAuth},
		// POST /api/auth/login：表示前台用户登录路由。
		{Method: http.MethodPost, Path: "/api/auth/login", Meta: shared.AuthLogin, DocumentPath: docAuth},
		// POST /api/auth/refresh：表示访问令牌刷新路由。
		{Method: http.MethodPost, Path: "/api/auth/refresh", Meta: shared.AuthRefresh, DocumentPath: docAuth},
		// POST /api/auth/logout：表示前台用户退出登录路由。
		{Method: http.MethodPost, Path: "/api/auth/logout", Meta: shared.AuthLogout, DocumentPath: docAuth},
		// GET /api/user/profile：表示当前用户资料路由。
		{Method: http.MethodGet, Path: "/api/user/profile", Meta: shared.UserProfile, DocumentPath: docUser},
		// GET /api/internal/config-reload/status：内网查询配置热加载状态。
		{Method: http.MethodGet, Path: confighandler.InternalConfigReloadStatusPath, Meta: shared.SystemConfigReloadStatus, DocumentPath: docSystem},
		// POST /api/internal/config-reload：内网手动触发配置热加载。
		{Method: http.MethodPost, Path: confighandler.InternalConfigReloadRunPath, Meta: shared.SystemConfigReloadRun, DocumentPath: docSystem},
	}
}
