<!--
    Panvara
    docs/getting-started/draft-publish-acceptance.md    2026-07-19
     ______     __  __     ______     ______     __     __
    /\  ___\   /\ \_\ \   /\  ___\   /\___  \   /\ \  _ \ \
    \ \___  \  \ \  __ \  \ \  __\   \/_/  /__  \ \ \/ ".\ \
     \/\_____\  \ \_\ \_\  \ \_____\   /\_____\  \ \__/".~\_\
      \/_____/   \/_/\/_/   \/_____/   \/_____/   \/_/   \/_/.com

    @link    : https://github.com/shezw/panvara
    @author  : shezw
    @email   : hello@shezw.com
-->

# Draft → Publish 完整验收

::: danger 本页仍不会上线新模型
这条旅程会把 Candidate 登记进 Revision Registry，并写入不可变 Module Release；它不会激活 Candidate、迁移 Record 或改变当前 Runtime。验收成功后，业务 API 继续使用发布前的 Revision，这是正确结果。
:::

本页接续 [Draft → Validate → Plan 完整验收](draft-plan-acceptance.md)，面向不阅读 Go 代码的使用者，约需 **10–15 分钟**。

## 1. 准备一个可发布 Plan

在 `draft-plan-acceptance` 页面依次完成第 1–7 节，生成 `$WORK/plan.json` 后暂停，不要执行其第 8 节“Candidate 不在 Registry”的最终断言。保持两个终端和终端 B 中的这些变量：

- `BASE`、`MODULE`、`WORK`、`BASELINE`；
- `DRAFT_URL`、`SOURCE_URL`、`DRAFT_ETAG_TWO`；
- `PLAN_ID`、`PLAN_HASH`、`CANDIDATE`；
- `admin_curl` 安全调用函数。

先确认上下文仍然有效：

```sh
test -n "$PLAN_ID"
test -n "$CANDIDATE"
test "$PLAN_ID" = "$(jq -er '.plan_id' "$WORK/plan.json")"
test "$CANDIDATE" = "$(jq -er '.candidate_revision' "$WORK/plan.json")"
curl -fsS "$BASE/readyz" | jq -e '.status == "ready"'
```

## 2. 保存发布前的运行事实

```sh
RUNTIME_BEFORE="$(curl -fsS \
  "$BASE/api/core/v1alpha1/modules/$MODULE/openapi.json" \
  | jq -er '."x-panvara-revision"')"
test "$RUNTIME_BEFORE" = "$BASELINE"

admin_curl -fsS \
  "$BASE/api/admin/v1alpha1/$MODULE/organization?limit=100" \
  -o "$WORK/publish-records-before.json"
jq -S '.data' "$WORK/publish-records-before.json" \
  > "$WORK/publish-records-before.normalized.json"

admin_curl -fsS \
  "$BASE/api/admin/core/v1alpha1/modules/$MODULE/revisions?limit=100" \
  -o "$WORK/publish-revisions-before.json"
jq -e --arg candidate "$CANDIDATE" \
  '[.data[] | select(.revision == $candidate)] | length == 0' \
  "$WORK/publish-revisions-before.json"
```

最后一条输出 `true`，证明 Candidate 在 Publish 前尚未登记。

## 3. 首次 Publish

成功与错误响应都应带 `Cache-Control: private, no-store`，避免浏览器或共享代理保存发布与 Credential provenance。

