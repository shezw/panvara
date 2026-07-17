<!--
    Panvara
    docs/testing.md    2026-07-15
     ______     __  __     ______     ______     __     __
    /\  ___\   /\ \_\ \   /\  ___\   /\___  \   /\ \  _ \ \
    \ \___  \  \ \  __ \  \ \  __\   \/_/  /__  \ \ \/ ".\ \
     \/\_____\  \ \_\ \_\  \ \_____\   /\_____\  \ \__/".~\_\
      \/_____/   \/_/\/_/   \/_____/   \/_____/   \/_/   \/_/.com

    @link    : https://github.com/shezw/panvara
    @author  : shezw
    @email   : hello@shezw.com
-->

# 验证测试框架

## 1. 质量原则

测试围绕风险、不变量和兼容契约组织，不以覆盖率百分比替代质量判断。高覆盖但没有验证事务原子性、版本兼容、权限隔离和重试幂等，仍然不可发布。

每个缺陷修复先增加可复现回归测试；随机测试和 Fuzz 发现的最小样本进入 corpus。禁止用自动重试掩盖 Flaky Test。

## 2. 分层测试

| 层 | 目标 | 工具与方式 | 当前状态 |
| --- | --- | --- | --- |
| Domain | Money、ProjectContext、模型规则、模块图、不可变 Revision、版本化 Draft | testing、表驱动、property、fuzz | alpha.3b 增加 Draft 身份、Source 边界、generation 与 no-op 不变量 |
| Application | Record 用例、校验、并发版本、Kernel 生命周期、Revision 复验、Draft/Validation/Plan | fake Port、失败注入、race | alpha.3b 增加 owner 授权、CAS、重放与确定性变化计划；发布仍待后续 |
| HTTP | 状态码、JSON/raw Source、鉴权、错误信封、ETag、幂等、OpenAPI | httptest、契约断言 | alpha.2 CRUD + alpha.3a Registry + alpha.3b Draft Planning 契约 |
| PostgreSQL | migration、事务、唯一/引用、分页、重启恢复、追加式 Registry/Validation/Plan | PostgreSQL 18.4 Service URL 或临时 Docker | alpha.3b 增加 Draft CAS、创建幂等和不可变派生事实 integration |
| Provider | Adapter 是否满足 Capability | 共享 conformance suite | alpha.3 planned |
| AppModule | 严格解码、IR、Data Schema Identity、OpenAPI、Manager Schema、恶意输入 | golden、fuzz、语义校验 | alpha.3a format 1 指纹差异矩阵已建立；迁移兼容矩阵待后续 alpha.3 |
| Profile/E2E | Lite/Server 纵向闭环 | 真实 listener + PostgreSQL 18.4 | 原 CRUD、Registry 与 invalid → replace → valid → plan 共同验证；发布 E2E 待后续 |
| Non-functional | race、benchmark、load、fault | Go race/bench + 专项压测 | 基础 race 已建立 |
| Supply chain | 漏洞、SBOM、许可、签名 | govulncheck、生成/验证工具 | RC 前 |
| Documentation | 模块指南、内部链接、静态站点、Golden Path | manifest contract、VitePress build、人工/E2E 复核 | alpha.2 已建立基础门禁 |

测试替身只能替代应用 Port；数据库语义、消息确认和 Provider 协议不可仅靠 Mock 宣称通过。

## 3. 当前 PR 门禁

Docker-free 的 `make verify` 依次验证：

1. gofmt 无漂移。
2. go vet 全包通过。
3. 单元测试以随机顺序、单次非缓存执行。
4. Race Detector 全包通过。
5. `cmd/panvara` 可构建。

独立 required integration Job 使用 Go 1.26.5 与 PostgreSQL 18.4 Service 执行 `make test-e2e`：

1. `make test-integration` 验证 fresh/idempotent migration、带既有 Record/unique/reference/Registry 数据的逐版升级、Record 约束、Registry 不可变性，以及 Draft 创建重放、Source CAS/no-op、项目隔离、Validation/Plan 追加事实和重启恢复。
2. `make test-server-smoke` 验证模块 Artifact、原有 CRUD/ETag/Record 持久化、Registry 重启事实，以及 raw-body invalid Draft → Replace → valid Validation → Plan 的真实 HTTP 旅程和全部无执行副作用断言。

该 Job 同时设置 `PANVARA_TEST_DATABASE_URL` 与 `PANVARA_REQUIRE_DOCKER=1`；数据库不可用或版本不是 18.4 时必须失败，不能 Skip。Go 1.25.12 兼容 Job 只运行 vet、无 tag 的 unit 和 build，设置 `GOTOOLCHAIN=local`，不需要 Docker。

独立 Documentation Job 使用 Node.js 22 执行 `make docs-setup` 和 `make docs-check`，验证基础页面、模块清单、必需章节、示例 AppModule 映射、内部链接和静态构建。文字与命令的真实性仍由模块 E2E 和非专业用户 Golden Path 复核，不能只凭页面成功渲染判定。

本地 `make test-integration`/`make test-server-smoke` 也设置 `PANVARA_REQUIRE_DOCKER=1`：没有外部测试 URL且 Docker 不可用时会失败。默认 `make test` 与 `make verify` 不包含 integration tag，不会隐式启动 Docker。

## 4. 门禁分级

### Pull Request：目标 10–15 分钟

- `make verify`。
- PostgreSQL 18.4 Store integration 与 Server HTTP persistence smoke。
- AppModule decode/compiler/record validation Fuzz seed、Canonical IR 与 Artifact golden。
- Public/Admin HTTP 契约与 bootstrap admin 鉴权负例。

OpenAPI breaking check、`govulncheck` 和供应链扫描仍需在 RC 前补齐。Provider conformance 不属于 alpha.2，因为当前没有 Provider Runtime。

