<!--
    Panvara
    docs/development.md    2026-07-18
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
| Node.js | 22+，仅文档 | VitePress 文档预览与构建；不进入 Go Runtime |
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
- P0-01a/P0-01b 持久化执行作用域、Credential 认证、访问管理与 Application Access Kernel；P0-02a 不可变 Publish Facts。这不等于完整 P0-01/P0-02、IAM 或多 Environment 数据隔离。
- 通过 `PANVARA_TEST_DATABASE_URL` 使用 PostgreSQL 18.4，或由集成测试启动一次性 Docker 容器；不复用手工 Compose 数据。
- Mailpit 或 Console Email Adapter 延后到 alpha.3。

Valkey、NATS、MinIO 和 OpenTelemetry Collector 只有在对应 Port/Adapter 进入实现后才加入 Full Profile，当前不制造“看起来完整”的空依赖。

## 3. 首次启动 Lite

当前 alpha.3b 开发切片合并到默认分支前，从对应验收分支克隆；合并后可以省略 `--branch`：

    git clone --branch codex/alpha3b-draft-plan --single-branch https://github.com/shezw/panvara.git
    cd panvara
    make doctor
    make verify
    make run

另一个终端验证：

    curl http://127.0.0.1:8080/healthz
    curl http://127.0.0.1:8080/readyz
    curl http://127.0.0.1:8080/version

`make verify` 与 `make run` 不需要 Docker，也不会隐式启动 PostgreSQL。

## 4. 启动 Server 开发切片

先检查工具、创建本地配置并显式加载。`make local-init` 不会覆盖已有文件：

    make doctor-server
    make local-init
    set -a; . ./.env; . ./.env.local; set +a
    make infra-up

再启动 Server：

    make run-server

某个 Project ID 第一次由 Server 装配时，会在 migration 后用一个事务持久化 Project、生成默认 Environment，并创建 `bootstrap-admin` Principal 与 `project.owner` Grant。`PANVARA_ENVIRONMENT_KEY` 未设置时默认为 `default`。后续以同一 Project ID 和相同配置重启会复用持久化作用域；该 Project 的 Key、Locale、Time Zone、Currency 或 Environment Key 漂移会拒绝启动。新 Project ID 配合新唯一 Key 会创建另一套隔离事实，不会迁移原项目。

不要在示例、配置文件、模块 Source、Git 或 `--admin-token` 参数中保存真实 Token；命令行参数通常对同机进程可见。生产环境应由 Secret Manager 注入。marker 尚不存在时 Server 要求 Token 为 32–1024 字节，只含 HTTP Bearer 安全 ASCII（字母、数字、`-._~+/`，`=` 只能尾随），且不以 Service Credential 保留前缀 `pvk1.` 开头；P0-01b 只把 SHA-256 digest、hint、Credential metadata 与永久 marker 持久化到 PostgreSQL，原始 Token 不入库，但环境变量与启动配置中的明文生命周期不作清除保证。

Credential 只认证 project-local Principal，不能单独授予权限。每个新旧 Admin 用例都由 Application 层读取 active Credential/Principal 与持久化 Owner Grant；Credential revoke 返回 401，Grant revoke 返回 403，权威状态不可用返回 503。Principal disable/Credential revoke 是终态；Grant 可显式 PUT 重新授予，但重启/bootstrap 不补回。详细边界与本地验收见[访问管理指南](modules/access-administration.md)，架构决策见 [ADR-0005](adr/0005-project-local-access-administration.md)。

另一个终端验证：

    curl http://127.0.0.1:8080/readyz
    curl http://127.0.0.1:8080/api/core/v1alpha1/modules/crm.leads/openapi.json
    curl http://127.0.0.1:8080/api/core/v1alpha1/modules/crm.leads/ui-schema.json

停止 Server 后可关闭基础设施：

    make infra-down

Compose 端口只绑定 127.0.0.1，开发密码只用于本机；生产配置不得复用。
PostgreSQL 18 官方镜像把持久化根目录改为 /var/lib/postgresql，Compose 已按 18+ 规则挂载，不能沿用 17 及以下的 /var/lib/postgresql/data。
所有本地、测试和自备 PostgreSQL 数据库还必须使用 UTF8 `server_encoding`；可以用 `SHOW server_encoding;` 验证。非 UTF8 数据库可能拒绝多语言 Source、标签或生成制品。当前启动/Migrate 尚未自动 fail-fast 检查这一条件，接入外部数据库时由开发者或运维先行确认。

