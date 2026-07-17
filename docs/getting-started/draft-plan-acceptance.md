<!--
    Panvara
    docs/getting-started/draft-plan-acceptance.md    2026-07-16
     ______     __  __     ______     ______     __     __
    /\  ___\   /\ \_\ \   /\  ___\   /\___  \   /\ \  _ \ \
    \ \___  \  \ \  __ \  \ \  __\   \/_/  /__  \ \ \/ ".\ \
     \/\_____\  \ \_\ \_\  \ \_____\   /\_____\  \ \__/".~\_\
      \/_____/   \/_/\/_/   \/_____/   \/_____/   \/_/   \/_/.com

    @link    : https://github.com/shezw/panvara
    @author  : shezw
    @email   : hello@shezw.com
-->

# Draft → Validate → Plan 完整验收

::: danger 本页不会发布、激活或迁移
这条路径只保存工作 Draft、记录 Validation 并生成 Change Plan。Candidate 不进入 Revision Registry，当前运行 Revision 不变，业务 Record 不迁移。页面中没有 Publish、Activate 或迁移执行步骤。
:::

这条 Golden Path 面向不阅读 Go 代码的验收者，只使用标准 shell、`curl` 和 `jq`，不需要 Manager UI，也不需要直接操作数据库。默认从正在运行的 `crm.leads` 取得 Baseline，先保存一个故意无效的 Draft，再覆盖为有效 Source 并生成 Plan，约需 **10–15 分钟**。

## 1. 准备 Server 与安全临时目录

终端 A 在仓库根目录启动 Server：

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

看到 Server 监听后不要关闭终端 A。终端 B 在同一仓库根目录执行：

```sh
set -eu
set +x
umask 077

set -a
. ./.env
. ./.env.local
set +a

BASE=http://127.0.0.1:8080
MODULE=crm.leads
WORK="$(mktemp -d "${TMPDIR:-/tmp}/panvara-draft-plan.XXXXXX")"
chmod 700 "$WORK"
cleanup() { rm -rf "$WORK"; }
trap cleanup EXIT HUP INT TERM

test -n "${PANVARA_ADMIN_TOKEN:-}"

admin_curl() {
  printf 'Authorization: Bearer %s\n' "$PANVARA_ADMIN_TOKEN" |
    curl --header @- "$@"
}

curl -fsS "$BASE/readyz" | jq -e '.status == "ready"'
```

`admin_curl` 通过标准输入把 Header 交给 curl，Token 不出现在 URL、Body 或 curl 参数中。验收期间不要执行 `set -x`、`curl -v` 或 trace，也不要打印环境变量。临时目录只允许当前用户访问，退出时会自动删除 Source 和响应。

## 2. 从当前运行模型取得 Baseline

当前运行 Revision 只从 OpenAPI 读取：

```sh
BASELINE="$(curl -fsS \
  "$BASE/api/core/v1alpha1/modules/$MODULE/openapi.json" \
  | jq -er '."x-panvara-revision"
    | select(test("^sha256:[0-9a-f]{64}$"))')"

admin_curl -fsS \
  "$BASE/api/admin/core/v1alpha1/modules/$MODULE/revisions/$BASELINE" \
  -o "$WORK/baseline.json"

jq -e --arg baseline "$BASELINE" \
  '.module == "crm.leads" and .revision == $baseline' \
  "$WORK/baseline.json"
```

输出 `true` 表示 Baseline 是同项目、同模块中已登记的精确 Revision。不要用 Registry List 第一项替代这一步；Registry 没有 Active 或“当前”排序语义。

在任何 Draft 操作前保存业务数据快照：

```sh
admin_curl -fsS \
  "$BASE/api/admin/v1alpha1/$MODULE/organization?limit=100" \
  -o "$WORK/records-before.json"
jq -S '.data' "$WORK/records-before.json" > "$WORK/records-before.normalized.json"
```

空数组也是合法快照。

## 3. 创建一个故意无效的 Draft

`examples/drafts/crm-leads-invalid.yaml` 是有效 YAML，但 `stage` 重复声明了 `won` enum option。Draft 应允许保存这个中间态：

