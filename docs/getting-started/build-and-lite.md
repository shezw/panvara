<!--
    Panvara
    docs/getting-started/build-and-lite.md    2026-07-14
     ______     __  __     ______     ______     __     __
    /\  ___\   /\ \_\ \   /\  ___\   /\___  \   /\ \  _ \ \
    \ \___  \  \ \  __ \  \ \  __\   \/_/  /__  \ \ \/ ".\ \
     \/\_____\  \ \_\ \_\  \ \_____\   /\_____\  \ \__/".~\_\
      \/_____/   \/_/\/_/   \/_____/   \/_____/   \/_/   \/_/.com

    @link    : https://github.com/shezw/panvara
    @author  : shezw
    @email   : hello@shezw.com
-->

# 编译并运行 Lite

Lite 是最小运行方式，不读取模型，也不连接数据库。先用它确认 Go 工具链和 Panvara 核心工作正常。

## 1. 编译

在 Panvara 仓库根目录执行：

```sh
make build
./bin/panvara --version
```

编译结果保存在 `bin/panvara`。版本输出是 JSON，并应包含：

```json
{"distribution":"0.1.0-alpha.2"}
```

实际输出还会包含 Core API、模型协议、IR、Commit 和 Go 版本等信息。

## 2. 运行快速自检

```sh
make verify
```

该命令会检查格式、静态问题、单元测试、竞态和构建。它不启动 Docker，也不验证真实 PostgreSQL。第一次执行可能需要下载 Go 依赖。

最终没有错误并回到命令提示符，就表示通过。

## 3. 启动 Lite

在终端 A 执行：

```sh
make run
```

预期看到：

```text
panvara 0.1.0-alpha.2 profile=lite address=127.0.0.1:8080
```

保持终端 A 运行。在另一个终端 B 执行：

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

## 4. 停止 Lite

回到终端 A，按 `Ctrl+C`。这只停止 Panvara，不会修改或删除任何数据。

如果端口被占用或请求失败，前往[故障排查](./troubleshooting)。下一步是[CRM Leads 完整验收](./crm-leads-acceptance)。
