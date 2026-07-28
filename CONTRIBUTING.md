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

开始前请先阅读[参与贡献](docs/contributing/index.md)和[认识 Panvara](docs/guides/concepts.md)，确认当前工具、能力边界与验证方式。

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
- 用户可感知变化在同一提交中更新最相关的 `docs/guides/`、`docs/reference/` 或 `docs/releases/` 页面。
- 首次使用路径受影响时同步更新 `docs/index.md`、`docs/getting-started/index.md` 和 README。
- 已知限制、兼容风险和恢复方式没有被隐藏。

公开文档要求与 Pull Request 流程见[参与贡献](docs/contributing/index.md)。
