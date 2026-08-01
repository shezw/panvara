<!--
    Panvara
    docs/getting-started index.md    2026-07-28
     ______     __  __     ______     ______     __     __
    /\  ___\   /\ \_\ \   /\  ___\   /\___  \   /\ \  _ \ \
    \ \___  \  \ \  __ \  \ \  __\   \/_/  /__  \ \ \/ ".\ \
     \/\_____\  \ \_\ \_\  \ \_____\   /\_____\  \ \__/".~\_\
      \/_____/   \/_/\/_/   \/_____/   \/_/   \/_/.com

    @link    : https://github.com/shezw/panvara
    @author  : shezw
    @email   : hello@shezw.com
-->

# 10 分钟启动 Lite

::: info Current Distribution
本页使用当前二进制内建 Distribution 标识 **v0.1.0-alpha.2** 对应的 Lite。`/version` 的 `distribution` 字段实际返回不带前缀的 `0.1.0-alpha.2`。Lite 不读取 AppModule、不连接数据库，也不提供业务 API；它只用于确认 Panvara 可以在你的电脑上编译和运行。
:::

已安装工具且网络可用时，这条路径通常可在 10 分钟内完成。第一次下载 Go 依赖所需时间取决于网络速度。

## 准备工具

支持 macOS、Linux，以及 Windows 11 的 WSL2。需要：

- Git；
- Go 1.25 或更高版本，推荐项目当前工具链版本；
- Make；
- curl。

先确认工具可用：

```sh
git --version
go version
make --version
curl --version
```

## 1. 下载 Panvara

```sh
git clone https://github.com/shezw/panvara.git
cd panvara
git switch --detach cfea044bfee90b7b8d62de79e42d2503258d7781
make doctor
```

成功时最后一行是：

```text
Environment check passed.
```

`make doctor` 只检查环境，不安装软件，也不修改系统设置。

当前尚无与 v0.1.0-alpha.2 对应的 GitHub Release。这里固定源码提交，是为了让 `doctor`、构建与 Lite 命令可复现。同一快照还包含隔离标注的 Source Preview；二进制版本字段不能用于推断这些预览能力已成熟或进入 Distribution。

## 2. 编译并核对版本

```sh
make build
./bin/panvara --version
```

`bin/panvara` 是编译结果。版本输出是 JSON，其中应包含：

```json
{"distribution":"0.1.0-alpha.2"}
```

实际响应还会带 Core、模型协议、提交和 Go 版本信息。

## 3. 启动 Lite

在终端 A 执行：

```sh
make run
```

保持这个终端运行。日志应包含：

```text
panvara 0.1.0-alpha.2 profile=lite address=127.0.0.1:8080
```

在终端 B 执行：

```sh
curl -fsS http://127.0.0.1:8080/healthz
curl -fsS http://127.0.0.1:8080/readyz
curl -fsS http://127.0.0.1:8080/version
```

前两条预期分别返回：

```json
{"status":"alive"}
{"status":"ready"}
```

第三条的 `distribution` 应为 `0.1.0-alpha.2`。

## 4. 停止 Lite

回到终端 A，按 `Ctrl+C`。Lite 没有连接数据库，因此这一步不会删除业务数据。

## 下一步

- 想加载 AppModule 并保存真实数据：继续[运行 PostgreSQL Server](/guides/server)。
- 想先理解模型、API 与存储的关系：阅读[认识 Panvara](/guides/concepts)。
- 端口被占用或命令失败：查看[故障排查](/reference/troubleshooting)。

> Lite 成功只证明本机工具链和基础 HTTP 进程可用，不代表 Server、PostgreSQL 或业务 API 已经验证。