Panvara 不隐式加载 `.env`。需要覆盖默认值时，通过 Shell、IDE 或可信的环境管理器显式导出 `.env` 与 `.env.local`；仓库中的 `.env.example` 故意不给管理员 Token 设置可用默认值。
多个 clone/worktree 并行开发时，为 PANVARA_COMPOSE_PROJECT 和 PANVARA_POSTGRES_PORT 设置不同值，避免共用容器、数据卷或宿主端口。

## 5. 统一命令

| 命令 | 用途 |
| --- | --- |
| make doctor / doctor-server | 检查 Lite / Server 的本地工具和 Docker 状态 |
| make local-init | 创建 Git 忽略的 `.env` 与 `.env.local`，不覆盖已有配置 |
| make fmt | 格式化 Go |
| make fmt-check | 检查未格式化文件，不修改工作区 |
| make test | 随机顺序运行单元和 seed corpus |
| make test-race | 开启 Race Detector |
| make test-integration | 强制运行 PostgreSQL 18.4 Store 集成测试，包含 Access 生命周期，以及 P0-02a Publish 幂等、并发、事务回滚、append-only 与升级约束；不可跳过 |
| make test-server-smoke | 强制运行 Server HTTP/持久化 smoke，包含访问管理、轮换、撤权即时生效与重启不自动恢复；不可跳过 |
| make test-e2e | 顺序运行以上两个当前 Server 必需集成目标；不代表完整 P0-01 已覆盖 |
| make vet | Go 静态检查 |
| make build | 生成 bin/panvara |
| make verify | Docker-free 快速 PR 门禁；不包含集成目标 |
| make run | 运行 Lite |
| make run-server | 从进程环境运行 Server；Token 不转换为 CLI 参数 |
| make infra-up/down | 管理本地 PostgreSQL |
| make docs-setup | 使用 package-lock 安装文档依赖 |
| make docs-serve | 在 127.0.0.1:5173 本地预览文档 |
| make docs-check | 验证模块文档契约并构建静态站点 |

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
| PANVARA_ENVIRONMENT_KEY | default | 当前 Project 唯一默认 Environment 的可读 Key；该 Project 首次启动后漂移会拒绝启动 |
| PANVARA_ADMIN_TOKEN | 无 | marker 不存在时必需；32–1024 字节、仅 Bearer 安全 ASCII、非 `pvk1.` 前缀；之后可省略 |
| PANVARA_TEST_DATABASE_URL | 无 | 测试专用 PostgreSQL 18.4 URL；设置后不启动 Docker 容器 |
| PANVARA_REQUIRE_DOCKER | 无 | `1` 时 Docker/数据库不可用必须失败；Make 集成目标和 CI 已设置 |

当前实际配置优先级是：CLI > 环境变量 > 内置默认值，尚无配置文件加载器或 Secret Reference Runtime。普通参数可以使用 CLI；管理员 Token 应避免 CLI，只从 Secret Manager 注入的环境变量读取，不进入模型、仓库、日志或普通环境样例。配置优先级只决定启动输入，不允许覆盖已经持久化的 Project/Environment 事实。

## 7. CI 与依赖策略

当前 GitHub Actions 包含：

- Node.js 22：模块文档契约检查与 VitePress 静态构建。
- Go 1.26.5：fmt-check、vet、unit、race、build。
- PostgreSQL 18.4：独立 required Job 执行 `make test-e2e`；使用 Service URL，并设置 `PANVARA_REQUIRE_DOCKER=1`，测试不得 Skip。当前覆盖 P0-01a/P0-01b 与 P0-02a runnable slice，但不宣称完整 P0-01/P0-02 或 IAM。
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

## 9. 文档同步约束

任何用户可感知的模块、配置、CLI、API、模型、运行方式或兼容行为变化，都必须在同一变更中更新对应 `docs/modules/` 指南。新模块还要登记 `docs/_meta/modules.json`，并提供非专业用户可执行的验收步骤。

完整 Definition of Done、模块模板和豁免条件见[文档同步规范](contributing/documentation.md)。没有可执行引导和验收步骤的功能，不视为完成。
