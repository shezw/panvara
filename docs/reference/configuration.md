<!--
    Panvara
    docs/reference/configuration.md    2026-07-18
     ______     __  __     ______     ______     __     __
    /\  ___\   /\ \_\ \   /\  ___\   /\___  \   /\ \  _ \ \
    \ \___  \  \ \  __ \  \ \  __\   \/_/  /__  \ \ \/ ".\ \
     \/\_____\  \ \_\ \_\  \ \_____\   /\_____\  \ \__/".~\_\
      \/_____/   \/_/\/_/   \/_____/   \/_____/   \/_/   \/_/.com

    @link    : https://github.com/shezw/panvara
    @author  : shezw
    @email   : hello@shezw.com
-->

# 配置参考

## 加载顺序

Panvara alpha.2 的实际优先级是：CLI 参数 > 环境变量 > 内置默认值。程序不会自动读取 `.env` 文件。

本地开发建议：

1. `make local-init` 创建 `.env` 和 `.env.local`。
2. 普通本地设置写入 `.env`。
3. Token、端口冲突等仅本机设置写入 `.env.local`。
4. 每个新终端显式加载两个文件。

```sh
set -a; . ./.env; . ./.env.local; set +a
```

## Server 变量

| 变量 | 示例或默认 | Server 必需 | 敏感 | 说明 |
| --- | --- | :---: | :---: | --- |
| `PANVARA_PROFILE` | `lite`；示例为 `server` |  | 否 | 运行方式；Make 的 Server 命令会显式选择 server |
| `PANVARA_HTTP_ADDR` | `127.0.0.1:8080` |  | 否 | HTTP 监听地址 |
| `PANVARA_DATABASE_URL` | `postgres://...` | ✓ | 生产环境是 | PostgreSQL 连接 URL |
| `PANVARA_MODULE_SOURCE` | `examples/modules/crm-leads.yaml` | ✓ | 否 | YAML/JSON 模型文件路径 |
| `PANVARA_MODULE_FORMAT` | `auto` |  | 否 | `auto`、`yaml` 或 `json` |
| `PANVARA_PROJECT_ID` | UUIDv7 | ✓ | 否 | 当前单项目 ID |
| `PANVARA_PROJECT_KEY` | `default` |  | 否 | 可读项目标识 |
| `PANVARA_PROJECT_LOCALE` | `en-US` |  | 否 | BCP 47 语言与地区 |
| `PANVARA_PROJECT_TIME_ZONE` | `UTC` |  | 否 | IANA 时区，例如 `Asia/Shanghai` |
| `PANVARA_PROJECT_CURRENCY` | `USD` |  | 否 | 项目默认币种，例如 `CNY`、`EUR` |
| `PANVARA_ENVIRONMENT_KEY` | `default` |  | 否 | 当前唯一默认 Environment 的可读 Key |
| `PANVARA_ADMIN_TOKEN` | 32–1024 字节、仅 Bearer 安全 ASCII、非 `pvk1.` 前缀 | 首次 marker 初始化 ✓ | ✓ | 首启只持久化 digest/hint；marker 已存在后可省略，相同可用，不同值拒绝 |

## Project、Environment 与访问 bootstrap

P0-01a 开发切片在 migration 之后、HTTP 就绪之前建立最小持久化执行作用域：

- 某个 `PANVARA_PROJECT_ID` 第一次由 Server 装配时，按 `PANVARA_PROJECT_*` 持久化 Project，生成 UUIDv7 默认 Environment，并创建 `bootstrap-admin` Principal 与精确作用域内的 `project.owner` Grant。
- `PANVARA_ENVIRONMENT_KEY` 未设置时默认为 `default`。Environment ID 由 Server 生成，不通过配置指定。
- 后续以同一 Project ID 和相同配置重启会复用持久化身份；该 Project 的 Key、Locale、Time Zone、Currency 或 Environment Key 与数据库不一致时拒绝启动，不会静默更新。新的 Project ID 与新的全局唯一 Key 会创建另一套 Project 事实，而不是修改或迁移旧数据。
- 首次 `0005` 初始化要求 32–1024 字节、只含 HTTP Bearer 安全 ASCII（字母、数字、`-._~+/`，`=` 只能尾随）且不以保留前缀 `pvk1.` 开头的 Token，并原子持久化 API Credential 的 SHA-256 digest、hint 与永久 marker；原始 Token 不入库。`pvk1.` 只用于 Panvara 签发的 Service Credential，bootstrap 使用会与 selector 语义冲突。marker 存在后启动可省略 Token，提供相同值可核对，提供不同值拒绝启动。revoked bootstrap Credential 永不复活。
- 从 `0004` 升级且 marker 尚不存在时，如果旧本地 Token 恰好以 `pvk1.` 开头，只能在首次 `0005` 初始化前换成新的合法随机值；marker 成功创建后禁止再改变。
- 所有 Bearer Credential 只认证 Principal。Application 层读取 active Credential/Principal 与 active Owner Grant 决定授权；Grant 被撤销后 Server 重启不会自动恢复，但另一个有效 Owner 可显式 PUT 重新授予。

