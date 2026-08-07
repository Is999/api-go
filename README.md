# API 前台服务

`api` 是面向前台用户的 HTTP 服务，负责注册登录、用户资料、登录态、运行期配置查询和少量受控内网运维能力。后台管理、运营任务、管理员权限和异步任务编排由同工作区的 `admin-go` 负责。

本文面向首次接手项目的开发与运维人员，说明服务边界、启动方式、主要契约和验证入口。接口字段与安全策略以 [API 文档中心](docs/site/文档首页.md)中的专题文档为准。

> 使用 AI 修改代码、配置、SQL、脚本或文档前，必须先阅读 [AGENTS.md](AGENTS.md)、[AI 开发规范](docs/site/角色文档/后端开发/AI开发规范.md) 和 [AI 开发提示词](docs/site/角色文档/后端开发/AI开发提示词.md)。

## 快速导航

| 目标 | 文档或入口 |
| --- | --- |
| 按角色和任务查找文档 | [API 文档中心](docs/site/文档首页.md) |
| 开始本地开发 | [本地启动](#本地启动) |
| 查询接口契约 | [接口文档统一规范](docs/site/接口文档/接口文档统一规范.md) |
| 新增路由或组件 | [开发扩展指南](docs/site/角色文档/后端开发/开发扩展指南.md) |
| 同步前端安全策略 | [前端安全策略同步](docs/site/角色文档/后端开发/前端安全清单同步.md) |
| 初始化新库或交付存量库 SQL | [数据库初始化与变更交付治理](docs/site/角色文档/运维/数据库迁移治理.md) |
| 准备发布 | [部署发布指南](docs/site/角色文档/运维/部署发布指南.md) |
| 排查认证指标 | [认证安全指标与告警](docs/site/角色文档/后端开发/认证安全指标与告警.md) |

## 服务边界

API 负责前台请求热路径：

- 用户注册、登录、刷新和退出。
- JWT 与 Redis session 生命周期、会话上限和即时撤销。
- 用户资料读取与修改，以及多种身份索引定位。
- 请求参数校验、统一业务码和中英文消息。
- 可选签名验签、字段级加解密和请求防重放。
- 运行期系统配置读取与受控热加载。
- 存活、就绪、指标和内网文档资源接口。

API 不承载管理员后台、任务队列、Scheduler、运营工作流或大规模离线处理。此类能力应放在 `admin-go`，避免扩大前台服务的依赖面和请求延迟。

## 技术栈

- Go `1.26.5`、go-zero HTTP 框架。
- GORM + MySQL，支持主从读写路由和站点扩展库。
- go-redis，支持单机、Cluster 和 redsync 分布式锁。
- JWT + Redis session 登录态。
- 可选签名验签、AES/RSA 字段级加解密和请求防重放。
- OpenTelemetry Trace、Prometheus 指标和结构化访问日志。

## 分支与表路由

`api-go` 与 `admin-go` 必须成对使用同名分支和同一套路由配置。三条长期分支互斥，不能叠加：

| 分支 | 定位 | 数据路由职责 |
| --- | --- | --- |
| `main` | 唯一公共开发与交付基线 | 默认单表；`user.route_shard_count` 支持 `1/2/4/.../1024`，生产拆分需选用对应候选分支的迁移流程 |
| `table-sharding/shardingsphere-proxy-alternative` | ShardingSphere-Proxy 候选方案 | 应用访问逻辑表；Proxy 负责物理路由，部署和回填工具由 Admin 同名分支维护 |
| `table-sharding/app-table-sharding` | 应用内固定桶分表候选方案 | API 计算物理表名；在线复制和切换由 Admin 同名分支执行 |

Proxy 分支相对 `main` 只允许方案文档差异；应用分表分支只允许物理表路由、身份定位和对应文档差异。合并后执行下列命令，退出码必须为 `0`，报告中不得出现允许清单之外的代码、配置、SQL 或接口差异：

```bash
make branch-drift-check
```

在候选分支可通过 `BRANCH_VARIANT=proxy` 或 `BRANCH_VARIANT=app` 指定检查目标。

## 运行架构

```text
cmd/api
  -> bootstrap.LoadConfig / bootstrap.Wire
  -> 初始化日志、Trace、MySQL、Redis、Collector 和 ServiceContext
  -> 从 RouteSpecs 注册公开与内网路由
  -> middleware 执行恢复、链路、访问日志、鉴权和安全策略
  -> handler 解析请求并触发 Validate
  -> logic 编排业务规则和事务
  -> model / cache / infra 访问数据库、Redis 和外部依赖
  -> helper.JSONResp 输出统一响应
```

HTTP 响应统一为：

```json
{
  "status": true,
  "code": 1,
  "message": "成功",
  "data": {},
  "traceId": "4bf92f3577b34da6a3ce929d0e0e4736",
  "spanId": "00f067aa0ba902b7"
}
```

`traceId` 关联完整请求链路，`spanId` 定位当前处理片段；中间件同时回传 `X-Trace-Id` 和 `X-Span-Id` 响应头。

## 目录结构

本分支使用 ShardingSphere-Proxy 管理用户物理表。运行配置保持 `user.route_shard_count=1`，共享的 `internal/sharding` 返回逻辑表名；身份目录和固定 `shard_no` 提供路由条件，物理拓扑与迁移工具由 Admin 同名分支维护。

| 目录 | 职责 |
| --- | --- |
| `cmd` | API 与迁移命令入口；只处理参数和进程退出 |
| `common` | 业务码、i18n、Redis Key、嵌入资产和稳定公共契约 |
| `docs` | 接口文档、开发与运维说明、Prometheus 和 Grafana 资产 |
| `deploy` | Docker、systemd 和本地集成环境 |
| `etc` | 标准样例、dnmp 样例和本地运行配置 |
| `internal/bootstrap` | 配置加载、组件装配、热加载和生命周期 |
| `internal/handler` | 路由规格、参数解析、安全链路和响应写出 |
| `internal/logic` | 用例编排、规则校验、事务和缓存边界 |
| `internal/model` | GORM Model、身份目录、固定分片字段和 Proxy 逻辑表访问 |
| `internal/sharding` | 与 main 共用固定桶映射；本分支配置为 1，仅返回逻辑表名 |
| `internal/middleware` | 鉴权、签名、加解密、内网 Ops、日志和恢复 |
| `internal/security` | 路由字段级签名、加密和大小限制契约 |
| `internal/infra` | MySQL、Redis、日志、Trace 和 Collector 适配 |
| `internal/types` | 请求、响应、列表项和参数校验契约 |

## 统一注册点

| 对象 | 权威入口 |
| --- | --- |
| 启动组件 | `internal/bootstrap/components:DefaultSpecs` |
| 注册清单 | `internal/bootstrap/manifest:Default`；用于发现和校验入口，不执行组件装配 |
| HTTP 路由 | `internal/handler/routes.go:builtinRouteModuleSpecs` 与各模块 `RouteSpecs` |
| 路由安全清单 | `internal/handler/route_security_manifest.go:DefaultRouteSecurityManifest` |
| 运行时扩展 | 各能力归属包的 `RuntimeRegistrySpecs`；清单项需在组件文档标明“生产已启用”或“基础能力/未激活” |
| 数据库初始化基线 | `internal/database/migrations.go:DefaultMigrations` |
| 运行期配置 Key | `internal/logic/config/sys_config_key.go:SysConfigKey`；当前无业务 route/logic 调用，属于基础能力/未激活 |
| Redis Key | `common/rediskeys` |

完整规则见[组件注册清单](docs/site/角色文档/后端开发/组件注册清单.md)。

## 核心契约

### 登录态

JWT 携带稳定 `sid`、唯一 `jti` 和 `auth_version`。会话通过同槽 Redis Hash、ZSET 和 Lua 原子维护，支持多实例部署、刷新与退出并发收口，以及单用户最多 8 个会话。

### 用户身份

用户通过 `user_identity_username`、`user_identity_email`、`user_identity_phone` 和 `user_identity_oauth` 定位所在物理表。邮箱和手机号身份只保存 HMAC `identity_hash`，避免分表后扫描全部用户表。

### 路由安全

`security.secret_key` 配置后启用 `X-App-Id`、`X-Signature`、`X-Crypto`、`X-Cipher` 和 `X-Key-Version` 等安全头。签名与加密字段必须由路由安全策略逐字段声明；不得默认处理完整请求体、大对象或分页列表。

### 内网接口

`/internal/system/...`、`/internal/users/:id/runtime-sync` 和 `/internal/docs/...` 只注册到独立 `internal_server`，并校验私网来源、Ops 令牌、HMAC、时间窗口和 Redis nonce。生产跨主机监听必须使用 mTLS。

API 不直接向浏览器公开文档站。Admin 通过内网 `/internal/docs` 读取 Markdown 资源，再从 `/api/docs/api` 向已授权管理员展示；根路径返回[API 文档中心](docs/site/文档首页.md)。

## 本地启动

### 1. 准备依赖

必需依赖为 Go `1.26.5`、MySQL 和 Redis；Collector、OTLP 等依赖按启用能力准备。

```bash
cp etc/config.dnmp.sample.yaml etc/config.yaml
go mod download
```

修改本地配置中的数据库、Redis、`app_id`、`jwt_secret`、安全密钥和运维令牌。不要把真实生产密钥写入样例文件。

### 2. 初始化本地空库

```bash
make migrate-status MIGRATE_CONFIG=./etc/config.yaml
make migrate-dry-run MIGRATE_CONFIG=./etc/config.yaml
make migrate-up MIGRATE_CONFIG=./etc/config.yaml
```

以上命令只用于确认无业务数据的全新空库。仓库内 DDL/DML 是完整初始化基线，不负责升级已有数据库；存量库变化由开发在被忽略的 `data/sql-changes/<change-id>/` 生成本地增量 SQL，通过发布工单交给 DBA/运维命令行执行，SQL 文件不提交仓库、不追加迁移版本。完整边界见[数据库初始化与变更交付治理](docs/site/角色文档/运维/数据库迁移治理.md)。

### 3. 启动服务

```bash
go run ./cmd/api -f ./etc/config.yaml
```

查看构建版本：

```bash
go run ./cmd/api -version
go run ./cmd/migrate -version
```

## 配置边界

- `etc/config.sample.yaml`：标准配置样例。
- `etc/config.dnmp.sample.yaml`：本地 dnmp 环境样例。
- `etc/config.yaml`：本地实际配置，不应提交生产密钥。

项目自有 YAML 样例中的每个固定配置字段必须保留紧邻字段上方、与字段同缩进的中文注释。注释至少写明消费组件和用途，并如实补充源码已定义的取值/单位、缺省与空值、热加载或重启、敏感信息及跨字段约束；父节点概述不能替代子字段说明。动态 map 的重复数据项由父字段统一定义 key/value、空值和合并语义，第三方 schema 与纯数据文件按 [AI 开发规范](docs/site/角色文档/后端开发/AI开发规范.md#yaml-配置字段行级注释)记录排除依据。

`config_files.runtime` 只允许外置 `internal/bootstrap/configload/runtimefile:sectionSpecs` 声明的运行期配置段。认证限流、热加载轮询等字段只有在已有应用器支持时才能热更新；HTTP、AppID/AppKey、JWT、Security、Collector、MySQL、Redis、OTLP、路由和组件注册等启动期能力变更后必须重启。

## 开发与验证

日常开发按改动范围运行：

```bash
make fmt-check
make test
make vet
make build
make build-tools
git diff --check
```

发布候选运行完整门禁：

```bash
make ci
```

`make ci` 包含格式、全量测试、race、vet、构建、密钥扫描、依赖漏洞检查、Prometheus 规则、分支差异和 diff 检查。缺少 `promtool` 时会尝试使用 Docker 镜像。

开发时遵守以下分层：

- Handler 只负责路由、参数、安全上下文和响应；业务流程进入 Logic。
- Logic 负责用例、事务、缓存和错误上下文，不临时创建基础设施连接。
- Model 优先使用 GORM 链式调用，表名与分表定位集中维护。
- 原生 SQL 与 Redis Lua 必须作为可审查的嵌入资产维护。
- 新增接口同步 types、RouteMeta、安全字段、业务码、i18n、接口文档和前端契约。
- 新增组件或运行时扩展必须接入统一注册点并补测试，不能只提交孤立实现。

## 观测与发布

- 存活探针：`/api/live`，不访问外部依赖。
- 就绪探针：`/api/ready`，检查启用的关键依赖和组件。
- Prometheus 指标：`/api/metrics`。

以下条目必须保留命令输出、接口响应、认证记录或发布工单作为证据，不能只填写“正常”或“已确认”：

- 新空库已按 `migrate-status -> migrate-dry-run -> migrate-up` 顺序初始化且进程退出码为 `0`；已有环境的 DBA/运维工单已记录 SQL SHA-256、执行顺序、影响行数和 `90_verify.sql` 或等价校验结果。候选版本中不存在一次性增量 SQL。
- `/api/live` 返回 HTTP 2xx 且不依赖外部服务；`/api/ready` 返回 HTTP 2xx，并且响应中所有已启用关键依赖与组件均为就绪；`/api/metrics` 可被目标 Prometheus 抓取。
- 认证冒烟覆盖登录、刷新和退出：刷新后旧 token 不能再次刷新，退出后目标 session 失效，用户会话数不超过服务端上限。启用签名或加密时，再覆盖合法安全头、错误签名、重放 nonce、字段超限和细分业务码。
- MySQL、Redis、Collector 和 Trace 的实际连接目标与发布环境清单一致；通过脱敏诊断、就绪检查或受控请求验证，禁止在交付记录中输出密码、token、私钥或完整 DSN。
- Admin 到 API 的内网运行态同步和文档代理分别完成一次受控调用，验证私网来源、Ops HMAC、时间窗口、nonce 防重放及失败响应；浏览器不能绕过 Admin 直接访问 API 内网文档资源。
- 配置与部署资产已通过密钥扫描；样例 `jwt_secret`、私钥、AES Key 和运维令牌未进入 Git、镜像、日志或发布工单正文。真实密钥只从目标环境的密钥管理渠道注入。

发布资产和回滚流程见[部署发布指南](docs/site/角色文档/运维/部署发布指南.md)。

## License

Internal use only.
