<!--
    Panvara
    docs/guides appmodule.md    2026-07-28
     ______     __  __     ______     ______     __     __
    /\  ___\   /\ \_\ \   /\  ___\   /\___  \   /\ \  _ \ \
    \ \___  \  \ \  __ \  \ \  __\   \/_/  /__  \ \ \/ ".\ \
     \/\_____\  \ \_\ \_\  \ \_____\   /\_____\  \ \__/".~\_\
      \/_____/   \/_/\/_/   \/_____/   \/_/   \/_/.com

    @link    : https://github.com/shezw/panvara
    @author  : shezw
    @email   : hello@shezw.com
-->

# 定义 AppModule

::: info Current Distribution
当前 Distribution 标识 **v0.1.0-alpha.2** 对应的 AppModule 能力可以严格读取 YAML/JSON，生成确定的模型 Revision、OpenAPI 3.1 和 Manager UI Schema，并由 Server 运行 Record API。
:::

## 写一个最小模型

把下面内容保存为 `contact-module.yaml`：

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

在已经完成 [Server 配置](./server)的终端中指定 Source：

```sh
export PANVARA_MODULE_SOURCE=/absolute/path/to/contact-module.yaml
export PANVARA_MODULE_FORMAT=auto
make run-server
```

成功日志应包含：

```text
module=demo.contacts revision=sha256:<64个十六进制字符>
```

检查生成契约：

```sh
curl -fsS http://127.0.0.1:8080/api/core/v1alpha1/modules/demo.contacts/openapi.json
curl -fsS http://127.0.0.1:8080/api/core/v1alpha1/modules/demo.contacts/ui-schema.json
```

OpenAPI 应包含 `demo.contacts` 的 Record 路径；UI Schema 应包含列表列与表单字段。

## 顶层字段

| 字段 | 要求 |
| --- | --- |
| `apiVersion` | 必须是 `panvara.dev/v1alpha1` |
| `kind` | 必须是 `AppModule` |
| `metadata.name` | 小写字母开头，可包含数字、`.`、`-` |
| `metadata.version` | 完整 SemVer，例如 `1.0.0` |
| `spec.resources` | 至少一个名称唯一的 Resource |

Source 最大 1 MiB。解析器会拒绝未知字段、重复 YAML/JSON Key、YAML Anchor/Alias、自定义 Tag、NUL 字符和不符合类型的值，不会静默忽略。

## 字段与约束

当前支持 `string`、`text`、`int`、`bool`、`decimal`、`enum`、`date`、`datetime`、`email`、`money` 和 `reference`。

常用选项：

- `required: true`：Create 时必须提供；
- `unique: true`：当前数据 Scope 内唯一；
- `target`：`reference` 指向的 Resource；
- `options`：`enum` 的允许值；
- `constraints.maxLength`：字符串、文本或邮箱的最大字符数；
- `constraints.precision` / `scale`：十进制范围。

唯一的 string/email 必须配置有界 `maxLength`，最大 512。

## API 与界面描述

- `api.public.operations` 当前只允许 `create`；
- `api.admin.operations` 可选 `list`、`get`、`create`、`patch`、`delete`；
- `writable` 是可写字段白名单；
- `filterable` 是列表可用的等值过滤字段；
- 非空 `sortable` 当前会被拒绝；
- `manager.list` 与 `manager.form` 只生成 JSON 描述，不会启动网页。

完整示例见固定源码快照中的 [`examples/modules/crm-leads.yaml`](https://github.com/shezw/panvara/blob/cfea044bfee90b7b8d62de79e42d2503258d7781/examples/modules/crm-leads.yaml)。

## 验收与限制

模型可用需同时满足：

1. Server 启动成功并显示模块名与 Revision；
2. OpenAPI 与 UI Schema 返回 200；
3. 相同 Source 重启得到相同 Revision；
4. 故意加入未知字段后，Server 会明确拒绝启动。

当前每个 Server 只加载一个 AppModule；不支持任意代码、动态 SQL、Action/Event、Provider 解析、热加载或自动数据升级。修改已使用的模型前，先阅读[版本与兼容](/releases/compatibility)。
