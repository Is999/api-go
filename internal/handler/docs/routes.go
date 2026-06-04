package docs

import (
	"net/http"

	apidocs "api/docs"
	"api/internal/handler/shared"
	"api/internal/svc"
)

// RouteSpecs 返回 API 内网文档资源路由规格。
func RouteSpecs() []shared.RouteSpec {
	return []shared.RouteSpec{
		{
			Method:       http.MethodGet,
			Path:         "/internal/docs/:file", // 内网读取 API 文档站根级 Markdown 资源。
			Meta:         shared.SystemDocsRootFile,
			DocumentPath: shared.RouteDocSystem,
			Chain:        shared.RouteSecurityInternal,
			Handler: func(_ *svc.ServiceContext) http.HandlerFunc {
				return apidocs.Handler()
			},
		},
		{
			Method:       http.MethodGet,
			Path:         "/internal/docs/:path/:file", // 内网读取 API 文档二级 Markdown 资源。
			Meta:         shared.SystemDocsFile,
			DocumentPath: shared.RouteDocSystem,
			Chain:        shared.RouteSecurityInternal,
			Handler: func(_ *svc.ServiceContext) http.HandlerFunc {
				return apidocs.Handler()
			},
		},
		{
			Method:       http.MethodGet,
			Path:         "/internal/docs/:path/:sub/:file", // 内网读取 API 文档三级 Markdown 资源。
			Meta:         shared.SystemDocsNestedFile,
			DocumentPath: shared.RouteDocSystem,
			Chain:        shared.RouteSecurityInternal,
			Handler: func(_ *svc.ServiceContext) http.HandlerFunc {
				return apidocs.Handler()
			},
		},
	}
}
