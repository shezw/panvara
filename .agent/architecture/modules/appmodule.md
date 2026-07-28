<!--
    Panvara
    docs/modules/appmodule.md    2026-07-15
     ______     __  __     ______     ______     __     __
    /\  ___\   /\ \_\ \   /\  ___\   /\___  \   /\ \  _ \ \
    \ \___  \  \ \  __ \  \ \  __\   \/_/  /__  \ \ \/ ".\ \
     \/\_____\  \ \_\ \_\  \ \_____\   /\_____\  \ \__/".~\_\
      \/_____/   \/_/\/_/   \/_____/   \/_____/   \/_/   \/_/.com

    @link    : https://github.com/shezw/panvara
    @author  : shezw
    @email   : hello@shezw.com
-->

# AppModule 使用指南

## 用途

AppModule 是 Panvara 的数据模型声明文件。你用 YAML 或 JSON 写出“有哪些数据、每个字段是什么、哪些接口可以使用、管理界面应显示什么”，Panvara 再生成稳定的运行模型、OpenAPI 和 Manager UI Schema。

它适合为新项目快速补充受控业务模块，不需要在模块中编写 Go、SQL 或 JavaScript。

## 当前状态

alpha.2 已实现：

- 严格读取 YAML 与 JSON 格式的 `panvara.dev/v1alpha1` AppModule。
- 对字段、约束、引用关系、API 白名单和 Manager 配置做语义校验。
- 生成确定性的 Canonical IR 与 `sha256:` Revision Hash。
- 生成 OpenAPI 3.1 和 `manager.panvara.dev/v1alpha1` UI Schema。
- 在 Server 启动时编译一个 AppModule，并让 Record Runtime 使用它。

当前 alpha.3a 开发切片还会生成 Data Schema Identity format 1，并在 Server 启动时把 Source、Canonical IR、OpenAPI 与 Manager UI Schema 登记为不可变父 Revision；数据结构身份作为可按 format 追加、不可覆盖的子事实保存。

::: danger 登记或发布都不等于激活
Server 仍直接读取本地 Source 来决定当前 Runtime。alpha.3b/P0-02a 已有 Draft/Plan 与显式 Publish API，但 Registry 本身没有通用上传、激活、回滚或在线编辑界面；Module Release 也不会改变 Record namespace 或 Runtime。
:::

## 前置条件

- 已完成 [运行模式](runtime-profiles.md) 中的 Server 环境。
- 准备一个 `.yaml`、`.yml` 或 `.json` 文件。
- 使用 `PANVARA_MODULE_SOURCE` 指向该文件。
- 模块 Source 最大 1 MiB。

只想阅读模型时不需要 PostgreSQL；但当前没有独立的“只校验模型”CLI，因此实际编译验收要启动 Server。

## 最小示例

将下面内容保存为 `contact-module.yaml`：

```yaml
apiVersion: panvara.dev/v1alpha1
kind: AppModule
metadata:
  name: demo.contacts
  version: 1.0.0
  labels:
    en-US: Contacts
    zh-CN: 联系人
spec:
  resources:
    - name: contact
      fields:
        - name: email
          type: email
          required: true
          unique: true
          constraints:
            maxLength: 320
      api:
        public:
          operations: [create]
          writable: [email]
        admin:
          operations: [list, get, create, patch, delete]
          writable: [email]
          filterable: [email]
      manager:
        list:
          columns: [id, email, created_at]
          filters: [email]
        form:
          fields: [email]
```

在已经导出其他 Server 环境变量的终端中运行：

```bash
export PANVARA_MODULE_SOURCE=/absolute/path/to/contact-module.yaml
export PANVARA_MODULE_FORMAT=auto
make run-server
```

成功时启动日志会包含类似结果：

```text
module=demo.contacts revision=sha256:<64个十六进制字符>
```

然后访问生成物：

```bash
curl http://127.0.0.1:8080/api/core/v1alpha1/modules/demo.contacts/openapi.json
curl http://127.0.0.1:8080/api/core/v1alpha1/modules/demo.contacts/ui-schema.json
```

## 配置

### 顶层结构

| 配置 | 作用 | alpha.2 要求 |
| --- | --- | --- |
| `apiVersion` | 模型协议版本 | 必须是 `panvara.dev/v1alpha1` |
| `kind` | 文档种类 | 必须是 `AppModule` |
| `metadata.name` | 模块唯一名称 | 小写字母开头，可使用数字、`.`、`-` |
| `metadata.version` | 模块业务版本 | 完整 SemVer，例如 `1.0.0` |
| `metadata.labels` | 多语言显示名 | 可选，例如 `en-US`、`zh-CN` |
| `spec.resources` | 数据资源列表 | 每个资源名称在模块内唯一 |
| `spec.requires.modules` | 依赖模块与版本范围 | 会进入 IR；当前单模块 Server 不负责装配依赖图 |
| `spec.requires.capabilities` | 需要的能力 | 只进入 IR/Hash，alpha.2 不解析 Provider |
| `spec.provides`、`conflicts` | 提供能力、冲突模块 | 已校验并进入 IR，当前不驱动运行期装配 |

所有 module、resource 与 field label 值必须是 1–256 字节的有效 UTF-8，并且不能包含 NUL（U+0000）；JSON 的 `\u0000` 会先解码，因此同样会被模型校验拒绝。

### 字段类型

