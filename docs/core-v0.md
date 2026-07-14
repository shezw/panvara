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

固定验证场景为 crm-leads：定义 Organization 与 Lead，发布模型，生成 CRUD 和 Manager Schema，写入 PostgreSQL，触发一条受控邮件事件，并完成版本回滚。身份 Account 不作为动态 Resource。

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

| 版本轴 | 当前值 | 规则 |
| --- | --- | --- |
| Distribution | 0.1.0-alpha.1 | 整体制品遵循 SemVer |
| Core API | core.panvara.dev/v1alpha1 | experimental；固定外部 HTTP/CLI 契约，不包含 internal Go 包 |
| AppModule | panvara.dev/v1alpha1 | experimental；每个模块另有业务 SemVer 和 Revision Hash |
| Provider API | provider.panvara.dev/v1alpha1 | planned；首个 Adapter 通过 conformance 后才转 experimental |
| IR Format | 1 | planned；Canonical IR 落地后转 experimental |
| Database | 组件序列号 + checksum | 迁移不使用产品 SemVer |

升级兼容必须明确比较每一轴，禁止仅凭 Distribution 版本推断模块或数据兼容。

## 4. alpha.1 已实现边界

- buildinfo：公开所有版本轴和构建元数据。
- profile：把运行 Role 与业务 Feature 分开的组合描述；alpha.1 只允许 Lite 启动。
- project：显式 ProjectContext，分离 UUIDv7 ProjectID 与可读 ProjectKey，并包含 Locale、IANA Time Zone 和 Currency。
- actor：显式 ActorContext，区分匿名/认证 Actor、Project 边界与角色。
- value：int64 最小货币单位的 Money，阻止跨币种计算和溢出。
- appmodule：字段子集、资源关系、模块依赖/冲突、确定性拓扑排序。
- appmodule 的 Module Dependency 与 Capability Requirement 已分开；SemVer Range 和 Provider Resolution 在 alpha.2 实现。
- kernel：按注册顺序启动、逆序停止，部分启动失败自动回滚。
- httpserver：healthz、readyz、version。
- cmd/panvara：Lite 启动、信号退出和版本输出。

alpha.1 的 AppModule 只证明安全语义和注册不变量，尚未提供 YAML 解码、持久化和代码生成。

## 5. v0.1 纳入范围

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

## 6. v0.1 明确不做

- 完整 Website Builder、CMS、Commerce、Tax、Inventory 和所有支付 Adapter。
- 完整密码登录、OIDC 与社交登录；它们属于后续 Identity 模块和 Provider。
- 多租户共享表、数据库 RLS 和计费控制面。
- Redis/Valkey、NATS、Kubernetes、Service Mesh 的强制依赖。
- 远程微服务协议和自动服务发现。
- arbitrary code、任意 SQL、任意表达式执行。
- LLM 直接发布活动模型。
- native table 优化器、自动分库和跨区强一致。

这些不是永久否定，而是防止 Core 在协议尚未验证前被基础设施细节绑死。

## 7. 模块发布不变量

- 同一输入必须产生字节稳定的 Canonical IR 与相同 Revision Hash。
- Validate 无副作用；Plan 不修改活动状态；Publish 只写不可变事实。
- Activate 必须原子切换活动 Revision 或完整失败。
- 数据破坏性变更默认拒绝，除非显式迁移策略和备份条件满足。
- 节点重启后可从数据库恢复最后活动 Revision。
- Rollback 不删除新 Revision，只切回已验证旧版本并记录审计事件。
- 动态 Action 只能调用白名单 Capability，并受超时、权限和幂等约束。
- 分布式节点以 ProjectReleaseSnapshot + epoch 激活；请求、Job、Event 在执行期间固定 epoch。

## 8. v0.1 完成定义

必须同时满足：

- 新环境按 development.md 可以在 15 分钟内启动 Lite 和 PostgreSQL 开发环境。
- crm-leads 从声明到 CRUD、Manager、Event、Email 和回滚形成纵向闭环。
- 当前及前一 Go 版本兼容门禁通过。
- PostgreSQL 迁移 fresh、upgrade、restart、rollback-policy 测试通过。
- AppModule golden、fuzz、兼容矩阵和恶意输入测试通过。
- Provider conformance suite 能验证 Console/SMTP Adapter。
- race、govulncheck、SBOM、许可证和真实构建工件检查通过。
- 文档清楚标注 Preview 限制、升级策略与已知风险。
