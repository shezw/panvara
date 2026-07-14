<!--
    Panvara
    docs/core-v0.md    2026-07-14
     ______     __  __     ______     ______     __     __
    /\  ___\   /\ \_\ \   /\  ___\   /\___  \   /\ \  _ \ \
    \ \___  \  \ \  __ \  \ \  __\   \/_/  /__  \ \ \/ ".\ \
     \/\_____\  \ \_\ \_\  \ \_____\   /\_____\  \ \__/".~\_\
      \/_____/   \/_/\/_/   \/_____/   \/_____/   \/_/   \/_/.com

    @link    : https://github.com/shezw/panvara
    @author  : shezw
    @email   : hello@shezw.com
-->

# Panvara Core v0

## 1. 首版目标

Core v0.1 的任务不是做一个缩小版“万能平台”，而是用一条完整纵向场景证明以下能力可以长期演进：

- 同一 Core 可以按 Profile 精简或组合部署。
- AppModule 能从受控声明形成确定、可版本化的运行模型。
- 模块、Provider、数据库和分发版本互不绑死。
- 单进程可以自然演进为 Server + Manager + Worker，而不重写领域规则。
- 全球化原语从第一天进入模型，不等业务数据固化后再补。

固定验证场景为 `crm-leads`。alpha.2 检查点只覆盖 Organization 与 Lead 的声明、编译、CRUD、Manager UI Schema 和 PostgreSQL 持久化；受控邮件事件与版本发布/回滚明确属于 alpha.3。身份 Account 不作为动态 Resource。

## 2. 版本路线

| 版本 | 主要交付 | 退出条件 |
| --- | --- | --- |
| v0.1.0-alpha.1 | 启动内核、Profile、版本轴、ProjectContext、Money、AppModule 内存校验/注册、健康接口 | Lite 可启动；unit/race/vet/build 全绿 |
| v0.1.0-alpha.2 | YAML/JSON 解码、v1alpha1 完整子集、Canonical IR、flex JSONB 存储、REST/OpenAPI、Manager UI Schema | crm-leads 可生成并完成持久化 CRUD |
| v0.1.0-alpha.3 | Draft/Validate/Plan/Publish/Activate/Rollback、迁移计划、审计、Outbox、本地 Worker、Email Capability | 发布失败可恢复，活动 Revision 可回滚，副作用可追踪 |
| v0.1.0-rc.1 | 协议冻结、升级兼容、参考 Provider、完整门禁和文档 | 无已知 P0/P1；N-1 升级通过 |
| v0.1.0 | Core Preview | 参考纵向场景和发布工件可复现 |

v0.1.0 是 Preview，不作“任意业务零代码生成”或“百万并发开箱即用”的生产承诺。

## 3. 六条独立版本轴

| 版本轴 | alpha.2 当前值 | 规则 |
| --- | --- | --- |
| Distribution | 0.1.0-alpha.2 | 整体制品遵循 SemVer |
| Core API | core.panvara.dev/v1alpha1 | experimental；固定外部 HTTP/CLI 契约，不包含 internal Go 包 |
| AppModule | panvara.dev/v1alpha1 | experimental；每个模块另有业务 SemVer 和 Revision Hash |
| Provider API | provider.panvara.dev/v1alpha1 | planned；alpha.2 没有 Provider Runtime |
| IR Format | 1 | experimental；当前为进程启动时编译的 Canonical IR |
| Database | 组件序列号 + checksum | 迁移不使用产品 SemVer |

升级兼容必须明确比较每一轴，禁止仅凭 Distribution 版本推断模块或数据兼容。

## 4. alpha.2 已实现边界

- 保留 alpha.1 的 buildinfo、ProjectContext、ActorContext、Money、Kernel 生命周期和运维端点。
- Profile：Lite 与 Server 可执行；Manager、Site、Commerce、Distributed 仍是 planned 并拒绝伪启动。
- AppModule Source：严格解码 YAML/JSON，拒绝未知字段、重复键、null、YAML Anchor/Alias、自定义 Tag、过深或过大的输入。
- AppModule Model：v1alpha1 字段、reference、基础约束、模块依赖/冲突与 SemVer Range；`requires.capabilities` 只进入 IR/Hash，尚无 Provider Resolution，也不影响 Bootstrap readiness 或 Runtime 行为。
- Compiler：生成字节稳定的 Canonical IR、Revision Hash、OpenAPI 3.1 和 Manager UI Schema。
- 数据：PostgreSQL flex JSONB Store，按 Project/Module/Resource/Revision 隔离；支持 UUIDv7 Record、唯一值、引用、乐观版本、游标分页、等值过滤和软删除。
- HTTP：Public 只允许模型声明的 Create；Admin 支持模型声明的 List/Get/Create/Patch/Delete，写操作使用 ETag/If-Match。
- 临时管理认证：Server 使用至少 32 字节的 bootstrap admin Bearer Token；`BootstrapAdminAuth` 验证器只保存 SHA-256 摘要，但环境变量与启动配置中的明文生命周期不作清除保证。它不是完整账号或身份系统。
- Server：启动时从文件编译一个 AppModule、运行 migration 并装配 PostgreSQL；重启后 Record 保留。
- 集成门禁：使用真实 PostgreSQL 18.4 验证 migration、Store 与 Server HTTP 持久化旅程。

alpha.2 没有模块 Registry/发布状态。Server 每次启动都从指定 YAML/JSON 文件重新编译模块；Revision Hash 只是内容身份，不代表该 Revision 已发布或激活。

