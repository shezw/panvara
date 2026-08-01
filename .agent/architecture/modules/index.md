<!--
    Panvara
    docs/modules/index.md    2026-07-18
     ______     __  __     ______     ______     __     __
    /\  ___\   /\ \_\ \   /\  ___\   /\___  \   /\ \  _ \ \
    \ \___  \  \ \  __ \  \ \  __\   \/_/  /__  \ \ \/ ".\ \
     \/\_____\  \ \_\ \_\  \ \_____\   /\_____\  \ \__/".~\_\
      \/_____/   \/_/\/_/   \/_____/   \/_____/   \/_/   \/_/.com

    @link    : https://github.com/shezw/panvara
    @author  : shezw
    @email   : hello@shezw.com
-->

# 模块使用指南

## 用途

这里是 Panvara 各模块的用户手册入口。它面向第一次接触项目、不会阅读 Go 源码的验收者，回答三件事：模块现在能做什么、如何用最少步骤运行、怎样判断结果正确。

建议先完成 [运行模式](runtime-profiles.md)、[执行作用域与访问内核](project-access.md)、[Project-local 访问管理](access-administration.md)和 [CRM Leads 示例](crm-leads.md)，再按需要了解模型与 API 的细节。

## 当前状态

当前 Distribution 对应 **Panvara v0.1.0-alpha.2**。alpha.2 已经形成“声明模型 → 启动 Server → PostgreSQL 持久化 → HTTP CRUD”的最小闭环；当前开发分支另包含 alpha.3a Revision Registry、alpha.3b Draft/Validate/Plan、P0-02a Publish Facts、Server Core P0-01a 执行作用域/访问内核与 P0-01b Project-local 访问管理切片，但仍是实验版本。

| 指南 | 当前状态 | 适合解决的问题 |
| --- | --- | --- |
| [AppModule](appmodule.md) | alpha.2 声明/编译切片 | 怎样用 YAML/JSON 描述数据、API 和管理界面信息 |
| [Revision Registry](revision-registry.md) | alpha.3a 开发切片 | 怎样查询不可变启动 Revision 与第一次登记的 Source |
| [Draft 与 Change Plan](draft-planning.md) | alpha.3b 开发切片 | 怎样保存候选 Source、定位错误并在执行前解释变化 |
| [Module Release 发布事实](release-publishing.md) | P0-02a runnable slice | 怎样把有效 Plan 发布为不可变事实，并证明没有激活或迁移 |
| [Record Runtime](record-runtime.md) | alpha.2 基础 CRUD 切片 | 数据如何创建、读取、修改、删除和校验 |
| [HTTP API](http-api.md) | alpha.2 基础接口切片 | 怎样通过 `curl` 或其他客户端调用 Panvara |
| [运行模式](runtime-profiles.md) | Lite/Server 可运行装配 | 什么时候不需要数据库，什么时候需要 PostgreSQL |
| [Project Context](project-context.md) | alpha.2 值对象 + P0-01a 持久化切片 | 项目 ID、语言、时区、币种和默认 Environment 怎样初始化 |
| [执行作用域与访问内核](project-access.md) | P0-01a 开发切片；不是完整 P0-01/IAM | Token 如何认证、持久化 Grant 如何授权，以及撤权为何跨重启生效 |
| [Project-local 访问管理](access-administration.md) | P0-01b runnable slice；不是完整 P0-01/IAM | 怎样管理 Service Principal、一次性 Credential、固定 Owner Grant、轮换与终态 |
| [CRM Leads](crm-leads.md) | alpha.2 参考纵向切片 | 怎样从零验收一条真实业务链路 |

文档中出现“计划”“alpha.3+”的内容均不能作为当前验收结果；只有明确标注 alpha.3a、alpha.3b、P0-01a、P0-01b 或 P0-02a 切片的能力可以按对应 Guideline 验收。alpha.3b 的 Plan 不表示发布；P0-02a 的 Publish 不表示 Activate 或迁移；P0-01b 也不表示 Account/ExternalIdentity/Session/ProjectMembership、动态 Role/Policy、RecordOwner 或多 Environment 数据隔离已经实现。

## 前置条件

阅读文档没有技术前置条件。实际操作时建议准备：