```sh
CREATE_URL="$BASE/api/admin/core/v1alpha1/modules/$MODULE/drafts?baseline_revision=$BASELINE"
CREATE_KEY="draft-$(openssl rand -hex 16)"

STATUS="$(admin_curl -sS -X POST "$CREATE_URL" \
  -H 'Content-Type: application/yaml; charset=utf-8' \
  -H "Idempotency-Key: $CREATE_KEY" \
  --data-binary @examples/drafts/crm-leads-invalid.yaml \
  -D "$WORK/draft-create.headers" \
  -o "$WORK/draft-create.json" \
  -w '%{http_code}')"
test "$STATUS" = 201

DRAFT_ID="$(jq -er '.draft_id' "$WORK/draft-create.json")"
DRAFT_VERSION_ONE="$(jq -er '.draft_version' "$WORK/draft-create.json")"
DRAFT_ETAG_ONE="$(awk 'tolower($1) == "etag:" {
  gsub(/\r/, "", $2); print $2
}' "$WORK/draft-create.headers")"

jq -e --arg baseline "$BASELINE" '
  (.draft_id | test("^[0-9a-f]{8}-[0-9a-f]{4}-7[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$")) and
  .module == "crm.leads" and
  .baseline_revision == $baseline and
  .draft_version == 1 and
  .source_format == "yaml" and
  (.source_hash | test("^sha256:[0-9a-f]{64}$")) and
  (.created_at | type == "string") and
  (.updated_at | type == "string")
' "$WORK/draft-create.json"

test "$DRAFT_ETAG_ONE" = '"1"'

DRAFT_URL="$BASE/api/admin/core/v1alpha1/modules/$MODULE/drafts/$DRAFT_ID"
SOURCE_URL="$DRAFT_URL/source"
admin_curl -fsS "$SOURCE_URL" \
  -D "$WORK/draft-source.headers" \
  -o "$WORK/draft-source.yaml"

DRAFT_SOURCE_ETAG_ONE="$(awk 'tolower($1) == "etag:" {
  gsub(/\r/, "", $2); print $2
}' "$WORK/draft-source.headers")"
SOURCE_HASH_HEADER="$(awk 'tolower($1) == "x-panvara-source-hash:" {
  gsub(/\r/, "", $2); print $2
}' "$WORK/draft-source.headers")"

test "$DRAFT_SOURCE_ETAG_ONE" = "$DRAFT_ETAG_ONE"
test "$SOURCE_HASH_HEADER" = "$(jq -er '.source_hash' "$WORK/draft-create.json")"
cmp examples/drafts/crm-leads-invalid.yaml "$WORK/draft-source.yaml"
grep -qi '^cache-control: private, no-store' "$WORK/draft-source.headers"
```

这里的 `201` 只表示 Draft 已创建，不表示模型有效。GET Source 返回同一个数字 Draft Version ETag，可直接用于 PUT；`X-Panvara-Source-Hash` 才是内容身份。

### 验证 Create 网络重放

用同一个 Idempotency Key 和同一份原始 Body 重放：

```sh
STATUS="$(admin_curl -sS -X POST "$CREATE_URL" \
  -H 'Content-Type: application/yaml; charset=utf-8' \
  -H "Idempotency-Key: $CREATE_KEY" \
  --data-binary @examples/drafts/crm-leads-invalid.yaml \
  -D "$WORK/draft-create-replay.headers" \
  -o "$WORK/draft-create-replay.json" \
  -w '%{http_code}')"
test "$STATUS" = 200

jq -e -s '
  .[0].draft_id == .[1].draft_id and
  .[0].draft_version == .[1].draft_version and
  .[0].source_hash == .[1].source_hash and
  .[0].created_at == .[1].created_at
' "$WORK/draft-create.json" "$WORK/draft-create-replay.json"
```

应输出 `true`。首次为 201，重放为 200；系统没有创建第二个 Draft。不要把 Token 当作 Idempotency Key，也不要为同一个重试生成新 Key。

## 4. Validate 并观察作者错误

