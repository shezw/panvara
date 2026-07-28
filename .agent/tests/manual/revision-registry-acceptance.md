<!--
    Panvara
    docs/getting-started/revision-registry-acceptance.md    2026-07-15
     ______     __  __     ______     ______     __     __
    /\  ___\   /\ \_\ \   /\  ___\   /\___  \   /\ \  _ \ \
    \ \___  \  \ \  __ \  \ \  __\   \/_/  /__  \ \ \/ ".\ \
     \/\_____\  \ \_\ \_\  \ \_____\   /\_____\  \ \__/".~\_\
      \/_____/   \/_/\/_/   \/_____/   \/_____/   \/_/   \/_/.com

    @link    : https://github.com/shezw/panvara
    @author  : shezw
    @email   : hello@shezw.com
-->

# Revision Registry 完整验收

::: danger 登记不等于发布或激活
本页只验收“启动后留下不可变、可复验的 Revision 事实”。它不会发布、激活、回滚或切换模块。当前运行 Revision 只从 OpenAPI 的 `x-panvara-revision` 获取，绝不使用 Registry List 第一项推断。
:::

这条路径面向不阅读 Go 代码的验收者，默认使用 `examples/modules/crm-leads.yaml`，完整操作约需 **10–15 分钟**。请准备两个终端：终端 A 运行 Server，终端 B 复制验收命令。

开始前只需要理解三个词：

- **父 Revision**：一份不可变的完整模型制品，包含 Source、Canonical IR、生成物和首次登记信息。
- **Data Schema Identity**：父 Revision 下的一项数据结构算法身份，由 `format` 和 `fingerprint` 组成。
- **幂等**：相同内容重复启动，不产生重复记录，也不改写第一次登记的信息。

::: warning 身份数组可以追加，已有身份不能改写
当前编译器登记 format 1。未来新增投影算法时，可以给同一父 Revision 追加 format 2、3 等身份，但不能改变父 Revision Hash 或覆盖已有 format。验收命令会按 `format == 1` 选择身份，绝不依赖数组位置。
:::

## 1. 准备并启动

在终端 A 的仓库根目录执行：

```sh
make doctor-server
make local-init
set -a
. ./.env
. ./.env.local
set +a
make infra-up
make run-server
```

看到 Server 开始监听后不要关闭终端 A。

在终端 B 载入同一配置并声明快捷变量：

```sh
set -a
. ./.env
. ./.env.local
set +a

BASE=http://127.0.0.1:8080
MODULE=crm.leads
AUTH="Authorization: Bearer $PANVARA_ADMIN_TOKEN"
curl -fsS "$BASE/readyz" | jq -e '.status == "ready"'
```

最后一条命令输出 `true` 表示可以继续。

## 2. 验证认证边界

不带 Token 查询 Registry：

```sh
curl -sS -o /tmp/panvara-registry-unauthorized.json \
  -w '%{http_code}\n' \
  "$BASE/api/admin/core/v1alpha1/modules/$MODULE/revisions"
```

预期状态码是 `401`。如果是 `200`，立即停止验收，这是权限缺陷。

## 3. 从 OpenAPI 取得当前 Revision

```sh
REVISION="$(curl -fsS "$BASE/api/core/v1alpha1/modules/$MODULE/openapi.json" \
  | jq -er '."x-panvara-revision"')"
printf '%s\n' "$REVISION"
```

预期输出以 `sha256:` 开头。后续都使用这个值；不要运行 `jq -r '.data[0].revision'` 来猜当前版本。

## 4. 验证 List 与 Detail

```sh
curl -fsS -D /tmp/panvara-revisions-before.headers \
  "$BASE/api/admin/core/v1alpha1/modules/$MODULE/revisions?limit=100" \
  -H "$AUTH" -o /tmp/panvara-revisions-before.json

jq -e --arg revision "$REVISION" \
  '[.data[] | select(.revision == $revision)] | length == 1' \
  /tmp/panvara-revisions-before.json

curl -fsS -D /tmp/panvara-revision-before.headers \
  "$BASE/api/admin/core/v1alpha1/modules/$MODULE/revisions/$REVISION" \
  -H "$AUTH" -o /tmp/panvara-revision-before.json

jq -e '((keys | sort) == ([
  "module", "revision", "module_version", "data_schema_identities",
  "spec_version", "ir_format", "source_format",
  "source_hash", "origin", "registered_by", "registered_at"
] | sort))' /tmp/panvara-revision-before.json

jq -e '
  .data_schema_identities as $identities |
  ($identities | type == "array" and length >= 1) and
  ($identities == ($identities | sort_by(.format))) and
  (($identities | map(.format) | unique | length) == ($identities | length)) and
  all($identities[];
    ((keys | sort) == (["format", "fingerprint"] | sort)) and
    (.format | type == "number") and
    (.format == (.format | floor)) and .format >= 1 and
    (.fingerprint | test("^sha256:[0-9a-f]{64}$"))) and
  ([$identities[] | select(.format == 1)] | length == 1)
' /tmp/panvara-revision-before.json

DATA_SCHEMA_V1_FINGERPRINT="$(jq -er '
  .data_schema_identities
  | map(select(.format == 1))
  | if length == 1 then .[0].fingerprint
    else error("format 1 data schema identity is missing or duplicated")
    end
' /tmp/panvara-revision-before.json)"

grep -qi '^cache-control: private, no-store' \
  /tmp/panvara-revisions-before.headers
grep -qi '^cache-control: private, no-store' \
  /tmp/panvara-revision-before.headers

jq '{module, revision, module_version, data_schema_identities,
  source_format, source_hash,
  origin, registered_by, registered_at}' /tmp/panvara-revision-before.json

printf 'Data Schema format 1 fingerprint: %s\n' \
  "$DATA_SCHEMA_V1_FINGERPRINT"
```

