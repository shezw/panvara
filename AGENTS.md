<!--
    Panvara
    AGENTS.md    2026-07-14
     ______     __  __     ______     ______     __     __
    /\  ___\   /\ \_\ \   /\  ___\   /\___  \   /\ \  _ \ \
    \ \___  \  \ \  __ \  \ \  __\   \/_/  /__  \ \ \/ ".\ \
     \/\_____\  \ \_\ \_\  \ \_____\   /\_____\  \ \__/".~\_\
      \/_____/   \/_/\/_/   \/_____/   \/_____/   \/_/   \/_/.com

    @link    : https://github.com/shezw/panvara
    @author  : shezw
    @email   : hello@shezw.com
-->

# Panvara Agent 开发约束

## 文档与功能同步

任何新增或变更用户可感知模块的开发，都必须在同一变更中更新 17 页公开 IA 中最相关的指南、参考或状态页。没有清晰能力边界、可执行引导和验证方式的功能，不视为完成。

用户可感知变化包括但不限于：模块能力、配置、CLI、HTTP API、模型 Schema、运行方式、安装依赖、错误信息、安全要求、数据兼容与升级行为。

实施时遵守以下规则：

1. 公开文档固定分为概览、快速开始、使用指南、API 与配置、版本与兼容、参与贡献六个一级入口；具体 17 页以 `scripts/check-docs.mjs` 的门禁清单为准。
2. 用户行为、命令或契约变化时，更新最相关的 `docs/guides/`、`docs/reference/` 或 `docs/releases/` 页面；首页或首次使用路径受影响时，同步更新 `docs/index.md`、`docs/getting-started/index.md` 和 README。
3. 示例命令必须实际执行，写明运行位置、预期结果和失败后的恢复方式；公开页面必须区分 `Current Distribution`、`Source Preview` 与 `Planned / Unavailable`。
4. 内部架构、阶段边界和工程验收写入 `.agent` 的持久资料目录，不进入 VitePress 公开构建、搜索或 sitemap。
5. 内部重构只有在不改变外部行为、构建、配置和运维语义时可以不改公开指南，但交付说明必须写明判断依据。
6. 完成前执行 `make docs-check`，并确保相关代码测试与集成测试通过。

完整规范见 [`.agent/development/quality/documentation.md`](.agent/development/quality/documentation.md)。
