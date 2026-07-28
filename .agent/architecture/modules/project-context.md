<!--
    Panvara
    docs/modules/project-context.md    2026-07-18
     ______     __  __     ______     ______     __     __
    /\  ___\   /\ \_\ \   /\  ___\   /\___  \   /\ \  _ \ \
    \ \___  \  \ \  __ \  \ \  __\   \/_/  /__  \ \ \/ ".\ \
     \/\_____\  \ \_\ \_\  \ \_____\   /\_____\  \ \__/".~\_\
      \/_____/   \/_/\/_/   \/_____/   \/_____/   \/_/   \/_/.com

    @link    : https://github.com/shezw/panvara
    @author  : shezw
    @email   : hello@shezw.com
-->

# Project Context 使用指南

## 用途

Project Context 表示“一次业务运行属于哪个项目，以及这个项目默认使用什么语言、时区和币种”。Panvara 把它作为显式边界，并在 P0-01a 中将定义持久化；P0-01b 的 Credential 与 Grant 管理继续精确绑定该 Project/默认 Environment。

当前仍默认一个 Server 进程只服务一个 Project。

## 当前状态

alpha.2 的值对象与 P0-01a/P0-01b 可运行切片当前已实现：

- UUIDv7 格式的稳定 Project ID，以及持久化的 Project Key、Locale、Time Zone、Currency 和状态。
- 某个 Project ID 首次由 Server 装配时生成一个 UUIDv7 默认 Environment，并持久化固定 `bootstrap-admin` Principal 与 `project.owner` Grant。
- Locale、IANA Time Zone 和三字母 Currency 默认值。
- Record 按 Project ID 隔离。
- 匿名 Public Actor 与 project-local bootstrap/Service Principal 都绑定到同一 Project；active Credential 只认证 Principal，持久化 Grant 才决定 Admin 授权。
- 首启 Token 的 digest/hint 与永久 marker 绑定默认 Environment；Service Principal、Credential 与 Owner Grant 由[访问管理 API](access-administration.md)管理。

启动配置只定义某个 Project ID 的第一组 bootstrap 事实。该 Project 已存在后，相同配置重启会复用持久化 Project 与 Environment；其 Project Key、Locale、Time Zone、Currency 或 Environment Key 漂移时 Server 拒绝启动。P0-01b marker 存在后可省略 Token；相同 Token 可核对，不同值拒绝。新的 Project ID 配合新的全局唯一 Project Key 会创建另一套隔离事实，不会修改或迁移旧 Project。当前没有 Project/Environment 管理 API，也没有按域名/Header 动态切换项目。完整授权边界见[执行作用域与访问内核指南](project-access.md)和[访问管理指南](access-administration.md)，设计理由见 [ADR-0004](../adr/0004-persistent-execution-scope-access-kernel.md)与 [ADR-0005](../adr/0005-project-local-access-administration.md)。

## 前置条件

- 使用 Server Profile；Lite 不读取 Project Context。
- 准备一个稳定的 UUIDv7 作为 Project ID。
- 选择项目默认 Locale、IANA Time Zone 和 Currency。
- 为默认 Environment 选择 Key；未配置 `PANVARA_ENVIRONMENT_KEY` 时使用 `default`。
- 保持 Project ID 与已有数据库数据对应，不要随意更换。

仓库 `.env.example` 已包含一组可用于本地验收的值。

## 最小示例

```bash
export PANVARA_PROJECT_ID=01981234-5678-7abc-8def-0123456789ab
export PANVARA_PROJECT_KEY=crm
export PANVARA_PROJECT_LOCALE=zh-CN
export PANVARA_PROJECT_TIME_ZONE=Asia/Shanghai
export PANVARA_PROJECT_CURRENCY=CNY
export PANVARA_ENVIRONMENT_KEY=default
```

与其他 Server 配置一起启动：

```bash
make run-server
```

该 Project ID 首次启动会在一个事务中持久化 Project、生成默认 Environment，并创建 `bootstrap-admin` Principal 与 Owner Grant；如果这些值有效，Server 会继续登记模块并启动。后续以同一 Project ID 重启必须使用相同 Project/Environment 定义；配置无效或与持久化事实冲突都会在启动阶段明确失败。

## 配置

| 配置 | 示例或默认 | 规则 | 首次持久化后的行为 |
| --- | --- | --- | --- |
| `PANVARA_PROJECT_ID` | `0198...` | 必须是 UUIDv7；应长期稳定 | 用于查找持久化 Project，不应作为“修改项目”的手段 |
| `PANVARA_PROJECT_KEY` | `crm` | 小写字母/数字开头，可含 `.`、`_`、`-`，最长 64 字符 | 漂移时拒绝启动 |
| `PANVARA_PROJECT_LOCALE` | `zh-CN` | 校验 BCP 47 形状，最长 128 字符 | 漂移时拒绝启动 |
| `PANVARA_PROJECT_TIME_ZONE` | `Asia/Shanghai` | 必须是有效 IANA 名称或 `UTC`；拒绝 `Local` | 漂移时拒绝启动 |
| `PANVARA_PROJECT_CURRENCY` | `CNY` | 三个 ASCII 字母，保存时大写 | 漂移时拒绝启动 |
| `PANVARA_ENVIRONMENT_KEY` | `default` | 与 Project Key 相同的字符规则；缺省为 `default` | 默认 Environment Key 漂移时拒绝启动 |