### Nightly

- 全包 race 和持续 Fuzz。
- Lite/Server E2E；planned Profile 只做静态定义和 fail-fast 检查。
- PostgreSQL 所有受支持版本矩阵。
- Provider sandbox、Outbox 重复和发布故障注入延后到 alpha.3 及后续。
- alpha.2 可先覆盖进程中止与数据库短暂失败。
- Benchmark 历史对比与小规模持续负载。

### Release

- 从 N-1 数据与活动 Revision 升级。
- 失败升级和回滚策略演练。
- 使用真正发布二进制/容器完成 E2E。
- SBOM、许可证、漏洞、镜像最小权限和签名检查。
- 容量测试、长稳测试和恢复演练报告。

## 5. alpha.2 已验证的高价值用例

- AppModule 相同输入产生稳定 IR 和 Hash。
- 未知字段类型、重复字段、坏引用、依赖缺失、冲突和环全部拒绝。
- 模块注册顺序确定且依赖优先。
- Kernel 部分启动失败按逆序回滚。
- Money 禁止跨币种运算和 int64 溢出。
- ProjectContext 拒绝无效 Locale、Time Zone 和 Currency。
- fresh 数据库可迁移，重复执行 migration 无副作用。
- Public/Admin 权限和 owner scope 不可互相绕过。
- 非空 sortable 声明和未允许的 Public 读取操作被拒绝。
- Store 保持 Project/Module/Resource/Revision 边界、唯一值、引用完整性、乐观并发和软删除约束。
- Server 重装配后仍可读取 PostgreSQL Record。
- Module Revision 与 Data Schema Identity format 1 的包含/排除规则稳定；身份数组按 format 升序且已有 format 不可改写。
- bootstrap Registry 重启幂等、保留首次 Source，项目间不可越权读取。
- Registry 父/子表直接 UPDATE、DELETE、TRUNCATE 被数据库拒绝；强制篡改后读取与启动 fail closed。
- Registry 只读 HTTP 验证认证、身份数组、limit、`private, no-store`、Source `private, no-cache`、Content-Type、强 ETag、304、404 与 405。
- Draft raw Source 接口验证 Content-Type、NUL-free UTF-8、1 MiB、Content-Encoding、Baseline 固定和 owner 项目隔离。
- Create Draft 验证持久化 Idempotency Key：首次创建、同请求重放、同 Key 异请求冲突和重启恢复。
- Replace 验证数字 ETag、缺失/畸形/过期前置条件、原子 CAS，以及相同 Source no-op 不增加 generation。
- Validation 验证 invalid 是 2xx 领域结果、issues 稳定排序、valid Candidate 可复现、并发覆盖不能落下错误代次事实。
- Plan 验证变化顺序与 Hash 稳定、Validation 绑定、风险分类、重放幂等，以及 Candidate 未登记、OpenAPI/Record/Runtime 不变。
- Lite 不依赖外部服务可启动；Server 缺少数据库、模块、Project ID 或管理员 Token 时 fail-fast。

以下能力仍是后续 alpha.3 的测试目标，不是 alpha.3b 已通过项：Publish 事务、Outbox、Activate/Rollback、通用业务 Idempotency Key、副作用、Provider 错误分类、Revision epoch、数据迁移执行和 N-1 Schema 激活升级。alpha.3b 的 Idempotency Key 只覆盖 Create Draft。

Webhook 签名与重放测试在第一个 callback 型 Provider Capability 进入范围时成为强制门禁；Console/SMTP Email 阶段不伪造这一覆盖。

## 6. Fuzz 与兼容测试

alpha.2 当前 Fuzz target 覆盖：

- Descriptor 校验不会 panic。
- YAML/JSON 严格解码不会 panic。
- Compiler 对相同输入产生确定 IR/Hash，且任意输入不会 panic。
- Record JSON 解码和模型校验不会 panic。

Plan Format 1 覆盖模型结构 diff 的固定种子和确定性；迁移执行、表达式引擎和发布兼容矩阵仍属于后续阶段。

Nightly 示例：

    go test -run=^$ -fuzz=FuzzDescriptorValidateNeverPanics -fuzztime=10m ./internal/domain/appmodule

alpha.3 建立 AppModule 迁移兼容矩阵时至少覆盖：

- 同 Schema 版本的旧模块在新 Core 上运行。
- 新增可选字段和新 Resource 是兼容变更。
- 删除/改名/收窄类型被识别为潜在破坏变更。
- 未知 apiVersion 快速失败，不静默降级。

## 7. 基准与容量

首版不写一个缺少证据的“百万并发”数字。先建立可重复基准：

- 模型 Validate/Compile 的耗时和分配。
- Catalog 与不可变 Registry 并发读取；Registry 激活仍延后到后续 alpha.3。
- CRUD 热路径 p50/p95/p99。
- Outbox 发布吞吐和积压恢复速度在 alpha.3 引入 Outbox 后测试。
- 每节点 CPU、内存、连接数和 GC。

容量报告必须记录制品版本、模型 Hash、硬件、数据库规格、数据量、负载模型、错误率和延迟。扩容到数百节点时，以实测瓶颈决定缓存、队列、分区和服务拆分。

## 8. 测试文件约定

- 测试与被测包同目录；仅在验证公开 API 时使用外部 test package。
- 表驱动用例包含可读 name，失败消息同时给出 got/want。
- 可以 t.Parallel 的测试应隔离状态后并行。
- 集成测试创建独立数据库/Schema，不依赖开发者已有数据。
- 时间、随机数、ID 和外部调用通过 Port 注入，避免 sleep 驱动测试。
- 任何跳过项必须说明外部条件，CI 不接受长期无主的 skip。