- Git，用于下载仓库。
- Go 1.26.5；项目最低兼容 Go 1.25。
- Make，用于执行统一命令。
- `curl`，用于验收 HTTP 接口。
- `jq`，用于 Revision Registry JSON 验收。
- Docker 与 Docker Compose，仅在运行 Server、本地 PostgreSQL 或端到端测试时需要。

如果暂时没有 Docker，可以先运行 Lite，它不需要数据库。

## 最小示例

第一次验收按以下顺序操作：

1. 在 [运行模式](runtime-profiles.md) 中启动 Lite，确认 `/healthz`、`/readyz` 和 `/version`。
2. 启动 PostgreSQL 和 Server。
3. 在 [执行作用域与访问内核](project-access.md) 中确认当前 Project ID 首次启动时持久化 Project、生成默认 Environment，并创建 Principal 与 Owner Grant。
4. 在 [Project-local 访问管理](access-administration.md) 中创建 Service Principal，按 issue → verify → revoke 验证 Credential 轮换。
5. 在 [CRM Leads](crm-leads.md) 中创建 Organization 与 Lead。
6. 在 [HTTP API](http-api.md) 中确认认证、ETag 和错误响应。
7. 在 [Revision Registry](revision-registry.md) 中确认启动 Revision 只登记一次。
8. 在 [Draft 与 Change Plan](draft-planning.md) 中保存无效候选，修正后检查变化计划。
9. 在 [Module Release 发布事实](release-publishing.md) 中发布有效 Plan，确认 Candidate 进入 Registry 但 Runtime 不变。
10. 在 [AppModule](appmodule.md) 中复制最小模型，开始定义自己的模块。

只想快速确认代码质量时，在仓库根目录执行：

```bash
make verify
```

该命令不启动 Docker，也不会运行 PostgreSQL 集成测试。

## 配置

这些指南采用统一约定：

- “可运行/已验证切片”表示当前仓库中有对应代码和自动化测试，但不自动推导模块功能完整或生产就绪。
- “计划”表示设计方向，不应尝试按当前命令启动。
- 所有命令默认在仓库根目录执行。
- 默认服务地址是 `http://127.0.0.1:8080`。
- 示例配置只适用于本机开发，不是生产部署模板。
- 管理员 Token 使用 `<admin-token>` 或环境变量表示，不把真实凭据写入 Git。
- Credential 只认证 Principal；Admin 授权以 PostgreSQL 中 active Credential、active Principal 与未撤销的精确 Owner Grant 为准。

后续每个功能阶段都应同步更新对应模块指南；没有使用说明和可复现验收步骤的模块，不视为完成用户交付。

## 验收

一份可验收的模块指南至少应满足：

- 包含用途、状态、前置条件、最小示例和配置说明。
- 给出可复制的命令以及成功时应观察到的状态码或输出。
- 明确写出当前限制，避免把设计目标当成已经实现。
- 说明模型或数据升级是否安全。
- 所有站内链接可从本页到达。

