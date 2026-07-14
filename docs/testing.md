<!--
    Panvara
    docs/testing.md    2026-07-14
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
| Domain | Money、ProjectContext、模型规则、模块图 | testing、表驱动、property、fuzz | 已建立 |
| Application | 用例、事务、幂等、生命周期 | fake Port、失败注入、race | Kernel 已建立 |
| HTTP | 状态码、JSON、错误信封、OpenAPI | httptest、契约 golden | 运维接口已建立 |
| PostgreSQL | 迁移、事务、查询、重启恢复 | Testcontainers + 真 PostgreSQL | alpha.2 |
| Provider | Adapter 是否满足 Capability | 共享 conformance suite | alpha.3 |
| AppModule | 解码、IR、兼容、恶意输入 | golden、fuzz、compat matrix | 语义 seed 已建立 |
| Profile/E2E | Lite/Server/Manager 纵向闭环 | 真实二进制和临时依赖 | CLI 装配已建立；真实 E2E 待建 |
| Non-functional | race、benchmark、load、fault | Go race/bench + 专项压测 | 基础 race 已建立 |
| Supply chain | 漏洞、SBOM、许可、签名 | govulncheck、生成/验证工具 | RC 前 |

测试替身只能替代应用 Port；数据库语义、消息确认和 Provider 协议不可仅靠 Mock 宣称通过。

## 3. 当前 PR 门禁

make verify 依次验证：

1. gofmt 无漂移。
2. go vet 全包通过。
3. 单元测试以随机顺序、单次非缓存执行。
4. Race Detector 全包通过。
5. cmd/panvara 可构建。

CI 额外用前一 Go 主版本运行 vet、test、build。当前仓库无外部依赖，因此门禁不要求 Docker。

## 4. 后续门禁分级

### Pull Request：目标 10–15 分钟

- 当前 make verify。
- PostgreSQL fresh migration 与核心 Repository 集成测试。
- AppModule golden 与 seed corpus。
- OpenAPI breaking check。
- 参考 Provider conformance。
- Lite 和 Server 启动 smoke。
- govulncheck。

### Nightly

- 全包 race 和持续 Fuzz。
- 所有已实现 Profile 的 E2E；planned Profile 只做静态定义和 fail-fast 检查。
- PostgreSQL 所有受支持版本矩阵。
- Provider sandbox 测试。
- 进程中止、数据库短暂失败、消息重复等故障注入。
- Benchmark 历史对比与小规模持续负载。

### Release

- 从 N-1 数据与活动 Revision 升级。
- 失败升级和回滚策略演练。
- 使用真正发布二进制/容器完成 E2E。
- SBOM、许可证、漏洞、镜像最小权限和签名检查。
- 容量测试、长稳测试和恢复演练报告。

## 5. v0.1 高价值用例

- AppModule 相同输入产生稳定 IR 和 Hash。
- 未知字段类型、重复字段、坏引用、依赖缺失、冲突和环全部拒绝。
- 模块注册顺序确定且依赖优先。
- Kernel 部分启动失败按逆序回滚。
- Money 禁止跨币种运算和 int64 溢出。
- ProjectContext 拒绝无效 Locale、Time Zone 和 Currency。
- 发布事务不会产生“数据库已变更但活动 Revision 未记录”的中间态。
- Outbox 与业务写入同事务，重复投递不重复执行效果。
- fresh 数据库和 N-1 数据库都可迁移；重复启动无副作用。
- Activate 失败后仍使用旧 Revision；进程重启恢复最后有效版本。
- destructive schema change 默认拒绝。
- Public/Admin 权限和 owner scope 不可互相绕过。
- Provider 超时、限流、永久失败和可重试失败分类一致。
- 同一 Idempotency Key 并发创建只产生一条 Record、Outbox 和外部效果。
- Activate 与并发请求/Job 固定各自开始时的 Revision epoch。
- Lite 不依赖外部服务可启动；Server 缺数据库时 readyz 失败而 healthz 仍成功。

Webhook 签名与重放测试在第一个 callback 型 Provider Capability 进入范围时成为强制门禁；Console/SMTP Email 阶段不伪造这一覆盖。

## 6. Fuzz 与兼容测试

当前 Fuzz target 保证任意 Descriptor 字符串输入不会导致 panic。alpha.2 增加：

- YAML/JSON 解码器。
- Canonical IR 序列化器。
- 模型迁移 diff。
- 表达式和过滤器解析器。
- HTTP 错误信封与分页 token。

Nightly 示例：

    go test -run=^$ -fuzz=FuzzDescriptorValidate -fuzztime=10m ./internal/domain/appmodule

AppModule 兼容矩阵至少覆盖：

- 同 Schema 版本的旧模块在新 Core 上运行。
- 新增可选字段和新 Resource 是兼容变更。
- 删除/改名/收窄类型被识别为潜在破坏变更。
- 未知 apiVersion 快速失败，不静默降级。

## 7. 基准与容量

首版不写一个缺少证据的“百万并发”数字。先建立可重复基准：

- 模型 Validate/Compile 的耗时和分配。
- Registry 激活和并发读取。
- CRUD 热路径 p50/p95/p99。
- Outbox 发布吞吐和积压恢复速度。
- 每节点 CPU、内存、连接数和 GC。

容量报告必须记录制品版本、模型 Hash、硬件、数据库规格、数据量、负载模型、错误率和延迟。扩容到数百节点时，以实测瓶颈决定缓存、队列、分区和服务拆分。

## 8. 测试文件约定

- 测试与被测包同目录；仅在验证公开 API 时使用外部 test package。
- 表驱动用例包含可读 name，失败消息同时给出 got/want。
- 可以 t.Parallel 的测试应隔离状态后并行。
- 集成测试创建独立数据库/Schema，不依赖开发者已有数据。
- 时间、随机数、ID 和外部调用通过 Port 注入，避免 sleep 驱动测试。
- 任何跳过项必须说明外部条件，CI 不接受长期无主的 skip。
