<!--
    Panvara
    docs/getting-started/index.md    2026-07-15
     ______     __  __     ______     ______     __     __
    /\  ___\   /\ \_\ \   /\  ___\   /\___  \   /\ \  _ \ \
    \ \___  \  \ \  __ \  \ \  __\   \/_/  /__  \ \ \/ ".\ \
     \/\_____\  \ \_\ \_\  \ \_____\   /\_____\  \ \__/".~\_\
      \/_____/   \/_/\/_/   \/_____/   \/_____/   \/_/   \/_/.com

    @link    : https://github.com/shezw/panvara
    @author  : shezw
    @email   : hello@shezw.com
-->

# 使用与验收 Guideline

这套 Guideline 面向第一次接触 Panvara 的使用者。你不需要理解 Go 架构或数据库内部实现，只需要能打开终端、复制命令并对照预期结果。

## 最终目标

完成后，你会亲自验证：

- Panvara 可以在本机编译成一个可执行文件。
- Lite 运行模式不依赖数据库也能启动。
- Server 运行模式可以连接本地 PostgreSQL。
- CRM 模型会生成 API 和管理界面描述。
- 你可以创建组织和销售线索，再把线索查询出来。
- 停止并重新启动 Server 后，刚才的数据仍然存在。
- 你可以验证启动 Revision 只登记一次，并下载第一次登记的原始 Source。
- 你可以保存一个有错误的 Draft，修正后生成 Change Plan，并证明它没有发布、激活或迁移数据。

## 建议顺序

| 步骤 | 页面 | 大约时间 | 成功标志 |
| --- | --- | ---: | --- |
| 1 | [安装开发工具](./prerequisites) | 5–20 分钟 | `make doctor-server` 全部显示 `[ok]` |
| 2 | [创建本地环境](./local-environment) | 3 分钟 | `.env`、`.env.local` 已创建，PostgreSQL 为 healthy |
| 3 | [编译并运行 Lite](./build-and-lite) | 3–8 分钟 | `/readyz` 返回 `ready` |
| 4 | [CRM Leads 完整验收](./crm-leads-acceptance) | 8–15 分钟 | 重启后仍能查到新建线索 |
| 5 | [Revision Registry 完整验收](./revision-registry-acceptance) | 10–15 分钟 | 重启后同一 Revision 仍恰好一条，format 1 身份可复核 |
| 6 | [Draft → Validate → Plan 完整验收](./draft-plan-acceptance) | 10–15 分钟 | invalid → replace → valid → plan 成功，当前 Revision 与 Record 不变 |

第一次下载 Go、Node 或 Docker 镜像的时间不计入表格，因为它取决于网络速度。

## 两种运行方式

| 名称 | 通俗解释 | 是否需要 Docker | 当前用途 |
| --- | --- | --- | --- |
| Lite | 只启动 Panvara 核心和健康接口 | 否 | 验证程序可以编译和运行 |
| Server | 加载一个模型、连接数据库并开放业务 API | 是，或自备 PostgreSQL 18.4 | 验收 alpha.2、alpha.3a Registry 与 alpha.3b Draft Planning 开发切片 |

文档中出现的 “Profile” 就是这里的“运行方式”。

## 验收边界

当前 Manager 产物是供前端消费的 UI Schema JSON，并不是已经完成的可视化管理后台。支付、登录、邮件、模块 Publish/Activate/Rollback、数据迁移和跨节点分布式管理也尚未实现；它们不能作为 alpha.2、alpha.3a 或 alpha.3b 开发切片的验收项。

::: danger 登记不等于发布或激活
alpha.3a 只登记 Server 启动时已经选择的模块。Registry 没有活动指针，不能通过 List 顺序判断当前版本；当前 Revision 以 OpenAPI 的 `x-panvara-revision` 为准。
:::

::: danger Plan 不会执行变化
alpha.3b 允许保存 Draft、生成 Validation 与 Change Plan，但 Candidate 不进入 Registry，也不会成为当前运行模型。即使 Plan 显示 `compatible`，也不能据此认为数据已经迁移或模型可以直接激活。
:::

::: danger 模型变更与数据
alpha.2 会把每个模型版本放在独立数据空间。直接修改正在使用的模型文件后重启，旧数据不会被删除，但在新模型版本下会暂时看不到。完成发布与迁移能力前，请保存原模型文件并先备份数据库。
:::

遇到问题时，不要跳过失败步骤，直接前往[故障排查](./troubleshooting)。
