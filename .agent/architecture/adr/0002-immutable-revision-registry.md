<!--
    Panvara
    docs/adr/0002-immutable-revision-registry.md    2026-07-15
     ______     __  __     ______     ______     __     __
    /\  ___\   /\ \_\ \   /\  ___\   /\___  \   /\ \  _ \ \
    \ \___  \  \ \  __ \  \ \  __\   \/_/  /__  \ \ \/ ".\ \
     \/\_____\  \ \_\ \_\  \ \_____\   /\_____\  \ \__/".~\_\
      \/_____/   \/_/\/_/   \/_____/   \/_____/   \/_/   \/_/.com

    @link    : https://github.com/shezw/panvara
    @author  : shezw
    @email   : hello@shezw.com
-->

# ADR-0002：使用不可变 Revision Registry 保存启动事实

- 状态：已接受
- 日期：2026-07-15
- 适用范围：alpha.3a 开发切片

::: danger 登记不等于发布或激活
Registry 只证明某个项目的 Server 曾成功编译并登记该 Revision。它没有 Draft、Publish、Activate、Rollback、active 指针或热切换语义。
:::

## 背景

alpha.2 只在启动时从本地 Source 编译模块。进程退出后，数据库没有可审计的原始 Source、Canonical IR、生成物与身份元数据，后续发布系统也缺少可信起点。

alpha.3a 需要先建立最小、不可变、项目隔离的事实库，但不能提前引入尚未设计完成的发布状态机，也不能改变当前 Runtime 或 Record namespace。

## 决策

Server 在数据库迁移完成后、HTTP 和 readiness 对外可用前，将当前编译结果以 `origin=bootstrap`、`registered_by=system:bootstrap` 登记到 PostgreSQL。登记失败或已保存事实无法通过校验时，Server 启动失败。

Registry 使用 `(project_id, module_name, revision_hash)` 唯一标识一个不可变父 Revision，并保存：

- Module Version 与 Module Revision；
- Spec 与 IR 格式；
- 原始 Source 格式、Hash 和字节；
- Canonical IR、OpenAPI、Manager UI Schema；
- 首次登记的来源、执行者和 UTC 时间。

Data Schema Identity 是该父 Revision 下的不可变子事实，以 `(project_id, module_name, revision_hash, format)` 唯一标识，内容为对应算法的 fingerprint。当前启动登记写入 format 1；未来算法可以向同一父 Revision 追加 format 2、3 等身份，但不能覆盖已有 format。相同父 Revision 与 format 出现不同 fingerprint 时视为 corruption。

同一 Canonical IR 使用等价但字节不同的 Source 再次启动时，登记是幂等的，并保留第一次登记的 Source 与 provenance。父 Revision 与子 Identity 表都禁止 UPDATE、DELETE 和 TRUNCATE；Store Port 也不提供修改或删除方法。

## 读取与授权

alpha.3a 只提供三个 owner 管理读取接口：List、Detail 和 Source。Application 用例必须显式接收 Project ID 与 Actor Context，并验证 `project.owner` 属于同一项目。所有 PostgreSQL 查询都带 Project 条件，List 默认 20、最大 100，不提供 cursor。

Detail 与 Source 读取不是对数据库字节的盲目信任。Application 会用保存的 Source 重新编译，并核对父 Revision、当前已知 format 的 Data Schema Identity、Canonical IR、OpenAPI 和 Manager UI Schema；任何不一致都 fail closed，返回服务端错误，启动登记时发现损坏则拒绝启动。List 则只读取并校验有界元数据，不加载 Source 或生成物，也不重新编译，避免列表请求随制品大小放大。List 与 Detail 都将子身份作为按 format 升序的 `data_schema_identities` 数组返回，客户端必须按 format 选择算法，不能读取固定数组位置。

## HTTP 契约

只读接口位于：

- `GET /api/admin/core/v1alpha1/modules/{module}/revisions`
- `GET /api/admin/core/v1alpha1/modules/{module}/revisions/{revision}`
- `GET /api/admin/core/v1alpha1/modules/{module}/revisions/{revision}/source`

Source 返回首次登记的原始字节，Content-Type 由保存格式决定，强 ETag 精确等于带双引号的 Source Hash。错误方法返回 405；不存在的项目内事实返回 404；接口不接受 Project 查询参数。

List 与 Detail 包含 owner 范围内的 Registry 元数据，并且子身份数组未来可以只追加扩展，因此响应使用 `Cache-Control: private, no-store`。Source 字节不可变且支持 ETag 复验，响应使用 `Cache-Control: private, no-cache`；两类响应都不得被共享缓存保存。

## 结果

- 重启同一 Source 不产生重复事实，并可证明首次登记信息未被覆盖。
- 新投影算法可以给父 Revision 追加独立身份，而无需改变 Revision Hash 或重写历史制品。
- 后续发布系统可以从可校验的不可变制品开始，而不是依赖工作目录文件。
- 当前运行模块仍由启动配置决定；Registry 排序不表达活动状态。
- 存储完整制品增加数据库空间占用，但换取来源证明、离线验证和故障诊断能力。

## 被拒绝的方案

- 可变 `current_revision` 行：把登记与激活混在一起，无法证明历史事实。
- 只保存 Hash 或对象存储 URL：alpha.3a 尚无可靠对象存储依赖，无法在读取时独立复验。
- 以最新登记时间推断活动版本：重启、并发节点和回滚都会使该推断错误。
- 在 alpha.3a 加入 Publish/Activate API：会在迁移、审计和分布式收敛协议尚未定案时制造虚假承诺。

## 后续工作

Draft、Validate、Plan、Publish、Activate、Rollback、数据迁移、active epoch、Outbox 和多节点收敛需要独立 ADR 与端到端门禁。它们只能消费 Registry 的不可变事实，不能削弱本 ADR 的追加式约束。
