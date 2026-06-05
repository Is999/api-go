# API 服务

`api` 是面向前台用户的 HTTP API 服务，负责前台认证、用户资料、登录态、运行期配置查询和少量内网运维能力。后台管理、运营任务和管理端权限由 `admin` 项目负责；本服务只保留前台链路必须具备的运行时能力，避免把管理后台职责混入前台 API。

## 技术栈

- Go `1.26.4`
- go-zero HTTP 服务框架
- GORM + MySQL，支持主从读写路由和站点扩展库
- go-redis，支持单机、Cluster 和 redsync 分布式锁
- JWT + Redis session 登录态
- 可选签名验签、AES/RSA 加解密和请求防重放
- OpenTelemetry Trace、结构化访问日志、Prometheus 指标

## 运行链路

```text
cmd/api
  -> bootstrap.LoadConfig / bootstrap.Wire
  -> 初始化 logger、trace、MySQL、Redis、Collector、ServiceContext
  -> handler.RegisterHandlers
  -> recover -> trace -> access log -> recover
  -> handler/shared.RespHandler 解析请求并触发 go-zero Validate
  -> logic 执行业务编排
  -> model / cache / infra 访问数据库、Redis 和基础设施
  -> helper.JSONResp 输出统一 status/code/message/data
```

所有 HTTP 响应保持统一结构：

```json
{
  "status": true,
  "code": 1000,
  "message": "成功",
  "data": {}
}
```

## 目录结构

`api` 延续 `admin` 的 go-zero 分层方式：入口只负责启动参数，`bootstrap` 负责装配，`handler/logic/model/types` 分别承担 HTTP 入口、用例编排、数据访问和接口契约。启动组件、HTTP 路由、数据库迁移、Redis Key、运行期配置和轻量扩展都有固定登记位置，避免后续能力散落在调用点。

```text
api
├── cmd                       # 二进制入口
│   ├── api                   # HTTP 服务启动入口
│   └── migrate               # 数据库迁移命令入口
├── common                    # 跨包公共能力：业务码、i18n、Redis Key、嵌入资产、运行态配置
├── docs                      # 开发规范、接口文档、运维手册和监控资产
│   ├── site
│   │   ├── 角色文档
│   │   │   ├── 后端开发    # AI 规范、扩展指南、组件清单、安全同步说明
│   │   │   └── 运维        # 部署发布、数据库迁移治理
│   │   └── 接口文档
│   │       └── 前台系统    # 认证、用户、系统、健康检查接口
│   ├── prometheus            # Prometheus 告警规则
│   ├── grafana               # Grafana 面板
│   └── handler.go            # 内网文档资源读取入口
├── deploy                    # Docker、systemd 和本地集成依赖
├── etc                       # 配置模板和本地运行配置
├── helper                    # HTTP JSON 响应和轻量通用函数
└── internal                  # 主工程代码
    ├── bootstrap             # 配置加载、组件装配、热加载、启动关闭
    ├── config                # YAML 配置结构和解析契约
    ├── database              # 数据库迁移、迁移状态表和 SQL 资产
    ├── handler               # HTTP 路由注册、RouteMeta、RouteContract 和请求入口
    ├── infra                 # MySQL、Redis、redsync、日志、Trace、Collector 适配
    ├── logic                 # 用例编排、规则校验、缓存保护和运行时扩展
    ├── middleware            # 鉴权、签名、加解密、内网 Ops、访问日志、Recover
    ├── model                 # GORM Model、表名、分表定位和数据访问
    ├── requestctx            # 链路字段、调用方、耗时和 trace 元数据
    ├── routealias            # 路由别名常量
    ├── security              # 路由字段级签名、加密、大小限制和测试向量
    ├── svc                   # ServiceContext 与基础设施依赖聚合
    └── types                 # API 请求、响应、列表项和参数校验
```

## 分层约定

