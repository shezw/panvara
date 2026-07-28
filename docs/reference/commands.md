<!--
    Panvara
    docs/reference commands.md    2026-07-28
     ______     __  __     ______     ______     __     __
    /\  ___\   /\ \_\ \   /\  ___\   /\___  \   /\ \  _ \ \
    \ \___  \  \ \  __ \  \ \  __\   \/_/  /__  \ \ \/ ".\ \
     \/\_____\  \ \_\ \_\  \ \_____\   /\_____\  \ \__/".~\_\
      \/_____/   \/_/\/_/   \/_____/   \/_/   \/_/.com

    @link    : https://github.com/shezw/panvara
    @author  : shezw
    @email   : hello@shezw.com
-->

# 命令

除非特别说明，所有命令都在 Panvara 仓库根目录执行。

## 用户运行命令

| 命令 | 作用 | 依赖 | 本地影响 |
| --- | --- | --- | --- |
| `make help` | 显示仓库常用命令 | Make | 无 |
| `make doctor` | 检查 Lite 工具与 Go 版本 | Git、Go、Make、curl | 无 |
| `make doctor-server` | 追加检查 Docker、Compose 与 OpenSSL | Server 工具 | 无 |
| `make build` | 编译 `bin/panvara` | Go | 写入 `bin/` |
| `make run` | 启动 Lite | Go | 启动本地进程 |
| `make local-init` | 创建本地配置与 Secret | OpenSSL | 新建但不覆盖 `.env*` |
| `make infra-up` | 启动 PostgreSQL 18.4 | Docker Compose | 创建/启动容器与数据卷 |
| `make run-server` | 启动 PostgreSQL Server | 已加载配置和可用数据库 | 启动本地进程 |
| `make infra-down` | 停止 PostgreSQL | Docker Compose | 删除容器，保留数据卷 |

加载本地配置：

```sh
set -a; . ./.env; . ./.env.local; set +a
```

查看二进制版本与参数：

```sh
./bin/panvara --version
./bin/panvara -h
```

## Contributor 验证命令

| 命令 | 范围 | Docker |
| --- | --- | :---: |
| `make fmt` | 修改 Go 文件为 gofmt 格式 | 否 |
| `make fmt-check` | 只检查 Go 格式 | 否 |
| `make vet` | Go 静态检查 | 否 |
| `make test` | 单元测试与已登记 seed corpus | 否 |
| `make test-race` | Race Detector | 否 |
| `make verify` | fmt-check、vet、test、race、build | 否 |
| `make test-integration` | PostgreSQL 集成测试 | 是 |
| `make test-server-smoke` | 真实 Server HTTP 持久化冒烟 | 是 |
| `make test-e2e` | 运行两个数据库验证目标 | 是 |

`make verify` 不连接 PostgreSQL。修改 Server、数据库或 HTTP 行为时，还应执行 `make test-e2e`。

## 文档贡献命令

| 命令 | 作用 |
| --- | --- |
| `make docs-setup` | 使用 lockfile 安装 Node.js 文档依赖 |
| `make docs-serve` | 在 `127.0.0.1:5173` 本地预览 |
| `make docs-build` | 构建静态站点 |
| `make docs-check` | 检查公开文档约束并构建 |

需要 Node.js 22+ 与 npm 10+。文档依赖不会加入 Panvara Go 二进制。

## 数据影响提醒

- `make infra-down` 保留数据库卷；
- `make local-init` 不覆盖已有 Secret；
- `make fmt` 会修改 Go 源文件；
- 集成测试必须使用可丢弃数据库；
- 永久删除本地数据库只应按[故障排查](/reference/troubleshooting#彻底重置本地数据库)的双重警示操作。
