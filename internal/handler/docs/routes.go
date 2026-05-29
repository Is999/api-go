package docs

import (
	"net/http"

	apidocs "api/docs"
	"api/internal/handler/shared"
	"api/internal/middleware"
	"api/internal/svc"
)

// API 内网文档资源路由路径常量。
const (
	// InternalDocsFilePath 表示内网接口文档二级资源路由。
	InternalDocsFilePath = "/internal/docs/:path/:file"
	// InternalDocsNestedFilePath 表示内网接口文档三级资源路由。
	InternalDocsNestedFilePath = "/internal/docs/:path/:sub/:file"
)

// RouteSpecs 返回 API 内网文档资源路由规格。
func RouteSpecs() []shared.RouteSpec {
	return []shared.RouteSpec{
		{
			Method:       http.MethodGet,
			Path:         InternalDocsFilePath, // 内网读取 API 接口文档二级资源。
			Meta:         shared.SystemDocsFile,
			DocumentPath: shared.RouteDocSystem,
			Chain:        shared.RouteSecurityInternal,
			Handler: func(svcCtx *svc.ServiceContext, _ *middleware.AuthMiddleware) http.HandlerFunc {
				opsMw := middleware.NewOpsMiddleware(svcCtx)
				return opsMw.Handle(apidocs.Handler())
			},
		},
		{
			Method:       http.MethodGet,
			Path:         InternalDocsNestedFilePath, // 内网读取 API 接口文档三级资源。
			Meta:         shared.SystemDocsNestedFile,
			DocumentPath: shared.RouteDocSystem,
			Chain:        shared.RouteSecurityInternal,
			Handler: func(svcCtx *svc.ServiceContext, _ *middleware.AuthMiddleware) http.HandlerFunc {
				opsMw := middleware.NewOpsMiddleware(svcCtx)
				return opsMw.Handle(apidocs.Handler())
			},
		},
	}
}