- `cmd` 只处理启动参数和命令模式，真实装配进入 `bootstrap.Wire`。
- `bootstrap` 负责配置加载、组件生命周期、热加载边界和注册清单派生，不把业务规则写进启动入口。
- `handler` 只声明 `RouteSpecs`、解析请求、透传上下文和统一响应；复杂流程进入 `logic`。
- `logic` 负责用例编排、规则校验、事务边界和错误上下文，不能绕过 `svc` 临时创建数据库、Redis 或外部连接。
- `model` 优先使用 GORM 链式调用，表名、分表定位和数据访问方法保持在模型层收口。
- `types` 中请求以 `Req` 结尾，响应以 `Resp` 结尾，列表项以 `Item` 结尾；所有请求结构体实现 go-zero `Validate()`。
- `middleware` 和 `security` 只承载通用安全链路；签名、加密字段由路由安全策略显式声明，不能在业务代码临时扩大范围。
- `common` 只放跨包稳定契约和小型公共能力，领域规则优先留在所属 `logic/model` 包。

## 统一注册点

| 注册对象 | 统一入口 | 说明 |
| --- | --- | --- |
| 启动组件 | `internal/bootstrap/components.go:defaultComponentSpecs` | MySQL、命名扩展库、Redis 等组件从规格派生健康检查和关闭顺序 |
| 注册清单 | `internal/bootstrap/registrations.go:DefaultRegistrationManifest` | 从启动组件、路由模块和运行时扩展派生，不手写第二套清单 |
| HTTP 路由 | `internal/handler/routes.go:builtinRouteModuleSpecs` + 各模块 `RouteSpecs` | 真实路由、RouteContract 和安全链路都从路由规格派生 |
| 路由安全清单 | `internal/handler/route_security_manifest.go:DefaultRouteSecurityManifest` | 汇总 method、path、chain 和字段级签名加密策略，用于文档和前端同步 |
| 运行时扩展 | 各能力归属包 `RuntimeRegistrySpecs` | `collectorx`、`auth`、`config`、`logic` 分别声明自身轻量扩展入口 |
| 数据库迁移 | `internal/database/migrations.go:DefaultMigrations` | 迁移版本、名称和 SQL 资产集中登记，执行结果写入 `schema_migrations` |
| 运行期配置 Key | `internal/logic/config/sys_config_key.go:SysConfigKey` | 业务读取 `sys_config` 前先声明 key，再使用类型化 getter |
| Redis Key | `common/rediskeys` | Key 模板集中定义并带静态检查，业务代码不散落高基数字符串 |

## 核心能力

- 前台认证：注册、登录、刷新、退出，登录态使用 JWT + Redis session，支持多实例部署。
- 用户资料：按 `user_account` 定位用户所在物理表，避免未来拆表后扫描所有用户表。
- 参数校验：请求结构体在 `internal/types` 实现 go-zero `Validate()`，handler 解析请求后自动触发基础校验和字段归一化。
- 安全链路：`security.secret_key` 配置后启用 `X-App-Id`、`X-Signature`、`X-Crypto`、`X-Cipher`、`X-Key-Version` 等请求头校验；未配置秘钥时允许普通 JSON 请求。
- 路由治理：路由由 `RouteSpec` 单点描述，并同步生成 route contract、route security manifest 和接口文档引用。
- 运行期配置：`hot_reload.enabled=true` 后监听主配置文件和允许外置的运行期配置段；HTTP 监听、MySQL、Redis、OTLP 等启动期配置变化只提示重启，不在线重建核心组件。
- 内网运维接口：`/internal/system/...`、`/internal/users/:id/runtime-sync` 和 `/internal/docs/...` 只允许内网来源并要求 `X-Ops-Token`。
- 业务配置缓存：`sys_config` 使用 Redis Hash、本地缓存、redsync 回源锁和空值占位保护。
- Collector：轻量业务事件可投递到同步 Processor 或 Redis Stream，认证风控事件默认会输出脱敏指标。
- 可观测性：提供 `/api/live`、`/api/ready`、`/api/metrics`，并配套访问日志、错误链路日志和 OpenTelemetry Trace。

## 本地启动

准备 MySQL、Redis 后，复制样例配置并调整连接信息、`app_id`、`jwt_secret`、安全秘钥和运维令牌：

```bash
cp etc/config.dnmp.sample.yaml etc/config.yaml
go mod download
```

执行迁移前先查看和预览：

