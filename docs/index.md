---
layout: home

hero:
  name: Panvara
  text: 用模型搭建完整应用
  tagline: 从一个可读的模型文件开始，获得 API、数据存储和管理界面描述，并用 Draft、Change Plan 与不可变 Release 安全管理模型变化。
  actions:
    - theme: brand
      text: 15 分钟开始验收
      link: /getting-started/
    - theme: alt
      text: 查看 CRM 示例
      link: /modules/crm-leads

features:
  - title: 模型驱动
    details: 使用受控 YAML 或 JSON 描述数据和操作，Panvara 编译并校验后提供运行能力。
  - title: 一套 Core，多种组合
    details: Lite 适合最小运行，Server 加入 PostgreSQL 和业务 API；后续能力按真实场景拆分。
  - title: 全球化原语
    details: 项目从第一天明确语言、时区和币种，为全球身份、支付与消息 Provider 留出边界。
  - title: 可验证的文档
    details: 每个模块都必须给出前置条件、最小示例、验收步骤、限制和升级影响。
  - title: 分阶段变化
    details: alpha.3b 先验证并解释变化；P0-02a 再记录不可变发布事实，但不会激活、迁移或切换 Runtime。
---

<!--
    Panvara
    docs/index.md    2026-07-14
     ______     __  __     ______     ______     __     __
    /\  ___\   /\ \_\ \   /\  ___\   /\___  \   /\ \  _ \ \
    \ \___  \  \ \  __ \  \ \  __\   \/_/  /__  \ \ \/ ".\ \
     \/\_____\  \ \_\ \_\  \ \_____\   /\_____\  \ \__/".~\_\
      \/_____/   \/_/\/_/   \/_____/   \/_____/   \/_/   \/_/.com

    @link    : https://github.com/shezw/panvara
    @author  : shezw
    @email   : hello@shezw.com
-->

## 当前可以验收什么？

`v0.1.0-alpha.2` 已经打通一条真实链路：读取 CRM Leads 模型、生成内部模型与接口描述、连接 PostgreSQL、通过 Public/Admin API 创建和查询数据，并在 Server 重启后保留数据。当前开发分支继续提供 alpha.3a Registry、alpha.3b Draft/Validate/Plan 与 P0-02a Publish Facts 三个可独立验收的控制面切片。

::: warning 这是实验版本
当前版本适合开发、学习和架构验证，不应直接承载生产业务。Draft 与 Plan 只准备变化，Publish 只记录事实；Manager 页面、模块激活、迁移与回滚、完整身份系统、Provider、Outbox 和分布式运行仍在后续阶段。
:::

`server` 当前表示一条可以运行和验收的纵向切片，并非 Server Core 已完成。请从[当前 Server 架构事实](/architecture-review-server-current)核对现状，从 [Server Core 能力清单](/roadmap/server-core)与 [Manager 范围及验收](/roadmap/manager)查看后续完整范围。

从[使用与验收 Guideline](/getting-started/)开始，不需要先读完整架构文档。已经完成 Registry 验收时，可以依次进入 [Draft → Validate → Plan](/getting-started/draft-plan-acceptance)与 [Draft → Publish](/getting-started/draft-publish-acceptance)。
