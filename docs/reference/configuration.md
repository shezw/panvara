<!--
    Panvara
    docs/reference configuration.md    2026-07-28
     ______     __  __     ______     ______     __     __
    /\  ___\   /\ \_\ \   /\  ___\   /\___  \   /\ \  _ \ \
    \ \___  \  \ \  __ \  \ \  __\   \/_/  /__  \ \ \/ ".\ \
     \/\_____\  \ \_\ \_\  \ \_____\   /\_____\  \ \__/".~\_\
      \/_____/   \/_/\/_/   \/_____/   \/_/   \/_/.com

    @link    : https://github.com/shezw/panvara
    @author  : shezw
    @email   : hello@shezw.com
-->

# 配置

::: info Current Distribution
本页只列运行当前 Distribution 标识 **v0.1.0-alpha.2** 对应的 Lite/Server 和本地 PostgreSQL 所需用户配置，不包含测试门禁或文档部署变量。
:::

## 加载规则

优先级是：

1. CLI 参数；
2. 环境变量；
3. 内置默认值。

Panvara 不会自动读取 `.env`。本地使用：

```sh
make local-init
set -a; . ./.env; . ./.env.local; set +a
```

`make local-init` 不覆盖已有文件。`.env.local` 包含 Secret，不得提交、打印或复制到 Issue。

## Lite 与通用配置

| 变量 | Profile | 必填 | 默认 | 敏感 | 说明 |
| --- | --- | :---: | --- | :---: | --- |
| `PANVARA_PROFILE` | Lite/Server | 否 | `lite` | 否 | `make run-server` 会显式选择 `server` |
| `PANVARA_HTTP_ADDR` | Lite/Server | 否 | `127.0.0.1:8080` | 否 | HTTP 监听地址 |

只有 `lite` 与 `server` 可以启动。Manager、Site、Commerce 和 Distributed 当前不可用。

## Server 配置

| 变量 | 必填 | 默认或示例 | 敏感 | 说明 |
| --- | :---: | --- | :---: | --- |
| `PANVARA_DATABASE_URL` | 是 | `postgres://...` | 是 | PostgreSQL URL，通常含密码 |
| `PANVARA_MODULE_SOURCE` | 是 | `examples/modules/crm-leads.yaml` | 否 | AppModule 文件路径 |
| `PANVARA_MODULE_FORMAT` | 否 | `auto` | 否 | `auto`、`yaml` 或 `json` |
| `PANVARA_PROJECT_ID` | 是 | UUIDv7 | 否 | 当前本地项目标识 |
| `PANVARA_PROJECT_KEY` | 否 | `default` | 否 | 可读项目 Key |
| `PANVARA_PROJECT_LOCALE` | 否 | `en-US` | 否 | BCP 47 Locale |
| `PANVARA_PROJECT_TIME_ZONE` | 否 | `UTC` | 否 | IANA Time Zone，例如 `Asia/Shanghai` |
| `PANVARA_PROJECT_CURRENCY` | 否 | `USD` | 否 | ISO 风格币种代码，例如 `CNY` |
| `PANVARA_ENVIRONMENT_KEY` | 否 | `default` | 否 | 当前默认环境 Key |
| `PANVARA_ADMIN_TOKEN` | 首次本地初始化 | `make local-init` 生成 | 是 | Admin Bearer Credential；优先通过环境变量注入 |

`auto` 根据 `.yaml`、`.yml` 或 `.json` 扩展名判断格式。扩展名无法识别时，显式设置 `yaml` 或 `json`。

不要通过 `--admin-token` 传真实 Token；命令行参数通常可被同机其他进程看到。

## 本地 Compose 配置

这些变量由仓库的 Compose 命令使用，不是 Panvara 二进制参数：

| 变量 | 默认 | 敏感 | 说明 |
| --- | --- | :---: | --- |
| `PANVARA_COMPOSE_PROJECT` | `panvara` | 否 | Compose Project 名；并行工作目录应使用不同值 |
| `PANVARA_POSTGRES_PORT` | `5432` | 否 | PostgreSQL 宿主端口 |

改变 PostgreSQL 端口时，也要同步修改 `PANVARA_DATABASE_URL`。仓库 Compose 使用 PostgreSQL 18.4 与 UTF8 编码，只绑定本机地址。

## 对应 CLI 参数

查看完整帮助：

```sh
./bin/panvara -h
```

常用映射：

| 环境变量 | CLI |
| --- | --- |
| `PANVARA_PROFILE` | `--profile` |
| `PANVARA_HTTP_ADDR` | `--http` |
| `PANVARA_DATABASE_URL` | `--database-url` |
| `PANVARA_MODULE_SOURCE` | `--module-source` |
| `PANVARA_MODULE_FORMAT` | `--module-format` |
| `PANVARA_PROJECT_ID` | `--project-id` |

项目 Locale、时区、币种和环境也有同名语义的 CLI 参数；Secret 仍只使用环境变量或安全 Secret 注入。

## 安全规则

- `.env.example` 只放非敏感示例，真实 Token 保持为空；
- `.env` 与 `.env.local` 已被 Git 忽略，不要改变这条规则；
- 不复用示例数据库密码或本地 Admin Token；
- 不把 Credential 放进 URL、模块 Source、README、Issue 或日志；
- 签发类 Preview API 的原始 Token 只返回一次，应直接进入 Secret Store；
- 当前没有生产级 Secret Reference、TLS 终止或凭据轮换平台。

## 修改配置后的预期

- HTTP 地址或数据库端口改变后，客户端 URL 与数据库 URL 必须同步；
- AppModule Source 改变可能产生新 Revision 与独立数据空间；
- Project 标识改变会访问另一套数据 Scope，不是重命名已有项目；
- 任何会改变 Source 或数据 Scope 的操作前，先阅读[数据与升级](/releases/compatibility)。
