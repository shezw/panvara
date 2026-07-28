<!--
    Panvara
    docs/guides model-change-preview.md    2026-07-28
     ______     __  __     ______     ______     __     __
    /\  ___\   /\ \_\ \   /\  ___\   /\___  \   /\ \  _ \ \
    \ \___  \  \ \  __ \  \ \  __\   \/_/  /__  \ \ \/ ".\ \
     \/\_____\  \ \_\ \_\  \ \_____\   /\_____\  \ \__/".~\_\
      \/_____/   \/_/\/_/   \/_____/   \/_/   \/_/.com

    @link    : https://github.com/shezw/panvara
    @author  : shezw
    @email   : hello@shezw.com
-->

# 模型变更预览

::: warning Source Preview
本页固定到源码提交 [`38afe3e91e5a54c1a677b0acbe3a6a2a75668839`](https://github.com/shezw/panvara/tree/38afe3e91e5a54c1a677b0acbe3a6a2a75668839)。它**不属于 v0.1.0-alpha.2 Distribution 能力边界，不适合生产**。当前只有 Revision Registry、Draft、Validation、Change Plan 与不可变 Publish Facts；缺少 Activate、Rollback、数据升级执行、活动版本指针、可视化 Manager 和生产发布控制面。
:::

本实验会形成控制面事实，但最后必须证明当前 Runtime 没有改变。`published=true` 不等于上线。

## 源码依据

- [Revision Registry 用例](https://github.com/shezw/panvara/blob/38afe3e91e5a54c1a677b0acbe3a6a2a75668839/internal/application/appmodule/revision_registry.go)
- [Draft 工作流](https://github.com/shezw/panvara/blob/38afe3e91e5a54c1a677b0acbe3a6a2a75668839/internal/application/appmodule/draft_workflow.go)
- [Release Publisher](https://github.com/shezw/panvara/blob/38afe3e91e5a54c1a677b0acbe3a6a2a75668839/internal/application/release/publisher.go)
- [Draft HTTP 路由](https://github.com/shezw/panvara/blob/38afe3e91e5a54c1a677b0acbe3a6a2a75668839/internal/interfaces/httpapi/draft_handler.go)
- [Release HTTP 路由](https://github.com/shezw/panvara/blob/38afe3e91e5a54c1a677b0acbe3a6a2a75668839/internal/interfaces/httpapi/release_handler.go)

## 准备固定源码

除 Server 所需工具外，还要安装 `jq` 1.6+，用于读取和断言 JSON 响应：

```sh
jq --version
```

只在可丢弃的本地环境准备固定源码：

```sh
git clone https://github.com/shezw/panvara.git panvara-model-preview
cd panvara-model-preview
git switch --detach 38afe3e91e5a54c1a677b0acbe3a6a2a75668839

make doctor-server
make local-init
set -a; . ./.env; . ./.env.local; set +a
make infra-up
make run-server
```

保持 Server 运行。第二个终端进入同一目录：

```sh
set -eu
set +x
umask 077
set -a; . ./.env; . ./.env.local; set +a

BASE=http://127.0.0.1:8080
MODULE=crm.leads
WORK="$(mktemp -d "${TMPDIR:-/tmp}/panvara-model-preview.XXXXXX")"
chmod 700 "$WORK"
trap 'rm -rf "$WORK"' EXIT HUP INT TERM

admin_curl() {
  printf 'Authorization: Bearer %s\n' "$PANVARA_ADMIN_TOKEN" |
    curl --header @- "$@"
}

curl -fsS "$BASE/readyz" | jq -e '.status == "ready"'
```

不要启用 Shell trace、打印 Token 或保存带认证 Header 的调试日志。

## 1. 从 Registry 核对当前 Baseline

当前运行 Revision 只能从 OpenAPI 读取：

```sh
BASELINE="$(curl -fsS \
  "$BASE/api/core/v1alpha1/modules/$MODULE/openapi.json" \
  | jq -er '."x-panvara-revision"
    | select(test("^sha256:[0-9a-f]{64}$"))')"

admin_curl -fsS \
  "$BASE/api/admin/core/v1alpha1/modules/$MODULE/revisions/$BASELINE" \
  | jq -e --arg baseline "$BASELINE" \
      '.module == "crm.leads" and .revision == $baseline'
```

不要从 Registry List 的第一项推断当前版本；Registry 没有 active 语义。

## 2. 创建并验证一个无效 Draft

```sh
CREATE_URL="$BASE/api/admin/core/v1alpha1/modules/$MODULE/drafts?baseline_revision=$BASELINE"
CREATE_KEY="preview-draft-$(openssl rand -hex 16)"

STATUS="$(admin_curl -sS -X POST "$CREATE_URL" \
  -H 'Content-Type: application/yaml; charset=utf-8' \
  -H "Idempotency-Key: $CREATE_KEY" \
  --data-binary @examples/drafts/crm-leads-invalid.yaml \
  -D "$WORK/draft-create.headers" \
  -o "$WORK/draft-create.json" \
  -w '%{http_code}')"
test "$STATUS" = 201

DRAFT_ID="$(jq -er '.draft_id' "$WORK/draft-create.json")"
DRAFT_ETAG_ONE="$(awk 'tolower($1) == "etag:" {
  gsub(/\r/, "", $2); print $2
}' "$WORK/draft-create.headers")"

DRAFT_URL="$BASE/api/admin/core/v1alpha1/modules/$MODULE/drafts/$DRAFT_ID"
SOURCE_URL="$DRAFT_URL/source"
VALIDATIONS_URL="$DRAFT_URL/validations"

STATUS="$(admin_curl -sS -X POST "$VALIDATIONS_URL" \
  -H "If-Match: $DRAFT_ETAG_ONE" \
  -o "$WORK/validation-invalid.json" \
  -w '%{http_code}')"
test "$STATUS" = 201

jq -e '
  .valid == false and
  (.violations | length > 0) and
  .effects.revision_registered == false and
  .effects.published == false and
  .effects.activated == false and
  .effects.records_migrated == false and
  .effects.runtime_changed == false
' "$WORK/validation-invalid.json"
```

Validation 请求成功，但候选 Source 无效；这是 2xx + `valid=false`，不是 HTTP 失败。

## 3. 替换为有效 Source 并再次验证

```sh
STATUS="$(admin_curl -sS -X PUT "$SOURCE_URL" \
  -H 'Content-Type: application/yaml; charset=utf-8' \
  -H "If-Match: $DRAFT_ETAG_ONE" \
  --data-binary @examples/drafts/crm-leads-valid.yaml \
  -D "$WORK/draft-replace.headers" \
  -o "$WORK/draft-replace.json" \
  -w '%{http_code}')"
test "$STATUS" = 200

DRAFT_ETAG_TWO="$(awk 'tolower($1) == "etag:" {
  gsub(/\r/, "", $2); print $2
}' "$WORK/draft-replace.headers")"

STATUS="$(admin_curl -sS -X POST "$VALIDATIONS_URL" \
  -H "If-Match: $DRAFT_ETAG_TWO" \
  -o "$WORK/validation-valid.json" \
  -w '%{http_code}')"
test "$STATUS" = 201

VALIDATION_ID="$(jq -er '.validation_id' "$WORK/validation-valid.json")"
CANDIDATE="$(jq -er '.candidate_revision' "$WORK/validation-valid.json")"

jq -e '
  .valid == true and
  .violations == [] and
  .effects.revision_registered == false and
  .effects.runtime_changed == false
' "$WORK/validation-valid.json"
```

Candidate Hash 已计算，但此时仍未登记为发布事实。

## 4. 生成 Change Plan

```sh
jq -n --arg validation_id "$VALIDATION_ID" \
  '{validation_id: $validation_id}' >"$WORK/plan-request.json"

STATUS="$(admin_curl -sS -X POST "$DRAFT_URL/plans" \
  -H 'Content-Type: application/json' \
  -H "If-Match: $DRAFT_ETAG_TWO" \
  --data-binary @"$WORK/plan-request.json" \
  -o "$WORK/plan.json" \
  -w '%{http_code}')"
test "$STATUS" = 201

PLAN_ID="$(jq -er '.plan_id' "$WORK/plan.json")"

jq -e '
  (.summary.classification |
    IN("compatible", "review_required", "migration_required", "unsupported")) and
  .migration_execution_supported == false and
  .effects.plan_recorded == true and
  .effects.revision_registered == false and
  .effects.published == false and
  .effects.activated == false and
  .effects.records_migrated == false and
  .effects.runtime_changed == false
' "$WORK/plan.json"
```

Plan 解释变化，不执行变化；即使分类看起来兼容，也不能据此上线。

## 5. 写入不可变 Publish Facts

发布前保存当前 Runtime：

```sh
RUNTIME_BEFORE="$(curl -fsS \
  "$BASE/api/core/v1alpha1/modules/$MODULE/openapi.json" \
  | jq -er '."x-panvara-revision"')"
test "$RUNTIME_BEFORE" = "$BASELINE"
```

发布精确 Plan：

```sh
jq -n --arg plan_id "$PLAN_ID" \
  '{plan_id: $plan_id}' >"$WORK/publish-request.json"

PUBLISH_KEY="preview-publish-$(openssl rand -hex 16)"
RELEASES_URL="$BASE/api/admin/core/v1alpha1/modules/$MODULE/releases"

STATUS="$(admin_curl -sS -X POST "$RELEASES_URL" \
  -H 'Content-Type: application/json' \
  -H "Idempotency-Key: $PUBLISH_KEY" \
  --data-binary @"$WORK/publish-request.json" \
  -o "$WORK/publish.json" \
  -w '%{http_code}')"
test "$STATUS" = 201

jq -e --arg candidate "$CANDIDATE" '
  .candidate_revision == $candidate and
  .effects.revision_registered == true and
  .effects.published == true and
  .effects.activated == false and
  .effects.records_migrated == false and
  .effects.runtime_changed == false and
  .effects.activation_supported == false
' "$WORK/publish.json"
```

## 6. 证明没有激活或切换 Runtime

```sh
admin_curl -fsS \
  "$BASE/api/admin/core/v1alpha1/modules/$MODULE/revisions/$CANDIDATE" \
  | jq -e --arg candidate "$CANDIDATE" '.revision == $candidate'

RUNTIME_AFTER="$(curl -fsS \
  "$BASE/api/core/v1alpha1/modules/$MODULE/openapi.json" \
  | jq -er '."x-panvara-revision"')"

test "$RUNTIME_AFTER" = "$RUNTIME_BEFORE"
test "$RUNTIME_AFTER" != "$CANDIDATE"
```

两条 `test` 都成功才是本页的最终通过条件：Candidate 已成为不可变登记/发布事实，但当前 Runtime 仍使用原 Baseline。

## 这项预览没有什么？

- 没有 Activate、Rollback、流量切换或活动版本指针；
- 没有复制、转换或校验旧 Record 的数据升级执行器；
- 没有可视化 Draft、Plan 或发布界面；
- 没有后台 Worker、Outbox 或多节点收敛；
- 没有生产发布审批、备份恢复或稳定兼容承诺。

实验结束后停止 Server，并执行 `make infra-down`。不要删除原 AppModule Source；恢复旧 Runtime 仍依赖精确的原 Source。
