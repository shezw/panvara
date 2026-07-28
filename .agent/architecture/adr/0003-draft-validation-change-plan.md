<!--
    Panvara
    docs/adr/0003-draft-validation-change-plan.md    2026-07-16
     ______     __  __     ______     ______     __     __
    /\  ___\   /\ \_\ \   /\  ___\   /\___  \   /\ \  _ \ \
    \ \___  \  \ \  __ \  \ \  __\   \/_/  /__  \ \ \/ ".\ \
     \/\_____\  \ \_\ \_\  \ \_____\   /\_____\  \ \__/".~\_\
      \/_____/   \/_/\/_/   \/_____/   \/_____/   \/_/   \/_/.com

    @link    : https://github.com/shezw/panvara
    @author  : shezw
    @email   : hello@shezw.com
-->

# ADR-0003：以版本化 Draft 准备并验证模型变更

- 状态：已接受
- 日期：2026-07-16
- 适用范围：alpha.3b 开发切片

::: danger 准备变更不等于发布变更
Draft、Validation 和 Change Plan 都不会登记 Candidate Revision、发布或激活模块、迁移 Record，也不会切换当前进程的运行模型。
:::

## 背景

alpha.3a 已能保存并复验 Server 启动时登记的不可变 Revision，但用户仍需在工作目录中编辑 AppModule、重新编译并重启，才能知道新模型是否有效。模型驱动开发需要一个更短的安全反馈环：允许人或模型保存尚未完成的文本，得到可定位的校验结果，并在任何执行动作前看清相对某个可信 Revision 的变化。

这个反馈环未来可能由独立 Manager 服务提供，但当前面向中小开发者的 Server 不应为形式化微服务拆分增加运维负担。因此 alpha.3b 在同一进程组合能力，同时保持明确的 Application Port、项目作用域和可独立迁移的存储边界。

## 决策

### Draft 是可修改的工作副本

Draft 在 `(project_id, module_name)` 内使用 UUIDv7 身份。创建时必须显式选择一个精确 Baseline Revision，或明确选择 `none` 表示调用方不要求 Baseline（通常用于新模块）；系统不会检查该 Module 是否已有 Revision，也禁止从 Registry 的“最新一条”推断 Baseline。Draft 的 Module 和 Baseline 创建后不可变。

Draft Source 使用 `json` 或 `yaml` 格式，必须是有效且不含 NUL（U+0000）的 UTF-8，大小为 0 到 1 MiB。空白、语法错误、未知字段或不满足领域约束都可以保存，因为保存中间态正是 Draft 的用途。原始 NUL 字节在进入 PostgreSQL 前拒绝；JSON 中的 `\u0000` 只有在解码到标签等禁止 NUL 的模型位置后才作为作者错误出现在 Validation。Source 每次实质覆盖后重新计算 SHA-256，并使从 1 开始的 `generation` 单调递增；相同格式和字节的重放是 no-op。

创建请求使用持久化 Idempotency Key。同一项目、同一模块内，相同 Key 与相同创建意图返回原 Draft；创建意图精确包含 Baseline、Source Format 和 Source 字节，同一 Key 对应任一不同值时返回冲突。不同模块可以独立复用 Key。覆盖使用数据库原子 CAS，HTTP 强 ETag 精确为带双引号的 generation。缺少、畸形或过期的 `If-Match` 分别失败，不能静默覆盖并发编辑。

HTTP 直接使用原始 Source 请求体，Content-Type 仅接受 `application/json` 或 `application/yaml`，可附带 UTF-8 charset。Baseline 通过创建请求的显式查询参数传入。这样 1 MiB 上限始终约束真实 Source，而不受 JSON 转义和信封开销影响。

### Validation 是不可变的代次事实

Validate 必须携带当前 Draft ETag。结果绑定 Project、Module、Draft ID、generation、Source Format、Source Hash 和 Validation Format。同一 Draft generation 与 format 的重复请求返回同一事实，不生成不同身份。Store 在写入事务中再次比较 Draft generation 与 Source Hash；若编译期间发生覆盖，校验结果不得落库。