| `type` | JSON 输入形态 | 示例 |
| --- | --- | --- |
| `string`、`text` | 字符串 | `"Panvara"` |
| `int` | 64 位整数 | `42` |
| `bool` | 布尔值 | `true` |
| `decimal` | 十进制字符串 | `"12.3400"` |
| `enum` | `options` 中的字符串 | `"new"` |
| `date` | `YYYY-MM-DD` | `"2026-07-14"` |
| `datetime` | RFC 3339 时间 | `"2026-07-14T08:00:00+08:00"` |
| `email` | 邮箱字符串 | `"user@example.com"` |
| `money` | 最小货币单位与币种对象 | `{"minor":1999,"currency":"USD"}` |
| `reference` | 目标记录 UUIDv7 | `"0198..."` |

常用字段选项：

- `required: true`：创建时必须提供。
- `unique: true`：在当前项目、模块、资源和 Revision 内唯一。
- `target`：`reference` 字段的目标资源名称。
- `options`：`enum` 的允许值。
- `constraints.maxLength`：限制 string、text、email 的字符数。
- `constraints.precision`、`scale`：限制 decimal。

### API 与 Manager

- `api.public.operations`：alpha.2 只允许 `create`。
- `api.admin.operations`：可选 `list`、`get`、`create`、`patch`、`delete`。
- `writable`：该入口允许写入的字段白名单。
- `filterable`：List 可使用的等值过滤字段。
- `sortable`：alpha.2 尚未实现，只要非空就会拒绝模型。
- `manager.list`、`manager.form`：只生成 UI Schema；不会启动网页。

完整可运行示例见 [`examples/modules/crm-leads.yaml`](../../examples/modules/crm-leads.yaml)。机器可读 Schema 位于 [`internal/spec/appmodule/v1alpha1/schema.json`](../../internal/spec/appmodule/v1alpha1/schema.json)。

### 三类内容身份

- Module Revision：完整 Canonical IR 的指纹；当前仍用于 Record namespace。
- Data Schema Identity：由 `format + fingerprint` 组成；当前 format 1 只投影 Resource、Field、引用、enum 与数据约束，不包含 SemVer、标签、Manager、API、Capability 或依赖。
- Source Hash：原始 YAML/JSON 字节指纹；同一 Module Revision 可以由字节不同的等价 Source 产生。

Registry 会把 Data Schema Identities 按 format 升序返回。新增投影算法可以给同一父 Revision 追加新 format，但不能改变父 Revision Hash 或覆盖已有身份。

详细规则见 [ADR-0001](../adr/0001-module-data-revision-identities.md)。

## 验收

按以下结果判断 AppModule 是否可用：

1. `make run-server` 启动成功，没有 `compile AppModule` 错误。
2. 日志显示正确的模块名称和 `sha256:` Revision。
3. `openapi.json` 返回 HTTP 200，内容含你的模块名和资源路径。
4. `ui-schema.json` 返回 HTTP 200，内容含资源、列表字段和表单字段。
5. 使用相同 Source 重启时 Revision 不变。
6. 输入未知字段、重复 YAML Key 或错误字段类型时，Server 应拒绝启动，而不是忽略错误。
7. Server 模式下，当前 OpenAPI 的 Revision 可在 [Revision Registry](revision-registry.md) 中查到且恰好一条。

## 常见问题

### 为什么只改了标签，重启后看不到原来的数据？

标签会进入完整 Canonical IR，因此会产生新 Module Revision，当前 Runtime 仍将它视为新的数据命名空间。标签不进入 Data Schema format 1 投影，所以两个 Revision 的 format 1 fingerprint 可以保持相同；这不代表已经自动迁移或激活。

### YAML 和 JSON 可以混用吗？

一个文件只能使用一种格式。`PANVARA_MODULE_FORMAT=auto` 会根据 `.yaml`、`.yml` 或 `.json` 扩展名判断。

### 为什么 `sortable` 配置启动失败？

动态排序是后续计划。alpha.2 为避免产生虚假能力，会主动拒绝非空 `sortable`。

### 可以在模块里执行自定义代码或 SQL 吗？

不可以。AppModule 只能表达受控数据、接口和显示信息。

### 为什么 `unique: true` 的 string/email 还必须配置 `maxLength`？

alpha.2 的唯一索引使用有界 Canonical 值，必须提前限制长度；最大为 512 字节。

## 当前限制

- 每个 Server 进程只加载一个 AppModule。
- 最多 128 个 Resource，每个 Resource 最多 256 个 Field。
- 不支持 `null`、任意表达式、动态 SQL、YAML Anchor/Alias 或自定义 Tag。
- Public API 只支持 Create。
- 不支持动态排序、Action、Event、审计、Provider 解析和模块热加载。
- Manager 配置只产生 JSON Schema，没有可视化应用。
- 完整 Module Revision 改变后没有数据迁移或自动回滚；Registry 只保留事实。

## 兼容与升级

`panvara.dev/v1alpha1`、IR Format 1 与 Data Schema Format 1 都是实验协议，不保证跨 alpha 版本无修改兼容。新的 Data Schema 算法必须使用新 format 并追加身份，不能重新解释 format 1。当前编译器只接受这个 AppModule 版本，也没有旧版本 Converter。

alpha.2 升级模型时必须：

1. 保存旧 Source 和启动日志中的 Revision Hash。
2. 复制为新文件后再修改，不覆盖唯一副本。
3. 提升 `metadata.version`，但不要误以为版本号会自动迁移数据。
4. 在独立数据库或可丢弃项目 ID 下验证新模型。
5. 需要旧数据时继续使用完全相同的旧 Source/Hash。

alpha.3a 本身只完成不可变 bootstrap 登记；当前分支随后以 alpha.3b 实现 Draft/Validation/Plan，并以 P0-02a 实现不可变 Publish Facts。Activate、Rollback 与数据迁移执行仍属于后续 alpha.3 工作。