当前技术门禁见 [验证测试框架](../testing.md)。这些门禁包含 P0-01a/P0-01b 最小访问闭环与 P0-02a Publish Facts，但不等于完整 P0-01/P0-02。非专业验收者先完成[执行作用域与访问内核](project-access.md#验收)、[Project-local 访问管理](access-administration.md#验收)及 [CRM Leads](crm-leads.md) 用户旅程，再按 [Revision Registry 完整验收](../getting-started/revision-registry-acceptance.md)、[Draft → Validate → Plan 完整验收](../getting-started/draft-plan-acceptance.md)和 [Draft → Publish 完整验收](../getting-started/draft-publish-acceptance.md)依次验证。

## 常见问题

### 我应该从哪一页开始？

从 [运行模式](runtime-profiles.md) 开始。如果 Lite 已经成功，再进入 [CRM Leads](crm-leads.md)。

### 文档中的 Manager 是可以打开的管理后台吗？

不是。alpha.2 只生成 Manager UI Schema，尚未提供可视化 Manager 应用。

### 可以跳过 PostgreSQL 吗？

Lite 可以；Server 和 Record Runtime 不可以。

### Registry List 第一项是当前模块吗？

不是。Registry 不记录活动状态。当前运行 Revision 只能从 OpenAPI 的 `x-panvara-revision` 获取。

### 有 Registry 后还需要保存模块 Source 吗？

Registry 会保存第一次登记的 Source，P0-02a 也能把有效 Candidate 发布为不可变事实。P0-02b 只允许数据身份完全不变的 compatible Release 激活；迁移与回滚仍不存在。仍应把模型当作版本化源码维护，并在首次激活前备份数据库。

### Validation 通过后是否可以直接上线？

不可以。Validation 只产生 Candidate 身份，Change Plan 只解释它相对 Baseline 的变化；Candidate 不进入 Registry，当前运行 Revision 和业务 Record 都不会改变。

### Publish 成功后是否可以直接上线？

不可以直接上线。P0-02a 只登记 Candidate Revision 和 Module Release，Publish 响应仍为 `activated=false`、`runtime_changed=false`。只有数据身份完全不变的 compatible Release 可以再显式 Activate；其他结果失败关闭。

### Token 正确是否就拥有 Admin 权限？

不是。Credential 只认证一个 project-local Principal；持久化 `project.owner` Grant 才授权。Project、默认 Environment、Principal、Credential 或 Grant 非 active/已撤销时都会被拒绝。Principal disable 与 Credential revoke 是终态；Grant 可由另一个 Owner 显式 PUT 重新授予，但重启/bootstrap 不会自动恢复。

## 当前限制

- 文档站本身只描述当前仓库能力，不代表 Panvara 已进入稳定版本。
- alpha.2 只有 Lite 和 Server 两种可运行 Profile。
- alpha.3a Registry 只有启动登记和 owner 只读接口，没有发布或活动版本管理。
- alpha.3b 有 Draft、Validation、Change Plan 与 Publish Facts；P0-02b 增加了窄 compatible Activate，但仍没有 Draft UI、Review/Migration 激活、Rollback 或数据迁移执行。
- P0-01b 只有 project-local Service Principal/API Credential 与固定 Owner Grant；没有 Account、ExternalIdentity、Session、ProjectMembership、动态 Role/Policy 或 RecordOwner。
- Record、Revision 与 Draft 尚无 `environment_id`，只有默认 Environment 可以执行，未实现多 Environment 数据隔离。
- 没有可视化 Manager、Provider Runtime、消息队列或分布式控制面。
- 示例以单机、单项目和本地 PostgreSQL 为主。
- 生产安全、容量和升级策略尚未形成稳定承诺。

## 兼容与升级

这些页面随 Panvara Distribution 版本维护，当前基线仍为 `v0.1.0-alpha.2`；alpha.3a、alpha.3b、P0-01a 与 P0-01b 是开发切片。升级时不要只看 Panvara 版本，还要核对 AppModule API、Module Revision、按 format 选择的 Data Schema Identity、Draft Version、Validation/Plan format、Source Hash 和数据库迁移版本。

Migration 0004 升级后，当前 Project ID 第一次由 Server 装配时会创建持久化 Project、生成默认 Environment，并创建 bootstrap Principal 与 Owner Grant。此后以同一 Project ID 启动时必须保持其 Project/Environment 设置一致；已撤销 Grant 不会因重启恢复。新的 Project ID 与唯一 Key 会创建另一套事实，不会迁移旧数据。该迁移没有给现有业务事实增加 `environment_id`，因此不能把升级解释为获得多 Environment 隔离。决策背景见 [ADR-0004](../adr/0004-persistent-execution-scope-access-kernel.md)。

Migration 0005 在下一次启动时用原 bootstrap Token 原子写入 digest、hint、Credential 与永久 marker。marker 创建后重启可省略 Token；相同值可核对，不同值拒绝。任何 revoked/disabled 终态不会由启动或升级恢复。决策背景见 [ADR-0005](../adr/0005-project-local-access-administration.md)。

Migration 0006 增加 append-only Module Release 与发布专用幂等绑定。它只追加发布事实，不改写旧 Revision、Draft、Plan 或 Record；未来 Activate/Rollback 必须引用 Release 并追加新事实。决策背景见 [ADR-0006](../adr/0006-immutable-module-release-publish-facts.md)。

在 alpha.3 的发布与迁移能力完成前：

1. 将已经运行的模块 Source 与启动日志中的 Revision Hash 一起保存为不可变制品。
2. 修改 Source 前备份数据库。
3. 先在可丢弃环境验收新 Source。
4. 不要用“覆盖原文件并重启”代替数据升级。
