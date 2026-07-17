<!--
    Panvara
    docs/adr/0004-persistent-execution-scope-access-kernel.md    2026-07-18
     ______     __  __     ______     ______     __     __
    /\  ___\   /\ \_\ \   /\  ___\   /\___  \   /\ \  _ \ \
    \ \___  \  \ \  __ \  \ \  __\   \/_/  /__  \ \ \/ ".\ \
     \/\_____\  \ \_\ \_\  \ \_____\   /\_____\  \ \__/".~\_\
      \/_____/   \/_/\/_/   \/_____/   \/_____/   \/_/   \/_/.com

    @link    : https://github.com/shezw/panvara
    @author  : shezw
    @email   : hello@shezw.com
-->

# ADR-0004：持久化执行作用域并在 Application 层授权

- 状态：已接受
- 日期：2026-07-18
- 适用范围：Server Core P0-01a 可运行切片

::: warning P0-01a 不等于完整身份与权限系统
本决策建立最小、权威、可持久化的授权闭环，但不提供 Account、Credential、Membership、动态 Role/Policy、Record Owner 或多 Environment 数据隔离。
:::

## 背景

此前 Server 从启动配置构造 Project Context，数据库没有 Project 或 Environment 权威事实。Record Application 用例没有统一 Actor/Scope 授权入口，Revision 与 Draft 则依赖 Actor Context 中调用方自报的 Role。HTTP Bearer Token 能限制路由，却不能保证 Application 用例在 HTTP 之外复用时仍受保护，也无法在不重启、不更换 Token 的情况下撤销权限。

Panvara 面向中小开发者，需要先保持一体化部署简单，同时为后续 Manager、Worker、Provider 和按需求拆分服务保留稳定安全边界。因此当前阶段需要解决执行身份、作用域、授权职责和持久化权威来源，而不是提前实现一个庞大 IAM 产品。

## 决策

### 持久化四类最小事实

Migration 0004 新增：

- `panvara_project`：稳定 Project 身份、默认全球化设置和状态；
- `panvara_environment`：Project 内 Environment 身份、Key、默认标记和状态；
- `panvara_principal`：Project 内可授权主体及状态；
- `panvara_access_grant`：精确 Project + Environment + Principal + Role Grant 及撤销时间。

某个 `project_id` 第一次由 Server 装配时，在单个事务中创建 Project、一个生成 UUIDv7 的默认 Environment、`bootstrap-admin` Principal 和 `project.owner` Grant。并发初始化按该 Project 取得事务级 advisory lock，结果必须收敛。相同配置重启是幂等的；同一 `project_id` 的已持久化配置与启动配置不一致时拒绝启动。

Bootstrap 对每个新的 `project_id` 只创建第一组事实。Project 已存在后，启动路径只验证身份与设置，不自动补回缺失或已撤销 Grant，避免一次重启越过运维人员的撤权决定。不同 `project_id` 配合不同的全局唯一 Project Key 会形成另一套隔离事实，因此多个固定单 Project 进程可以共享数据库；这不等于一个进程支持动态多 Project 路由。

### Execution 是每个用例的完整输入

Application 层定义不可变 Execution：

```text
Execution = Project Scope + Environment Scope + Actor + Surface
```

Actor Project 必须与 Scope Project 一致。Surface 只有 `public` 与 `admin`。每个公开用例在内部固定自己的 Operation，调用方不能用参数把低权限操作替换为高权限操作。

HTTP 层负责认证 Token 并建立 Execution，但不拥有最终授权决定。未来 gRPC、Job 或进程内调用也必须构造相同 Execution 并经过同一 Authorizer。

### Access Kernel 位于 Application 层

Authorizer 在任何业务 Store、RevisionReader 或变更逻辑之前运行：

1. 验证 Execution 完整且无 Project 矛盾；
2. 从权威存储确认 Project 与默认 Environment 都 active；
3. 拒绝未知 Operation；
4. Public Surface 仅允许 Record Operation；
5. Admin Surface 要求非匿名 Principal，并查询精确、active、未撤销的 `project.owner` Grant。

