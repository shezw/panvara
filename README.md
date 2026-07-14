<!--
    Panvara
    README.md    2026-07-14
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

Panvara 是面向中小开发团队的、数据模型驱动的可组合全栈框架。它希望用同一套 Core 构建 App、Website、管理后台和 E-commerce，并通过可替换的全球化 Provider 接入身份、支付、消息、存储与其他第三方能力。

当前版本是 **v0.1.0-alpha.1**：仓库只证明 Core 的边界、生命周期、模型描述、运行 Profile 和验证框架，不宣称已经具备生产级业务能力。

## 快速开始

开发环境使用 Go 1.26.5；模块最低兼容 Go 1.25。

    make verify
    make run

服务默认监听 127.0.0.1:8080：

- GET /healthz：进程存活
- GET /readyz：Core 是否可服务
- GET /version：独立版本轴

需要 PostgreSQL 时再启动本地基础设施：

    make infra-up

## 设计文档

- [总体架构](docs/arch.md)
- [开发环境](docs/development.md)
- [Core v0 版本与边界](docs/core-v0.md)
- [验证测试框架](docs/testing.md)

## 当前原则

- Core 默认以单进程模块化单体启动，也允许按 Profile 组合和部署。
- alpha.1 只有 Lite 可执行；其余 Profile 是已定义但不会伪装启动的演进目标。
- 业务模型、应用编排、接口与基础设施遵循单向依赖。
- Lite 模式不要求 PostgreSQL、Valkey、NATS 或 Kubernetes。
- 模块和 Provider 使用显式协议版本；分发版本不替代协议兼容性。
- 动态模型只能表达受控数据和动作，不允许任意代码、SQL 或 Shell。

## License

[MIT](LICENSE)
