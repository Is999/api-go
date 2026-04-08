# gozero-api

`gozero-api` 是面向前台用户项目的 HTTP API 服务。后台管理与 cron 继续由 `gozero-admin` 承担，本服务只保留前台交互所需的结构性核心能力：

- go-zero HTTP 服务骨架
- 统一 `status/code/message/data` 响应
- JWT + Redis session 登录态，支持多实例部署
- 可选签名验签、AES/RSA 加解密和请求防重放
- 配置热加载状态查询与受保护的手动触发
- 系统配置缓存、通用缓存保护和业务事件收集器
- 认证风控事件脱敏埋点，支持轻量 Collector 聚合
- MySQL 读写分离与 Redis 单机/Cluster 客户端
- Trace、访问日志、错误链路日志和健康检查

## 本地启动

1. 创建数据库后通过 `make migrate-dry-run` 预览迁移，再通过 `make migrate-up` 执行未登记版本
2. 复制 `etc/config.dnmp.sample.yaml` 为 `etc/config.yaml`
3. 调整 MySQL、Redis、`app_key`、`jwt_secret`
4. 启动服务：

```bash
go run . -f ./etc/config.yaml
```

迁移命令：

```bash
make migrate-status MIGRATE_CONFIG=./etc/config.yaml
make migrate-dry-run MIGRATE_CONFIG=./etc/config.yaml
make migrate-up MIGRATE_CONFIG=./etc/config.yaml
```

## 发布验证

```bash
make ci
```

发布流程见 `docs/site/角色文档/运维/部署发布指南.md`。生产至少确认 `/api/live`、`/api/ready`、`/api/metrics` 内网可访问，`schema_migrations` 已登记当前版本，认证安全指标有基线数据。

发布模板位于 `deploy/`，包括 Dockerfile、systemd 单元和集成依赖 Compose；Prometheus 告警规则位于 `docs/prometheus/gozero-api-alerts.yml`。

## 框架核心能力

- 配置热加载：`hot_reload.enabled=true` 后监听主配置文件，运行期配置会刷新到 `ServiceContext`；HTTP 监听、MySQL、Redis、OTLP 等启动期配置变更会标记为需重启；HTTP 热加载接口仅注册 `/internal/system/...` 内网路由，需配置 `ops.config_reload_token` 并携带 `X-Ops-Token`。
- 配置校验：启动期会拦截弱 `jwt_secret`、无 Redis 地址、无效 Collector Redis 载体和公网运维白名单等明显危险配置。
- 认证限流：`auth.login_rate_limit` 按 IP 和用户名保护登录入口，`auth.register_rate_limit` 按 IP 保护注册入口。
- 认证风控事件：注册、登录、刷新、退出、登录态鉴权失败、限流和用户级 session 失效会投递 `auth.security` 脱敏事件；默认 Processor 会统计 `gozero_api_auth_security_events_total{app_id,action,reason,category}`，Collector 关闭不影响接口响应。
- 安全链路：`security.secret_key` 配置秘钥后启用 `X-App-Id`、`X-Signature`、`X-Crypto`、`X-Cipher`、`X-Key-Version` 等请求头；未配置秘钥时兼容普通 JSON 请求。
- 前端安全清单：`docs/site/route_security_manifest.json` 固化前台 route policy 同步快照，未知路由不默认参与全量签名或整包加密。
- 系统配置缓存：业务优先声明 `SysConfigKey` 并通过类型化 getter 读取；底层仍优先读取 Redis Hash，缓存缺失时使用 redsync 锁保护主库回源，并写入短 TTL 空值占位防穿透。
- 缓存保护：Redis JSON 缓存写入带 TTL 抖动，并提供 redsync 缓存重建锁，减少缓存雪崩和击穿风险。
- 收集器：`collector` 支持同步 Processor 和 Redis Stream 两种载体，适合前台轻量业务事件统一投递。

## 表名约束

前台用户表使用 `api_` 前缀，例如 `api_user`，不复用后台 `admin`、`admin_role` 等表名。运行期系统配置表固定使用 `sys_config`，不添加 `api_` 前缀；后续新增其它前台表仍应保持 `api_` 前缀或明确业务前缀，避免后台和前台表名冲突。

## AI 开发入口

修改代码、配置、SQL 或文档前先读：

- `AGENTS.md`
- `docs/site/角色文档/后端开发/AI开发规范.md`
- `docs/site/角色文档/后端开发/AI开发提示词.md`