```sh
VALIDATIONS_URL="$DRAFT_URL/validations"

STATUS="$(admin_curl -sS -X POST "$VALIDATIONS_URL" \
  -H "If-Match: $DRAFT_ETAG_ONE" \
  -o "$WORK/validation-invalid.json" \
  -w '%{http_code}')"
test "$STATUS" = 201

jq -e --argjson draft_version "$DRAFT_VERSION_ONE" '
  .draft_version == $draft_version and
  .validation_format == 1 and
  .validation_id == .validation_hash and
  .valid == false and
  .candidate_revision == null and
  .data_schema_identities == [] and
  .stale == false and
  (.validation_hash | test("^sha256:[0-9a-f]{64}$")) and
  .warnings == [] and
  (.violations | type == "array" and length >= 1) and
  all(.violations[];
    (.code | type == "string" and length > 0) and
    (.stage | type == "string" and length > 0) and
    (.path | type == "string") and
    (.message | type == "string" and length > 0) and
    (.severity == "error" or .severity == "warning")) and
  .effects.revision_registered == false and
  .effects.published == false and
  .effects.activated == false and
  .effects.records_migrated == false and
  .effects.runtime_changed == false
' "$WORK/validation-invalid.json"

jq '{valid, violations, effects}' "$WORK/validation-invalid.json"
```

HTTP 201 表示“Validation 事实创建成功”；`valid=false` 才表示作者 Source 有问题。它不是 422 或服务故障。

重放相同 Draft Version 的 Validation：

```sh
STATUS="$(admin_curl -sS -X POST "$VALIDATIONS_URL" \
  -H "If-Match: $DRAFT_ETAG_ONE" \
  -o "$WORK/validation-invalid-replay.json" \
  -w '%{http_code}')"
test "$STATUS" = 200

jq -e -s '
  .[0].validation_id == .[1].validation_id and
  .[0].validation_hash == .[1].validation_hash and
  .[0].draft_version == .[1].draft_version
' "$WORK/validation-invalid.json" "$WORK/validation-invalid-replay.json"
```

## 5. 用有效 Source 覆盖 Draft

有效 Fixture 把 Module Version 改为 `1.1.0`，并只增加一个 `contacted` enum option：

```sh
STATUS="$(admin_curl -sS -X PUT "$SOURCE_URL" \
  -H 'Content-Type: application/yaml; charset=utf-8' \
  -H "If-Match: $DRAFT_ETAG_ONE" \
  --data-binary @examples/drafts/crm-leads-valid.yaml \
  -D "$WORK/draft-replace.headers" \
  -o "$WORK/draft-replace.json" \
  -w '%{http_code}')"
test "$STATUS" = 200

DRAFT_VERSION_TWO="$(jq -er '.draft_version' "$WORK/draft-replace.json")"
DRAFT_ETAG_TWO="$(awk 'tolower($1) == "etag:" {
  gsub(/\r/, "", $2); print $2
}' "$WORK/draft-replace.headers")"

test "$DRAFT_VERSION_TWO" = 2
test "$DRAFT_ETAG_TWO" = '"2"'
jq -e -s '
  .[1].draft_id == .[0].draft_id and
  .[1].draft_version == (.[0].draft_version + 1) and
  .[1].source_hash != .[0].source_hash and
  .[1].baseline_revision == .[0].baseline_revision
' "$WORK/draft-create.json" "$WORK/draft-replace.json"
```

Baseline 保持不变；只有 Source、Source Hash、Draft Version 和更新审计发生变化。

### 验证相同 Source Replace 是 no-op

```sh
STATUS="$(admin_curl -sS -X PUT "$SOURCE_URL" \
  -H 'Content-Type: application/yaml; charset=utf-8' \
  -H "If-Match: $DRAFT_ETAG_TWO" \
  --data-binary @examples/drafts/crm-leads-valid.yaml \
  -D "$WORK/draft-replace-replay.headers" \
  -o "$WORK/draft-replace-replay.json" \
  -w '%{http_code}')"
test "$STATUS" = 200

jq -e -s '
  .[0].draft_version == .[1].draft_version and
  .[0].source_hash == .[1].source_hash and
  .[0].updated_at == .[1].updated_at
' "$WORK/draft-replace.json" "$WORK/draft-replace-replay.json"
```

### 验证旧 ETag 不能覆盖新内容

```sh
STATUS="$(admin_curl -sS -X PUT "$SOURCE_URL" \
  -H 'Content-Type: application/yaml; charset=utf-8' \
  -H "If-Match: $DRAFT_ETAG_ONE" \
  --data-binary @examples/drafts/crm-leads-invalid.yaml \
  -o "$WORK/stale-replace.json" \
  -w '%{http_code}')"
test "$STATUS" = 412
jq -e '.code == "precondition_failed" and (.request_id | length > 0)' \
  "$WORK/stale-replace.json"
```