### 4.1 alpha.2 持久化已知风险

Record Scope 包含启动时计算的 Revision Hash。任何 Canonical IR 变化——包括进入 IR 的标签或具有语义的展示顺序变化——都会形成全新的空数据命名空间。旧 Revision 的 Record、唯一值和引用完整保留且按 Revision 隔离，但不会自动迁移或重绑定；只有切回完全相同的 Source/Hash 才会重新访问旧命名空间。

alpha.3 的发布、迁移和回滚闭环完成前，任何持久化环境都必须：

- 把已运行的模块 Source + Hash 作为不可变制品保存，不直接覆盖编辑；覆盖 Source 不是升级。
- 变更 Source 前备份数据库；需要保留数据时不要依赖重启自动迁移。
- 先在可丢弃环境验证新 Revision，无法接受 Scope 切换时保持旧 Source。

alpha.3 必须用 ADR 定案并验证：显式且幂等的数据迁移；迁移时保留 `record_id`；在目标 namespace 重建 unique/reference 约束；全部校验成功后原子 Activate；失败或回滚继续使用旧 namespace。同时必须决定 ModuleRevision 与 DataSchemaRevision 是继续绑定，还是分成独立版本轴，避免纯 UI/标签变化无意切换数据命名空间。

## 5. alpha.3 延后能力

- Draft、Validate、Plan、Publish、Activate、Rollback 与不可变 Revision Registry。
- Schema 变更计划、升级兼容检查和 last-known-good 恢复。
- 审计、Transactional Outbox、进程内 Worker 和邮件副作用。
- Provider Descriptor、Capability Resolution、Console/SMTP Email Adapter。
- Manager 前端应用；alpha.2 只生成 UI Schema。
- 动态排序；alpha.2 会拒绝任何非空 `sortable` 声明。
- Idempotency Key、发布 epoch、多节点模块收敛和分布式 Runtime。
- 授权下沉 Application：新增第二入口前，Use Case 必须显式接收 Project、Actor、Surface 和 Operation；Interfaces 不能继续作为唯一授权边界。
- 业务写入与 Outbox 同事务、ProjectReleaseSnapshot + epoch，以及宽唯一值的 Hash 索引加原值碰撞复核评估。

## 6. v0.1 目标范围（不等于 alpha.2 当前能力）

- 单制品、单 PostgreSQL、默认单项目。
- AppModule 字段：string、text、int、bool、decimal、enum、date、datetime、email、money、reference。
- flex JSONB 存储与必要的系统列、索引和约束。
- Public/Admin REST CRUD 与 OpenAPI。
- 角色、所有者和字段基础权限。
- Core 内置 ActorContext、bootstrap project owner 与摘要存储的临时管理 API Token。
- Draft、Plan、Publish、Activate、Rollback 与不可变 Revision。
- 审计事件和 Transactional Outbox。
- 进程内 Worker 与 PostgreSQL Outbox 拉取；为后续 NATS Adapter 保留 Port。
- Provider Descriptor、凭据引用和 Email Capability。
- Console Email 与 SMTP/Mailpit 参考 Adapter。
- crm-leads 参考模块及最小 Manager 页面。

## 7. v0.1 明确不做

- 完整 Website Builder、CMS、Commerce、Tax、Inventory 和所有支付 Adapter。
- 完整密码登录、OIDC 与社交登录；它们属于后续 Identity 模块和 Provider。
- 多租户共享表、数据库 RLS 和计费控制面。
- Redis/Valkey、NATS、Kubernetes、Service Mesh 的强制依赖。
- 远程微服务协议和自动服务发现。
- arbitrary code、任意 SQL、任意表达式执行。
- LLM 直接发布活动模型。
- native table 优化器、自动分库和跨区强一致。

这些不是永久否定，而是防止 Core 在协议尚未验证前被基础设施细节绑死。

## 8. 模块发布不变量（alpha.3 起验证）

- 同一输入必须产生字节稳定的 Canonical IR 与相同 Revision Hash。
- Validate 无副作用；Plan 不修改活动状态；Publish 只写不可变事实。
- Activate 必须原子切换活动 Revision 或完整失败。
- 数据破坏性变更默认拒绝，除非显式迁移策略和备份条件满足。
- 节点重启后可从数据库恢复最后活动 Revision。
- Rollback 不删除新 Revision，只切回已验证旧版本并记录审计事件。
- 动态 Action 只能调用白名单 Capability，并受超时、权限和幂等约束。
- 分布式节点以 ProjectReleaseSnapshot + epoch 激活；请求、Job、Event 在执行期间固定 epoch。

这些是不变式目标，不表示 alpha.2 已经存在发布器或回滚器。

## 9. v0.1 完成定义

必须同时满足：

- 新环境按 development.md 可以在 15 分钟内启动 Lite 和 PostgreSQL 开发环境。
- crm-leads 从声明到 CRUD、Manager、Event、Email 和回滚形成纵向闭环；其中 Event、Email、发布和回滚仍待 alpha.3。
- 当前及前一 Go 版本兼容门禁通过。
- PostgreSQL 迁移 fresh、upgrade、restart、rollback-policy 测试通过。
- AppModule golden、fuzz、兼容矩阵和恶意输入测试通过。
- Provider conformance suite 能验证 Console/SMTP Adapter。
- race、govulncheck、SBOM、许可证和真实构建工件检查通过。
- 文档清楚标注 Preview 限制、升级策略与已知风险。
