---
layout: home

hero:
  name: Panvara
  text: 从数据模型到可运行 API
  tagline: 用一份严格的 YAML 或 JSON 模型生成 HTTP 契约、PostgreSQL 数据能力和管理界面描述。
  actions:
    - theme: brand
      text: 10 分钟启动 Lite
      link: /getting-started/
    - theme: alt
      text: 运行 Server
      link: /guides/server
    - theme: alt
      text: 查看能力状态
      link: /releases/status

features:
  - title: 模型驱动
    details: 声明资源、字段、约束与开放操作，由 Panvara 统一校验并生成契约。
  - title: Lite 与 Server
    details: 先用无数据库的 Lite 验证安装，再用 PostgreSQL Server 运行真实 CRUD。
  - title: 契约可查
    details: 每个模块都能导出 OpenAPI 3.1 与 Manager UI Schema，便于客户端继续集成。
---

<!--
    Panvara
    docs index.md    2026-07-28
     ______     __  __     ______     ______     __     __
    /\  ___\   /\ \_\ \   /\  ___\   /\___  \   /\ \  _ \ \
    \ \___  \  \ \  __ \  \ \  __\   \/_/  /__  \ \ \/ ".\ \
     \/\_____\  \ \_\ \_\  \ \_____\   /\_____\  \ \__/".~\_\
      \/_____/   \/_/\/_/   \/_____/   \/_/   \/_/.com

    @link    : https://github.com/shezw/panvara
    @author  : shezw
    @email   : hello@shezw.com
-->

## Panvara 适合谁？

Panvara 面向想快速验证数据型应用的个人与中小开发团队。你维护可读的 AppModule，Panvara 负责把模型编译为稳定标识、HTTP API、PostgreSQL Record 能力，以及供前端消费的界面描述。

它适合本地原型、学习和技术评估。它目前不是托管平台，也不是无需开发即可交付的 CRM 或管理后台。

::: info Current Distribution
当前二进制内建的 Distribution 标识是 **v0.1.0-alpha.2**。可使用 Lite、严格 YAML/JSON AppModule、PostgreSQL Server、CRM CRUD、HTTP API、OpenAPI 3.1 和 Manager UI Schema。
:::

## 现在能完成什么？

从当前 Distribution 出发，你可以：

- 不连接数据库启动 Lite，并检查进程健康、就绪状态和版本；
- 用 AppModule 声明资源、字段、约束、Public Create 与 Admin CRUD；
- 启动 PostgreSQL Server，创建、查询、修改和软删除 Record；
- 运行 CRM Leads 示例，并验证 Server 重启后数据仍然存在；
- 下载由当前模块生成的 OpenAPI 与 Manager UI Schema。

源码仓库中另有明确隔离的模型变更与访问管理预览。其中可以发布并显式激活数据结构未变化的 compatible 模型版本。它们不属于当前 Distribution 能力边界，入口位于[使用指南](/guides/)的 Source Preview 分区。

::: danger Planned / Unavailable
可视化 Manager、需要复核或数据转换的模型激活、Rollback、自动数据升级、账号登录、支付、后台 Worker 和分布式控制面目前不可用。
:::

## 生产使用边界

**当前 Distribution 能力边界不适合直接承载生产业务。** 当前协议仍处于 alpha，尚未提供稳定升级承诺、完整身份系统、生产级边缘安全、自动模型升级或灾难恢复流程。

如果你要快速判断环境是否可运行，从[快速开始](/getting-started/)开始；如果你需要评估功能是否存在，先查看[能力状态](/releases/status)。
