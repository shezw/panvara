<!--
    Panvara
    docs/testing.md    2026-07-18
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

当前门禁包含 **Server Core P0-01a 可运行切片**，只验证最小持久化执行作用域与 Owner Grant 授权闭环，不表示完整 P0-01、IAM 或多 Environment 数据隔离已经完成。

## 2. 分层测试

| 层 | 目标 | 工具与方式 | 当前状态 |
| --- | --- | --- | --- |
| Domain | Money、ProjectContext、Environment Scope、模型规则、模块图、不可变 Revision、版本化 Draft | testing、表驱动、property、fuzz | P0-01a 增加 UUIDv7 Environment ID、精确 Scope/Execution 与跨 Project 拒绝；alpha.3b 增加 Draft 不变量 |
| Application | Access Kernel、Record 用例、校验、并发版本、Kernel 生命周期、Revision 复验、Draft/Validation/Plan | fake Port、失败注入、race | P0-01a 固定 Operation、忽略 Actor 自报 Role、读取权威 Grant 并 fail closed；完整 IAM 仍待后续 |
| HTTP | 状态码、JSON/raw Source、认证、授权错误信封、ETag、幂等、OpenAPI | httptest、契约断言 | alpha.2 CRUD + alpha.3a Registry + alpha.3b Draft Planning + P0-01a Execution 传递 |
| PostgreSQL | migration、Project/Environment/Principal/Grant、事务、唯一/引用、分页、重启恢复、追加式 Registry/Validation/Plan | PostgreSQL 18.4 Service URL 或临时 Docker | P0-01a 增加并发 bootstrap、漂移/状态/约束与撤权持久性；alpha.3b 增加 Draft integration |
| Provider | Adapter 是否满足 Capability | 共享 conformance suite | alpha.3 planned |
| AppModule | 严格解码、IR、Data Schema Identity、OpenAPI、Manager Schema、恶意输入 | golden、fuzz、语义校验 | alpha.3a format 1 指纹差异矩阵已建立；迁移兼容矩阵待后续 alpha.3 |
| Profile/E2E | Lite/Server 纵向闭环 | 真实 listener + PostgreSQL 18.4 | CRUD、Registry、Draft Plan 与 Grant 撤销即时生效/重启不恢复共同验证；完整 P0-01 和发布 E2E 待后续 |
| Non-functional | race、benchmark、load、fault | Go race/bench + 专项压测 | 基础 race 已建立 |
| Supply chain | 漏洞、SBOM、许可、签名 | govulncheck、生成/验证工具 | RC 前 |
| Documentation | 模块指南、内部链接、静态站点、Golden Path | manifest contract、VitePress build、人工/E2E 复核 | alpha.2 基础门禁同步覆盖 P0-01a 的能力与限制说明 |

测试替身只能替代应用 Port；数据库语义、消息确认和 Provider 协议不可仅靠 Mock 宣称通过。

## 3. 当前 PR 门禁

Docker-free 的 `make verify` 依次验证：

1. gofmt 无漂移。
2. go vet 全包通过。
3. 单元测试以随机顺序、单次非缓存执行。
4. Race Detector 全包通过。
5. `cmd/panvara` 可构建。

独立 required integration Job 使用 Go 1.26.5 与 PostgreSQL 18.4 Service 执行 `make test-e2e`：

1. `make test-integration` 验证 fresh/idempotent migration、带既有 Record/unique/reference/Registry 数据的逐版升级、Record 与 Registry 约束、Draft 追加事实，以及 P0-01a 的 Project/Environment/Principal/Grant 首次 bootstrap、并发收敛、幂等重启、配置漂移、active 状态、非默认 Environment 拒绝、数据库约束和撤权后不自动补回。
2. `make test-server-smoke` 验证模块 Artifact、CRUD/ETag/Record 持久化、Registry 重启事实、Draft → Validation → Plan 真实 HTTP 旅程，以及同一 Token 在 Owner Grant 撤销后立即被 Record/Revision/Draft Admin 用例拒绝、Server 重启后仍被拒绝。

该 Job 同时设置 `PANVARA_TEST_DATABASE_URL` 与 `PANVARA_REQUIRE_DOCKER=1`；数据库不可用或版本不是 18.4 时必须失败，不能 Skip。Go 1.25.12 兼容 Job 只运行 vet、无 tag 的 unit 和 build，设置 `GOTOOLCHAIN=local`，不需要 Docker。

独立 Documentation Job 使用 Node.js 22 执行 `make docs-setup` 和 `make docs-check`，验证基础页面、模块清单、必需章节、示例 AppModule 映射、内部链接和静态构建。文字与命令的真实性仍由模块 E2E 和非专业用户 Golden Path 复核，不能只凭页面成功渲染判定。

本地 `make test-integration`/`make test-server-smoke` 也设置 `PANVARA_REQUIRE_DOCKER=1`：没有外部测试 URL且 Docker 不可用时会失败。默认 `make test` 与 `make verify` 不包含 integration tag，不会隐式启动 Docker。

## 4. 门禁分级

### Pull Request：目标 10–15 分钟

- `make verify`。
- PostgreSQL 18.4 Store integration 与 Server HTTP persistence smoke。
- AppModule decode/compiler/record validation Fuzz seed、Canonical IR 与 Artifact golden。
- Public/Admin HTTP 契约、bootstrap Token 认证负例，以及持久化 Owner Grant 授权与撤权跨重启负例。

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

## 5. 当前已验证的高价值用例

- AppModule 相同输入产生稳定 IR 和 Hash。
- 未知字段类型、重复字段、坏引用、依赖缺失、冲突和环全部拒绝。
- 模块注册顺序确定且依赖优先。
- Kernel 部分启动失败按逆序回滚。
- Money 禁止跨币种运算和 int64 溢出。
- ProjectContext 拒绝无效 Locale、Time Zone 和 Currency。
- Environment ID 可生成并解析有效 UUIDv7；Scope/Execution 保留精确 Project + Environment 边界并拒绝跨 Project Actor。
- fresh 数据库可迁移，重复执行 migration 无副作用。
- P0-01a 首次 bootstrap 在事务中持久化 Project、生成默认 Environment，并创建 `bootstrap-admin` Principal 与 `project.owner` Grant；并发调用收敛到同一 Environment ID，相同配置重启幂等。
- 已持久化 Project Key、Locale、Time Zone、Currency 或 Environment Key 与启动配置漂移时拒绝启动。
- Token 只认证 Principal，Actor 自报 Role 不参与授权；Admin 只接受 PostgreSQL 中 active、未撤销且精确匹配 Scope/Principal 的 Owner Grant，授权状态读取失败时 fail closed。
- 非默认或 disabled Environment、disabled Project/Principal 均被拒绝；Project/Environment/Principal/Grant 表不包含 Token、Secret、Password 或 Credential 列。
- 撤销 Owner Grant 后，Record、Revision 与 Draft Admin 请求使用同一 Token 立即返回 403；Server 重启不会补回 Grant，受控恢复 Grant 后无需重启即可恢复访问。
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

以上 P0-01a 用例只证明固定 bootstrap Principal、默认 Environment 与单一 `project.owner` Grant 的最小闭环。当前没有 Account/Credential/Membership 生命周期、动态 Role/Policy、Grant 管理 API、身份 Provider，也没有给 Record、Revision、Draft 数据补齐 `environment_id`；这些缺口不能被现有通过项解释为完整 P0-01 或多 Environment 隔离。

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