```sh
RELEASES_URL="$BASE/api/admin/core/v1alpha1/modules/$MODULE/releases"
PUBLISH_KEY="publish-$(openssl rand -hex 16)"

jq -n --arg plan_id "$PLAN_ID" \
  '{plan_id: $plan_id}' > "$WORK/publish-request.json"

STATUS="$(admin_curl -sS -X POST "$RELEASES_URL" \
  -H 'Content-Type: application/json' \
  -H "Idempotency-Key: $PUBLISH_KEY" \
  --data-binary @"$WORK/publish-request.json" \
  -D "$WORK/publish.headers" \
  -o "$WORK/publish.json" \
  -w '%{http_code}')"
test "$STATUS" = 201

RELEASE_ID="$(jq -er '.release_id' "$WORK/publish.json")"
LOCATION="$(awk 'tolower($1) == "location:" {
  gsub(/\r/, "", $2); print $2
}' "$WORK/publish.headers")"

test "$LOCATION" = "/api/admin/core/v1alpha1/modules/$MODULE/releases/$RELEASE_ID"
grep -qi '^cache-control: private, no-store' "$WORK/publish.headers"

jq -e \
  --arg plan "$PLAN_ID" \
  --arg plan_hash "$PLAN_HASH" \
  --arg candidate "$CANDIDATE" '
  (.release_id | test("^[0-9a-f]{8}-[0-9a-f]{4}-7[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$")) and
  .module == "crm.leads" and
  .plan_id == $plan and
  .plan_hash == $plan_hash and
  .candidate_revision == $candidate and
  .draft_generation == 2 and
  (.validation_id | test("^sha256:[0-9a-f]{64}$")) and
  (.source_hash | test("^sha256:[0-9a-f]{64}$")) and
  (.data_schema_identity.format == 1) and
  (.data_schema_identity.fingerprint | test("^sha256:[0-9a-f]{64}$")) and
  (.outcome | IN("compatible", "review_required", "migration_required")) and
  (.risk | IN("none", "low", "medium", "high")) and
  (.published_by | type == "string" and length > 0) and
  (.published_credential_id | test("^[0-9a-f-]{36}$")) and
  (.request_id | type == "string" and length > 0) and
  (.published_at | type == "string" and length > 0) and
  .effects.revision_registered == true and
  .effects.published == true and
  .effects.activated == false and
  .effects.records_migrated == false and
  .effects.runtime_changed == false and
  .effects.activation_supported == false
' "$WORK/publish.json"

jq '{release_id,plan_id,candidate_revision,outcome,risk,effects}' \
  "$WORK/publish.json"
```

输出 `true` 后，你只证明了发布事实已经写入。不要根据 `published=true` 修改流量或删除旧模型。

## 4. 验证网络重放与同 Plan 新 Key

先使用同一个 Key 和同一个 Body 重放：

```sh
STATUS="$(admin_curl -sS -X POST "$RELEASES_URL" \
  -H 'Content-Type: application/json' \
  -H "Idempotency-Key: $PUBLISH_KEY" \
  --data-binary @"$WORK/publish-request.json" \
  -o "$WORK/publish-replay.json" \
  -w '%{http_code}')"
test "$STATUS" = 200

jq -e -s '
  .[0].release_id == .[1].release_id and
  .[0].plan_id == .[1].plan_id and
  .[0].published_at == .[1].published_at and
  .[0].request_id == .[1].request_id
' "$WORK/publish.json" "$WORK/publish-replay.json"
```

再给同一个 Plan 使用一个新 Key：

```sh
PUBLISH_ALIAS_KEY="publish-$(openssl rand -hex 16)"
STATUS="$(admin_curl -sS -X POST "$RELEASES_URL" \
  -H 'Content-Type: application/json' \
  -H "Idempotency-Key: $PUBLISH_ALIAS_KEY" \
  --data-binary @"$WORK/publish-request.json" \
  -o "$WORK/publish-alias.json" \
  -w '%{http_code}')"
test "$STATUS" = 200

jq -e -s '
  .[0].release_id == .[1].release_id and
  .[0].candidate_revision == .[1].candidate_revision and
  .[0].published_at == .[1].published_at
' "$WORK/publish.json" "$WORK/publish-alias.json"
```

两次都输出 `true` 表示没有重复发布。新 Key 已绑定到同一个 Release。

### Draft 改变后，已发布事实仍可重放

发布成功后继续编辑 Draft 会让原 Plan 对新的首次发布失效，但不能让已经提交的 Release 失去幂等入口。先把 Draft 推进一代：

```sh
{
  printf '# post-publish Draft change\n'
  cat examples/drafts/crm-leads-valid.yaml
} > "$WORK/post-publish-draft.yaml"

STATUS="$(admin_curl -sS -X PUT "$SOURCE_URL" \
  -H 'Content-Type: application/yaml; charset=utf-8' \
  -H "If-Match: $DRAFT_ETAG_TWO" \
  --data-binary @"$WORK/post-publish-draft.yaml" \
  -o "$WORK/post-publish-draft.json" \
  -w '%{http_code}')"
test "$STATUS" = 200
test "$(jq -er '.draft_version' "$WORK/post-publish-draft.json")" = 3
```

