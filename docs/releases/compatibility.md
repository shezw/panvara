<!--
    Panvara
    docs/releases compatibility.md    2026-07-28
     ______     __  __     ______     ______     __     __
    /\  ___\   /\ \_\ \   /\  ___\   /\___  \   /\ \  _ \ \
    \ \___  \  \ \  __ \  \ \  __\   \/_/  /__  \ \ \/ ".\ \
     \/\_____\  \ \_\ \_\  \ \_____\   /\_____\  \ \__/".~\_\
      \/_____/   \/_/\/_/   \/_____/   \/_/   \/_/.com

    @link    : https://github.com/shezw/panvara
    @author  : shezw
    @email   : hello@shezw.com
-->

# 数据与升级

::: info Current Distribution
当前 Distribution 标识 **v0.1.0-alpha.2** 对应的 Record 能力会把数据绑定到完整 AppModule Revision。当前没有自动数据升级或回滚工具；改变模型前必须自行保留 Source、备份数据库并验证恢复路径。
:::

## 当前兼容基线

| 项目 | 当前基线 | 承诺 |
| --- | --- | --- |
| Distribution | `v0.1.0-alpha.2`（`/version` 返回 `0.1.0-alpha.2`） | alpha，不承诺长期兼容 |
| Go | 最低 1.25 | 推荐使用项目当前工具链 |
| PostgreSQL | 18.4 | 仓库 Compose 的本地验证版本 |
| AppModule | `panvara.dev/v1alpha1` | 实验协议 |
| HTTP | `/v1alpha1/` | 客户端以生成 OpenAPI 为准 |

升级任何一项前，先在可丢弃环境重跑 Lite、Server、CRM 和你的关键客户端。

## 为什么改模型后数据“消失”？

Panvara 对 AppModule 的规范内容计算 Revision。任何进入规范模型的变化都可能得到新 Revision；Record 数据按 Revision 隔离。

- 旧 Record 不会因为新模型自动删除；
- 新 Revision 不会自动读取旧 Revision 的 Record；
- 把业务版本号改大不会触发数据转换；
- UI 标签或 API 描述等变化也可能影响完整 Revision；
- 空白和无语义的 YAML Key 顺序变化通常不会改变 Revision。

因此“覆盖模型文件并重启”不是升级方案。

## 变更前清单

1. 保存当前 AppModule Source 的不可变副本。
2. 从 OpenAPI 顶层 `x-panvara-revision` 记录当前运行 Revision。
3. 使用经过验证的 PostgreSQL 工具创建备份，并实际测试恢复。
4. 复制 Source 到新文件后再修改，不覆盖唯一旧副本。
5. 提升 `metadata.version`，用于表达业务版本，但不要把它当成数据升级器。
6. 在独立数据库或可丢弃 Project 中启动新 Source。
7. 重新检查 OpenAPI、UI Schema、CRUD、唯一值、引用与客户端兼容性。

备份文件、AppModule Source、配置与 Revision 记录应一起保存；只保留其中一项不足以恢复。

## 安全回到原模型

如果新 Source 不符合预期：

1. 停止 Server；
2. 恢复原 AppModule Source 的精确字节或语义等价内容；
3. 保持原 Project ID 与数据库 URL；
4. 启动 Server；
5. 检查 OpenAPI 的 `x-panvara-revision` 等于已记录旧值；
6. 查询旧 Record 并完成应用级一致性检查。

新 Revision 中产生的数据会留在它自己的 Scope，不会自动合并回旧 Revision。数据库已发生其他不可逆改变时，应从已验证备份恢复，而不是继续覆盖模型。

## Source Preview 的边界

::: warning Source Preview
固定源码快照可以生成 Change Plan 和 Publish Facts，但仍没有 Activate、Rollback 或数据升级执行。发布事实不会切换 Runtime，也不会复制 Record，不能代替上述备份与恢复流程。
:::

## 客户端兼容建议

- 每次部署都读取当前模块生成的 OpenAPI；
- 不把 `v1alpha1` 当作稳定长期协议；
- Patch/Delete 始终使用最新 ETag；
- 未识别字段和状态码按失败处理，不静默推断；
- 模型 Revision 改变时，把它视为需要重新验证的契约变化；
- Manager UI Schema 变化不表示可视化 Manager 已经可用。

## 生产边界

当前没有零停机升级、自动备份恢复、数据转换、双写、流量切换、回滚编排或跨版本 SLA。需要这些能力的系统不应使用当前 Panvara Distribution 承载生产数据。