实际编辑中遇到 412 时，先 GET `$DRAFT_URL` 取得最新 Draft Version 和 ETag，比较 Source 后人工合并；不要自动用新 ETag 重放旧内容。

## 6. 再次 Validate 并取得 Candidate

```sh
STATUS="$(admin_curl -sS -X POST "$VALIDATIONS_URL" \
  -H "If-Match: $DRAFT_ETAG_TWO" \
  -o "$WORK/validation-valid.json" \
  -w '%{http_code}')"
test "$STATUS" = 201

VALIDATION_ID="$(jq -er '.validation_id' "$WORK/validation-valid.json")"
CANDIDATE="$(jq -er '.candidate_revision' "$WORK/validation-valid.json")"

jq -e --arg baseline "$BASELINE" '
  .baseline_revision == $baseline and
  .validation_format == 1 and
  (.validation_hash | test("^sha256:[0-9a-f]{64}$")) and
  .draft_version == 2 and
  .valid == true and
  .violations == [] and
  (.candidate_revision | test("^sha256:[0-9a-f]{64}$")) and
  .candidate_revision != $baseline and
  (.data_schema_identities | length == 1) and
  .data_schema_identities[0].format == 1 and
  (.data_schema_identities[0].fingerprint | test("^sha256:[0-9a-f]{64}$")) and
  .effects.revision_registered == false and
  .effects.published == false and
  .effects.activated == false and
  .effects.records_migrated == false and
  .effects.runtime_changed == false
' "$WORK/validation-valid.json"
```

Candidate Hash 已计算，但尚未进入 Registry。再次 POST 应返回 200，并保持相同 Validation ID：

```sh
STATUS="$(admin_curl -sS -X POST "$VALIDATIONS_URL" \
  -H "If-Match: $DRAFT_ETAG_TWO" \
  -o "$WORK/validation-valid-replay.json" \
  -w '%{http_code}')"
test "$STATUS" = 200

jq -e -s '
  .[0].validation_id == .[1].validation_id and
  .[0].validation_hash == .[1].validation_hash and
  .[0].candidate_revision == .[1].candidate_revision and
  .[0].data_schema_identities == .[1].data_schema_identities
' "$WORK/validation-valid.json" "$WORK/validation-valid-replay.json"
```

## 7. 生成确定性的 Change Plan

```sh
jq -n --arg validation_id "$VALIDATION_ID" \
  '{validation_id: $validation_id}' > "$WORK/plan-request.json"

PLANS_URL="$DRAFT_URL/plans"
STATUS="$(admin_curl -sS -X POST "$PLANS_URL" \
  -H 'Content-Type: application/json' \
  -H "If-Match: $DRAFT_ETAG_TWO" \
  --data-binary @"$WORK/plan-request.json" \
  -o "$WORK/plan.json" \
  -w '%{http_code}')"
test "$STATUS" = 201

PLAN_ID="$(jq -er '.plan_id' "$WORK/plan.json")"
PLAN_HASH="$(jq -er '.plan_hash' "$WORK/plan.json")"

jq -e --arg baseline "$BASELINE" --arg candidate "$CANDIDATE" '
  .plan_format == 1 and
  .baseline_revision == $baseline and
  .candidate_revision == $candidate and
  .draft_version == 2 and
  (.plan_id | test("^sha256:[0-9a-f]{64}$")) and
  (.plan_hash | test("^sha256:[0-9a-f]{64}$")) and
  (.plan_id != .plan_hash) and
  (.baseline_data_schema_identities | length == 1) and
  (.candidate_data_schema_identities | length == 1) and
  (.summary.classification | IN("compatible", "review_required", "migration_required", "unsupported")) and
  (.summary.risk | IN("none", "low", "medium", "high", "critical")) and
  .summary.change_count == (.changes | length) and
  (.changes | type == "array" and length >= 2) and
  (any(.changes[]; .path | contains("stage"))) and
  .data_schema_changed == true and
  .record_namespace_changed == true and
  .migration_execution_supported == false and
  .stale == false and
  .effects.plan_recorded == true and
  .effects.revision_registered == false and
  .effects.published == false and
  .effects.activated == false and
  .effects.records_migrated == false and
  .effects.runtime_changed == false
' "$WORK/plan.json"

jq '{plan_id, plan_hash, summary, changes, effects}' \
  "$WORK/plan.json"
```

