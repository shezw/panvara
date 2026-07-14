<!--
    Panvara
    docs/getting-started/local-environment.md    2026-07-14
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

alpha.2 尚未合并到默认分支时，使用验收分支：

```sh
git clone --branch codex/alpha2-model-runtime --single-branch https://github.com/shezw/panvara.git
cd panvara
```

alpha.2 合并到 `main` 后，改用普通克隆：

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

- `.env`：数据库地址、模型文件、项目语言、时区和币种等普通开发配置。
- `.env.local`：随机生成的本地管理员 Token，权限限制为当前用户读取。

再次执行不会覆盖已有配置或 Token。每个新终端都需要加载它们：

```sh
set -a
. ./.env
. ./.env.local
set +a
```

这几行只把配置载入当前终端。Panvara 不会自动读取 `.env`，也不会把 Token 写入日志。

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
