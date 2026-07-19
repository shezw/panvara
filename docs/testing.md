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

当前门禁包含 **Server Core P0-01a/P0-01b 与 P0-02a 可运行切片**，验证持久化执行作用域、Service Principal/API Credential/Owner Grant 管理、最小安全审计及不可变 Publish Facts，不表示完整 P0-01/P0-02、P0-05、IAM 或多 Environment 数据隔离已经完成。

## 2. 分层测试

| 层 | 目标 | 工具与方式 | 当前状态 |
| --- | --- | --- | --- |
| Domain | Money、ProjectContext、Environment Scope、Principal/Credential/Grant、模型规则、不可变 Revision、版本化 Draft、Module Release | testing、表驱动、property、fuzz | P0-02a 增加 environment-scoped immutable Release、publish outcome 与 credential provenance |
| Application | Access Kernel、Credential authentication/administration、Record、Revision、Draft/Validation/Plan、Release Publish/Get | fake Port、失败注入、race | Publish 在 snapshot 前解析重放；首次发布精确链复验、Source 重编译、stale/unsupported、固定 Operation 与 fail closed |
| HTTP | 状态码、严格 JSON/raw Source、认证、授权错误信封、ETag、幂等、OpenAPI | httptest、契约断言 | CRUD/Registry/Draft/Release + P0-01b Access Administration；Release 覆盖 strict ASCII、重复/混淆字段、Location/no-store |
| PostgreSQL | migration、Project/Environment/Principal/Credential/Grant、Release、安全审计、事务、分页与重启恢复 | PostgreSQL 18.4 Service URL 或临时 Docker | 重放解析短事务、首次 Publish 原子事务、append-only Release、升级与无半状态；既有 Access/Draft integration |
| Provider | Adapter 是否满足 Capability | 共享 conformance suite | alpha.3 planned |
| AppModule | 严格解码、IR、Data Schema Identity、OpenAPI、Manager Schema、恶意输入 | golden、fuzz、语义校验 | alpha.3a format 1 指纹差异矩阵已建立；迁移兼容矩阵待后续 alpha.3 |
| Profile/E2E | Lite/Server 纵向闭环 | 真实 listener + PostgreSQL 18.4 | CRUD、Registry、Draft Plan、Publish/重放/重启不改变 Runtime/Record，以及 Credential/Grant 生命周期共同验证 |
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

1. `make test-integration` 验证 fresh/idempotent migration、逐版升级、Record/Registry/Draft/Release 事实，以及 P0-01a/P0-01b 的 Scope/Credential/Grant、marker/digest、终态、last-owner、事务审计与 append-only 约束；Release 另验证 Draft stale 后的原 Key/Plan alias、冲突优先级、并发、首次发布 stale、事务回滚和权限边界。
2. `make test-server-smoke` 验证 Artifact、CRUD、Registry、Draft → Validate → Plan → Publish 真实 HTTP 旅程，以及 Access Administration 的 strict JSON、一次性 Token、轮换、授权与重启；Release 覆盖转义字段拒绝、Draft stale 后重放与重启后 POST 重放，且 Publish 前后及重启后的 OpenAPI、ETag 与 Record 字节必须不变。

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
- P0-01a 首次 bootstrap 在事务中持久化 Project、默认 Environment、`bootstrap-admin` Principal 与 Owner Grant；P0-01b 再原子写入 digest-only bootstrap Credential、hint、永久 marker 与成功审计。
- marker 缺失时 Token 必须为 32–1024 字节、仅含 Bearer 安全 ASCII、非 `pvk1.` 前缀；marker 存在后 Token 可省略、相同可用、改变拒绝；revoked bootstrap Credential 不会被重启复活。
- 已持久化 Project Key、Locale、Time Zone、Currency 或 Environment Key 与启动配置漂移时拒绝启动。
- Credential 只认证 Principal，Actor 自报 Role 不参与授权；Admin 只接受 PostgreSQL 中 active Credential/Principal 与精确 Scope 的 Owner Grant，授权状态读取失败时 fail closed。
- Service Principal 创建、Credential 一次性签发/列表/revoke、Owner Grant 查询/授予/撤销均由固定 Application Operation 授权；Principal disable 与 Credential revoke 是终态。
- 移除最后一个 active Principal + Credential + Owner Grant path 返回 `last_owner_path` 并回滚；Grant 可由另一个 Owner 显式 PUT 重新授予，restart/bootstrap 不自动恢复。
- 访问 mutation 与 success audit 同事务；已认证授权拒绝走独立 append-only denied audit。此最小审计不等于 P0-05 通用 Audit/Idempotency/Outbox。
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
- Publish 先在短事务内二次授权并解析已绑定 Key/已发布 Plan；只有未命中的首次发布才验证精确 Plan 查找、当前 Draft/Validation/Plan 链、Source 重编译与 Candidate identity/Canonical IR；stale 与 unsupported 不产生发布写入。
- PostgreSQL Publish 在 Project advisory lock 下事务内二次授权，并原子登记 Revision、Module Release、专用 Idempotency Binding 与成功安全审计；失败不留下半状态。
- Draft 已变化后，同 Key/同意图、同 Plan/新 Key、重启后 POST 重放均返回原 Release；同 Key/异意图在 Plan 查找前冲突；Release、Key Binding 的 UPDATE/DELETE/TRUNCATE 被拒绝。
- Release HTTP 严格拒绝未知/重复/大小写别名/Unicode 混淆/`\u` 转义字段名、多个 Key、query 与多余 Body；首次 201 + Location，重放/Detail 200，全部 `private, no-store`。
- Publish 后 Candidate 可读，但 OpenAPI 当前 Revision、业务 Record 字节与 ETag 在发布前后和多次重启后保持不变。
- Lite 不依赖外部服务可启动；Server 缺少数据库、模块或 Project ID 时 fail-fast；marker 尚未创建且缺 Token 时 fail-fast，marker 存在后可省略 Token。

以上 P0-01a/P0-01b 用例只证明 project-local Service Principal/API Credential、默认 Environment 与固定 `project.owner` Grant 的可运行闭环。当前没有 Account、ExternalIdentity、Session、ProjectMembership、动态 Role/Policy、RecordOwner 或身份 Provider，也没有给 Record、Revision、Draft 数据补齐真正的 Environment 事实；这些缺口不能被现有通过项解释为完整 P0-01 或多 Environment 隔离。

以下能力仍是后续测试目标，不是 P0-02a 已通过项：Outbox、Activate/Rollback、通用业务 Idempotency Key、副作用、Provider 错误分类、Revision epoch、数据迁移执行和 N-1 Schema 激活升级。当前 Idempotency Key 只覆盖 Create Draft 与 Release Publish 的专用事实。

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
