<!--
    Panvara
    docs/guides access-preview.md    2026-07-28
     ______     __  __     ______     ______     __     __
    /\  ___\   /\ \_\ \   /\  ___\   /\___  \   /\ \  _ \ \
    \ \___  \  \ \  __ \  \ \  __\   \/_/  /__  \ \ \/ ".\ \
     \/\_____\  \ \_\ \_\  \ \_____\   /\_____\  \ \__/".~\_\
      \/_____/   \/_/\/_/   \/_____/   \/_/   \/_/.com

    @link    : https://github.com/shezw/panvara
    @author  : shezw
    @email   : hello@shezw.com
-->

# 访问管理预览

::: warning Source Preview
本页固定到源码提交 [`38afe3e91e5a54c1a677b0acbe3a6a2a75668839`](https://github.com/shezw/panvara/tree/38afe3e91e5a54c1a677b0acbe3a6a2a75668839)。它**不属于 v0.1.0-alpha.2 Distribution 能力边界，不适合生产**。当前只有 project-local Service Principal、一次性 API Credential 与固定 Owner Grant；缺少账号登录、External Identity、Session、项目成员、动态角色/策略、Record Owner、MFA、限流和完整多环境隔离。
:::

## 源码依据

- [Application 访问管理用例](https://github.com/shezw/panvara/blob/38afe3e91e5a54c1a677b0acbe3a6a2a75668839/internal/application/access/administration.go)
- [HTTP 访问管理路由](https://github.com/shezw/panvara/blob/38afe3e91e5a54c1a677b0acbe3a6a2a75668839/internal/interfaces/httpapi/access_handler.go)
- [PostgreSQL 访问管理存储](https://github.com/shezw/panvara/blob/38afe3e91e5a54c1a677b0acbe3a6a2a75668839/internal/infrastructure/postgres/access_admin_store.go)

这些链接是本页契约的固定证据；后续源码可能变化，不能把本页命令套用到任意分支。

## 准备固定源码

只在可丢弃的本地环境操作。除 Server 所需工具外，还要安装 `jq` 1.6+，用于读取和断言 JSON 响应：

```sh
jq --version
```

然后准备固定源码：

```sh
git clone https://github.com/shezw/panvara.git panvara-access-preview
cd panvara-access-preview
git switch --detach 38afe3e91e5a54c1a677b0acbe3a6a2a75668839

make doctor-server
make local-init
set -a; . ./.env; . ./.env.local; set +a
make infra-up
make run-server
```

保持 Server 运行。在第二个终端进入同一目录，加载配置：

```sh
set -eu
set +x
umask 077
set -a; . ./.env; . ./.env.local; set +a
BASE=http://127.0.0.1:8080

curl -fsS "$BASE/readyz" | jq -e '.status == "ready"'
```

不要启用 `set -x`，不要使用 `curl -v`，不要打印 Token。

## 1. 创建 Service Principal

```sh
SERVICE_PRINCIPAL_ID="$(curl -fsS -X POST \
  "$BASE/api/admin/core/v1alpha1/access/principals" \
  -H "Authorization: Bearer $PANVARA_ADMIN_TOKEN" \
  -H 'Content-Type: application/json' \
  --data '{"display_name":"local preview"}' \
  | jq -er '.id')"

printf 'principal=%s\n' "$SERVICE_PRINCIPAL_ID"
```

成功返回 `201`。Principal ID 不是 Secret，可以记录；Credential Token 不可以。

## 2. 一次性接收 Credential

签发成功的响应只返回一次原始 Token。用权限为 0600 的临时文件接收，并只输出脱敏 metadata：

```sh
ISSUE_RESPONSE="$(mktemp)"
chmod 600 "$ISSUE_RESPONSE"

curl -fsS -X POST \
  "$BASE/api/admin/core/v1alpha1/access/principals/$SERVICE_PRINCIPAL_ID/credentials" \
  -H "Authorization: Bearer $PANVARA_ADMIN_TOKEN" \
  -H 'Content-Type: application/json' \
  --data '{"label":"local preview"}' \
  >"$ISSUE_RESPONSE"

SERVICE_TOKEN="$(jq -er '.token' "$ISSUE_RESPONSE")"
SERVICE_CREDENTIAL_ID="$(jq -er '.credential.id' "$ISSUE_RESPONSE")"
jq '{credential}' "$ISSUE_RESPONSE"
rm -f "$ISSUE_RESPONSE"
unset ISSUE_RESPONSE
```

不要把签发响应保存到 CI 日志、Shell 历史、Issue 或聊天中。数据库只应保存摘要与 hint，而不是原始 Token。

## 3. 授予并验证 Owner Grant

```sh
curl -fsS -X PUT \
  "$BASE/api/admin/core/v1alpha1/access/principals/$SERVICE_PRINCIPAL_ID/grants/project.owner" \
  -H "Authorization: Bearer $PANVARA_ADMIN_TOKEN" \
  | jq -e '.active == true'

curl -fsS \
  "$BASE/api/admin/core/v1alpha1/access/principals" \
  -H "Authorization: Bearer $SERVICE_TOKEN" \
  | jq -e --arg id "$SERVICE_PRINCIPAL_ID" \
      '.data | any(.id == $id)'
```

两条命令均输出 `true`，表示新 Credential 完成认证，且固定 Owner Grant 完成授权。

## 4. 显式撤销 Credential

使用仍有效的 bootstrap Owner 撤销 Service Credential：

```sh
curl -fsS -X POST \
  "$BASE/api/admin/core/v1alpha1/access/credentials/$SERVICE_CREDENTIAL_ID/revoke" \
  -H "Authorization: Bearer $PANVARA_ADMIN_TOKEN" \
  | jq -e '.status == "revoked"'

STATUS="$(curl -sS -o /dev/null -w '%{http_code}' \
  "$BASE/api/admin/core/v1alpha1/access/principals" \
  -H "Authorization: Bearer $SERVICE_TOKEN")"
test "$STATUS" = 401

unset SERVICE_TOKEN PANVARA_ADMIN_TOKEN
```

撤销是终态，重启不会恢复该 Credential。安全轮换顺序始终是 `issue → verify → explicit revoke`。

## 状态码与安全边界

| 状态 | 含义 |
| --- | --- |
| 401 | Credential 缺失、错误、已撤销，或 Principal 已停用 |
| 403 | Credential 已认证，但当前 Project/Environment 缺少 active Owner Grant |
| 409 | 生命周期冲突，例如试图移除最后一条可用 Owner 路径 |
| 503 | 权威认证或授权存储不可用；系统会 fail closed |

所有访问管理响应都应使用 `Cache-Control: private, no-store`；签发响应还应使用 `Pragma: no-cache`。

## 这项预览没有什么？

- 没有用户注册、登录、密码、MFA 或 Session；
- 没有 Google、Apple、微信等外部身份接入；
- 没有 Project Membership、动态 Role/Policy 或 Record Owner；
- 没有面向公网的 TLS、限流与生产凭据管理；
- 没有完整多环境业务数据隔离；
- 没有“改环境变量即可恢复权限”的后门。

完成实验后在 Server 终端按 `Ctrl+C`，再执行 `make infra-down`。普通停止会保留本地数据卷。