不能编译的作者 Source 会形成 `status=invalid` 的成功校验结果，包含稳定排序的 code、path、message 与 severity；它不是 HTTP 422。通过校验时，结果保存 Candidate Revision、当前格式的 Data Schema Identity 和可复现的编译制品身份，但不会写入 Revision Registry。Validation Hash 由版本化的规范结果计算，包含 Project、Draft、generation 与输入/结果身份以绑定精确快照，但不包含派生事实的随机数据库行 ID、Actor 或时间。

存储、Registry 或运行环境故障不是作者校验结果。这类故障必须 fail closed，并且不能留下一个伪造的 invalid Validation。

### Change Plan 是不可变的解释，不是执行任务

Plan 只接受当前 generation 中有效 Validation 的精确身份，并再次完整复验显式 Baseline。不存在或跨 Project/Module 的 Baseline 对调用方表现为不存在；Source 或制品无法重现时视为 Registry corruption，拒绝生成 Plan。

Plan Format 1 比较 Baseline 与 Candidate 的规范 Descriptor，生成稳定排序、机器可读的变化。最小分类覆盖：

- Module 版本、标签、依赖、能力与冲突；
- Resource 新增、删除及标签；
- Field 新增、删除、类型、required、unique、reference target、enum 与约束；
- Public/Admin API 与 Manager 显示配置。

调用方选择 `none` 时，Plan 必须产生 `module.baseline_absent` review，明确说明兼容性与数据迁移需求无法判断；它不能把“没有提供 Baseline”解释为“没有既有 Revision 或数据”。此时列出的 Resource 也只能标为待审核，`requires_migration=false` 仅表示尚未判定迁移为必需，不表示数据安全。

没有稳定字段 ID 时，rename 必须表示为 remove 加 add，不能启发式宣称安全。删除、类型或引用目标变化属于 unsupported；新增 required、启用 unique、删除 enum 选项或收紧约束至少需要 migration；纯展示、标签和通常的可选新增可以降低风险，但仍不能据此宣称可以激活。

Plan 的 `outcome` 使用 `compatible`、`review_required`、`migration_required` 或 `unsupported`；`risk` 使用 `none`、`low`、`medium`、`high` 或 `critical`。Plan Hash 的规范输入只包括算法 format、Module、Baseline、Candidate、Data Schema format、变化和汇总结论，不包括 Project、Draft、generation、Source、Actor 或时间。

`plan_id` 与 `plan_hash` 是两条不同身份轴。`plan_id` 精确绑定 Project、Draft、generation、Validation、Source 和 `plan_hash`，用于读取一个不可变快照；`plan_hash` 只表示计划的规范语义。重放同一 Validation 会返回同一 `plan_id`，而语义等价但字节不同的新 Draft generation 会得到不同 `plan_id`、相同 `plan_hash`。

相同 Validation 与 Plan Format 重放返回同一不可变 Plan。Draft 后续更新不会删除或改写旧 Validation/Plan，只会使它们相对当前 generation 变成 stale。Validation 不重复保存最大 1 MiB 的 Source；旧结果保留输入 Hash 和结构化结果用于审计，但只有当前 generation 可以继续生成 Plan。

## 项目隔离与持久化

PostgreSQL migration 0003 建立三个边界：

- `panvara_module_draft`：唯一可变表，只允许 Source 的 CAS 覆盖；
- `panvara_module_draft_validation`：追加式输入身份与校验结果；
- `panvara_module_draft_plan`：追加式规范变化计划。

所有主键、外键、唯一约束和查询都包含 Project 与 Module。精确 Baseline 使用外键指向 alpha.3a Registry；`none` 不建立引用。Validation 与 Plan 表禁止 UPDATE、DELETE 和 TRUNCATE。Application 用例必须独立验证 Actor 是同项目的 `project.owner`，不能只依赖 HTTP 中间件。

