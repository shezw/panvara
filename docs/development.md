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
| Docker/Compose | 条件必需 | Lite/快速测试不需要；本地 Server 与必需集成测试需要，除非提供 PostgreSQL 18.4 URL |
| Make | 推荐 | 统一人和 CI 的入口，不隐藏实际 Go 命令 |
| IDE | 任意支持 gopls 的编辑器 | 保存时 gofmt，开启静态诊断 |

go.mod 同时声明最低 Go 1.25.0 和建议 toolchain go1.26.5。开发与主 CI 使用当前 patch；兼容 CI 使用 Go 1.25.12 并设置 GOTOOLCHAIN=local，防止自动切回新工具链。
Panvara 可执行文件嵌入 Go 的 IANA Time Zone Database，避免精简容器缺少系统 zoneinfo 时产生环境差异。

## 2. 两档环境

### Minimal

适合 Core、模型语义和接口开发：

- Go 1.26.5。
- Lite Profile。
- Core 生命周期与 health/ready/version 运维端点；不装配业务 Record API。
- `slog` 基础错误日志；OpenTelemetry Adapter 尚未实现。
- 无 PostgreSQL、Valkey、NATS、对象存储。

### Standard

适合 alpha.2 Server、持久化和 HTTP API 开发：

- Minimal 全部能力。
- PostgreSQL 18.4。
- `crm-leads` YAML/JSON 模块文件。
- 通过 `PANVARA_TEST_DATABASE_URL` 使用 PostgreSQL 18.4，或由集成测试启动一次性 Docker 容器；不复用手工 Compose 数据。
- Mailpit 或 Console Email Adapter 延后到 alpha.3。

Valkey、NATS、MinIO 和 OpenTelemetry Collector 只有在对应 Port/Adapter 进入实现后才加入 Full Profile，当前不制造“看起来完整”的空依赖。

## 3. 首次启动 Lite

    git clone https://github.com/shezw/panvara.git
    cd panvara
    make verify
    make run

另一个终端验证：

    curl http://127.0.0.1:8080/healthz
    curl http://127.0.0.1:8080/readyz
    curl http://127.0.0.1:8080/version

`make verify` 与 `make run` 不需要 Docker，也不会隐式启动 PostgreSQL。

## 4. 启动 alpha.2 Server

先启动本机 PostgreSQL，并显式导出非敏感示例配置：

    make infra-up
    set -a; . ./.env.example; set +a

再单独生成管理员 Token，并只通过环境变量传入：

    export PANVARA_ADMIN_TOKEN="$(openssl rand -hex 32)"
    make run-server

不要在示例、配置文件、模块 Source、Git 或 `--admin-token` 参数中保存真实 Token；命令行参数通常对同机进程可见。生产环境应由 Secret Manager 注入。alpha.2 要求 Token 至少 32 字节；`BootstrapAdminAuth` 验证器只保存 SHA-256 摘要，但环境变量与启动配置中的明文生命周期不作清除保证。

另一个终端验证：

    curl http://127.0.0.1:8080/readyz
    curl http://127.0.0.1:8080/api/core/v1alpha1/modules/crm.leads/openapi.json
    curl http://127.0.0.1:8080/api/core/v1alpha1/modules/crm.leads/ui-schema.json

停止 Server 后可关闭基础设施：

    make infra-down

Compose 端口只绑定 127.0.0.1，开发密码只用于本机；生产配置不得复用。
PostgreSQL 18 官方镜像把持久化根目录改为 /var/lib/postgresql，Compose 已按 18+ 规则挂载，不能沿用 17 及以下的 /var/lib/postgresql/data。

Panvara 不隐式加载 `.env`。需要覆盖默认值时，通过 Shell、IDE 或可信的环境管理器显式导出 `.env.example` 中的变量；该文件故意不给管理员 Token 设置可用默认值。
多个 clone/worktree 并行开发时，为 PANVARA_COMPOSE_PROJECT 和 PANVARA_POSTGRES_PORT 设置不同值，避免共用容器、数据卷或宿主端口。

## 5. 统一命令

