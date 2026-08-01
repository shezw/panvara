<!--
    Panvara
    docs/guides model-change-preview.md    2026-08-02
     ______     __  __     ______     ______     __     __
    /\  ___\   /\ \_\ \   /\  ___\   /\___  \   /\ \  _ \ \
    \ \___  \  \ \  __ \  \ \  __\   \/_/  /__  \ \ \/ ".\ \
     \/\_____\  \ \_\ \_\  \ \_____\   /\_____\  \ \__/".~\_\
      \/_____/   \/_/\/_/   \/_____/   \/_____/   \/_/   \/_/.com

    @link    : https://github.com/shezw/panvara
    @author  : shezw
    @email   : hello@shezw.com
-->

# 发布并激活模型版本

::: warning Source Preview
本页固定到源码提交 [`cfea044bfee90b7b8d62de79e42d2503258d7781`](https://github.com/shezw/panvara/tree/cfea044bfee90b7b8d62de79e42d2503258d7781)。它**不属于 v0.1.0-alpha.2 Distribution 能力边界，不适合生产**。

当前只支持单 Server 显式激活“数据结构完全未变化”的 compatible Release。需要数据转换、人工复核、回滚或多个 Server 自动同步的变更仍不可用。
:::

本指南会完成一条可验收路径：创建候选版本、预览变化、发布、确认 Publish 不会自动上线、显式 Activate、确认已有 Record 仍可见，最后重启 Server 验证活动版本恢复。

## 准备可丢弃环境

需要 Git、Go、Make、Docker、curl、OpenSSL 和 `jq` 1.6+。请使用新的本地数据库或新的 Project；如果之前运行过本预览，先按[彻底重置本地数据库](/reference/troubleshooting#彻底重置本地数据库)清理可丢弃环境。

```sh
git clone https://github.com/shezw/panvara.git panvara-model-preview
cd panvara-model-preview
git switch --detach cfea044bfee90b7b8d62de79e42d2503258d7781

make doctor-server
make local-init
set -a; . ./.env; . ./.env.local; set +a
make infra-up
make run-server
```

保持 Server 运行。第二个终端进入同一目录，准备请求环境：

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

不要启用 Shell trace，也不要打印 Token 或保存认证 Header。

## 1. 记录当前活动版本和一条业务数据

```sh
ACTIVE_URL="$BASE/api/admin/core/v1alpha1/modules/$MODULE/active"

admin_curl -fsS "$ACTIVE_URL" >"$WORK/active-before.json"

BASELINE="$(jq -er '.runtime_revision' "$WORK/active-before.json")"
NAMESPACE_BEFORE="$(jq -er '.record_namespace_revision' "$WORK/active-before.json")"
EPOCH_BEFORE="$(jq -er '.epoch' "$WORK/active-before.json")"

OPENAPI_BEFORE="$(curl -fsS \
  "$BASE/api/core/v1alpha1/modules/$MODULE/openapi.json" \
  | jq -er '."x-panvara-revision"')"
test "$OPENAPI_BEFORE" = "$BASELINE"

DEMO_SUFFIX="$(date +%s)-$$"
RECORD_NAME="Panvara Activate ${DEMO_SUFFIX}"
RECORD_URL="$BASE/api/admin/v1alpha1/$MODULE/organization"

STATUS="$(admin_curl -sS -X POST "$RECORD_URL" \
  -H 'Content-Type: application/json' \
  --data "{\"name\":\"$RECORD_NAME\"}" \
  -o "$WORK/record.json" \
  -w '%{http_code}')"
test "$STATUS" = 201

RECORD_ID="$(jq -er '.id' "$WORK/record.json")"
```

这里记录的 `record_namespace_revision` 是已有 Record 的存储标识。兼容激活后，它必须保持不变。

## 2. 创建并验证候选版本

本次只把模块业务版本从 `1.0.0` 改为 `1.0.1`，不改变字段、约束或资源：

```sh
sed 's/^  version: 1\.0\.0$/  version: 1.0.1/' \
  examples/modules/crm-leads.yaml \
  >"$WORK/crm-leads-compatible.yaml"
grep -q '^  version: 1.0.1$' "$WORK/crm-leads-compatible.yaml"

CREATE_URL="$BASE/api/admin/core/v1alpha1/modules/$MODULE/drafts?baseline_revision=$BASELINE"
CREATE_KEY="preview-draft-$(openssl rand -hex 16)"

STATUS="$(admin_curl -sS -X POST "$CREATE_URL" \
  -H 'Content-Type: application/yaml; charset=utf-8' \
  -H "Idempotency-Key: $CREATE_KEY" \
  --data-binary @"$WORK/crm-leads-compatible.yaml" \
  -D "$WORK/draft.headers" \
  -o "$WORK/draft.json" \
  -w '%{http_code}')"
test "$STATUS" = 201

DRAFT_ID="$(jq -er '.draft_id' "$WORK/draft.json")"
DRAFT_ETAG="$(awk 'tolower($1) == "etag:" {
  gsub(/\r/, "", $2); print $2
}' "$WORK/draft.headers")"
DRAFT_URL="$BASE/api/admin/core/v1alpha1/modules/$MODULE/drafts/$DRAFT_ID"

STATUS="$(admin_curl -sS -X POST "$DRAFT_URL/validations" \
  -H "If-Match: $DRAFT_ETAG" \
  -o "$WORK/validation.json" \
  -w '%{http_code}')"
test "$STATUS" = 201

VALIDATION_ID="$(jq -er '.validation_id' "$WORK/validation.json")"
CANDIDATE="$(jq -er '.candidate_revision' "$WORK/validation.json")"
jq -e '.valid == true and .violations == []' "$WORK/validation.json"
```

## 3. 预览变化并发布

```sh
jq -n --arg validation_id "$VALIDATION_ID" \
  '{validation_id: $validation_id}' >"$WORK/plan-request.json"

STATUS="$(admin_curl -sS -X POST "$DRAFT_URL/plans" \
  -H 'Content-Type: application/json' \
  -H "If-Match: $DRAFT_ETAG" \
  --data-binary @"$WORK/plan-request.json" \
  -o "$WORK/plan.json" \
  -w '%{http_code}')"
test "$STATUS" = 201

PLAN_ID="$(jq -er '.plan_id' "$WORK/plan.json")"
jq -e '
  .summary.classification == "compatible" and
  .data_schema_changed == false and
  .migration_execution_supported == false
' "$WORK/plan.json"

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

RELEASE_ID="$(jq -er '.release_id' "$WORK/publish.json")"
jq -e --arg candidate "$CANDIDATE" '
  .candidate_revision == $candidate and
  .outcome == "compatible" and
  .effects.published == true and
  .effects.activated == false and
  .effects.records_migrated == false and
  .effects.runtime_changed == false
' "$WORK/publish.json"

OPENAPI_AFTER_PUBLISH="$(curl -fsS \
  "$BASE/api/core/v1alpha1/modules/$MODULE/openapi.json" \
  | jq -er '."x-panvara-revision"')"
test "$OPENAPI_AFTER_PUBLISH" = "$BASELINE"
```

最后一条 `test` 证明 Publish 只登记 Release，不会自动上线。不要使用 Publish 响应中的 `activation_supported` 判断能否激活；Activate 会根据当前活动版本重新做安全检查。

## 4. 显式激活 Release

Activate 不接受 Body、Query 或 `Idempotency-Key`：

```sh
ACTIVATE_URL="$RELEASES_URL/$RELEASE_ID/activate"

STATUS="$(admin_curl -sS -X POST "$ACTIVATE_URL" \
  -D "$WORK/activate.headers" \
  -o "$WORK/activate.json" \
  -w '%{http_code}')"
test "$STATUS" = 201

jq -e \
  --arg release "$RELEASE_ID" \
  --arg candidate "$CANDIDATE" \
  --arg namespace "$NAMESPACE_BEFORE" \
  --argjson before "$EPOCH_BEFORE" '
  .release_id == $release and
  .runtime_revision == $candidate and
  .record_namespace_revision == $namespace and
  .epoch == ($before + 1) and
  .origin == "release"
' "$WORK/activate.json"

LOCATION="$(awk 'tolower($1) == "location:" {
  gsub(/\r/, "", $2); print $2
}' "$WORK/activate.headers")"
test "$LOCATION" = "/api/admin/core/v1alpha1/modules/$MODULE/active"

STATUS="$(admin_curl -sS -X POST "$ACTIVATE_URL" \
  -o "$WORK/activate-replay.json" \
  -w '%{http_code}')"
test "$STATUS" = 200
test "$(jq -er '.epoch' "$WORK/activate-replay.json")" = "$((EPOCH_BEFORE + 1))"
```

首次激活返回 `201`；对当前同一 Release 重放返回 `200`，不会再次增加 epoch。

## 5. 验证新模型、旧 Record 与重启恢复

```sh
admin_curl -fsS "$ACTIVE_URL" >"$WORK/active-after.json"

jq -e \
  --arg candidate "$CANDIDATE" \
  --arg namespace "$NAMESPACE_BEFORE" \
  --argjson before "$EPOCH_BEFORE" '
  .runtime_revision == $candidate and
  .record_namespace_revision == $namespace and
  .epoch == ($before + 1)
' "$WORK/active-after.json"

OPENAPI_AFTER_ACTIVATE="$(curl -fsS \
  "$BASE/api/core/v1alpha1/modules/$MODULE/openapi.json" \
  | jq -er '."x-panvara-revision"')"
test "$OPENAPI_AFTER_ACTIVATE" = "$CANDIDATE"

admin_curl -fsS "$RECORD_URL/$RECORD_ID" \
  | jq -e --arg name "$RECORD_NAME" '.data.name == $name'
```

三项检查分别证明：活动 Runtime 已切换、Record 存储标识未变、激活前创建的数据仍然可见。

现在回到第一个终端，按 `Ctrl+C` 停止 Server，再执行：

```sh
make run-server
```

第二个终端重新检查：

```sh
curl -fsS "$BASE/readyz" | jq -e '.status == "ready"'

admin_curl -fsS "$ACTIVE_URL" \
  | jq -e --arg candidate "$CANDIDATE" \
      --argjson before "$EPOCH_BEFORE" \
      '.runtime_revision == $candidate and .epoch == ($before + 1)'

admin_curl -fsS "$RECORD_URL/$RECORD_ID" \
  | jq -e --arg name "$RECORD_NAME" '.data.name == $name'
```

全部返回真值，才表示活动版本与原数据都已跨重启恢复。

## 失败时怎么处理

| 结果 | 含义 | 处理方式 |
| --- | --- | --- |
| `409 activation_conflict` | Baseline 已不是当前版本、发生并发切换，或目标是历史 Release | 重新读取 `/active`，从新的 Baseline 创建 Draft |
| `422 not_activatable` | 数据结构发生变化，或 Plan 需要复核/迁移 | 不要绕过；改用仅改变版本元数据的候选，或等待迁移流程 |
| `503 release_unavailable` | 数据库、发布事实或本地 Runtime 状态不确定 | 停止该 Server，重启后先读取 `/active` 再决定是否重试 |

## 当前边界

- 没有 Rollback API，也没有数据转换、回填或迁移执行器；
- 没有多个 Server 之间的主动通知、确认或自动收敛；
- 没有可视化发布界面、审批流或生产流量控制；
- 激活后，替换本地 Source 或重启 Server**不会回退**活动版本；
- 需要恢复时，只能使用激活前已经验证过的数据库备份，并接受备份之后写入可能丢失。

实验结束后停止 Server，再执行 `make infra-down`。该命令保留本地数据库卷。
