<!--
    Panvara
    docs/adr/0006-immutable-module-release-publish-facts.md    2026-07-19
     ______     __  __     ______     ______     __     __
    /\  ___\   /\ \_\ \   /\  ___\   /\___  \   /\ \  _ \ \
    \ \___  \  \ \  __ \  \ \  __\   \/_/  /__  \ \ \/ ".\ \
     \/\_____\  \ \_\ \_\  \ \_____\   /\_____\  \ \__/".~\_\
      \/_____/   \/_/\/_/   \/_____/   \/_____/   \/_/   \/_/.com

    @link    : https://github.com/shezw/panvara
    @author  : shezw
    @email   : hello@shezw.com
-->

# ADR-0006：不可变 Module Release 发布事实

- 状态：Accepted（P0-02a runnable slice）
- 日期：2026-07-19
- 关联：[ADR-0002](0002-immutable-revision-registry.md)、[ADR-0003](0003-draft-validation-change-plan.md)、[ADR-0004](0004-persistent-execution-scope-access-kernel.md)、[ADR-0005](0005-project-local-access-administration.md)

## 上下文

Panvara 已能把候选 Source 保存为 Draft，对精确 Draft Version 生成不可变 Validation，并生成相对于固定 Baseline 的 Change Plan。此前流程在 Plan 后停止：有效 Candidate 不进入 Revision Registry，也没有可查询的发布事实。

发布、激活与迁移是三个不同决策。当前运行时仍由启动配置选择 Source；若在第一个发布切片中同时加入 active pointer、数据迁移、热切换、Outbox 与多节点收敛，任何失败都很难判断究竟影响了哪一层。P0-02a 因此只建立可复验、可审计、不可变的发布事实，为后续 Activate/Migration 保留明确输入。

## 目标

- 由一个已持久化的 `plan_id` 定位完整 Draft → Validation → Plan 链，不要求调用方重复提交容易漂移的字段。
- 在发布事务中重新校验 Draft 当前代次、Source、Validation、Plan、Candidate 制品与 Project/Environment 授权。
- 幂等登记 Candidate Revision，并创建 environment-scoped、不可变的 Module Release。
- 让原 Key 重放、进程重启后重放与同 Plan 新 Key 重放都返回同一个事实。
- 明确证明 Publish 不会激活 Revision、迁移 Record、改变 Runtime 或影响当前 OpenAPI。

## 约束

- 当前只允许 active 默认 Environment；Draft 与 Revision 的既有表仍是 project-scoped，不构成真正多 Environment 数据隔离。
- 当前调用者必须是 active API Credential 认证的 active Principal，并拥有精确 Scope 的 active `project.owner` Grant。
- 当前 Plan format 支持 `compatible`、`review_required`、`migration_required` 与 `unsupported`。前三种允许记录发布事实；`unsupported` 拒绝发布。
- P0-05 通用 Idempotency/Audit/Outbox 尚未完成。本切片只有 Release 专用幂等绑定，并复用当前最小安全审计。
- 发布不允许上传任意代码、SQL 或运行时表达式。

## 备选方案

### A. Plan 通过后直接修改当前 Runtime

该方案调用简单，但把事实登记、数据兼容、迁移与运行时切换合并为一次不可解释操作。进程崩溃、节点落后或迁移失败时缺少稳定恢复点，也无法证明当前数据面未改变。

### B. 客户端重新提交 Draft、Validation、Plan 与 Candidate 全部字段

该方案减少服务端查询，但让客户端能够提交互相矛盾的身份，增加 TOCTOU 与协议负担。权威链本来已经持久化，因此重复字段没有新的可信度。

### C. 以 `plan_id` 建立不可变发布事实，激活与迁移继续分离

该方案保持请求最小，服务端可以从权威事实重建完整链并在事务中再次核验。它不能立即上线 Candidate，但给后续 Activate、Migration 与分布式收敛提供了稳定输入。

## 决策

选择方案 C。

### 发布链

```mermaid
flowchart LR
    Draft["current Draft generation"] --> Validation["valid immutable Validation"]
    Validation --> Plan["non-stale immutable Plan"]
    Plan --> Compile["recompile exact Source"]
    Compile --> Revision["immutable Candidate Revision"]
    Revision --> Release["environment-scoped Module Release"]
    Release --> Stop["stop: no activate / migration / runtime switch"]
```

Application 只接受 `module`、`plan_id` 与 `Idempotency-Key`。它先固定 `release.publish` Operation 完成预授权和输入校验，再调用 PostgreSQL `ResolveReplay`：该短事务在 Project lock 下二次授权，并在读取可变 Draft 前解析已绑定 Key 或已发布 Plan。已绑定 Key/不同意图优先冲突；已绑定 Key/相同意图与已发布 Plan/新 Key 都返回原 Release，不要求原 Plan 对当前 Draft 仍然有效。

只有重放未命中时，Application 才读取 Plan 所属 Draft、Validation 与 Source；重新编译 Source 后必须得到完全相同的 Candidate Revision、Data Schema Identity 与 Canonical IR。OpenAPI 与 Manager Schema 由这次受控 Compiler 生成并随 Revision 保存；当前 Plan 不单独保存它们的发布前字节，因此本切片不宣称跨 Core 版本比较这两项生成物。首次发布遇到任何代次、Source、身份或 Plan 漂移都 fail closed。

