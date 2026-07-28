<!--
    Panvara
    docs/getting-started/local-environment.md    2026-07-18
     ______     __  __     ______     ______     __     __
    /\  ___\   /\ \_\ \   /\  ___\   /\___  \   /\ \  _ \ \
    \ \___  \  \ \  __ \  \ \  __\   \/_/  /__  \ \ \/ ".\ \
     \/\_____\  \ \_\ \_\  \ \_____\   /\_____\  \ \__/".~\_\
      \/_____/   \/_/\/_/   \/_____/   \/_____/   \/_/   \/_/.com

    @link    : https://github.com/shezw/panvara
    @author  : shezw
    @email   : hello@shezw.com
-->

# 创建本地环境

所有命令都在终端执行。代码建议放在你平时保存项目的目录，不要放到系统目录。

## 1. 下载当前开发版

当前 alpha.3b 开发切片尚未合并到默认分支。要验收本套文档中的 Registry、Draft、Validation 与 Change Plan，请使用对应验收分支：

```sh
git clone --branch codex/alpha3b-draft-plan --single-branch https://github.com/shezw/panvara.git
cd panvara
```

alpha.3b 合并到 `main` 后，才改用普通克隆：

```sh
git clone https://github.com/shezw/panvara.git
cd panvara
```

确认当前目录正确：

```sh
git status --short --branch
make doctor-server
```

## 2. 生成本地配置

```sh
make local-init
```

首次执行会创建两个不会提交到 Git 的文件：

- `.env`：数据库地址、模型文件、Project 设置和默认 Environment Key 等普通开发配置。
- `.env.local`：首次 marker 初始化使用的随机 bootstrap Token，权限限制为当前用户读取。

再次执行不会覆盖已有配置或 Token。每个新终端都需要加载它们：

```sh
set -a
. ./.env
. ./.env.local
set +a
```

这几行只把配置载入当前终端。Panvara 不会自动读取 `.env`，也不会把原始 Token 写入日志或数据库；数据库保存 SHA-256 digest、hint 和永久 marker。

新生成的 `.env` 包含 `PANVARA_ENVIRONMENT_KEY=default`。如果 `.env` 来自较早版本，可以手工补上这一行；不补时 Server 的内置默认值也是 `default`。当前 Project ID 第一次由 Server 装配时会持久化 Project、生成 UUIDv7 默认 Environment，并创建 `bootstrap-admin` Principal 与 `project.owner` Grant。后续以同一 Project ID 启动时，其 Project/Environment 设置必须与数据库一致，漂移会拒绝启动；新的 Project ID 与唯一 Key 会创建另一套隔离事实，不会迁移旧数据。

marker 不存在时 Token 必须为 32–1024 字节，只含 HTTP Bearer 安全 ASCII（字母、数字、`-._~+/`，`=` 只能尾随），且不得以保留前缀 `pvk1.` 开头；`pvk1.` 只用于 Panvara 签发的 Service Credential。`make local-init` 生成的随机值符合要求。首次成功启动原子持久化 digest/hint、Credential 与永久 marker。后续启动可以省略 Token，仍提供时必须相同，改变会拒绝启动。Credential revoke 与 Principal disable 是终态；Grant 撤销后重启不会自动恢复，但另一个 Owner 可以显式 PUT 重新授予。

所有 Admin 用例统一由 `Credential → Principal → project.owner Grant` 授权。完整说明和不输出 Secret 的本地验收见[执行作用域与访问内核指南](../modules/project-access.md)和[Project-local 访问管理指南](../modules/access-administration.md)。P0-01b 仍不包含 Account/ExternalIdentity/Session/ProjectMembership、动态 Role/Policy、RecordOwner 或多 Environment 数据隔离。

::: danger 不要提交本地密钥
`.env` 和 `.env.local` 已被 `.gitignore` 排除。不要删除忽略规则，也不要把真实 Token 复制到模块 YAML、README、Issue 或聊天记录中。
:::

## 3. 启动 PostgreSQL

确保 Docker 已启动，然后执行：

```sh
make infra-up
docker compose -f deploy/compose/compose.yaml ps
```

成功时，`postgres` 服务的状态包含 `healthy`。数据库端口只绑定到本机 `127.0.0.1:5432`。

## 4. 停止环境

```sh
make infra-down
```

该命令停止并删除容器，但保留数据库数据卷，所以重新启动后数据仍在。这是默认且安全的行为。

彻底删除本地数据库属于危险操作，请只在明确不需要数据时参考[故障排查中的重置说明](./troubleshooting#彻底重置本地数据库)。

下一步：[编译并运行 Lite](./build-and-lite)。
