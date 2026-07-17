<!--
    Panvara
    docs/architecture-review-server-current.md    2026-07-18
     ______     __  __     ______     ______     __     __
    /\  ___\   /\ \_\ \   /\  ___\   /\___  \   /\ \  _ \ \
    \ \___  \  \ \  __ \  \ \  __\   \/_/  /__  \ \ \/ ".\ \
     \/\_____\  \ \_\ \_\  \ \_____\   /\_____\  \ \__/".~\_\
      \/_____/   \/_/\/_/   \/_____/   \/_____/   \/_/   \/_/.com

    @link    : https://github.com/shezw/panvara
    @author  : shezw
    @email   : hello@shezw.com
-->

# 当前 Server 架构事实审计

本文以包含 migration `0004_project_environment_access.sql` 的当前实现为审计基线，只记录能够从代码和测试直接确认的事实，不描述未来方案。未来范围与待办分别见 [Server Core 能力清单](roadmap/server-core) 和 [Manager 范围与验收](roadmap/manager)。

## 状态结论

当前存在一个可以启动的 `server` Profile。它验证的是：单个进程加载单个 Project 与单个 AppModule、连接 PostgreSQL、提供基础 Record API，保存 Revision Registry 与 Draft/Validation/Plan，并持久化一个默认 Environment、`bootstrap-admin` Principal 和精确作用域内的 `project.owner` Grant。

P0-01a 的可运行切片已经完成：Record 的 5 个用例、Revision 的 3 个读取用例与 Draft 的 8 个用例都在 Application 层接收完整 `Execution` 并调用同一个 Access Kernel。完整 P0-01 尚未完成；当前没有 Account、Membership、持久化 Credential 生命周期、Record Owner、动态 Role/Policy、多 Environment 事实隔离，也没有尚未实现的 Release、Migration 与 Provider 用例授权。

仓库中没有表示“Server Core 完成”的独立成熟度模型。`internal/bootstrap/profile/profile.go` 的布尔字段只被启动入口用于判断 Profile 是否有可执行装配路径；它不检查身份、发布、迁移、事件、Provider、Worker 或分布式能力。

## 运行装配

`cmd/panvara/server_runtime.go:buildServerApplication` 是当前 Server 的组合根，按以下顺序装配：

1. 从一个文件读取最多 1 MiB 的 YAML 或 JSON Source。
2. 使用 AppModule Compiler 生成 Canonical IR、Revision、OpenAPI 与 Manager UI Schema。
3. 从启动配置构造一个 `ProjectContext`、一个匿名 Actor、一个 `bootstrap-admin` Actor 与进程内 Token 验证器。
4. 连接一个 PostgreSQL 数据库并执行包含 `0004` 的 migration。
5. `ProjectAccessStore.EnsureBootstrapScope` 在某个 Project ID 首次启动时原子创建 Project、默认 Environment、Principal 与 Owner Grant；同一 Project ID 的后续启动只核对持久化身份和设置并返回同一 Scope。新的 Project ID 与唯一 Key 会形成另一套 Scope。
6. 以 `ProjectAccessStore` 作为 `GrantReader` 装配 Application Access Policy。
7. 登记 bootstrap Module Revision，装配 Draft Workflow、Record Service 与 HTTP Handler。
8. 使用数据库 Ping 作为 readiness 来源。

```mermaid
flowchart LR
    Config["CLI / environment configuration"] --> Source["One AppModule source file"]
    Source --> Compiler["AppModule Compiler"] --> Artifacts["IR / Revision / OpenAPI / Manager Schema"]
    Config --> Context["ProjectContext + bootstrap token verifier"]
    Config --> PG[("PostgreSQL")]
    PG --> Migration["0001..0004 migrations"]
    Migration --> Bootstrap["EnsureBootstrapScope"]
    Bootstrap --> Facts["Project + default Environment<br/>Principal + project.owner Grant"]
    Facts --> Scope["Persistent Project/Environment Scope"]
    Scope --> HTTP["HTTP identity composition"]
    Context --> HTTP
    Artifacts --> HTTP
    HTTP --> Records["Record: 5 use cases"]
    HTTP --> Revisions["Revision: 3 read use cases"]
    HTTP --> Drafts["Draft: 8 use cases"]
    Records --> Policy["Application Access Policy"]
    Revisions --> Policy
    Drafts --> Policy
    Policy -->|"scope + active grant lookup"| Facts
    Records --> PG
    Revisions --> PG
    Drafts --> PG
    Policy -->|"deny / unavailable: no business store call"| Rejected["Fail closed"]
```

## 已存在的代码边界

