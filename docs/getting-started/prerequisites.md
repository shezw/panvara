<!--
    Panvara
    docs/getting-started/prerequisites.md    2026-07-15
     ______     __  __     ______     ______     __     __
    /\  ___\   /\ \_\ \   /\  ___\   /\___  \   /\ \  _ \ \
    \ \___  \  \ \  __ \  \ \  __\   \/_/  /__  \ \ \/ ".\ \
     \/\_____\  \ \_\ \_\  \ \_____\   /\_____\  \ \__/".~\_\
      \/_____/   \/_/\/_/   \/_____/   \/_____/   \/_/   \/_/.com

    @link    : https://github.com/shezw/panvara
    @author  : shezw
    @email   : hello@shezw.com
-->

# 安装开发工具

## 支持范围

这条入门路径面向 macOS、Linux 和 Windows 11 的 WSL2 环境。原生 PowerShell 命令尚未形成经过验证的完整路径，Windows 用户建议在 WSL2 的 Ubuntu 终端内操作。

## 必需工具

| 工具 | Lite | Server | 建议版本 | 用途 |
| --- | :---: | :---: | --- | --- |
| Git | ✓ | ✓ | 当前稳定版 | 下载源码 |
| Go | ✓ | ✓ | 1.26.5；最低 1.25 | 编译 Panvara |
| Make | ✓ | ✓ | 系统当前版 | 提供统一命令 |
| curl | ✓ | ✓ | 系统当前版 | 验证 HTTP API |
| jq |  | Registry 验收 | 1.6+ | 阅读 JSON 响应与执行验收断言 |
| Docker + Compose |  | ✓ | Docker Compose v2 | 运行 PostgreSQL 18.4 |
| OpenSSL |  | ✓ | 系统当前版 | 生成本地管理员 Token |
| Node.js |  |  | 22+，仅文档维护者 | 预览和构建文档站 |

请优先使用官方安装包或说明：[Go 下载](https://go.dev/dl/)、[Git 下载](https://git-scm.com/downloads)、[Docker Desktop](https://docs.docker.com/desktop/)、[WSL 安装](https://learn.microsoft.com/windows/wsl/install)。

macOS 可以先执行 `xcode-select --install` 获得 Git 和 Make。Ubuntu/WSL2 可以安装基础命令：

```sh
sudo apt update
sudo apt install -y git make curl jq openssl build-essential
```

`build-essential` 提供 Race Detector 在 Linux 上可能需要的 C 编译器。Go 与 Docker 建议继续按各自官方说明安装，以免系统软件源提供过旧版本。

## 手工确认

在终端逐条执行：

```sh
git --version
go version
make --version
curl --version
jq --version
docker version
docker compose version
openssl version
```

每条命令都应该打印版本，而不是 `command not found`。执行 Server 验收前，请先打开 Docker Desktop，或启动 Linux Docker Engine。

## 让 Panvara 自动检查

下载仓库后，在仓库根目录执行：

```sh
make doctor
make doctor-server
```

- `make doctor` 只检查 Lite 所需工具。
- `make doctor-server` 还会确认 Docker daemon、Compose 和 OpenSSL。
- Revision Registry Guideline 还需要手工确认 `jq --version`；当前 doctor 不自动检查 jq。
- 检查不会安装软件，也不会修改你的电脑。

成功时最后一行是：

```text
Environment check passed.
```

如果出现 `[missing]`，先按该行提示安装或启动对应工具，再重新执行检查。
