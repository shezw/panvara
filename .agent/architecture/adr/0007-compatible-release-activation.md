<!--
    Panvara
    .agent/architecture/adr/0007-compatible-release-activation.md    2026-08-02
     ______     __  __     ______     ______     __     __
    /\  ___\   /\ \_\ \   /\  ___\   /\___  \   /\ \  _ \ \
    \ \___  \  \ \  __ \  \ \  __\   \/_/  /__  \ \ \/ ".\ \
     \/\_____\  \ \_\ \_\  \ \_____\   /\_____\  \ \__/".~\_\
      \/_____/   \/_/\/_/   \/_____/   \/_____/   \/_/   \/_/.com

    @link    : https://github.com/shezw/panvara
    @author  : shezw
    @email   : hello@shezw.com
-->

# ADR-0007：兼容 Release 激活与活动 Runtime Snapshot

- 状态：Accepted（P0-02b runnable slice）
- 日期：2026-08-02
- 关联：[ADR-0002](0002-immutable-revision-registry.md)、[ADR-0003](0003-draft-validation-change-plan.md)、[ADR-0004](0004-persistent-execution-scope-access-kernel.md)、[ADR-0006](0006-immutable-module-release-publish-facts.md)

## 上下文

ADR-0006 已把 Draft → Validation → Plan 的结果发布为不可变 Module Release，但 Publish 明确不会改变正在服务请求的 Runtime。下一步需要建立可恢复的活动事实，并让一个 Server 进程在不重启的情况下切换到经过复验的 Candidate。

现有 Record 存储同时把完整 `revision_hash` 当作 Validator 身份和物理数据命名空间。即使只改变 Module 版本号，Candidate Revision 也会变化；若直接把 Candidate Hash 写入 Record Scope，旧 Record 会全部变得不可见。因此激活模型必须区分“解释规则与制品的 Runtime Revision”和“定位已有数据的 Record Namespace Revision”。

当前 Plan 的 `compatible` 分类还包含新增可选字段、移除 unique 等数据投影变化。没有索引演进或 Migration 执行器时，仅凭该分类不足以证明运行时切换安全。

## 决策

### 1. PostgreSQL 是活动版本的唯一权威

每个 Project/Environment 保存一个可变 active pointer，指向一个不可变 Project Release Snapshot。Snapshot 使用从 1 开始、每次恰好加一的 `release_epoch`，并包含一个或多个不可变 Module Binding：

- `runtime_revision_hash`：当前请求使用的 Module 规则、OpenAPI 与 Manager Schema；
- `record_namespace_revision_hash`：Record、Unique 与 Reference 的持久化命名空间；
- 可选 `release_id`：bootstrap binding 为空，Release 激活后指向精确发布事实。

第一次启动创建 epoch 1 的 bootstrap Snapshot，此时两个 Revision 相同。后续启动只恢复数据库 pointer 指向的 Revision；本地 Source 可以登记新的 bootstrap Revision，但不得隐式覆盖 active pointer。

Snapshot header 和 Module Binding 只允许追加。Pointer 只允许把 epoch 更新为 `old + 1`；删除、跳号、倒退和改写历史全部由数据库约束拒绝。

### 2. 本切片只允许数据身份完全不变的 compatible 激活

Activate 必须同时满足：

1. 调用者通过固定 `release.activate` Operation 的应用层预授权；
2. 目标 Release 属于精确 Project、Environment 与 Module；
3. Release outcome 为 `compatible`；
4. Release baseline 等于事务内当前 active runtime revision；
5. Candidate Revision 与 Release 保存的 revision、source 和 data-schema identity 完全一致；
6. Candidate 的 data-schema format/fingerprint 与当前 Record Namespace Revision 完全一致。

满足条件时，新 Snapshot 只切换 Runtime Revision，并继承当前 Record Namespace。`review_required`、`migration_required`、无 baseline、baseline 已过期或 data-schema identity 改变均失败关闭。它们必须等待显式 Review/Migration 能力，不能由客户端布尔确认绕过。

### 3. Activate 使用 active-baseline CAS