| 层 | 当前组件 | 直接代码依据 |
| --- | --- | --- |
| Spec | 严格 AppModule v1alpha1 YAML/JSON DTO、Schema 与解码 | `internal/spec/appmodule/v1alpha1` |
| Domain | AppModule Descriptor、Revision、Draft；Project、Environment、Scope、Actor、Money 值类型 | `internal/domain/appmodule`、`internal/domain/project`、`internal/domain/actor`、`internal/domain/value` |
| Application | Access Kernel、Compiler、Catalog、Revision Registry、Draft Workflow、Record CRUD | `internal/application/access`、`internal/application/appmodule`、`internal/application/record` |
| Interfaces | 健康/就绪/版本端点、模块制品、Admin/Public Record、Revision 与 Draft HTTP | `internal/interfaces/httpserver`、`internal/interfaces/httpapi` |
| Infrastructure | PostgreSQL Project/Access、Record、Revision、Draft Store 与 migration | `internal/infrastructure/postgres`、`db/migrations` |
| Bootstrap | Lite/Server Profile 选择与 Server 组合根 | `internal/bootstrap/profile`、`cmd/panvara` |

## 持久化执行作用域

Migration `0004_project_environment_access.sql` 新增四组事实：

| 事实 | 当前约束与行为 |
| --- | --- |
| `panvara_project` | UUIDv7 `project_id`、唯一 `project_key`、默认 Locale/Time Zone/Currency 与 `active`/`disabled` 状态；启动配置与已有设置冲突时拒绝启动 |
| `panvara_environment` | Project 内 UUIDv7 身份与 Key；部分唯一索引保证每个 Project 至多一个 `is_default=true`；bootstrap 创建一个默认 Environment |
| `panvara_principal` | Project 内 Principal 身份与 `active`/`disabled` 状态；当前组合根只创建固定 `bootstrap-admin` |
| `panvara_access_grant` | 精确绑定 Project、Environment、Principal 与 Role；数据库约束当前只接受 `project.owner`，`revoked_at` 表示撤销 |

首次初始化按 Project 获取事务级 advisory lock，在一个事务内插入四组事实。相同设置的重启复用持久化 Environment ID；Project 已存在后，启动只核对 Project 设置、默认 Environment Key 与 Principal，不会补回缺失或已撤销的 Grant。

`ProjectAccessStore.ScopeActive` 只把 active Project 下 active 且标记为默认的精确 Environment 视为可执行。`0001`–`0003` 的 Record、Revision 与 Draft 表仍只有 `project_id`，没有 `environment_id`；因此当前是“执行契约携带 Environment，但只允许默认 Environment”，不是多 Environment 数据隔离。

bootstrap Admin Token 没有写入 `0004` 表。`BootstrapAdminAuth` 仍在进程内保存 Token 的 SHA-256 摘要并把匹配请求认证成固定 Principal；Token 只完成认证，Admin 授权取决于数据库中的 active、未撤销 Owner Grant。

## Application Access Kernel

`internal/application/access` 定义 `Execution = Project/Environment Scope + Actor + Surface`、16 个固定 Operation、`Authorizer` 与 `GrantReader`。Actor 所属 Project 必须与 Scope Project 一致，Surface 只接受 `public` 或 `admin`。Policy 不读取 Actor 自报 Role：它先查询精确 Scope 是否 active，再按 Surface 和 Operation 决定是否读取持久化 `project.owner` Grant；未知操作或权威状态读取失败都 fail closed。

| Application 边界 | 已授权用例 | Operation 数量 |
| --- | --- | --- |
| Record Service | List、Get、Create、Update/Patch、Delete | 5 |
| Revision Registry | List、Get、GetSource | 3 |
| Draft Workflow | Create、Get、GetSource、Replace、Validate、Plan、GetValidation、GetPlan | 8 |

Public Surface 只能通过 Access Kernel 进入 Record Operation，之后 Record Service 仍执行 AppModule 的资源/Surface/Operation Policy。Admin Surface 要求非匿名 Actor 和精确作用域内 active、未撤销的 `project.owner` Grant。HTTP Adapter 负责验证 Token 与构造 `Execution`，但最终允许/拒绝发生在 Application 用例内。

`RevisionRegistry.RegisterBootstrap` 是当前唯一不接收 `Execution` 的登记路径；它只由 Server 组合根在 migration 和 Scope 初始化后调用，没有 HTTP 路由。

## AppModule 当前表达力

`internal/spec/appmodule/v1alpha1/document.go` 与 `internal/domain/appmodule/descriptor.go` 当前包含：