现在原 Plan 已 stale，但原 Key/原 Plan 与新 Key/相同已发布 Plan 都必须先解析到原 Release：

```sh
STATUS="$(admin_curl -sS -X POST "$RELEASES_URL" \
  -H 'Content-Type: application/json' \
  -H "Idempotency-Key: $PUBLISH_KEY" \
  --data-binary @"$WORK/publish-request.json" \
  -o "$WORK/publish-after-stale.json" \
  -w '%{http_code}')"
test "$STATUS" = 200

PUBLISH_STALE_ALIAS_KEY="publish-stale-$(openssl rand -hex 16)"
STATUS="$(admin_curl -sS -X POST "$RELEASES_URL" \
  -H 'Content-Type: application/json' \
  -H "Idempotency-Key: $PUBLISH_STALE_ALIAS_KEY" \
  --data-binary @"$WORK/publish-request.json" \
  -o "$WORK/publish-stale-alias.json" \
  -w '%{http_code}')"
test "$STATUS" = 200

jq -e -s '
  .[0].release_id == .[1].release_id and
  .[0].release_id == .[2].release_id and
  .[0].published_at == .[1].published_at and
  .[0].published_at == .[2].published_at
' "$WORK/publish.json" \
  "$WORK/publish-after-stale.json" \
  "$WORK/publish-stale-alias.json"
```

同一个已绑定 Key 改成另一个意图时，即使 Plan 不存在，也必须优先返回幂等冲突：

```sh
MISSING_PLAN="sha256:$(printf '%064d' 0)"
jq -n --arg plan_id "$MISSING_PLAN" \
  '{plan_id: $plan_id}' > "$WORK/publish-conflict-request.json"

STATUS="$(admin_curl -sS -X POST "$RELEASES_URL" \
  -H 'Content-Type: application/json' \
  -H "Idempotency-Key: $PUBLISH_KEY" \
  --data-binary @"$WORK/publish-conflict-request.json" \
  -o "$WORK/publish-conflict.json" \
  -w '%{http_code}')"
test "$STATUS" = 409
jq -e '.code == "idempotency_key_conflict"' \
  "$WORK/publish-conflict.json"
```

## 5. GET Release 并核对 Registry

```sh
RELEASE_URL="$BASE$LOCATION"
STATUS="$(admin_curl -sS "$RELEASE_URL" \
  -D "$WORK/release-get.headers" \
  -o "$WORK/release-get.json" \
  -w '%{http_code}')"
test "$STATUS" = 200
grep -qi '^cache-control: private, no-store' "$WORK/release-get.headers"

jq -e -s '
  .[0].release_id == .[1].release_id and
  .[0].draft_id == .[1].draft_id and
  .[0].validation_id == .[1].validation_id and
  .[0].plan_id == .[1].plan_id and
  .[0].candidate_revision == .[1].candidate_revision and
  .[0].effects == .[1].effects
' "$WORK/publish.json" "$WORK/release-get.json"

admin_curl -fsS \
  "$BASE/api/admin/core/v1alpha1/modules/$MODULE/revisions/$CANDIDATE" \
  -o "$WORK/candidate-revision.json"

jq -e --arg candidate "$CANDIDATE" '
  .revision == $candidate and
  (.data_schema_identities[] | select(.format == 1) |
    .fingerprint == input.data_schema_identity.fingerprint)
' "$WORK/candidate-revision.json" "$WORK/publish.json"
```

Candidate 现在可以从 Registry 读取。Registry 的 `origin` 可能是 `publish`，也可能在同一 Revision 早已由启动流程登记时保持 `bootstrap`；两者都不代表 active。

## 6. 证明 Runtime 与 Record 没有变化

```sh
RUNTIME_AFTER="$(curl -fsS \
  "$BASE/api/core/v1alpha1/modules/$MODULE/openapi.json" \
  | jq -er '."x-panvara-revision"')"
test "$RUNTIME_AFTER" = "$RUNTIME_BEFORE"
test "$RUNTIME_AFTER" != "$CANDIDATE"

admin_curl -fsS \
  "$BASE/api/admin/v1alpha1/$MODULE/organization?limit=100" \
  -o "$WORK/publish-records-after.json"
jq -S '.data' "$WORK/publish-records-after.json" \
  > "$WORK/publish-records-after.normalized.json"
cmp "$WORK/publish-records-before.normalized.json" \
  "$WORK/publish-records-after.normalized.json"
```