Application 在准备 Candidate Runtime 前读取当前 Snapshot；PostgreSQL 事务随后锁定 active pointer，并再次核对 expected epoch、runtime revision 和目标 Release。两个基于同一 baseline 的不同 Release 并发激活时最多一个成功，另一个返回 activation conflict。

若目标 Release 已经是当前 active，返回原 Snapshot 且不推进 epoch。若它只存在于历史 Snapshot、当前已切到其他版本，再次激活属于 Rollback，本切片拒绝。路径中的精确 Release ID 已是完整激活意图，因此不另建 Activation Idempotency-Key 表。

事务内必须重新校验 Credential、Principal、Environment 与 `project.owner` Grant；Snapshot、Binding、Pointer 和 `release.activate` 成功安全审计全部提交或全部回滚。

### 4. 每个请求固定一个不可变进程内 Snapshot

Candidate Source 在写 active pointer 前重新编译，并逐字节核对 Canonical IR、OpenAPI、Manager Schema 与 data-schema identity。完整 HTTP Handler、Compiled Module、Record Validator 和 Record Namespace 先构造成不可变的 prepared Runtime。

数据库提交成功后，Server 通过一个 `atomic.Pointer` 安装新 Runtime。HTTP 请求在入口只加载一次 pointer，在途请求完整使用旧 epoch，后续请求完整使用新 epoch，不允许逐字段热改 Handler 或 Validator。Install 只接受更大的 epoch；同 epoch/同内容幂等，更小 epoch 被拒绝，同 epoch/不同内容使 Runtime 进入不可用状态。

数据库提交与进程内 pointer 交换不构成同一个 ACID 事务。Prepare 必须在提交前完成，使正常 Install 成为不可失败的内存交换；若检测到提交后不一致，readiness 立即失败。进程若在数据库提交后、内存安装前崩溃，重启从数据库 active pointer 恢复。

## HTTP 契约

- `POST /api/admin/core/v1alpha1/modules/{module}/releases/{release}/activate`
- `GET /api/admin/core/v1alpha1/modules/{module}/active`

两个端点只接受 Credential-backed project owner，不接受查询参数或请求体，并返回 `Cache-Control: private, no-store`。Activate 首次创建 Snapshot 返回 201，当前 Release 重放返回 200；响应包含 Module、Release、Runtime Revision、Record Namespace Revision、epoch 与激活 provenance，并以 epoch 生成 ETag。

固定错误语义：400 为请求身份无效；401/403 为 Credential 或 Grant 拒绝；404 为 Release/active fact 不存在；409 为 baseline/rollback/concurrent activation conflict；422 为当前版本不可激活；503 为事实损坏、存储或 Runtime 不可用。

## 结果与边界

本决策提供：

- Publish 与 Activate 分离的真实状态机；
- 单调、可审计、可重启恢复的活动 Snapshot；
- 数据身份不变升级时 Record 不消失；
- 单进程内每请求 epoch 固定和原子 Runtime 切换；
- 为未来多 Module Snapshot 保留的数据结构。

本决策不提供：

- data-schema identity 改变后的兼容证明、索引演进或 Record Migration；
- `review_required` / `migration_required` 激活；
- Rollback、写入 fence、Migration Run 或数据恢复；
- 多 Server 节点的 prepared/ACK/activate、主动通知或收敛；
- 跨区域一致性或独立分区协调。

这些能力分别属于后续 Migration/Rollback 与分布式 Runtime 阶段，不得从当前单进程行为外推。

## 迁移与回退

数据库 migration `0007_compatible_release_activation.sql` 只追加 Snapshot、Binding 与 Pointer 结构，并为 Release 增加精确 Candidate 引用键；既有 Release、Revision、Draft 和 Record 不被重写。升级后第一次 Server 启动为尚无 pointer 的 Environment 建立 bootstrap epoch 1。

回退到不理解 `0007` 的二进制前必须停止 Activate 写入并恢复升级前数据库备份。禁止删除 Snapshot、倒退 pointer 或修改 epoch 来伪造回滚。
