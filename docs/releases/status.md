<!--
    Panvara
    docs/releases status.md    2026-07-28
     ______     __  __     ______     ______     __     __
    /\  ___\   /\ \_\ \   /\  ___\   /\___  \   /\ \  _ \ \
    \ \___  \  \ \  __ \  \ \  __\   \/_/  /__  \ \ \/ ".\ \
     \/\_____\  \ \_\ \_\  \ \_____\   /\_____\  \ \__/".~\_\
      \/_____/   \/_/\/_/   \/_____/   \/_/   \/_/.com

    @link    : https://github.com/shezw/panvara
    @author  : shezw
    @email   : hello@shezw.com
-->

# 能力状态

当前二进制内建的 Distribution 标识是 **v0.1.0-alpha.2**（`/version` 返回 `0.1.0-alpha.2`），当前尚无对应 GitHub Release。本页描述能力边界，不是下载公告；该标识面向本地开发与技术评估，**不适合生产**。

公开指南统一固定到源码快照 `38afe3e91e5a54c1a677b0acbe3a6a2a75668839`。Distribution 标识定义公开能力边界，不是源码提交边界；该快照编译出的二进制仍会报告 `0.1.0-alpha.2`，不能仅根据版本字段推断同一源码中的 Source Preview 已成熟或进入 Distribution。

## 状态定义

| 标签 | 含义 |
| --- | --- |
| Current Distribution | 当前二进制内建标识为 v0.1.0-alpha.2，并有公开指南可验证 |
| Source Preview | 固定源码快照可实验，但不属于 Distribution、不适合生产 |
| Planned / Unavailable | 尚不可用，不提供操作指南 |

每项能力只归属一个状态。

## Current Distribution

| 能力 | 当前可做什么 | 主要入口 |
| --- | --- | --- |
| Lite | 无数据库启动，检查 health、ready 与版本 | [快速开始](/getting-started/) |
| AppModule | 严格读取 YAML/JSON，校验并编译模型 | [定义 AppModule](/guides/appmodule) |
| PostgreSQL Server | 加载一个模块并持久化 Record | [运行 Server](/guides/server) |
| CRM CRUD | Organization/Lead 的 Public Create 与 Admin CRUD | [CRM Leads](/guides/crm-leads) |
| HTTP API | 运维、生成物、Public/Admin Record 端点 | [HTTP API](/reference/http-api) |
| OpenAPI 3.1 | 导出当前模块的机器可读 API 契约 | [HTTP API](/reference/http-api) |
| Manager UI Schema | 导出列表/表单描述 JSON；不是 Manager 网页 | [认识 Panvara](/guides/concepts) |

## Source Preview

这些能力仅针对固定提交 `38afe3e91e5a54c1a677b0acbe3a6a2a75668839`：

| 能力 | 当前源码事实 | 明确不是 |
| --- | --- | --- |
| Revision Registry | 查询不可变 Revision 与原始 Source | 活动版本管理 |
| Draft / Validation / Plan | 保存候选、验证并解释变化 | 执行变化或上线许可 |
| Publish Facts | 登记 Candidate 与不可变 Release 事实 | Activate、Rollback 或数据升级 |
| Principal / Credential / Owner Grant | 本地 Service Credential 与固定 Owner Grant | 账号登录或完整 IAM |

只从[访问管理预览](/guides/access-preview)和[模型变更预览](/guides/model-change-preview)进入。

## Planned / Unavailable

| 能力 | 当前状态 |
| --- | --- |
| 可视化 Manager | 没有可登录或操作的管理网页 |
| Activate / Rollback | 没有活动版本指针或切换流程 |
| 自动数据升级 | 没有 Record 转换、执行与验证器 |
| 账号登录与外部身份 | 没有 Account、Session、MFA 或 Provider 登录 |
| 支付 | 没有 Payment Provider Runtime |
| Outbox / Worker | 没有后台任务与可靠事件投递 |
| 分布式控制面 | 没有多节点发布、收敛与流量切换 |
| 生产就绪承诺 | 没有稳定协议、容量、安全或灾备保证 |

“Planned / Unavailable”只表达尚不可用，不承诺时间表。

## 选择建议

- 只想确认工具链：用 Lite。
- 想评估数据型应用：运行 Server 与 CRM。
- 想集成客户端：以生成 OpenAPI 为准。
- 需要登录、支付、可视化后台、自动升级或生产 SLA：当前 Panvara 不适合该需求。
