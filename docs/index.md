---
layout: home

hero:
  name: Panvara
  text: 用模型搭建完整应用
  tagline: 从一个可读的模型文件开始，获得 API、数据存储和管理界面描述，并在执行前用 Draft 与 Change Plan 安全准备模型变化。
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
  - title: 先验证，再执行
    details: alpha.3b 可保存无效 Draft、返回结构化校验结果并解释变化；它不会发布、激活或迁移数据。
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

`v0.1.0-alpha.2` 已经打通一条真实链路：读取 CRM Leads 模型、生成内部模型与接口描述、连接 PostgreSQL、通过 Public/Admin API 创建和查询数据，并在 Server 重启后保留数据。当前开发分支继续提供 alpha.3a Registry 和 alpha.3b Draft/Validate/Plan 两个可独立验收的控制面切片。

::: warning 这是实验版本
当前版本适合开发、学习和架构验证，不应直接承载生产业务。Draft 与 Plan 只准备变化；Manager 页面、模块发布、激活、迁移与回滚、完整身份系统、Provider、Outbox 和分布式运行仍在后续阶段。
:::

从[使用与验收 Guideline](/getting-started/)开始，不需要先读完整架构文档。已经完成 Registry 验收时，可以直接进入 [Draft → Validate → Plan](/getting-started/draft-plan-acceptance)。