`plan_id` 标识这一代 Draft/Validation 的精确快照，`plan_hash` 标识规范计划语义；当前 Fixture 应得到两个不同的 SHA-256 值。下方重放检查要求二者各自稳定，不要求二者彼此相等。

`record_namespace_changed=true` 是重要提醒：即使某项 Schema 变化看起来兼容，当前 Runtime 仍按完整 Module Revision 隔离 Record。Plan 不会因此复制数据或允许激活。

重放同一个 Plan 请求：

```sh
STATUS="$(admin_curl -sS -X POST "$PLANS_URL" \
  -H 'Content-Type: application/json' \
  -H "If-Match: $DRAFT_ETAG_TWO" \
  --data-binary @"$WORK/plan-request.json" \
  -o "$WORK/plan-replay.json" \
  -w '%{http_code}')"
test "$STATUS" = 200

jq -e -s '
  .[0].plan_id == .[1].plan_id and
  .[0].plan_hash == .[1].plan_hash and
  .[0].changes == .[1].changes and
  .[0].created_at == .[1].created_at
' "$WORK/plan.json" "$WORK/plan-replay.json"

test "$PLAN_ID" = "$(jq -er '.plan_id' "$WORK/plan-replay.json")"
test "$PLAN_HASH" = "$(jq -er '.plan_hash' "$WORK/plan-replay.json")"
```

## 8. 证明没有登记、发布、激活或迁移

首先确认当前运行 Revision 完全没变：

```sh
CURRENT_AFTER="$(curl -fsS \
  "$BASE/api/core/v1alpha1/modules/$MODULE/openapi.json" \
  | jq -er '."x-panvara-revision"')"
test "$CURRENT_AFTER" = "$BASELINE"
```

再确认 Candidate 没有进入 Registry：

```sh
admin_curl -fsS \
  "$BASE/api/admin/core/v1alpha1/modules/$MODULE/revisions?limit=100" \
  -o "$WORK/revisions-after.json"

jq -e --arg candidate "$CANDIDATE" \
  '[.data[] | select(.revision == $candidate)] | length == 0' \
  "$WORK/revisions-after.json"
```

最后比较业务 Record：

```sh
admin_curl -fsS \
  "$BASE/api/admin/v1alpha1/$MODULE/organization?limit=100" \
  -o "$WORK/records-after.json"
jq -S '.data' "$WORK/records-after.json" > "$WORK/records-after.normalized.json"
cmp "$WORK/records-before.normalized.json" "$WORK/records-after.normalized.json"

jq -e '
  .effects.revision_registered == false and
  .effects.published == false and
  .effects.activated == false and
  .effects.records_migrated == false and
  .effects.runtime_changed == false and
  .migration_execution_supported == false
' "$WORK/plan.json"
```

`cmp` 没有输出且最后一条输出 `true`，才表示业务数据快照未变。Draft、Validation 和 Plan 自身会写入控制面事实，这是本页预期；“无副作用”特指它们不会改变 Registry Candidate、运行模型或业务 Record。

## 9. 收尾与通过条件

终端 A 按 `Ctrl+C` 停止 Server。需要停止 PostgreSQL 时执行：

```sh
make infra-down
```

以下条件全部成立即可通过 alpha.3b 用户验收：

- Baseline 明确来自当前运行 OpenAPI，并在 Registry 中存在；
- 无效 Source 可以创建 Draft，Validation 以 2xx + `valid=false` 返回结构化 violations；
- Create、Validation 和 Plan 重放不产生重复事实；
- 相同 Source Replace 不递增 Draft Version，旧 ETag 不能覆盖不同内容；
- 修正后 Validation 产生 Candidate，Plan 稳定解释版本和 enum 变化；
- Candidate 不在 Registry，当前 OpenAPI Revision 与业务 Record 快照均未改变；
- 所有 Validation/Plan effects 明确表示未登记、未发布、未激活、未迁移、未切换 Runtime。

如果某一步失败，请保留响应中的 `request_id` 并查看 Server 日志。401 时重新加载环境但不要打印 Token；412 时读取最新 Draft 并人工合并；500 时不要直接修改数据库。概念和错误恢复详见 [Draft Planning 使用指南](../modules/draft-planning.md)。
