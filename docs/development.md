<!--
    Panvara
    docs/development.md    2026-07-14
     ______     __  __     ______     ______     __     __
    /\  ___\   /\ \_\ \   /\  ___\   /\___  \   /\ \  _ \ \
    \ \___  \  \ \  __ \  \ \  __\   \/_/  /__  \ \ \/ ".\ \
     \/\_____\  \ \_\ \_\  \ \_____\   /\_____\  \ \__/".~\_\
      \/_____/   \/_/\/_/   \/_____/   \/_____/   \/_/   \/_/.com

    @link    : https://github.com/shezw/panvara
    @author  : shezw
    @email   : hello@shezw.com
-->

# 开发环境建议

## 1. 基线选择

推荐“本机运行 Go，容器只运行基础设施”。这样调试、增量编译和测试最快，同时避免开发者本机安装多个数据库和消息组件。

| 项目 | 基线 | 说明 |
| --- | --- | --- |
| Go 开发工具链 | 1.26.5 | .go-version 和 toolchain 指令固定 |
| Go 最低语言/模块版本 | 1.25.0 | 用前一主版本做兼容门禁 |
| PostgreSQL | 18.4 | Compose 固定 patch 版本；生产支持矩阵在接入数据库时再冻结 |
| Docker/Compose | 可选 | Lite 不需要；Server 及以上用于本地依赖 |
| Make | 推荐 | 统一人和 CI 的入口，不隐藏实际 Go 命令 |
| IDE | 任意支持 gopls 的编辑器 | 保存时 gofmt，开启静态诊断 |

go.mod 同时声明最低 Go 1.25.0 和建议 toolchain go1.26.5。开发与主 CI 使用当前 patch；兼容 CI 使用 Go 1.25.12 并设置 GOTOOLCHAIN=local，防止自动切回新工具链。
Panvara 可执行文件嵌入 Go 的 IANA Time Zone Database，避免精简容器缺少系统 zoneinfo 时产生环境差异。

## 2. 两档环境

### Minimal

适合 Core、模型语义和接口开发：

- Go 1.26.5。
- Lite Profile。
- 内存 Registry。
- slog；OpenTelemetry 默认 no-op。
- 无 PostgreSQL、Valkey、NATS、对象存储。

### Standard

适合 alpha.2 之后的持久化与 Manager 开发：

- Minimal 全部能力。
- PostgreSQL 18.4。
- Mailpit 或 Console Email Adapter，进入 alpha.3 时增加。
- Testcontainers 只由集成测试临时创建依赖，不复用手工 Compose 数据。

Valkey、NATS、MinIO 和 OpenTelemetry Collector 只有在对应 Port/Adapter 进入实现后才加入 Full Profile，当前不制造“看起来完整”的空依赖。

## 3. 首次启动

    git clone https://github.com/shezw/panvara.git
    cd panvara
    make verify
    make run

另一个终端验证：

    curl http://127.0.0.1:8080/healthz
    curl http://127.0.0.1:8080/readyz
    curl http://127.0.0.1:8080/version

需要数据库时：

    make infra-up
    docker compose -f deploy/compose/compose.yaml ps
    make infra-down

Compose 端口只绑定 127.0.0.1，开发密码只用于本机；生产配置不得复用。
PostgreSQL 18 官方镜像把持久化根目录改为 /var/lib/postgresql，Compose 已按 18+ 规则挂载，不能沿用 17 及以下的 /var/lib/postgresql/data。

Panvara 不隐式加载 .env。需要覆盖默认值时，通过 Shell、IDE 或可信的环境管理器显式导出 .env.example 中的变量；这避免本地行为与容器/生产环境不一致。
多个 clone/worktree 并行开发时，为 PANVARA_COMPOSE_PROJECT 和 PANVARA_POSTGRES_PORT 设置不同值，避免共用容器、数据卷或宿主端口。

## 4. 统一命令

| 命令 | 用途 |
| --- | --- |
| make fmt | 格式化 Go |
| make fmt-check | 检查未格式化文件，不修改工作区 |
| make test | 随机顺序运行单元和 seed corpus |
| make test-race | 开启 Race Detector |
| make vet | Go 静态检查 |
| make build | 生成 bin/panvara |
| make verify | 本地完整 PR 门禁 |
| make run | 运行 Lite |
| make infra-up/down | 管理本地 PostgreSQL |

测试命令始终使用 ./...，避免新包因未加入手工列表而逃逸门禁。

## 5. 环境变量

| 变量 | 默认 | 用途 |
| --- | --- | --- |
| PANVARA_PROFILE | lite | 运行组合 |
| PANVARA_HTTP_ADDR | 127.0.0.1:8080 | HTTP 监听地址 |
| PANVARA_DATABASE_URL | 本机开发 URL | alpha.2 接入 PostgreSQL |
| PANVARA_LOG_LEVEL | info | 后续结构化日志过滤 |

配置优先级建议固定为：CLI > 环境变量 > 配置文件 > 安全默认值。密钥只通过 secret reference 注入，不进入模型、仓库、日志或普通环境样例。

## 6. CI 与依赖策略

当前 GitHub Actions 包含：

- Go 1.26.5：fmt-check、vet、unit、race、build。
- Go 1.25.12：vet、unit、build，且禁止工具链自动升级。

接入第三方依赖时遵守：

- 标准库足够时不增加包。
- 每个依赖必须对应明确 Port 或工程能力，并记录替代方案。
- Provider SDK 只能位于 Adapter，禁止进入 Domain/Application。
- 提交 go.mod 与 go.sum；Renovate/Dependabot 只提出小步升级。
- CI 缓存在出现 go.sum 后再开启，避免无意义配置。

## 7. Full 环境的触发条件

以下需求出现后再扩展 Compose：

- NATS JetStream：PostgreSQL Outbox 已无法满足多消费者、回放或吞吐要求。
- Valkey：真实压测证明缓存或分布式限流有收益。
- MinIO：对象存储 Adapter 进入验收。
- OpenTelemetry Collector：跨进程 trace 需要端到端验证。

生产部署建议最终提供容器镜像和 Helm/Kustomize 示例，但 Core v0.1 的开发闭环不依赖 Kubernetes。
