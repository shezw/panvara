<!--
    Panvara
    docs/modules/project-context.md    2026-07-14
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

Project Context 表示“一次业务运行属于哪个项目，以及这个项目默认使用什么语言、时区和币种”。Panvara 把它作为显式边界，避免不同项目的数据和全球化设置依赖进程全局变量。

alpha.2 默认一个 Server 进程只服务一个 Project。

## 当前状态

alpha.2 已实现：

- UUIDv7 格式的稳定 Project ID。
- 可读、可变的 Project Key。
- Locale、IANA Time Zone 和三字母 Currency 默认值。
- Record 按 Project ID 隔离。
- 匿名 Public Actor 和固定 `bootstrap-admin` Project Owner 都绑定到同一 Project。

Project Context 当前通过 Server 启动配置创建，没有 Project 管理 API，也没有按域名/Header 动态切换项目。

## 前置条件

- 使用 Server Profile；Lite 不读取 Project Context。
- 准备一个稳定的 UUIDv7 作为 Project ID。
- 选择项目默认 Locale、IANA Time Zone 和 Currency。
- 保持 Project ID 与已有数据库数据对应，不要随意更换。

仓库 `.env.example` 已包含一组可用于本地验收的值。

## 最小示例

```bash
export PANVARA_PROJECT_ID=01981234-5678-7abc-8def-0123456789ab
export PANVARA_PROJECT_KEY=crm
export PANVARA_PROJECT_LOCALE=zh-CN
export PANVARA_PROJECT_TIME_ZONE=Asia/Shanghai
export PANVARA_PROJECT_CURRENCY=CNY
```

与其他 Server 配置一起启动：

```bash
make run-server
```

如果这些值有效，Server 会继续编译模块、连接数据库并启动；任一值无效都会在启动阶段明确失败。

## 配置

| 配置 | 示例 | 规则 | 是否影响 Record 数据 Scope |
| --- | --- | --- | --- |
| `PANVARA_PROJECT_ID` | `0198...` | 必须是 UUIDv7；应长期稳定 | 是 |
| `PANVARA_PROJECT_KEY` | `crm` | 小写字母/数字开头，可含 `.`、`_`、`-`，最长 64 字符 | 否 |
| `PANVARA_PROJECT_LOCALE` | `zh-CN` | alpha.2 校验 BCP 47 形状，最长 128 字符 | 否 |
| `PANVARA_PROJECT_TIME_ZONE` | `Asia/Shanghai` | 必须是有效 IANA 名称或 `UTC`；拒绝 `Local` | 否 |
| `PANVARA_PROJECT_CURRENCY` | `CNY` | 三个 ASCII 字母，保存时大写 | 否 |

“不影响 Scope”不表示可以随意修改；这些值会影响未来显示、日历和金额默认行为。Project ID 是真正的数据隔离身份，Project Key 只是方便人阅读的名称。

时间原则：

- Record 时间点保存并返回 UTC。
- Project Time Zone 用于未来的显示和业务日历计算。
- 不使用宿主机 `Local`，避免换机器后含义变化。

金额原则：

- 业务金额使用 `minor` 最小单位与 `currency`。
- 不使用浮点数保存金额。
- Project Currency 是默认值，不会替代 Record 中显式的 money currency。

## 验收

1. 使用示例 Project Context 启动 Server，`/readyz` 返回 200。
2. 创建一条 Record，停止并重启相同配置后仍能读取。
3. 保持数据库和模块不变，但换成本地验收 ID `01981234-5678-7abc-9def-0123456789ab` 后，List 应返回独立数据集。
4. 切回原 Project ID 后，原记录再次可见。
5. 将 Time Zone 临时设为 `Local`，Server 应拒绝启动并提示使用 IANA 名称或 UTC。
6. 使用 `cny` 时 Server 应能启动（Core 会将其规范化为 `CNY`）；使用非三字母值时启动失败。alpha.2 暂无读取 Project Context 的 HTTP 接口。

第 3–4 步会创建新的数据隔离空间，只建议在本地验收数据库操作。

## 常见问题

### 怎样生成 UUIDv7？

alpha.2 CLI 尚未提供 Project 初始化命令。可以先使用 `.env.example` 的本地验收 ID；正式项目应由可信初始化工具生成并安全保存，后续版本会补齐初始化体验。

### Project Key 和 Project ID 有什么区别？

ID 是稳定的数据边界，不应改变；Key 是给人和路由阅读的名称，可以独立演进。

### 修改 Locale 会迁移数据吗？

不会。Locale 当前是上下文默认值，不参与 Record Scope，也不会翻译已有内容。

### 为什么不能用 `Local` 时区？

`Local` 的含义取决于运行机器。Panvara 要求明确的 IANA 名称，让开发机、容器和服务器得到一致结果。

### alpha.2 是完整多租户系统吗？

不是。它只证明 Project 边界能显式传递和隔离数据，每个进程仍固定一个项目。

## 当前限制

- 一个 Server 进程只支持一个 Project。
- 没有 Project 创建、查询、修改或删除 API。
- 没有域名、Token 或请求 Header 到 Project 的动态路由。
- Locale 只做结构校验，尚未接入完整 BCP 47 Parser。
- Currency 只校验三字母形状，尚未接入可版本化 ISO 4217 Catalog 和 minor-unit exponent。
- Project Time Zone 尚未自动应用到 Record API 的 UTC 时间输出。
- Bootstrap Admin 是临时单一 Owner，不是完整角色/组织模型。

## 兼容与升级

Project ID 是持久化身份。升级、重启、迁移机器或更换 Project Key 时都应保留同一个 ID；更改 ID 相当于切换到另一个项目的数据空间。

未来支持多项目路由时，Project Context 仍应作为每个 Use Case 的显式输入。任何新增字段都需要保持旧项目默认值可解释，并同步更新初始化指南和升级测试。
