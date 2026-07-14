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

Panvara 是面向中小开发团队的、数据模型驱动的可组合全栈框架。它希望用同一套 Core 构建 App、Website、管理后台和 E-commerce，并通过可替换 Provider 接入全球身份、支付、消息和存储能力。

当前版本是 **v0.1.0-alpha.2**，已经形成“模型声明 → 编译与接口描述 → PostgreSQL CRUD → HTTP API”的最小闭环。它适合本地开发和架构验收，暂不适合直接承载生产业务。

## 快速开始

第一次使用，请从[使用与验收 Guideline](docs/getting-started/index.md)开始。它按非专业技术人员视角说明安装、编译、本地环境、API 操作、预期结果和故障恢复。

只验收不依赖数据库的 Lite：

```sh
make doctor
make build
make run
```

另一个终端检查：

```sh
curl -fsS http://127.0.0.1:8080/healthz
curl -fsS http://127.0.0.1:8080/readyz
curl -fsS http://127.0.0.1:8080/version
```

验收 PostgreSQL 和 CRM Leads 完整链路：

```sh
make doctor-server
make local-init
set -a; . ./.env; . ./.env.local; set +a
make infra-up
make run-server
```

完整的创建组织、创建线索、查询和重启持久化步骤见 [CRM Leads 完整验收](docs/getting-started/crm-leads-acceptance.md)。

## 当前可以验收

- 严格 YAML/JSON AppModule 和稳定的模型版本指纹。
- 11 种字段类型、校验、唯一值、引用、等值过滤和软删除。
- PostgreSQL 18.4 持久化与 Server 重启恢复。
- Public Create 与 Bearer Token 保护的 Admin CRUD。
- 生成的 OpenAPI 3.1 与 Manager UI Schema。
- Lite 与 Server 两种运行方式。

Manager UI Schema 只是前端可消费的描述，尚未包含可视化 Manager。模块在线发布/回滚、完整身份、Provider、支付、Outbox、Worker 和分布式管理也仍在后续阶段。

> alpha.2 数据提醒：修改模型会形成新的独立数据空间。旧数据仍保留，但不会自动迁移到新模型。请保存原模型和版本指纹，修改前备份数据库；覆盖模型文件不等于升级。

## 文档站

文档源是普通 Markdown，使用 VitePress 1.x 稳定版提供本地搜索、响应式导航、暗色模式和 Mermaid。维护文档需要 Node.js 22+：

```sh
make docs-setup
make docs-serve
```

打开 `http://127.0.0.1:5173`。提交前执行 `make docs-check`。

项目约束：任何用户可感知模块的变化都必须在同一变更中更新对应引导文档；没有可执行引导和验收步骤的功能不视为完成。详见[文档同步规范](docs/contributing/documentation.md)。

## 文档入口

- [使用与验收 Guideline](docs/getting-started/index.md)
- [模块指南](docs/modules/index.md)
- [命令参考](docs/reference/commands.md)
- [配置参考](docs/reference/configuration.md)
- [文档同步规范](docs/contributing/documentation.md)
- [开发环境](docs/development.md)
- [总体架构](docs/arch.md)
- [验证测试框架](docs/testing.md)
- [Core v0 版本与边界](docs/core-v0.md)

## License

[MIT](LICENSE)