`cmp` 没有输出表示业务数据未改变。

## 7. 重启后再验证

在终端 A 按 `Ctrl+C`，然后使用完全相同的 `.env`、`.env.local` 和启动 Source 重新运行：

```sh
make run-server
```

回到终端 B：

```sh
curl -fsS "$BASE/readyz" | jq -e '.status == "ready"'

admin_curl -fsS "$RELEASE_URL" -o "$WORK/release-after-restart.json"
jq -e -s '
  .[0].release_id == .[1].release_id and
  .[0].published_at == .[1].published_at and
  .[0].candidate_revision == .[1].candidate_revision
' "$WORK/publish.json" "$WORK/release-after-restart.json"

STATUS="$(admin_curl -sS -X POST "$RELEASES_URL" \
  -H 'Content-Type: application/json' \
  -H "Idempotency-Key: $PUBLISH_KEY" \
  --data-binary @"$WORK/publish-request.json" \
  -o "$WORK/publish-replay-after-restart.json" \
  -w '%{http_code}')"
test "$STATUS" = 200
jq -e -s '
  .[0].release_id == .[1].release_id and
  .[0].published_at == .[1].published_at
' "$WORK/publish.json" "$WORK/publish-replay-after-restart.json"

test "$(curl -fsS \
  "$BASE/api/core/v1alpha1/modules/$MODULE/openapi.json" \
  | jq -er '."x-panvara-revision"')" = "$RUNTIME_BEFORE"

admin_curl -fsS \
  "$BASE/api/admin/v1alpha1/$MODULE/organization?limit=100" \
  | jq -S '.data' > "$WORK/publish-records-restart.normalized.json"
cmp "$WORK/publish-records-before.normalized.json" \
  "$WORK/publish-records-restart.normalized.json"
```

## 8. 负例与收尾

无 Token、未知字段、重复字段和转义后的字段名必须被拒绝：

```sh
STATUS="$(curl -sS -X POST "$RELEASES_URL" \
  -H 'Content-Type: application/json' \
  -H 'Idempotency-Key: unauthenticated-publish' \
  --data-binary @"$WORK/publish-request.json" \
  -o "$WORK/publish-unauthorized.json" \
  -w '%{http_code}')"
test "$STATUS" = 401

STATUS="$(admin_curl -sS -X POST "$RELEASES_URL" \
  -H 'Content-Type: application/json' \
  -H 'Idempotency-Key: unknown-field-publish' \
  --data "{\"plan_id\":\"$PLAN_ID\",\"activate\":true}" \
  -o "$WORK/publish-unknown-field.json" \
  -w '%{http_code}')"
test "$STATUS" = 400

STATUS="$(admin_curl -sS -X POST "$RELEASES_URL" \
  -H 'Content-Type: application/json' \
  -H 'Idempotency-Key: duplicate-field-publish' \
  --data "{\"plan_id\":\"$PLAN_ID\",\"plan_id\":\"$PLAN_ID\"}" \
  -o "$WORK/publish-duplicate-field.json" \
  -w '%{http_code}')"
test "$STATUS" = 400

STATUS="$(admin_curl -sS -X POST "$RELEASES_URL" \
  -H 'Content-Type: application/json' \
  -H 'Idempotency-Key: escaped-field-publish' \
  --data "{\"\\u0070lan_id\":\"$PLAN_ID\"}" \
  -o "$WORK/publish-escaped-field.json" \
  -w '%{http_code}')"
test "$STATUS" = 400
```

全部通过后，终端 A 按 `Ctrl+C`；需要停止 PostgreSQL 时执行 `make infra-down`。

验收结论应写成：

> Candidate Revision 已登记，Module Release 已持久化且重放稳定；当前 Runtime Revision 与业务 Record 在发布前、发布后和重启后均未改变。P0-02a Publish Facts 通过，但 Candidate 尚未 Activated。

遇到失败时保留响应中的 `request_id` 并查看 Server 日志。401 重新加载环境但不要打印 Token；409 先重放原 Key/原 Plan；422 检查 Plan 的 `summary.classification`；503 等数据库恢复后用原 Key 重试。概念和边界见 [Module Release 发布事实](../modules/release-publishing.md)。