Actor Context 中的 Role 不参与授权。读取 Scope/Grant 失败时返回 unavailable 并 fail closed。

Record 在 Access Kernel 后还必须执行 AppModule Operation Policy。第一道门判断主体能否进入该用例，第二道门判断具体模型是否开放对应 Public/Admin 操作。包括 Get 与 Delete 在内的五个 Record 用例都执行两道检查。

Revision Registry 的 `RegisterBootstrap` 是唯一明确例外：它只能由 Server composition root 在迁移与作用域初始化完成后调用，不暴露给 HTTP 或通用 Application 接口。它是系统启动登记，不接受用户身份。

### 当前只允许默认 Environment

Migration 0004 不向 Record、Revision、Draft 等既有事实表添加 `environment_id`，也不声称已经实现多 Environment 隔离。Scope Reader 只将 active 的默认 Environment 判定为可执行，其他 Environment 必须拒绝。

这个限制让执行契约从现在开始携带 Environment 身份，同时避免在没有数据键、唯一约束、游标和幂等语义迁移的情况下制造安全假象。真正多 Environment 支持必须作为独立升级阶段完成全表回填和兼容验证。

## 安全不变量

1. Token 只认证 Principal；持久化 Grant 才授权 Admin 操作。
2. 请求自报 Role 永远不能创建权限。
3. 已撤销 Grant 不能因 Server 重启恢复。
4. Actor、Project、Environment 或 Operation 不完整时不得访问业务 Store。
5. Public Surface 不得读取 Revision、Draft 或未来控制面用例。
6. 非默认 Environment 在数据完成隔离迁移前不得执行现有业务用例。
7. 未知操作和权威状态读取错误必须 fail closed。
8. 数据库不保存 bootstrap Admin Token 或等价可用 Credential。

## 结果

- Server 有了可撤销、重启稳定、数据库权威的最小 Owner 授权闭环。
- Record、Revision 和 Draft 在 HTTP 之外复用时仍由 Application 边界保护。
- 模块化单体保持单进程、单数据库的低运维成本；明确的 Authorizer/GrantReader Port 允许以后按需求拆分。
- 每次授权增加 PostgreSQL 查询。P0-01a 优先正确性与即时撤销；是否加入缓存以及如何失效必须由容量测试决定。
- 环境身份已经进入 Execution，但业务数据仍只有 Project 级物理隔离；文档和 API 不能把它描述成完整多环境。

## 被拒绝的方案

- 只在 HTTP Middleware 鉴权：Application 被其他适配器调用时可绕过，且不能表达每个用例的固定 Operation。
- 信任 Token 或 Actor 中的 Role：权限撤销依赖 Token 过期，调用方还可能伪造角色。
- 启动时总是 `upsert` Owner Grant：重启会撤销撤权决定，属于权限升级。
- 现在实现完整 IAM：会把账号、登录、组织、社交身份和策略引擎塞进本切片，拖慢 Server 核心闭环并扩大攻击面。
- 只增加 Environment 表并立即允许多环境请求：既有数据表没有 Environment 键，无法提供真实隔离。
- 修改 0001–0003 历史迁移：破坏已部署数据库的升级可重复性；因此 0004 采用追加迁移。

## 后续演进

完整 P0-01 至少还需要：

- Account/Principal/Credential 生命周期、Token 摘要与轮换、Membership；
- OIDC 与全球身份 Provider（如 Google、Apple、Facebook、微信）通过可替换 Adapter 接入；
- 动态 Role/Policy、Record Owner 和字段/动作级策略；
- 为既有事实补齐 `environment_id`，完成索引、约束、幂等键、游标、回填和回滚策略；
- 将 Release、Migration、Provider、Job/Event 用例接入相同 Access Kernel；
- 审计每次敏感授权变化，并在实测需要时设计缓存与跨节点失效。

微服务拆分后，边界服务不能只相信上游传入的 Role。可以传递已认证主体和相关性上下文，但拥有资源的服务仍需验证精确 Scope、Operation 与权威授权状态；协议版本必须保留 fail-closed 行为。