所有检查都应成功。Detail 中 `origin` 应为 `bootstrap`，`registered_by` 应为 `system:bootstrap`。`data_schema_identities` 必须按 format 升序且 format 不重复；当前验收只读取 format 1，不读取固定下标。

List 与 Detail 的 `Cache-Control: private, no-store` 表示 owner 元数据不能由共享缓存保存，也不应在客户端持久缓存。升级后的 Panvara 可能给父 Revision 追加新的算法身份，因此旧 Detail 不能被当作永久快照。

## 5. 验证原始 Source、Content-Type 与 ETag

```sh
SOURCE_URL="$BASE/api/admin/core/v1alpha1/modules/$MODULE/revisions/$REVISION/source"

curl -fsS -D /tmp/panvara-source-headers-before.txt \
  -o /tmp/panvara-source-before \
  -H "$AUTH" "$SOURCE_URL"

SOURCE_FORMAT="$(jq -r '.source_format' /tmp/panvara-revision-before.json)"
SOURCE_HASH="$(jq -r '.source_hash' /tmp/panvara-revision-before.json)"
SOURCE_ETAG="$(awk 'tolower($1) == "etag:" {gsub(/\r/, "", $2); print $2}' \
  /tmp/panvara-source-headers-before.txt)"
ACTUAL_HASH="sha256:$(openssl dgst -sha256 /tmp/panvara-source-before | awk '{print $NF}')"

test "$SOURCE_ETAG" = "\"$SOURCE_HASH\""
test "$ACTUAL_HASH" = "$SOURCE_HASH"
grep -qi '^cache-control: private, no-cache' \
  /tmp/panvara-source-headers-before.txt

case "$SOURCE_FORMAT" in
  yaml) grep -qi '^content-type: application/yaml; charset=utf-8' \
    /tmp/panvara-source-headers-before.txt ;;
  json) grep -qi '^content-type: application/json; charset=utf-8' \
    /tmp/panvara-source-headers-before.txt ;;
  *) printf 'unexpected source format: %s\n' "$SOURCE_FORMAT"; false ;;
esac
```

所有命令都应成功且没有错误输出。强 ETag 必须精确等于带双引号的 `source_hash`。`private, no-cache` 表示 Source 只能在私有缓存中保存，而且再次使用前必须向 Server 复验。再验证缓存协商：

```sh
curl -sS -o /dev/null -w '%{http_code}\n' \
  -H "$AUTH" -H "If-None-Match: \"$SOURCE_HASH\"" "$SOURCE_URL"
```

预期状态码是 `304`。

## 6. 用相同 Source 重启

回到终端 A，按 `Ctrl+C` 停止 Server，再执行同一条命令：

```sh
make run-server
```

终端 B 等 `/readyz` 恢复后执行：

```sh
curl -fsS "$BASE/readyz" | jq -e '.status == "ready"'

curl -fsS "$BASE/api/admin/core/v1alpha1/modules/$MODULE/revisions?limit=100" \
  -H "$AUTH" > /tmp/panvara-revisions-after.json

jq -e --arg revision "$REVISION" \
  '[.data[] | select(.revision == $revision)] | length == 1' \
  /tmp/panvara-revisions-after.json

curl -fsS "$BASE/api/admin/core/v1alpha1/modules/$MODULE/revisions/$REVISION" \
  -H "$AUTH" > /tmp/panvara-revision-after.json

jq -e -s '.[0].registered_at == .[1].registered_at and
  .[0].source_hash == .[1].source_hash and
  .[0].data_schema_identities == .[1].data_schema_identities' \
  /tmp/panvara-revision-before.json /tmp/panvara-revision-after.json
```

两次检查都应输出 `true`：同一 Revision 仍只有一条，首次登记时间、Source Hash 和当前构建已知的数据结构身份都没有被重启改写。未来升级到支持新 format 的 Panvara 时，数组可以追加新项，但任何已有 format 都不能改变。

## 7. 验证等价 YAML 保留第一次 Source

