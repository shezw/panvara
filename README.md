<!--
    Panvara
    README.md    2026-07-28
     ______     __  __     ______     ______     __     __
    /\  ___\   /\ \_\ \   /\  ___\   /\___  \   /\ \  _ \ \
    \ \___  \  \ \  __ \  \ \  __\   \/_/  /__  \ \ \/ ".\ \
     \/\_____\  \ \_\ \_\  \ \_____\   /\_____\  \ \__/".~\_\
      \/_____/   \/_/\/_/   \/_____/   \/_____/   \/_/   \/_/.com

    @link    : https://github.com/shezw/panvara
    @author  : shezw
    @email   : hello@shezw.com
-->

# Panvara

Panvara 是面向个人与中小开发团队的数据模型驱动全栈框架。你用一份严格的 YAML 或 JSON AppModule 声明资源、字段、约束与开放操作，Panvara 将它编译为可运行的 HTTP API、PostgreSQL Record 能力、OpenAPI 3.1 和 Manager UI Schema。

当前二进制内建 Distribution 标识为 **v0.1.0-alpha.2**，但当前尚无对应 GitHub Release。本文固定到源码快照 `cfea044bfee90b7b8d62de79e42d2503258d7781`，适合本地原型、学习和技术评估，暂不适合直接承载生产业务。

## 你可以用它做什么

- 先用无数据库的 Lite 确认工具链、进程健康和版本；
- 用 AppModule 定义数据结构和 Public Create / Admin CRUD；
- 在 PostgreSQL Server 中保存、查询、修改和软删除 Record；
- 从同一模型获取 OpenAPI 与前端可消费的 UI Schema。

## 5–10 分钟启动 Lite

需要 Git、Go 1.25+、Make 和 curl。第一次下载 Go 依赖的耗时取决于网络。

```sh
git clone https://github.com/shezw/panvara.git
cd panvara
git switch --detach cfea044bfee90b7b8d62de79e42d2503258d7781
make doctor
make build
./bin/panvara --version
make run
```

保持 `make run` 所在终端运行，在另一个终端检查：

```sh
curl -fsS http://127.0.0.1:8080/healthz
curl -fsS http://127.0.0.1:8080/readyz
curl -fsS http://127.0.0.1:8080/version
```

前两个请求应返回 `alive` 与 `ready`，版本响应应包含 `"distribution":"0.1.0-alpha.2"`。Lite 不连接数据库，也不提供业务 Record API；完整说明见[快速开始](docs/getting-started/index.md)。

## 文档入口

- [概览](docs/index.md)
- [快速开始](docs/getting-started/index.md)
- [使用指南](docs/guides/index.md)
- [HTTP API](docs/reference/http-api.md)
- [配置与命令](docs/reference/configuration.md)
- [版本、能力状态与兼容](docs/releases/status.md)
- [参与贡献](docs/contributing/index.md)

同一源码快照还包含隔离标注的 **Source Preview**，其中可以发布并显式激活“数据结构未变化”的 compatible 模型版本。它仍只适合单 Server、本地或可丢弃环境；二进制中的 Distribution 版本字段不表示这些预览能力已经成熟或进入发行。具体操作见[模型变更预览](docs/guides/model-change-preview.md)。

本地维护文档：

```sh
make docs-setup
make docs-serve
make docs-check
```

## License

[MIT](LICENSE)
