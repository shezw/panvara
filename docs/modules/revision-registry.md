<!--
    Panvara
    docs/modules/revision-registry.md    2026-07-15
     ______     __  __     ______     ______     __     __
    /\  ___\   /\ \_\ \   /\  ___\   /\___  \   /\ \  _ \ \
    \ \___  \  \ \  __ \  \ \  __\   \/_/  /__  \ \ \/ ".\ \
     \/\_____\  \ \_\ \_\  \ \_____\   /\_____\  \ \__/".~\_\
      \/_____/   \/_/\/_/   \/_____/   \/_____/   \/_/   \/_/.com

    @link    : https://github.com/shezw/panvara
    @author  : shezw
    @email   : hello@shezw.com
-->

# Revision Registry 使用指南

::: danger 登记不等于发布或激活
alpha.3a 的 Registry 只保存 Server 启动时成功编译的不可变事实。它不会发布、激活、回滚或热切换模块；当前运行 Revision 必须从 OpenAPI 的 `x-panvara-revision` 读取，不能取 List 第一项。
:::

## 用途

Revision Registry 保存每个项目中已成功编译的 AppModule Source、Canonical IR、生成物和身份元数据。它让开发者在 Server 重启后仍可查询“这个 Revision 从哪里来、有哪些数据结构算法身份、第一次登记了哪些 Source 字节”。

## 当前状态

该能力是 **alpha.3a 开发切片**；Panvara Distribution 仍为 `v0.1.0-alpha.2`。

当前已实现：Server 启动登记、PostgreSQL 追加式存储、项目 owner 只读 API、Source 下载与 ETag，以及 Detail/Source 读取时重新编译校验。List 只读取有界元数据，不加载 Source 或生成物。alpha.3b 的 Draft/Validation/Plan 是引用 Registry Baseline 的独立控制面，不会修改 Registry；Publish、Activate、Rollback、活动版本指针和运行时热切换仍不存在。

## 前置条件

- 使用 Server Profile，并已完成 [本地环境](../getting-started/local-environment.md)。
- PostgreSQL 18.4 可用，`PANVARA_PROJECT_ID`、模块 Source 和管理员 Token 已加载。
- 使用 `curl` 调用接口，使用 `jq` 读取 JSON。
- 先完成 [Revision Registry 完整验收](../getting-started/revision-registry-acceptance.md)中的边界说明。

## 最小示例

启动 Server 后，在另一个已加载 `.env` 与 `.env.local` 的终端执行：

```sh
BASE=http://127.0.0.1:8080
MODULE=crm.leads
REVISION="$(curl -fsS "$BASE/api/core/v1alpha1/modules/$MODULE/openapi.json" \
  | jq -er '."x-panvara-revision"')"

curl -fsS "$BASE/api/admin/core/v1alpha1/modules/$MODULE/revisions?limit=100" \
  -H "Authorization: Bearer $PANVARA_ADMIN_TOKEN" | jq

curl -fsS "$BASE/api/admin/core/v1alpha1/modules/$MODULE/revisions/$REVISION" \
  -H "Authorization: Bearer $PANVARA_ADMIN_TOKEN" | jq
```

成功时能找到与 `REVISION` 完全相同的记录。不要使用 `.data[0].revision` 推断当前运行版本。

## 配置

Registry 不增加新的环境变量。它沿用当前 Server 的：

- `PANVARA_PROJECT_ID`：登记和读取的项目边界；
- `PANVARA_MODULE_SOURCE`、`PANVARA_MODULE_FORMAT`：启动时编译并登记的 Source；
- `PANVARA_ADMIN_TOKEN`：示例使用的首启 bootstrap Credential；任何 active Principal + active Credential + active `project.owner` Grant 组合都可访问该管理接口。

List 只接受一个可选 `limit`，范围 1–100、默认 20；不支持 cursor 或其他查询参数。Detail 返回且仅返回：`module`、`revision`、`module_version`、`data_schema_identities`、`spec_version`、`ir_format`、`source_format`、`source_hash`、`origin`、`registered_by`、`registered_at`。

