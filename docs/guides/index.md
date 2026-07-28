<!--
    Panvara
    docs/guides index.md    2026-07-28
     ______     __  __     ______     ______     __     __
    /\  ___\   /\ \_\ \   /\  ___\   /\___  \   /\ \  _ \ \
    \ \___  \  \ \  __ \  \ \  __\   \/_/  /__  \ \ \/ ".\ \
     \/\_____\  \ \_\ \_\  \ \_____\   /\_____\  \ \__/".~\_\
      \/_____/   \/_/\/_/   \/_____/   \/_/   \/_/.com

    @link    : https://github.com/shezw/panvara
    @author  : shezw
    @email   : hello@shezw.com
-->

# 使用指南

按你想完成的任务选择页面。第一次使用建议先完成[快速开始](/getting-started/)。

## Current Distribution

以下是二进制内建 Distribution 标识 **v0.1.0-alpha.2** 对应的公开使用能力。所有操作指南沿用[快速开始](/getting-started/)固定的源码快照 `38afe3e91e5a54c1a677b0acbe3a6a2a75668839`；同一快照中的 Preview 代码不会因此进入 Distribution 能力边界。

| 我要做什么 | 指南 | 完成标志 |
| --- | --- | --- |
| 理解 Panvara 的基本概念 | [认识 Panvara](./concepts) | 能区分 Lite/Server、AppModule/Record 和 UI Schema/Manager |
| 保存真实业务数据 | [运行 Server](./server) | PostgreSQL 与 Server 就绪，重启后数据仍在 |
| 定义自己的数据模块 | [定义 AppModule](./appmodule) | Server 编译模型并生成 OpenAPI 与 UI Schema |
| 跑通一个业务示例 | [CRM Leads](./crm-leads) | 创建 Organization 和 Lead，随后能查询 |
| 正确调用数据接口 | [操作 Record](./records) | CRUD、分页、过滤和并发更新行为符合预期 |

## Source Preview

以下页面只适用于固定源码快照。它们不属于 **v0.1.0-alpha.2 Distribution 能力边界**，不适合生产：

| 我要评估什么 | 指南 | 明确缺失 |
| --- | --- | --- |
| Service Credential 与固定 Owner Grant | [访问管理预览](./access-preview) | 登录、Session、动态角色、细粒度权限 |
| 候选模型的登记、验证、计划和发布事实 | [模型变更预览](./model-change-preview) | Activate、Rollback、数据执行升级 |

完全不可用的能力只在[能力状态](/releases/status)中列出，不提供伪操作步骤。
