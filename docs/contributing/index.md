<!--
    Panvara
    docs/contributing index.md    2026-07-28
     ______     __  __     ______     ______     __     __
    /\  ___\   /\ \_\ \   /\  ___\   /\___  \   /\ \  _ \ \
    \ \___  \  \ \  __ \  \ \  __\   \/_/  /__  \ \ \/ ".\ \
     \/\_____\  \ \_\ \_\  \ \_____\   /\_____\  \ \__/".~\_\
      \/_____/   \/_/\/_/   \/_____/   \/_/   \/_/.com

    @link    : https://github.com/shezw/panvara
    @author  : shezw
    @email   : hello@shezw.com
-->

# 参与贡献

欢迎修正文档、补充测试和改进 Panvara。外部贡献者不需要 GitHub Pages 管理权限，也不需要任何内部工作流文件。

## 从干净的 main 开始

需要 Git、Go 1.25+ 与 Make：

```sh
git clone https://github.com/shezw/panvara.git
cd panvara
git switch main
make doctor
make build
make verify
```

通过条件：

- `make doctor` 最后一行是 `Environment check passed.`；
- `make build` 生成 `bin/panvara`；
- `make verify` 无错误退出。

如果贡献涉及 Server、PostgreSQL 或 HTTP 持久化，再安装 Docker Compose 与 OpenSSL：

```sh
make doctor-server
make test-e2e
```

集成测试只能使用可丢弃数据库。

## 修改代码

- 保持改动小而完整，沿用现有包边界和命名；
- 新行为同时补充单元测试；
- 数据库或 HTTP 行为同时补充对应集成验证；
- 不把 Secret、`.env.local`、数据库转储或生成二进制提交到 Git；
- 不把 Planned / Unavailable（尚不可用）能力写成当前可用。

提交前至少执行：

```sh
make fmt
make verify
```

涉及数据库链路再执行 `make test-e2e`。

## 修改公开文档

文档站需要 Node.js 22+ 与 npm 10+：

```sh
make docs-setup
make docs-serve
```

在 `http://127.0.0.1:5173` 检查导航、移动端排版、代码块和站内链接。提交前：

```sh
make docs-check
```

公开文档遵守三种状态：

- `Current Distribution`：只描述二进制内建标识 v0.1.0-alpha.2 对应的公开能力；
- `Source Preview`：必须固定源码引用，并明确非 Distribution、非生产与缺失能力；
- `Planned / Unavailable`：只在能力状态页说明尚不可用，不提供伪操作步骤。

用户指南应给出目标、前置条件、可复制步骤、成功标志、数据影响和当前限制。内部架构决策、项目排期、发布运维与完整工程门禁不属于公开使用文档。

## 提交 Pull Request

1. 从最新 `main` 创建主题分支。
2. 用清晰提交信息说明动机与行为变化。
3. 在 PR 中列出执行过的验证命令和结果。
4. 说明未覆盖风险、兼容影响与数据影响。
5. 对 Review 意见追加小而明确的提交。

如果发现安全问题，不要在公开 Issue 中粘贴 Secret、利用细节或真实数据；先通过仓库维护者提供的私密渠道联系。

## 报告问题

Issue 至少包含：

- 期望结果与实际结果；
- 最小复现步骤；
- `./bin/panvara --version` 的非敏感输出；
- 操作系统、Go 版本，以及相关 Docker 版本；
- HTTP 状态、错误 `code` 与 `request_id`；
- 已执行的恢复尝试。

不要附带 `.env`、`.env.local`、Authorization Header、签发响应或数据库密码。
