package docs

import "embed"

// FS 内嵌前台 API 文档资源，生产环境不依赖本地文件系统。
//
//go:embed all:site
var FS embed.FS
