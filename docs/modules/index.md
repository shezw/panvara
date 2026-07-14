<!--
    Panvara
    docs/modules/index.md    2026-07-14
     ______     __  __     ______     ______     __     __
    /\  ___\   /\ \_\ \   /\  ___\   /\___  \   /\ \  _ \ \
    \ \___  \  \ \  __ \  \ \  __\   \/_/  /__  \ \ \/ ".\ \
     \/\_____\  \ \_\ \_\  \ \_____\   /\_____\  \ \__/".~\_\
      \/_____/   \/_/\/_/   \/_____/   \/_____/   \/_/   \/_/.com

    @link    : https://github.com/shezw/panvara
    @author  : shezw
    @email   : hello@shezw.com
-->

# 模块使用指南

## 用途

这里是 Panvara 各模块的用户手册入口。它面向第一次接触项目、不会阅读 Go 源码的验收者，回答三件事：模块现在能做什么、如何用最少步骤运行、怎样判断结果正确。

建议先完成 [运行模式](runtime-profiles.md) 和 [CRM Leads 示例](crm-leads.md)，再按需要了解模型与 API 的细节。

## 当前状态

当前文档对应 **Panvara v0.1.0-alpha.2**。alpha.2 已经形成“声明模型 → 启动 Server → PostgreSQL 持久化 → HTTP CRUD”的最小闭环，但仍是实验版本。

| 指南 | alpha.2 状态 | 适合解决的问题 |
| --- | --- | --- |
| [AppModule](appmodule.md) | 已实现 | 怎样用 YAML/JSON 描述数据、API 和管理界面信息 |
| [Record Runtime](record-runtime.md) | 已实现 | 数据如何创建、读取、修改、删除和校验 |
| [HTTP API](http-api.md) | 已实现 | 怎样通过 `curl` 或其他客户端调用 Panvara |
| [运行模式](runtime-profiles.md) | Lite、Server 已实现 | 什么时候不需要数据库，什么时候需要 PostgreSQL |
| [Project Context](project-context.md) | 单项目模式已实现 | 项目 ID、语言、时区和币种怎样配置 |
| [CRM Leads](crm-leads.md) | 已实现的参考场景 | 怎样从零验收一条真实业务链路 |

文档中出现“计划”“alpha.3+”的内容均不能作为当前验收结果。

## 前置条件

阅读文档没有技术前置条件。实际操作时建议准备：

- Git，用于下载仓库。
- Go 1.26.5；项目最低兼容 Go 1.25。
- Make，用于执行统一命令。
- `curl`，用于验收 HTTP 接口。
- Docker 与 Docker Compose，仅在运行 Server、本地 PostgreSQL 或端到端测试时需要。

如果暂时没有 Docker，可以先运行 Lite，它不需要数据库。

## 最小示例

第一次验收按以下顺序操作：

1. 在 [运行模式](runtime-profiles.md) 中启动 Lite，确认 `/healthz`、`/readyz` 和 `/version`。
2. 启动 PostgreSQL 和 Server。
3. 在 [CRM Leads](crm-leads.md) 中创建 Organization 与 Lead。
4. 在 [HTTP API](http-api.md) 中确认认证、ETag 和错误响应。
5. 在 [AppModule](appmodule.md) 中复制最小模型，开始定义自己的模块。

只想快速确认代码质量时，在仓库根目录执行：

```bash
make verify
```

该命令不启动 Docker，也不会运行 PostgreSQL 集成测试。

## 配置

这些指南采用统一约定：

- “已实现”表示当前仓库中有实际代码和自动化测试。
- “计划”表示设计方向，不应尝试按当前命令启动。
- 所有命令默认在仓库根目录执行。
- 默认服务地址是 `http://127.0.0.1:8080`。
- 示例配置只适用于本机开发，不是生产部署模板。
- 管理员 Token 使用 `<admin-token>` 或环境变量表示，不把真实凭据写入 Git。

后续每个功能阶段都应同步更新对应模块指南；没有使用说明和可复现验收步骤的模块，不视为完成用户交付。

## 验收

一份可验收的模块指南至少应满足：

- 包含用途、状态、前置条件、最小示例和配置说明。
- 给出可复制的命令以及成功时应观察到的状态码或输出。
- 明确写出当前限制，避免把设计目标当成已经实现。
- 说明模型或数据升级是否安全。
- 所有站内链接可从本页到达。

当前 alpha.2 的完整技术门禁见 [验证测试框架](../testing.md)，但非专业验收者通常只需完成 [CRM Leads](crm-leads.md) 的用户旅程。

## 常见问题

### 我应该从哪一页开始？

从 [运行模式](runtime-profiles.md) 开始。如果 Lite 已经成功，再进入 [CRM Leads](crm-leads.md)。

### 文档中的 Manager 是可以打开的管理后台吗？

不是。alpha.2 只生成 Manager UI Schema，尚未提供可视化 Manager 应用。

### 可以跳过 PostgreSQL 吗？

Lite 可以；Server 和 Record Runtime 不可以。

### 为什么文档反复提醒保存模块 Source？

alpha.2 会根据 Source 计算 Revision。改变模型后，Server 会进入新的空数据命名空间，不会自动迁移旧数据。

## 当前限制

- 文档站本身只描述当前仓库能力，不代表 Panvara 已进入稳定版本。
- alpha.2 只有 Lite 和 Server 两种可运行 Profile。
- 没有可视化 Manager、身份系统、Provider Runtime、消息队列或分布式控制面。
- 示例以单机、单项目和本地 PostgreSQL 为主。
- 生产安全、容量和升级策略尚未形成稳定承诺。

## 兼容与升级

这些页面随 Panvara Distribution 版本维护，当前基线为 `v0.1.0-alpha.2`。升级时不要只看 Panvara 版本，还要核对 AppModule API、生成的 Revision Hash 和数据库迁移版本。

在 alpha.3 的发布与迁移能力完成前：

1. 将已经运行的模块 Source 与启动日志中的 Revision Hash 一起保存为不可变制品。
2. 修改 Source 前备份数据库。
3. 先在可丢弃环境验收新 Source。
4. 不要用“覆盖原文件并重启”代替数据升级。