这只是 P0-01a/P0-01b 可运行切片，不是完整 P0-01 或 IAM。当前有 project-local Service Principal/API Credential/固定 Owner Grant，但没有 Account、ExternalIdentity、Session、ProjectMembership、动态 Role/Policy、RecordOwner 或多 Environment 业务事实；Record、Revision 与 Draft 表仍无真正的 Environment 隔离。使用与验收见[执行作用域与访问内核指南](../modules/project-access.md)和[访问管理指南](../modules/access-administration.md)，架构约束见 [ADR-0004](../adr/0004-persistent-execution-scope-access-kernel.md)与 [ADR-0005](../adr/0005-project-local-access-administration.md)。

## 本地 Compose 变量

| 变量 | 默认 | 说明 |
| --- | --- | --- |
| `PANVARA_COMPOSE_PROJECT` | `panvara` | Compose 项目名；并行工作区应设不同值 |
| `PANVARA_POSTGRES_PORT` | `5432` | PostgreSQL 宿主端口；冲突时可改为 `55432` |

改变 PostgreSQL 端口时，必须同时修改 `PANVARA_DATABASE_URL` 中的端口。

Panvara 使用的 PostgreSQL 18.4 数据库必须采用 UTF8 `server_encoding`，否则多语言 Source、标签与生成制品可能无法保存或读取。自备数据库先执行 `SHOW server_encoding;` 并确认返回 `UTF8`；仓库 Compose 创建的数据库符合该要求。当前版本尚未在 Server 启动或 Migrate 阶段自动 fail-fast 检查非 UTF8 数据库，这是后续需要补齐的运维保护。

## Revision Registry

alpha.3a Registry 开发切片不增加配置项。Server 在 migration 与 P0-01a 作用域初始化之后、对外就绪之前，使用当前持久化 Project、`PANVARA_MODULE_SOURCE` 和 `PANVARA_MODULE_FORMAT` 进行幂等 bootstrap 登记；失败会阻止启动。

任一 active project-local Credential 都可认证 Registry、Draft/Validation/Plan 与 P0-02a Release 控制面；这些 Admin 用例仍由持久化 Owner Grant 授权。当前没有 active Revision、Activate 或 Rollback 配置，也不能用 Registry List 或 Release 时间配置运行版本。

`data_schema_identities` 也不是配置项。它是 Panvara 为不可变父 Revision 计算并按 format 升序返回的派生身份数组；新增算法只能追加新的 format，用户不能通过环境变量覆盖 fingerprint。

alpha.3b 不增加环境变量。Draft Baseline 必须由每个 Create 请求显式传入；Draft Version（内部 generation）、Source Hash、Validation/Plan Format 和 Idempotency Key 都是请求或持久化身份，不能用环境变量全局覆盖。Draft/Plan 不会取代 `PANVARA_MODULE_SOURCE`：Server 仍从启动配置读取当前运行模块。

P0-02a Publish 同样不增加环境变量。`plan_id` 与 `Idempotency-Key` 必须随每个 POST 显式提交，Release ID 由 Server 生成并持久化。Publish 不会改写 `PANVARA_MODULE_SOURCE`、`PANVARA_MODULE_FORMAT` 或任何 Project 配置；重启后 Runtime 仍由同一启动 Source 决定。使用与验收见[发布事实指南](../modules/release-publishing.md)。

## 测试变量

| 变量 | 默认 | 说明 |
| --- | --- | --- |
| `PANVARA_TEST_DATABASE_URL` | 空 | 指向专用 PostgreSQL 18.4；设置后集成测试不启动临时容器 |
| `PANVARA_REQUIRE_DOCKER` | 空 | `1` 表示数据库或 Docker 不可用时必须失败；Make 集成目标已设置 |

测试数据库必须可以丢弃，不要指向包含真实业务数据的数据库。

## 安全规则

- `.env.example` 只能保存非敏感示例，Token 必须保持为空。
- `.env` 和 `.env.local` 不提交 Git。
- 生产环境不要复用示例数据库密码或本地管理员 Token。
- CLI 参数通常能被同机进程观察，管理员 Token 不使用 `--admin-token`。
- PostgreSQL 只保存 Credential 的 SHA-256 digest、hint 与 metadata，不保存原始 Token；digest 也是敏感认证材料，不应查询、导出或记录。
- 不把 Credential 通过等同于已经授权；持久化 Grant 被撤销或作用域停用时，正确 Token 的 Admin 请求仍应返回 403；权威状态不可用时返回 503。
- 签发 API 只在成功 201 响应返回原始 Token 一次；接收方必须直接存入 Secret Store 或 0600 临时文件，禁止输出 Response Body。
- 生产凭据最终应由 Secret Manager 通过受控引用注入；alpha.2 尚未实现 Secret Reference Runtime。
