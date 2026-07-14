<!--
    Panvara
    README.md    2026-07-14
     ______     __  __     ______     ______     __     __
    /\  ___\   /\ \_\ \   /\  ___\   /\___  \   /\ \  _ \ \
    \ \___  \  \ \  __ \  \ \  __\   \/_/  /__  \ \ \/ ".\ \
     \/\_____\  \ \_\ \_\  \ \_____\   /\_____\  \ \__/".~\_\
      \/_____/   \/_/\/_/   \/_____/   \/_____/   \/_/   \/_/.com

    @link    : https://github.com/shezw/panvara
    @author  : shezw
    @email   : hello@shezw.com
-->

# Panvara

Panvara 是面向中小开发团队的、数据模型驱动的可组合全栈框架。它希望用同一套 Core 构建 App、Website、管理后台和 E-commerce，并通过可替换的全球化 Provider 接入身份、支付、消息、存储与其他第三方能力。

当前版本是 **v0.1.0-alpha.2**：仓库已经形成“模块声明 → Canonical IR → OpenAPI/Manager Schema → PostgreSQL CRUD → HTTP API”的最小纵向闭环，仍是实验版本，不宣称具备生产级业务能力。

## 快速开始

开发环境使用 Go 1.26.5；模块最低兼容 Go 1.25。

    make verify
    make run

`make verify` 是不启动 Docker 的快速回路。Lite 默认监听 `127.0.0.1:8080`：

- `GET /healthz`：进程存活。
- `GET /readyz`：Core 是否可服务。
- `GET /version`：独立版本轴。

运行 alpha.2 Server：

    make infra-up
    set -a; . ./.env.example; set +a
    export PANVARA_ADMIN_TOKEN="$(openssl rand -hex 32)"
    make run-server

Panvara 不自动读取 `.env`；示例文件只提供非敏感配置。管理员 Token 只建议通过环境变量或 Secret Manager 注入，不要写入模块、配置文件、命令行参数或 Git。

Server 启动时直接编译 [`examples/modules/crm-leads.yaml`](examples/modules/crm-leads.yaml)，自动执行 PostgreSQL migration，并提供：

- `POST /api/public/v1alpha1/crm.leads/lead`：匿名创建 Lead。
- `/api/admin/v1alpha1/crm.leads/{resource}`：Bearer Token 保护的管理 CRUD 与等值过滤。
- `/api/core/v1alpha1/modules/crm.leads/openapi.json`：生成的 OpenAPI。
- `/api/core/v1alpha1/modules/crm.leads/ui-schema.json`：生成的 Manager UI Schema。

Manager UI Schema 只是前端可消费的描述，alpha.2 尚未包含可视化 Manager 应用。当前也不支持动态排序、模块发布/激活/回滚、Outbox、Provider 或 Worker；这些属于 alpha.3 及后续阶段。

> alpha.2 持久化风险：Server 每次启动都会从 Source 重新计算 Revision。任何 Canonical IR 变化都会形成全新的空数据命名空间；旧 Revision 的 Record、唯一值和引用完整保留，但不会自动迁移或重绑定。只有切回完全相同的 Source/Hash 才会重新访问旧命名空间。alpha.3 发布/迁移能力完成前，持久化环境必须保存不可变 Source + Hash 并备份数据库；覆盖 Source 不是升级。

需要验证真实 PostgreSQL 18.4 与 Server HTTP 持久化链路时：

    make test-e2e

该命令需要可用 Docker，或通过 `PANVARA_TEST_DATABASE_URL` 指向 PostgreSQL 18.4；不满足条件会失败，不会跳过。

## 设计文档

- [总体架构](docs/arch.md)
- [开发环境](docs/development.md)
- [Core v0 版本与边界](docs/core-v0.md)
- [验证测试框架](docs/testing.md)

## 当前原则

- Core 默认以单进程模块化单体启动，也允许按 Profile 组合和部署。
- alpha.2 已实现 Lite 与 Server；Manager、Site、Commerce 和 Distributed Profile 仍会 fail-fast。
- 业务模型、应用编排、接口与基础设施遵循单向依赖。
- Lite 不要求外部服务；Server 只要求 PostgreSQL，不要求 Valkey、NATS、MinIO 或 Kubernetes。
- 模块和 Provider 使用显式协议版本；分发版本不替代协议兼容性。
- 动态模型只能表达受控数据和动作，不允许任意代码、SQL 或 Shell。

## License

[MIT](LICENSE)