| 命令 | 用途 |
| --- | --- |
| make fmt | 格式化 Go |
| make fmt-check | 检查未格式化文件，不修改工作区 |
| make test | 随机顺序运行单元和 seed corpus |
| make test-race | 开启 Race Detector |
| make test-integration | 强制运行 PostgreSQL 18.4 Store 集成测试；不可跳过 |
| make test-server-smoke | 强制运行 Server HTTP/持久化 smoke；不可跳过 |
| make test-e2e | 顺序运行以上两个 alpha.2 必需集成目标 |
| make vet | Go 静态检查 |
| make build | 生成 bin/panvara |
| make verify | Docker-free 快速 PR 门禁；不包含集成目标 |
| make run | 运行 Lite |
| make run-server | 从进程环境运行 Server；Token 不转换为 CLI 参数 |
| make infra-up/down | 管理本地 PostgreSQL |

单元测试命令始终使用 `./...`，避免新包因未加入手工列表而逃逸门禁。集成测试使用 `integration` build tag，保持 Go 1.25 兼容任务与默认快速回路不依赖 Docker。

## 6. 环境变量

| 变量 | 默认 | 用途 |
| --- | --- | --- |
| PANVARA_PROFILE | lite | 运行组合 |
| PANVARA_HTTP_ADDR | 127.0.0.1:8080 | HTTP 监听地址 |
| PANVARA_DATABASE_URL | 无 | Server 必需的 PostgreSQL URL |
| PANVARA_MODULE_SOURCE | 无 | Server 必需的 YAML/JSON Source；示例为 `examples/modules/crm-leads.yaml` |
| PANVARA_MODULE_FORMAT | auto | `auto`、`yaml` 或 `json` |
| PANVARA_PROJECT_ID | 无 | Server 必需的 UUIDv7 Project ID |
| PANVARA_PROJECT_KEY | default | 可读 Project Key |
| PANVARA_PROJECT_LOCALE | en-US | BCP 47 Locale |
| PANVARA_PROJECT_TIME_ZONE | UTC | IANA Time Zone |
| PANVARA_PROJECT_CURRENCY | USD | ISO 风格 Currency |
| PANVARA_ADMIN_TOKEN | 无 | Server 必需，至少 32 字节；只建议 Secret/环境变量注入 |
| PANVARA_TEST_DATABASE_URL | 无 | 测试专用 PostgreSQL 18.4 URL；设置后不启动 Docker 容器 |
| PANVARA_REQUIRE_DOCKER | 无 | `1` 时 Docker/数据库不可用必须失败；Make 集成目标和 CI 已设置 |

alpha.2 的实际配置优先级是：CLI > 环境变量 > 内置默认值，尚无配置文件加载器或 Secret Reference Runtime。普通参数可以使用 CLI；管理员 Token 应避免 CLI，只从 Secret Manager 注入的环境变量读取，不进入模型、仓库、日志或普通环境样例。

## 7. CI 与依赖策略

当前 GitHub Actions 包含：

- Go 1.26.5：fmt-check、vet、unit、race、build。
- PostgreSQL 18.4：独立 required Job 执行 `make test-e2e`；使用 Service URL，并设置 `PANVARA_REQUIRE_DOCKER=1`，测试不得 Skip。
- Go 1.25.12：vet、unit、build，且禁止工具链自动升级；不启动 Service、不执行 integration tag。

`make verify` 保持快速且 Docker-free，PostgreSQL 门禁由独立 Job 并行执行。这样个人开发者可以快速迭代，又不会让 PR 绕过真实数据库语义。

接入第三方依赖时遵守：

- 标准库足够时不增加包。
- 每个依赖必须对应明确 Port 或工程能力，并记录替代方案。
- Provider SDK 只能位于 Adapter，禁止进入 Domain/Application。
- 提交 go.mod 与 go.sum；Renovate/Dependabot 只提出小步升级。
- Go module/build cache 以 `go.sum` 为键；不缓存数据库数据目录。

## 8. Full 环境的触发条件

以下需求出现后再扩展 Compose：

- NATS JetStream：PostgreSQL Outbox 已无法满足多消费者、回放或吞吐要求。
- Valkey：真实压测证明缓存或分布式限流有收益。
- MinIO：对象存储 Adapter 进入验收。
- OpenTelemetry Collector：跨进程 trace 需要端到端验证。

生产部署建议最终提供容器镜像和 Helm/Kustomize 示例，但 Core v0.1 的开发闭环不依赖 Kubernetes。
