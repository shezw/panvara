<!--
    Panvara
    docs/reference/configuration.md    2026-07-15
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
| `PANVARA_ADMIN_TOKEN` | 至少 32 字节 | ✓ | ✓ | 临时管理员 Bearer Token，只通过可信环境注入 |

## 本地 Compose 变量

| 变量 | 默认 | 说明 |
| --- | --- | --- |
| `PANVARA_COMPOSE_PROJECT` | `panvara` | Compose 项目名；并行工作区应设不同值 |
| `PANVARA_POSTGRES_PORT` | `5432` | PostgreSQL 宿主端口；冲突时可改为 `55432` |

改变 PostgreSQL 端口时，必须同时修改 `PANVARA_DATABASE_URL` 中的端口。

Panvara 使用的 PostgreSQL 18.4 数据库必须采用 UTF8 `server_encoding`，否则多语言 Source、标签与生成制品可能无法保存或读取。自备数据库先执行 `SHOW server_encoding;` 并确认返回 `UTF8`；仓库 Compose 创建的数据库符合该要求。当前版本尚未在 Server 启动或 Migrate 阶段自动 fail-fast 检查非 UTF8 数据库，这是后续需要补齐的运维保护。

## Revision Registry

alpha.3a Registry 开发切片不增加配置项。Server 在 migration 之后、对外就绪之前，使用当前 `PANVARA_PROJECT_ID`、`PANVARA_MODULE_SOURCE` 和 `PANVARA_MODULE_FORMAT` 进行幂等 bootstrap 登记；失败会阻止启动。

`PANVARA_ADMIN_TOKEN` 保护 Registry 读取接口，以及 alpha.3b Draft、Validation 与 Plan 的 owner 控制面接口。当前没有 active Revision、Publish、Activate 或 Rollback 配置，也不能用 Registry List 顺序配置运行版本。

`data_schema_identities` 也不是配置项。它是 Panvara 为不可变父 Revision 计算并按 format 升序返回的派生身份数组；新增算法只能追加新的 format，用户不能通过环境变量覆盖 fingerprint。

alpha.3b 不增加环境变量。Draft Baseline 必须由每个 Create 请求显式传入；Draft Version（内部 generation）、Source Hash、Validation/Plan Format 和 Idempotency Key 都是请求或持久化身份，不能用环境变量全局覆盖。Draft/Plan 不会取代 `PANVARA_MODULE_SOURCE`：Server 仍从启动配置读取当前运行模块。

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
- 生产凭据最终应由 Secret Manager 通过受控引用注入；alpha.2 尚未实现 Secret Reference Runtime。