默认示例是 YAML。终端 B 创建一份只增加注释、编译语义相同但字节不同的临时文件：

```sh
EQUIVALENT_SOURCE=/tmp/panvara-crm-leads-equivalent.yaml
awk 'BEGIN {print "# equivalent source bytes for registry acceptance"} {print}' \
  "$PANVARA_MODULE_SOURCE" > "$EQUIVALENT_SOURCE"
cmp -s "$PANVARA_MODULE_SOURCE" "$EQUIVALENT_SOURCE" && false || true
```

回到终端 A，按 `Ctrl+C`，再使用临时 Source 启动：

```sh
PANVARA_MODULE_SOURCE=/tmp/panvara-crm-leads-equivalent.yaml make run-server
```

终端 B 执行：

```sh
curl -fsS "$BASE/readyz" | jq -e '.status == "ready"'

REVISION_AFTER="$(curl -fsS "$BASE/api/core/v1alpha1/modules/$MODULE/openapi.json" \
  | jq -er '."x-panvara-revision"')"
test "$REVISION_AFTER" = "$REVISION"

curl -fsS -o /tmp/panvara-source-after \
  -H "$AUTH" "$SOURCE_URL"
cmp /tmp/panvara-source-before /tmp/panvara-source-after

curl -fsS "$BASE/api/admin/core/v1alpha1/modules/$MODULE/revisions?limit=100" \
  -H "$AUTH" \
  | jq -e --arg revision "$REVISION" \
    '[.data[] | select(.revision == $revision)] | length == 1'
```

Revision 应保持相同，`cmp` 不应输出差异，登记仍恰好一条。这证明等价 YAML 不会覆盖第一次登记的 Source 字节。

## 8. 验证 Registry 不能删除

```sh
curl -sS -D /tmp/panvara-delete-headers.txt \
  -o /tmp/panvara-delete-response.json \
  -w '%{http_code}\n' -X DELETE -H "$AUTH" \
  "$BASE/api/admin/core/v1alpha1/modules/$MODULE/revisions/$REVISION"

grep -qi '^allow: GET' /tmp/panvara-delete-headers.txt
```

预期状态码是 `405`，并且 `Allow` 为 `GET`。随后再次 GET Detail 仍应返回 200。

## 9. 收尾与验收结论

终端 A 按 `Ctrl+C` 停止 Server。需要停止 PostgreSQL 时，在任一已加载配置的终端执行：

```sh
make infra-down
```

以下条件全部成立即可通过 alpha.3a Revision Registry 用户验收：

- 未认证读取被拒绝；
- 当前 OpenAPI Revision 在 Registry 中恰好一条；
- Detail 字段完整，身份数组按 format 升序且 format 1 恰好一项；
- List/Detail 使用 `private, no-store`，Source 使用 `private, no-cache`；
- Source Hash、Content-Type 和强 ETag 一致；
- 相同 Source 重启不改变首次登记信息；
- 等价 YAML 保留第一次 Source；
- DELETE 返回 405；
- 全程没有出现发布、激活或运行时切换行为。

## 出现问题时

| 现象 | 常见原因 | 恢复方式 |
| --- | --- | --- |
| `/readyz` 连接失败 | Server 未启动或端口被占用 | 回到终端 A 检查输出，修复后重新执行，不要跳过 readiness |
| `/readyz` 返回 503 | 数据库、migration 或启动登记失败 | 保留日志并处理根因；Registry 失败时不能继续验收 |
| Registry 返回 401 | 终端 B 没有加载 `.env.local` | 重新加载两个 env 文件；不要打印或粘贴 Token |
| Registry 返回 404 | 模块名或完整 Revision 错误 | 重新从 OpenAPI 提取，不要手工缩写 Hash |
| 当前 Revision 在 List 中为 0 | Hash 错误、登记失败，或历史超过 100 条 | 先直接 GET Detail；新数据库出现时应停止验收 |
| List 总数大于 1 | 数据库已有其他历史 Revision | 这是正常情况，只要求当前 Revision 恰好一条 |
| format 1 缺失或重复 | Registry 身份数据损坏 | 停止验收并保留数据库，禁止手工补写 |
| 下载 Source 的 Hash 不一致 | Source 或元数据损坏 | 立即停止；不能解释为 YAML 格式差异 |
| 本地 Source 与下载内容不同 | 当前文件变化，或首次登记的是等价 Source | 核对 Revision；Registry 必须继续保留首次 Source |
| 重启后已有 format 指纹改变 | 幂等或不可变约束被破坏 | 验收失败；不能覆盖已有 identity |
| `jq` 报解析错误 | HTTP 返回了错误 JSON | 对同一 URL 执行 `curl -i`，先检查状态码和认证 |

若任一步失败，请保存 `/tmp/panvara-*` 文件和 Server 日志，按[故障排查](./troubleshooting.md)处理。不要直接修改、删除或清空父 Revision 与 Data Schema Identity 数据库表。
