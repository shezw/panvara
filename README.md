<!--
    Panvara
    README.md    2026-07-18
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

当前 Distribution 仍是 **v0.1.0-alpha.2**，已经形成“模型声明 → 编译与接口描述 → PostgreSQL CRUD → HTTP API”的最小闭环。当前开发分支另包含 **alpha.3a Revision Registry**、**alpha.3b Draft/Validate/Plan**、**P0-02a Publish Facts**、**Server Core P0-01a 执行作用域与访问内核** 和 **P0-01b Project-local 访问管理**开发切片：候选 Source 可以被验证、规划，并显式发布为不可变 Candidate Revision 与 Module Release；P0-01a/P0-01b 提供最小持久化 Scope、机器 Credential 与 Owner Grant。这些切片不表示 Candidate 已激活、数据已迁移、完整发布生命周期或完整 P0-01 已完成。项目适合本地开发和架构验收，暂不适合直接承载生产业务。

> **状态校正：** `server` Profile 当前只是有一条可运行的最小纵向装配，不表示 Server Core 已完成。P0-01b 是可运行机器身份切片，P0-02a 也只有不可变 Publish Facts；完整 P0-01 仍缺 Account、ExternalIdentity、Session、ProjectMembership、动态 Role/Policy、RecordOwner 与真正多 Environment 事实，Activate/Rollback/Migration 仍不存在。P0-05 的通用 Audit、Idempotency 和 Outbox 也未完成。后续门禁见 [Server Core 能力清单](docs/roadmap/server-core.md)。

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

要把有效 Plan 登记为不可变 Release，并亲自证明 Candidate 入 Registry 但当前 Runtime 与 Record 不变，请继续完成 [Draft → Publish 完整验收](docs/getting-started/draft-publish-acceptance.md)。

要确认某个 Project ID 的持久化 Project/默认 Environment、首启 digest + marker、Service Principal/Credential/Grant 管理、显式轮换和终态防护，请按[执行作用域与访问内核指南](docs/modules/project-access.md)与 [Project-local 访问管理指南](docs/modules/access-administration.md)操作。

## 当前可以验收

- 严格 YAML/JSON AppModule 和稳定的模型版本指纹。
- 11 种字段类型、校验、唯一值、引用、等值过滤和软删除。
- PostgreSQL 18.4 持久化与 Server 重启恢复。
- Public Create，以及“Bearer Token 认证 Principal + 持久化 Owner Grant 授权”的 Admin CRUD。
- 生成的 OpenAPI 3.1 与 Manager UI Schema。
- Lite 与 Server 两种运行方式。
- alpha.3a 开发切片：启动时登记不可变 Module Revision 父制品、可按算法追加的 Data Schema Identities、原始 Source 与生成物，并提供项目 owner 只读 API。
- alpha.3b 开发切片：保存带固定 Baseline 和 Draft Version 的候选 Source，以结构化 Validation 检查无效/有效输入，并生成确定性 Change Plan。
- P0-02a runnable slice：先在短事务内解析已发布事实；未命中的精确 `plan_id` 才复验候选链，并在首次发布事务中幂等登记 Candidate Revision、environment-scoped Module Release、专用重放绑定与成功安全审计；提供 Publish 与 Detail HTTP API。
- P0-01a 开发切片：某个 Project ID 首次由 Server 装配时持久化 Project，生成默认 Environment，并创建 `bootstrap-admin` Principal 与 Owner Grant；同一 Project ID 的相同配置重启幂等，设置漂移会拒绝启动。
- P0-01b runnable slice：首启 Token 只持久化 SHA-256 digest、hint 与永久 marker；后续可省略、相同可用、改变拒绝。Owner 可管理 Service Principal、一次性签发/显式撤销 Credential 与固定 Grant；Principal disable 与 Credential revoke 终态不可恢复，Grant 只允许已授权的显式 PUT 重新授予，最后可用 Owner path 受保护。

所有新旧 Admin 用例统一由 `Credential → project-local Principal → project.owner Grant` 授权；认证/授权权威状态不可用时 fail closed。Google、Apple、Facebook、微信等未来登录必须先经过 ExternalIdentityVerifier、Account/ExternalIdentity/Session 与 ProjectMembership，再映射 project-local Principal；Provider Role/Email 绝不直接生成 Grant。

Manager UI Schema 只是前端可消费的描述，尚未包含可视化 Manager。P0-01b 也不是完整 IAM：没有 Account/ExternalIdentity/Session/ProjectMembership、动态 Role/Policy、RecordOwner，现有 Record、Revision 与 Draft 表也尚无真正的多 Environment 事实。P0-02a 只有 Publish Facts；Activate/Rollback、数据迁移、完整身份 Provider、支付、通用 Audit/Idempotency/Outbox、Worker 和分布式管理仍在后续阶段。

> **登记或发布都不等于激活。** Registry List 和 Release 时间都不表达当前运行 Revision；该值必须读取 OpenAPI 的 `x-panvara-revision`。P0-02a 的响应明确 `published=true`、`activated=false`、`runtime_changed=false`。

> **身份数组不是状态列表。** 每个 `data_schema_identities` 元素只是 `{format, fingerprint}`；新增投影算法可以给同一父 Revision 追加新 format，但已有身份和父 Revision 都不能改写。

> **准备变更不等于执行变更。** alpha.3b 的 Draft、Validation 和 Plan 不登记 Candidate，不发布、不激活、不迁移 Record，也不切换当前运行 Revision。Plan 的风险结论不能当作上线许可。

> **发布事实不等于上线。** P0-02a 会登记 Candidate 与 Module Release，但不会迁移数据、改变启动 Source 或切换 Runtime；完整 Activate 仍未实现。

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
- [执行作用域与访问内核指南](docs/modules/project-access.md)
- [Project-local 访问管理指南](docs/modules/access-administration.md)
- [Revision Registry 指南](docs/modules/revision-registry.md)
- [Revision Registry 完整验收](docs/getting-started/revision-registry-acceptance.md)
- [Draft 与 Change Plan 指南](docs/modules/draft-planning.md)
- [Draft → Validate → Plan 完整验收](docs/getting-started/draft-plan-acceptance.md)
- [Module Release 发布事实指南](docs/modules/release-publishing.md)
- [Draft → Publish 完整验收](docs/getting-started/draft-publish-acceptance.md)
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
- [ADR-0004：持久化执行作用域并在 Application 层授权](docs/adr/0004-persistent-execution-scope-access-kernel.md)
- [ADR-0005：Project-local Principal、Credential 与 Grant 管理](docs/adr/0005-project-local-access-administration.md)
- [ADR-0006：不可变 Module Release 发布事实](docs/adr/0006-immutable-module-release-publish-facts.md)

## License

[MIT](LICENSE)
