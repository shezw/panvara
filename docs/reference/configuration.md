<!--
    Panvara
    docs/reference/configuration.md    2026-07-14
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