- Module name、SemVer、labels、module requirements、capabilities 与 conflicts。
- Resource 与 Field。
- `string`、`text`、`int`、`bool`、`decimal`、`enum`、`date`、`datetime`、`email`、`money`、`reference` 共 11 种 Field Kind。
- Required、Unique、Reference、Enum、Max Length、Precision 与 Scale。
- Public/Admin operation、writable、filterable 与 sortable 声明；当前非空 sortable 会被拒绝。
- Manager list columns、filters 与 form fields 的展示顺序。

Compiler 位于 `internal/application/appmodule/compiler.go`。它输出 Canonical IR、完整 Revision Hash、Data Schema format 1 fingerprint、OpenAPI 3.1 与 `manager.panvara.dev/v1alpha1` UI Schema。

## Record 当前链路

`internal/application/record/service.go` 暴露 Create、Get、List、Update 与 Delete。PostgreSQL 实现位于 `internal/infrastructure/postgres/store.go`。

当前 Record Scope 由 Project、Module、Resource 与完整 Module Revision 组成。数据库保存：

- flex JSONB Record 与乐观版本。
- 唯一值索引。
- Reference 索引。
- 创建、更新时间与软删除时间。

HTTP 表面提供 Public Create 与模型允许的 Admin CRUD。Patch 和 Delete 使用强 ETag/`If-Match`；List 使用 limit、cursor 与等值 `filter[field]`。每个 Record Application 方法同时接收访问 `Execution` 与 Record Scope，先校验二者 Project 一致，再经过 Access Kernel 和 AppModule Operation Policy，最后才调用 Record Store。

## 模型变化准备链路

当前启动 Source 与候选 Source 是两条分离链路：

```mermaid
flowchart TB
    Startup["Configured startup Source"] --> Compile["Compile"]
    Compile --> Runtime["Current runtime module"]
    Compile --> Registry["Immutable bootstrap Revision"]

    Candidate["Candidate raw Source"] --> Draft["Versioned Draft"]
    Draft --> Validation["Immutable Validation"]
    Validation --> Plan["Immutable Change Plan"]
```

`internal/application/appmodule/revision_registry.go` 只登记和读取不可变 bootstrap Revision。`internal/application/appmodule/draft_workflow.go` 支持 Draft Create/Get/Replace、Validation 与 Plan。二者都没有 active pointer，也不会把 Candidate 切换到 Runtime。

## 当前数据库事实

| Migration | 当前权威事实 |
| --- | --- |
| `0001_flex_record.sql` | Record、唯一值与 Reference |
| `0002_module_revision_registry.sql` | Module Revision 与 Data Schema Identity |
| `0003_module_draft_workflow.sql` | Draft、Validation 与 Change Plan |
| `0004_project_environment_access.sql` | Project、Environment、Principal 与 `project.owner` Grant |

`0004` 是追加 migration，不修改 `0001`–`0003` 的表或历史数据。仓库当前 migration 中仍没有 Account、External Identity、Session、Membership、Credential、动态 Role/Policy、Record Owner、Release、Activation、Migration Run、Audit、Outbox、Event、Job、Provider、Asset、Node 或 Partition 表。

## 当前运行与运维接口

- `GET /healthz`：进程存活。
- `GET /readyz`：Lite 固定就绪；Server 使用 PostgreSQL Ping。
- `GET /version`：构建版本信息。
- Server 启动缺少数据库、Module Source、Project ID 或 bootstrap admin Token 时 fail fast。
- 当前只有一个 HTTP Listener，没有 gRPC、Worker 进程或消息消费者入口。

## 当前不存在的实现

在本次审计的 Go package、migration 与前端文件中，未发现以下实现：

- 可视化 Manager 应用；现有 Manager UI Schema 是 JSON 制品。
- Account、外部 Identity、Session、Team、Membership、API Credential/Service Account 生命周期、动态 Role/Permission/Policy 或 Grant 管理 API。
- Record Owner、字段/动作级授权，以及既有事实的 `environment_id`、回填、复合约束、游标和幂等隔离；非默认 Environment 当前会被拒绝。
- Publish、Activate、Rollback、活动 Revision、Release Snapshot、epoch 或数据迁移执行器。
- Release、Migration、Provider、Job/Event 用例及其 Access Kernel Operation；这些用例本身尚不存在。
- Audit Event、Transactional Outbox、Inbox、Event Bus、Job、Schedule、Retry 或 Dead Letter。
- Provider Descriptor、Provider Instance、Credential Reference、Capability Resolution 或第三方 Adapter Runtime。
- Asset/Object Storage、Search、Site、Commerce 或 Payment Runtime。
- 多 Project 请求路由、多节点发布收敛、Partition 路由、分布式锁、缓存或服务发现。

这些条目只表示当前仓库中没有相应实现，不表示未来范围或优先级。
