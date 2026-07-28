<!--
    Panvara
    docs/guides concepts.md    2026-07-28
     ______     __  __     ______     ______     __     __
    /\  ___\   /\ \_\ \   /\  ___\   /\___  \   /\ \  _ \ \
    \ \___  \  \ \  __ \  \ \  __\   \/_/  /__  \ \ \/ ".\ \
     \/\_____\  \ \_\ \_\  \ \_____\   /\_____\  \ \__/".~\_\
      \/_____/   \/_/\/_/   \/_____/   \/_/   \/_/.com

    @link    : https://github.com/shezw/panvara
    @author  : shezw
    @email   : hello@shezw.com
-->

# 认识 Panvara

::: info Current Distribution
本页描述二进制内建 Distribution 标识 **v0.1.0-alpha.2** 对应的产品心智模型。
:::

## 一条主线：Model → Compile → API / Storage

1. 你用 YAML 或 JSON 编写 **AppModule**。
2. Panvara 严格校验模型，并编译出确定的内部表示。
3. Server 根据模型开放 HTTP API，把 **Record** 持久化到 PostgreSQL。
4. 同一份模型还会生成 OpenAPI 3.1 与 Manager UI Schema。

AppModule 描述“允许存在什么数据、允许执行什么操作”；Record 是按该模型创建的实际业务数据。模型不是任意代码，不能嵌入 Go、JavaScript、SQL 或 Shell。

## Lite 与 Server

| 运行方式 | 连接数据库 | 加载 AppModule | 适合做什么 |
| --- | :---: | :---: | --- |
| Lite | 否 | 否 | 验证安装、进程健康与 Distribution 版本 |
| Server | 是 | 是 | 运行一个模块的业务 API 与 PostgreSQL Record |

Lite 不是无数据库版业务 Server。要使用 CRM 或自己的模型，必须运行 Server。

## AppModule 与 Record

AppModule 可以声明：

- Resource 与 Field；
- 必填、唯一、引用、枚举和长度等约束；
- Public Create 与 Admin CRUD 白名单；
- 列表过滤字段；
- Manager 列表与表单描述。

Panvara 为 Record 提供 UUIDv7、字段校验、唯一性、引用完整性、软删除、稳定分页，以及基于 ETag 的并发控制。数据按项目、模块、资源和模型 Revision 隔离。

## OpenAPI 与 UI Schema

- **OpenAPI** 是当前 HTTP 契约的机器可读描述，集成客户端时应以它为准。
- **Manager UI Schema** 描述列表列、筛选项和表单字段，供未来或自建前端消费。

UI Schema 不是网页。当前没有可以登录打开的可视化 Manager。

## 模型变化与数据

任何进入规范模型的变化都可能产生新的 Revision，并访问独立的数据空间。旧数据不会被自动删除，但新 Revision 不会自动看到或升级旧 Record。

改变已使用的模型前，请保存原始 Source 与 Revision，并备份 PostgreSQL。具体处理见[数据与升级](/releases/compatibility)。

## 当前边界

Panvara 当前适合单机、本地 PostgreSQL、单个 AppModule 的开发与评估；不适合生产。登录、支付、可视化 Manager、自动模型升级、后台 Worker 和分布式控制面目前不可用。