## HTTP 边界

alpha.3b 的最小 owner 接口为：

- `POST /api/admin/core/v1alpha1/modules/{module}/drafts?baseline_revision=<hash|none>`
- `GET /api/admin/core/v1alpha1/modules/{module}/drafts/{draft}`
- `GET /api/admin/core/v1alpha1/modules/{module}/drafts/{draft}/source`
- `PUT /api/admin/core/v1alpha1/modules/{module}/drafts/{draft}/source`
- `POST /api/admin/core/v1alpha1/modules/{module}/drafts/{draft}/validations`
- `GET /api/admin/core/v1alpha1/modules/{module}/drafts/{draft}/validations/{validation}`
- `POST /api/admin/core/v1alpha1/modules/{module}/drafts/{draft}/plans`
- `GET /api/admin/core/v1alpha1/modules/{module}/drafts/{draft}/plans/{plan}`

Create 要求 `Idempotency-Key`；Replace、Validate 和 Plan 要求 Draft ETag。元数据、Source 与派生结果都使用 `Cache-Control: private, no-store`，避免包含未发布模型的响应被持久缓存。Draft generation 作为元数据和 Source 响应的数字强 ETag，因此 GET Source 返回的 ETag 可以直接用于 PUT；精确 Source Hash 通过 Draft JSON 与 `X-Panvara-Source-Hash` 返回。

每个 Validation/Plan 响应必须显式返回 effects：`revision_registered=false`、`published=false`、`activated=false`、`records_migrated=false`、`runtime_changed=false`。Plan 可以额外声明 `plan_recorded=true`。

## 不变量

1. Draft 不进入 Registry，也不定义 Record namespace。
2. Baseline 必须由调用方显式固定，不能随 Registry 或 Server 重启漂移。
3. Validation 和 Plan 必须绑定精确 Draft generation 与 Source Hash。
4. 只有数据库原子 CAS 可以改变 Draft Source；并发覆盖最多一个成功。
5. 等价语义的 JSON/YAML、声明顺序和 Go map 迭代不能改变 Candidate 或 Plan 规范 Hash。
6. 任何失败都不能改变 Server 当前 OpenAPI Revision、已有 Record 或 Registry 内容。
7. 当前 Record namespace 仍按完整 Module Revision 隔离；Data Schema fingerprint 相同不表示无需迁移或可以激活。

## 结果

- 人和模型可以保存未完成的 AppModule 文本，不需要每次走编译、镜像和部署链路。
- 通过 ETag、持久化幂等键和代次快照避免网络重放与并发覆盖造成数据丢失。
- 用户能在执行前获得确定性、可审计、机器可读的变化说明。
- 当前单体 Server 保持简单，同时 Domain/Application/Store/HTTP 边界可在需求出现时拆到 Manager 服务。
- 保存派生结果增加数据库空间；alpha.3b 暂不提供清理策略，需要后续保留与配额设计。

## 被拒绝的方案

- 创建 Draft 时必须编译成功：无法支持人或模型的中间编辑状态。
- 以 JSON 字段承载 Source：使真实 Source 上限、转义和错误定位变得含糊。
- 从 Registry List 首项推断 Baseline：登记时间不表达当前运行或用户意图。
- Validate/Plan 只做瞬时计算：重启后无法审计，而且网络重试缺少稳定身份。
- 更新时删除旧结果：破坏审计链并制造读写竞态。
- Plan 直接登记 Candidate：把准备、发布和运行三个安全边界混在一起。

## 严格非目标

alpha.3b 不提供 Draft List/Delete/Rebase、Publish、Activate、Rollback、数据迁移、active pointer/epoch、Outbox、Worker、运行时热切换或多节点收敛。后续阶段只能消费这里的不可变 Validation/Plan，不能弱化 alpha.3a Registry 与本 ADR 的隔离和幂等约束。
