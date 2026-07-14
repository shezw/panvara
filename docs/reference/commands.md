<!--
    Panvara
    docs/reference/commands.md    2026-07-14
     ______     __  __     ______     ______     __     __
    /\  ___\   /\ \_\ \   /\  ___\   /\___  \   /\ \  _ \ \
    \ \___  \  \ \  __ \  \ \  __\   \/_/  /__  \ \ \/ ".\ \
     \/\_____\  \ \_\ \_\  \ \_____\   /\_____\  \ \__/".~\_\
      \/_____/   \/_/\/_/   \/_____/   \/_____/   \/_/   \/_/.com

    @link    : https://github.com/shezw/panvara
    @author  : shezw
    @email   : hello@shezw.com
-->

# 命令参考

除非特别说明，所有 `make` 命令都要在 Panvara 仓库根目录执行。

## 入门与运行

| 命令 | 做什么 | 外部依赖 | 是否修改本地状态 |
| --- | --- | --- | --- |
| `make help` | 显示常用命令 | Make | 否 |
| `make doctor` | 检查 Lite 所需工具 | Git、Go、Make、curl | 否 |
| `make doctor-server` | 额外检查 Docker、Compose、OpenSSL | Lite 工具 + Docker + OpenSSL | 否 |
| `make local-init` | 创建 `.env` 与私密 `.env.local` | OpenSSL | 是，不覆盖已有文件 |
| `make build` | 编译 `bin/panvara` | Go | 是，写入 `bin/` |
| `make run` | 运行 Lite | Go | 启动本地进程 |
| `make run-server` | 从当前环境运行 Server | Go、PostgreSQL、已加载配置 | 启动本地进程 |
| `make infra-up` | 启动本地 PostgreSQL 18.4 | Docker Compose | 创建容器和数据卷 |
| `make infra-down` | 停止 PostgreSQL | Docker Compose | 删除容器，保留数据卷 |
| `make clean` | 删除编译结果 | Shell | 删除 `bin/` |

`make local-init` 后，每个新终端需要运行：

```sh
set -a; . ./.env; . ./.env.local; set +a
```

## 代码验证

| 命令 | 范围 | Docker |
| --- | --- | :---: |
| `make fmt` | 修改所有 Go 文件为 gofmt 格式 | 否 |
| `make fmt-check` | 只检查格式，不修改 | 否 |
| `make vet` | Go 静态检查 | 否 |
| `make test` | 单元测试和 Fuzz seed corpus | 否 |
| `make test-race` | Race Detector | 否 |
| `make verify` | fmt-check、vet、test、race、build | 否 |
| `make test-integration` | PostgreSQL Store 集成测试 | 是，或提供测试数据库 URL |
| `make test-server-smoke` | 真实 Server HTTP 与持久化链路 | 是，或提供测试数据库 URL |
| `make test-e2e` | 依次运行两个必需集成目标 | 是，或提供测试数据库 URL |

`make verify` 通过不代表数据库链路已经通过；涉及 Server、数据库或 HTTP 行为时必须执行 `make test-e2e`。

## 文档

| 命令 | 做什么 | 首次运行 |
| --- | --- | --- |
| `make docs-setup` | 按 lockfile 安装文档依赖 | 需要 Node.js 22+ 和网络 |
| `make docs-serve` | 在 `127.0.0.1:5173` 实时预览 | 先执行 `docs-setup` |
| `make docs-build` | 生成静态站点 | 输出到 `docs/.vitepress/dist` |
| `make docs-check` | 检查模块文档契约并构建站点 | CI 使用 |

文档依赖只服务文档维护，不会加入 Panvara Go 二进制。

## 直接运行 Panvara

查看版本和命令参数：

```sh
./bin/panvara --version
./bin/panvara -h
```

普通配置可以使用 CLI 参数，但管理员 Token 应只通过 `PANVARA_ADMIN_TOKEN` 环境变量注入，避免出现在同机进程列表和 Shell 历史中。