发布允许 `compatible`、`review_required` 与 `migration_required` 三种 Outcome。后两种只是允许保存发布事实，不表示可以激活；`unsupported` 返回 `not_publishable`。

### 事务与幂等

每次 Publish 先执行一个重放解析事务：

1. 获取 Project advisory lock，并以调用 Credential、Principal、Scope 和 `project.owner` Grant 二次授权。
2. 若 Key 已绑定，先比较 scoped intent；相同则返回原 Release，不同则返回 409。
3. 若 Key 未绑定但同 Scope/Module/Plan 已发布，追加新 Key alias 与成功安全审计并提交。
4. 若两者都不存在，提交无写入的 miss；Application 才进入首次发布复验。

重放未命中后的首次发布事务完成：

1. 获取 Project advisory lock。
2. 以调用 Credential、Principal、Scope 和 `project.owner` Grant 二次授权。
3. 再次查找 Key 与同 Scope/Module/Plan，以收敛重放解析事务之后发生的并发；已收敛则返回原 Release，Plan alias 同时写 Key 与成功审计。
4. 仍未命中时锁定并重检 Draft、Validation 与 Plan。
5. 幂等登记 Candidate Revision；若相同 Revision 已由 bootstrap 登记，保留首次 provenance。
6. 插入 Module Release。
7. 写入 Release 专用 `Idempotency-Key → intent hash → release` 绑定。
8. 追加 `release.publish` 成功安全审计并提交。

数据库唯一约束保证同一 Project、Environment、Module 与 Plan 最多一个 Release。任何失败都不得留下只有 Revision、只有 Release、只有 Key 或虚假成功审计的半状态；alias Key 与其成功审计也在同一事务内提交或回滚。

### HTTP 契约

- `POST /api/admin/core/v1alpha1/modules/{module}/releases`
- `GET /api/admin/core/v1alpha1/modules/{module}/releases/{release}`

POST 不接受查询参数，要求一个且仅一个 `Idempotency-Key`，并使用精确 ASCII、无重复或未知字段的 JSON：

```json
{"plan_id":"sha256:..."}
```

首次创建返回 201 和 `Location`；同一事实重放返回 200。GET 只读取路径指定事实。响应均使用 `Cache-Control: private, no-store`。

固定错误语义：400 为请求或 ID 无效；401 为 Credential 无效/撤销；403 为 Scope 或 Grant 拒绝；404 为 Plan/Release 不存在；409 为 stale 或幂等冲突；422 为 Plan 不可发布；503 为权威认证、授权或存储不可用。

响应 `effects` 固定表达本切片边界：

```json
{
  "revision_registered": true,
  "published": true,
  "activated": false,
  "records_migrated": false,
  "runtime_changed": false,
  "activation_supported": false
}
```

## 数据与安全边界

Migration `0006_module_publish_facts.sql` 增加 append-only Module Release 和 Release Idempotency Binding，并把 Revision provenance 约束扩展到受控 Publish 来源。Release 精确保存 Scope、Module、Draft generation、Validation、Plan、Candidate Revision、Data Schema Identity、Outcome、风险与调用 Credential provenance。

Release 和幂等绑定拒绝 UPDATE、DELETE 与 TRUNCATE。Revision Registry、Release 和安全审计仍以 PostgreSQL 为权威；HTTP/Manager 不拥有发布状态。发布事务必须重用 Access Kernel 的固定 Operation 和事务内再授权，不能只信任 HTTP 已认证结果。

## 取舍与后果

正向后果：

- Candidate 首次成为可查询、不可变、可重放的发布事实。
- Publish 重试和重启恢复不会制造重复 Release。
- 相同 Candidate 已由 bootstrap 登记时不会覆盖首次来源证明。
- 后续 Activate/Migration 可以引用稳定 Release，而不必重新解释可变 Draft。

代价与剩余边界：

- `published` 只表示发布事实已记录，不表示线上正在使用。
- `migration_required` 可以发布但不能在当前版本激活；本切片没有 Migration Run。
- 当前专用幂等和安全审计不能替代 P0-05 通用能力。
- 环境归属只存在于 Release；Draft、Revision 与 Record 仍未完成多 Environment 隔离。

## 迁移与回滚

数据库迁移 forward-only 应用。升级后旧 Revision 与 Draft/Validation/Plan 保持原身份；第一次 Publish 只追加 Candidate Revision、Release、Key Binding 与安全审计。回退到不理解 `0006` 的二进制前必须停止发布写入并恢复数据库备份；禁止通过删除 Release 或幂等绑定伪造回滚。

业务上的 Rollback 不是删除 Publish：未来必须创建新的回滚事实并保持历史。本 ADR 没有定义 Activate/Rollback API。

## 明确非目标

- Active Release、Project Release Snapshot、epoch、热加载或重启后按 Release 恢复 Runtime。
- Record 数据迁移、Migration Run、校验、写入 fence 或回滚执行。
- Outbox、Worker、消息队列、多节点 prepared/ACK/activate 收敛。
- 通用业务幂等、通用 Audit 查询/保留策略。
- 完整 Account/ExternalIdentity/Session/ProjectMembership 或多 Environment 数据隔离。
