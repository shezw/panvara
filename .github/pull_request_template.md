<!--
    Panvara
    .github/pull_request_template.md    2026-07-14

    @link    : https://github.com/shezw/panvara
    @author  : shezw
    @email   : hello@shezw.com
-->

## 变更目标

<!-- 用户可以观察到什么变化？为什么需要它？ -->

## 模块与文档

- [ ] 我已列出受影响模块。
- [ ] 用户可感知变化已同步到对应 `docs/modules/` 指南。
- [ ] 快速开始、配置、命令、示例输出和故障排查已按需更新。
- [ ] 新模块已登记到 `docs/_meta/modules.json`。
- [ ] 本次仅为内部重构，不影响外部行为；下方已说明无需更新用户指南的依据。

受影响指南或豁免依据：

<!-- 例如：docs/modules/http-api.md；或说明为什么没有用户可感知变化。 -->

## 验证

- [ ] `make verify`
- [ ] `make docs-check`
- [ ] 对应模块的验收命令
- [ ] 如涉及 Server/PostgreSQL：`make test-e2e`

验证结果与必要证据：

<!-- 请记录命令结果、兼容风险以及尚未解决的问题。 -->
