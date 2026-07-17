<!--
    Panvara
    README.md    2026-07-15
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

当前 Distribution 仍是 **v0.1.0-alpha.2**，已经形成“模型声明 → 编译与接口描述 → PostgreSQL CRUD → HTTP API”的最小闭环。当前开发分支另包含 **alpha.3a Revision Registry** 与 **alpha.3b Draft/Validate/Plan** 开发切片：前者不可变地登记启动模块，后者允许保存候选 Source、验证并解释变化；两者都不表示 alpha.3 发布生命周期已经完成。项目适合本地开发和架构验收，暂不适合直接承载生产业务。

> **状态校正：** `server` Profile 当前只是有一条可运行的最小纵向装配，不表示 Server Core 已完成。身份与授权、发布/激活/回滚、迁移、审计、Outbox/Worker、Provider 和分布式收敛仍在 [Server Core 能力清单](docs/roadmap/server-core.md)；Manager 的完整范围和验收标准见 [Manager 路线图](docs/roadmap/manager.md)。

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

继续验收启动 Revision 的不可变登记、Source 下载和重启幂等，请按 [Revision Registry 完整验收](docs/getting-started/revision-registry-acceptance.md)操作。

要在不重启、不发布和不迁移数据的前提下体验“无效 Draft → 修正 → Validation → Change Plan”，请按 [Draft → Validate → Plan 完整验收](docs/getting-started/draft-plan-acceptance.md)操作。

## 当前可以验收

- 严格 YAML/JSON AppModule 和稳定的模型版本指纹。
- 11 种字段类型、校验、唯一值、引用、等值过滤和软删除。
- PostgreSQL 18.4 持久化与 Server 重启恢复。
- Public Create 与 Bearer Token 保护的 Admin CRUD。
- 生成的 OpenAPI 3.1 与 Manager UI Schema。
- Lite 与 Server 两种运行方式。
- alpha.3a 开发切片：启动时登记不可变 Module Revision 父制品、可按算法追加的 Data Schema Identities、原始 Source 与生成物，并提供项目 owner 只读 API。
- alpha.3b 开发切片：保存带固定 Baseline 和 Draft Version 的候选 Source，以结构化 Validation 检查无效/有效输入，并生成确定性 Change Plan。

Manager UI Schema 只是前端可消费的描述，尚未包含可视化 Manager。模块 Publish/Activate/Rollback、数据迁移、完整身份、Provider、支付、Outbox、Worker 和分布式管理也仍在后续阶段。

> **登记不等于发布或激活。** Registry List 不表达当前运行 Revision；该值必须读取 OpenAPI 的 `x-panvara-revision`。alpha.3a 不改变 Record namespace，也没有 Publish、Activate、Rollback 或热切换 API。

> **身份数组不是状态列表。** 每个 `data_schema_identities` 元素只是 `{format, fingerprint}`；新增投影算法可以给同一父 Revision 追加新 format，但已有身份和父 Revision 都不能改写。

> **准备变更不等于执行变更。** alpha.3b 的 Draft、Validation 和 Plan 不登记 Candidate，不发布、不激活、不迁移 Record，也不切换当前运行 Revision。Plan 的风险结论不能当作上线许可。

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
- [Revision Registry 指南](docs/modules/revision-registry.md)
- [Revision Registry 完整验收](docs/getting-started/revision-registry-acceptance.md)
- [Draft 与 Change Plan 指南](docs/modules/draft-planning.md)
- [Draft → Validate → Plan 完整验收](docs/getting-started/draft-plan-acceptance.md)
- [命令参考](docs/reference/commands.md)
- [配置参考](docs/reference/configuration.md)
- [文档同步规范](docs/contributing/documentation.md)
- [开发环境](docs/development.md)
- [总体架构](docs/arch.md)
- [当前 Server 架构事实](docs/architecture-review-server-current.md)
- [Server Core 能力清单与完成门禁](docs/roadmap/server-core.md)
- [Manager 范围与验收标准](docs/roadmap/manager.md)
- [验证测试框架](docs/testing.md)
- [GitHub Pages 文档部署](docs/deployment/github-pages.md)
- [Core v0 版本与边界](docs/core-v0.md)
- [ADR-0001：模块与数据结构身份](docs/adr/0001-module-data-revision-identities.md)
- [ADR-0002：不可变 Revision Registry](docs/adr/0002-immutable-revision-registry.md)
- [ADR-0003：版本化 Draft、Validation 与 Change Plan](docs/adr/0003-draft-validation-change-plan.md)

## License

[MIT](LICENSE)
