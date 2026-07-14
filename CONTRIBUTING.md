<!--
    Panvara
    CONTRIBUTING.md    2026-07-14
     ______     __  __     ______     ______     __     __
    /\  ___\   /\ \_\ \   /\  ___\   /\___  \   /\ \  _ \ \
    \ \___  \  \ \  __ \  \ \  __\   \/_/  /__  \ \ \/ ".\ \
     \/\_____\  \ \_\ \_\  \ \_____\   /\_____\  \ \__/".~\_\
      \/_____/   \/_/\/_/   \/_____/   \/_____/   \/_/   \/_/.com

    @link    : https://github.com/shezw/panvara
    @author  : shezw
    @email   : hello@shezw.com
-->

# 参与 Panvara 开发

开始前请先完成[开发环境](docs/development.md)配置，并阅读[总体架构](docs/arch.md)。

最小提交前检查：

```sh
make verify
make docs-setup
make docs-check
```

修改 PostgreSQL、Server 或 HTTP 纵向链路时，还需要可用的 Docker，并执行：

```sh
make test-e2e
```

## 完成定义

- 代码、配置、测试和文档表达同一个真实状态。
- 新能力有可重复的最小示例与验收步骤。
- 用户可感知的模块变化在同一提交中更新 `docs/modules/`。
- 新模块登记到 `docs/_meta/modules.json`。
- 快速开始受影响时同步更新 README 和 `docs/getting-started/`。
- 已知限制、兼容风险和恢复方式没有被隐藏。

文档章节与豁免规则见[文档同步规范](docs/contributing/documentation.md)。