Project ID 仍是现有 Record、Revision 和 Draft 的数据隔离身份，Project Key 只是方便人阅读的名称。Environment 已进入执行作用域，但既有事实表还没有 `environment_id`，因此当前只允许 active 的默认 Environment，不能把它描述成多 Environment 数据隔离。

时间原则：

- Record 时间点保存并返回 UTC。
- Project Time Zone 用于未来的显示和业务日历计算。
- 不使用宿主机 `Local`，避免换机器后含义变化。

金额原则：

- 业务金额使用 `minor` 最小单位与 `currency`。
- 不使用浮点数保存金额。
- Project Currency 是默认值，不会替代 Record 中显式的 money currency。

## 验收

1. 使用示例 Project Context 与 `PANVARA_ENVIRONMENT_KEY=default` 首次启动 Server，`/readyz` 返回 200。
2. 按[执行作用域与访问内核指南](project-access.md#最小示例)查询数据库，应看到一个 active Project、一个生成 UUIDv7 的 active 默认 Environment、`bootstrap-admin` Principal 和未撤销的 `project.owner` Grant。
3. 创建一条 Record，停止并使用完全相同的配置重启；Environment ID 不变，原 Record 仍可读取。
4. 临时把 `PANVARA_PROJECT_LOCALE` 改为 `en-US`，Server 应因持久化 Project 配置冲突拒绝启动；恢复原值后可再次启动。
5. 临时把 `PANVARA_ENVIRONMENT_KEY` 改为 `production`，Server 应因默认 Environment Key 冲突拒绝启动；恢复 `default` 后可再次启动。
6. 在可丢弃数据库中按[访问管理验收](access-administration.md#验收)使用 API 撤销 Owner Grant，同一 Credential 的 Admin 请求应返回 403；重启 Server 后仍为 403。另一个 Owner 可显式 PUT 重新授予，bootstrap 不会自动补回。
7. 将 Time Zone 临时设为 `Local`，Server 应拒绝启动；使用 `cny` 时会规范化为 `CNY`，使用非三字母值时启动失败。

第 4–6 步会故意触发启动失败或撤权，只能在本地验收数据库操作，并应按指南恢复原配置或 Grant。

## 常见问题

### 怎样生成 UUIDv7？

当前 CLI 尚未提供 Project 初始化命令。可以先使用 `.env.example` 的本地验收 ID；正式项目应由可信初始化工具生成并安全保存。Environment ID 在该 Project ID 首次由 Server 装配时生成，不需要手工配置。

### Project Key 和 Project ID 有什么区别？

ID 是稳定的数据边界，不应改变；Key 是给人和路由阅读的名称。P0-01a 尚无 Project 修改流程，因此不能通过改 `.env` 演进 Key；这样做会被当作配置漂移并拒绝启动。

### 修改 Locale 会迁移数据吗？

不会。首次持久化后直接修改启动配置会拒绝启动，也不会迁移或翻译已有内容。正式修改流程尚未实现。

### 为什么不能用 `Local` 时区？

`Local` 的含义取决于运行机器。Panvara 要求明确的 IANA 名称，让开发机、容器和服务器得到一致结果。

### P0-01a 是完整多租户或 IAM 系统吗？

不是。P0-01b 只增加 project-local Service Principal/API Credential 与固定 Owner Grant；每个进程仍固定一个项目，既有事实表仍只按 Project 隔离。没有 Account、ExternalIdentity、Session、ProjectMembership、动态 Role/Policy、RecordOwner 或多 Environment 数据隔离。

## 当前限制

- 一个 Server 进程只支持一个 Project。
- 没有 Project 或 Environment 创建、查询、修改、删除、切换 API。
- 没有域名、Token 或请求 Header 到 Project 的动态路由。
- Locale 只做结构校验，尚未接入完整 BCP 47 Parser。
- Currency 只校验三字母形状，尚未接入可版本化 ISO 4217 Catalog 和 minor-unit exponent。
- Project Time Zone 尚未自动应用到 Record API 的 UTC 时间输出。
- Bootstrap/Service Principal 与 API Credential 只覆盖机器身份；没有完整角色、Account、ExternalIdentity、Session 或组织模型。
- 只有默认 Environment 可以执行现有业务用例；Record、Revision 与 Draft 尚未按 Environment 隔离。

## 兼容与升级

Migration `0004_project_environment_access.sql` 追加 Project、Environment、Principal 与 Grant 表；`0005_project_access_administration.sql` 增加 Credential、永久 marker 和最小安全审计。旧数据库升级后，当前 Project 下一次启动用原 bootstrap Token 原子初始化 marker；之后可省略 Token。后续启动不会用配置覆盖持久化设置，也不会补回已撤销 Grant；Grant 只能由另一个 Owner 显式 PUT 重新授予。

Project ID 是持久化身份。升级、重启或迁移机器时都应保留同一个 ID、Project 设置和 Environment Key。更改这些启动值不是数据升级方式；配置漂移会拒绝启动。现有业务数据被解释为属于该 Project 的默认 Environment，但在完成 `environment_id` 回填、约束与兼容测试前并不具备真实的多 Environment 隔离。

未来支持多项目路由时，Project Context 与 Environment Scope 仍应作为每个 Use Case 的显式输入。任何新增字段都需要保持旧项目默认值可解释，并同步更新初始化指南和升级测试。