```bash
make migrate-status MIGRATE_CONFIG=./etc/config.yaml
make migrate-dry-run MIGRATE_CONFIG=./etc/config.yaml
make migrate-up MIGRATE_CONFIG=./etc/config.yaml
```

启动服务：

```bash
go run ./cmd/api -f ./etc/config.yaml
```

查看构建版本：

```bash
go run ./cmd/api -version
go run ./cmd/migrate -version
```

## 数据库迁移

迁移入口是 `cmd/migrate`，迁移定义来自 `internal/database.DefaultMigrations()`，SQL 资产放在 `internal/database/assets/*.sql.tmpl`。执行结果会登记到 `schema_migrations`，同一个库中 API 迁移使用独立版本号段，避免和 `admin` 项目冲突。

常用命令：

```bash
make migrate-status
make migrate-dry-run
make migrate-up
```

当前基础表包括：

- `user`：前台业务用户表。
- `user_account`：用户名到用户物理表位置的全局索引。
- `sys_config`：运行期系统配置表。
- `schema_migrations`：迁移版本登记表。

表结构和迁移治理要求见：

- `docs/site/角色文档/运维/数据库迁移治理.md`
- `docs/site/角色文档/运维/部署发布指南.md`

## 配置边界

配置文件入口：

- `etc/config.sample.yaml`：标准样例。
- `etc/config.dnmp.sample.yaml`：本地 dnmp 环境样例。
- `etc/config.yaml`：本地实际运行配置，不应提交生产秘钥。

`config_files.runtime` 只允许外置部分运行期配置段，当前由 `internal/bootstrap/config_files.go` 明确声明。新增配置时必须区分：

- 运行期参数：可热加载，例如部分认证、安全、Collector、运维令牌等配置。
- 启动期能力：必须重启，例如 HTTP 监听、MySQL、Redis、OTLP、路由和组件注册。

## 接口文档

接口文档位于 `docs/site/接口文档/前台系统/`：

- `认证接口.md`
- `用户接口.md`
- `健康检查接口.md`
- `系统接口.md`

安全字段和路由策略同步面：

- `internal/security/signature.go`
- `internal/handler/shared/route_meta.go`
- `internal/handler/route_security_contract.go`
- `docs/site/route_security_manifest.json`

新增或调整接口时，需要同步 Go types、RouteMeta、安全字段、接口文档、业务码和 i18n 文案。

## 验证命令

日常开发优先运行：

```bash
make fmt-check
make test
make build
make build-tools
git diff --check
```

发布前运行完整检查：

```bash
make ci
```

`make ci` 会执行格式检查、全量测试、主服务构建、迁移工具构建、秘钥扫描、Prometheus 规则检查和 `git diff --check`。如果本机没有 `promtool`，规则检查会优先尝试 Docker 镜像。

## 发布与观测

发布资产：

- `deploy/docker/Dockerfile`
- `deploy/systemd/api.service`
- `deploy/integration/docker-compose.yml`
- `docs/prometheus/api-alerts.yml`
- `docs/grafana/`

发布前至少确认：

- `make migrate-dry-run` 输出符合预期。
- `make migrate-up` 后 `schema_migrations` 登记成功。
- `/api/live`、`/api/ready`、`/api/metrics` 在内网可访问。
- MySQL、Redis、Collector、Trace 配置均指向目标环境。
- 生产配置中没有样例 `jwt_secret`、私钥、AES Key 或运维令牌。

## 开发约束

修改代码、配置、SQL、脚本或文档前先读：

- `AGENTS.md`
- `docs/site/角色文档/后端开发/AI开发规范.md`
- `docs/site/角色文档/后端开发/AI开发提示词.md`

关键要求：

- 请求参数放在 `internal/types`，需要实现 go-zero `Validate()`。
- handler 只负责解析、鉴权链路和响应写出，业务编排放在 `internal/logic`。
- 数据访问优先使用 GORM 链式调用，原生 SQL 必须放在 `internal/database/assets/*.sql.tmpl` 等代码资产中。
- Redis Key 必须走 `common/rediskeys`，禁止在业务代码散落高基数通配 key。
- 新增接口必须同步接口文档、RouteMeta、安全字段、业务码和 i18n。
- 新增运行期能力必须同步注册清单和测试，不能只加业务实现。