`data_schema_identities` 是按 `format` 升序的数组，每项形如 `{"format":1,"fingerprint":"sha256:..."}`。父 Module Revision 保存 Source、IR、生成物与首次登记信息；Data Schema Identity 是它下面的派生子身份。未来 Panvara 可以给同一父 Revision 追加新算法 format，但不能改变父 Revision Hash、覆盖已有 format 或改写首次登记信息。即使当前排序中 format 1 位于第一项，客户端也必须按 format 选择，不能用 `[0]` 表达算法需求。

List 与 Detail 带 `Cache-Control: private, no-store`。Source 的 Content-Type 为 `application/yaml; charset=utf-8` 或 `application/json; charset=utf-8`，强 ETag 精确为 `"<source_hash>"`，并带 `Cache-Control: private, no-cache`。

## 验收

完整、可复制的非专业用户验收见 [Revision Registry 完整验收](../getting-started/revision-registry-acceptance.md)。关键成功标志是：

1. 不带 Token 返回 401，带正确 Token 可以读取。
2. 当前 OpenAPI Revision 在 List 中恰好出现一次。
3. 相同 Source 和同一 Panvara 构建重启后，`registered_at`、`source_hash` 和 `data_schema_identities` 不变。
4. 等价 YAML 使用同一 Revision，并继续返回第一次登记的 Source 字节。
5. 对 Detail 发送 DELETE 返回 405，不会删除记录。

工程门禁对应 `make test`、`make test-integration`、`make test-server-smoke` 和 `make docs-check`。

## 常见问题

### List 第一项是不是当前活动 Revision？

不是。Registry 没有活动状态。请读取当前运行模块 OpenAPI 顶层的 `x-panvara-revision`。

### 为什么两份不同 YAML 只保留第一份？

如果它们编译成相同 Canonical IR，就属于同一不可变 Revision。幂等登记保留第一次 Source，避免后续重启改写来源证明。

### format 1 的 Data Schema fingerprint 相同是否表示可以直接迁移？

不是。它只表示 format 1 投影中的 Resource、Field 和数据约束相同。迁移与激活仍需未来的 Plan 和兼容规则。

### 为什么同一 Revision 的身份数组以后可能变长？

Module Revision 是不可变父制品，Data Schema Identity 是按算法 format 管理的不可变子事实。新增算法只能追加一个新的 format；它不会改变父 Revision Hash，也不能覆盖 format 1。List/Detail 使用 `no-store`，正是为了避免客户端长期保存缺少新身份的旧元数据。

### 读取为什么可能返回 500？

Detail 与 Source 会重新编译保存的 Source 并核对所有制品。持久化字节不一致时会 fail closed；List 只返回已校验形态的有界元数据，不承担制品复验。请保留数据库并查看服务日志，不要通过 SQL 修改记录。

## 当前限制

- 只登记 Server 启动时加载的单个模块，没有上传或远程登记 API。
- 只有 bootstrap `project.owner` 可读，没有账号体系和细粒度角色。
- List 没有 cursor，单次最多 100 条。
- 完整制品存放于 PostgreSQL，尚未外置到对象存储。
- 不提供更新、删除、发布、激活、回滚和热加载。
- alpha.3b Candidate 只有 Validation 身份，不会因为生成 Plan 自动进入 Registry。
- Registry 不改变 Record namespace；当前仍由完整 Module Revision 隔离数据。

## 兼容与升级

Registry 表由 forward-only 数据库迁移创建，父 Revision 与子 Data Schema Identity 都不得直接 UPDATE、DELETE 或 TRUNCATE。Module Revision、按 format 选择的 Data Schema Identity、Source Hash 和各自格式号是不同版本轴，升级时必须分别比较。

alpha.3a/alpha.3b 是开发切片，不提升 Distribution 版本，也不表示 alpha.3 发布生命周期已经完成。Draft Planning 还必须遵守 [ADR-0003](../adr/0003-draft-validation-change-plan.md)；后续 Publish/Activate 设计必须引用三份 ADR，并保持现有事实可验证、不可改写。
