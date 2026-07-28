<!--
    Panvara
    .agent/operations/github-pages.md    2026-07-28
     ______     __  __     ______     ______     __     __
    /\  ___\   /\ \_\ \   /\  ___\   /\___  \   /\ \  _ \ \
    \ \___  \  \ \  __ \  \ \  __\   \/_/  /__  \ \ \/ ".\ \
     \/\_____\  \ \_\ \_\  \ \_____\   /\_____\  \ \__/".~\_\
      \/_____/   \/_/\/_/   \/_____/   \/_____/   \/_/   \/_/.com

    @link    : https://github.com/shezw/panvara
    @author  : shezw
    @email   : hello@shezw.com
-->

# GitHub Pages 运维手册

Panvara 的正式公开文档站是：

<https://shezw.github.io/panvara/>

仓库通过 `.github/workflows/docs-pages.yml` 检查、构建并发布 VitePress 文档。正式站点只允许由 `main` 发布，不维护 `gh-pages` 分支，也不提交 `docs/.vitepress/dist`。

## 环境拓扑

```text
main / PR
   │
   ▼
documentation-pages
   ├── npm ci（Node.js 22，npm cache）
   ├── docs:lint + docs:build
   ├── Pages artifact（仅 main 的非 PR 运行）
   └── github-pages environment（仅 main 的非 PR 运行）
                         │
                         ▼
          https://shezw.github.io/panvara/
```

构建失败时不会替换当前线上版本。`deploy` 必须同时满足：

```text
github.event_name != 'pull_request' && github.ref == 'refs/heads/main'
```

该条件也用于 Pages 配置和制品上传，避免 PR 或功能分支产生可部署制品。

## 触发与发布边界

| 事件 | 来源 | Build | Deploy |
| --- | --- | --- | --- |
| `push` | `main` 且命中文档路径 | 是 | 是 |
| `pull_request` | 目标为 `main` 且命中文档路径 | 是 | 否 |
| `workflow_dispatch` | `main` | 是 | 是 |
| `workflow_dispatch` | 其他 ref | 是 | 否 |
| `push` | 功能分支 | 不触发 | 否 |

受监控的路径为：

- `.github/workflows/docs-pages.yml`
- `docs/**`
- `scripts/check-docs.mjs`
- `Makefile`
- `package.json`
- `package-lock.json`

GitHub 仓库还应在 **Settings → Environments → github-pages → Deployment branches and tags** 中只允许 `main`。环境规则和 workflow guard 共同形成纵深保护，不能用“所有分支”替代。

## 权限、缓存与密钥

- workflow 默认权限只有 `contents: read`。
- `build` 只有 `contents: read` 和 `pages: read`。
- `deploy` 单独获得 `pages: write` 与 `id-token: write`。
- 发布使用 GitHub OIDC 和临时 `GITHUB_TOKEN`，不配置长期 Pages 密钥。
- `actions/setup-node` 按 `package-lock.json` 使用 npm cache；依赖安装固定为 `npm ci`。
- PR 以编号隔离并发组，新的同一 PR 检查会取消旧检查；`main` 发布不会被 PR 取消。

## 配置项

| 配置 | 正式值 | 用途 |
| --- | --- | --- |
| `PANVARA_DOCS_BASE` | `/panvara/` | 保证资源和内部链接位于项目子路径 |
| `PANVARA_DOCS_SOURCE_BRANCH` | 当前 head ref；PR 使用源分支 | 生成“在 GitHub 上改进本页”链接 |
| Pages environment | `github-pages` | 记录部署 URL 与部署历史 |
| Node.js | `22` | 与 workflow 固定运行时一致 |

本地预览默认使用 `/`，不要把正式站点的 `/panvara/` 写死进内容链接。

## 发布步骤

1. 在提交前执行本地门禁：

   ```sh
   make docs-check
   git diff --check
   ```

2. 确认 PR 的 `Build documentation` 成功。PR 只验证构建，不会创建 Pages deployment。
3. 文档进入 `main` 后，等待 `documentation-pages` 的 `Build documentation` 和 `Deploy documentation` 均成功。
4. 打开运行详情，确认 head SHA 与预期提交一致。
5. 执行下方线上健康检查并记录结果。

需要人工补发时，只能在 Actions 页面选择 `main` 后运行 `workflow_dispatch`。选择功能分支只会执行构建，`deploy` 显示 skipped。

## 健康检查

六个一级导航必须返回 HTTP 200：

```sh
curl -fsS -o /dev/null https://shezw.github.io/panvara/
curl -fsS -o /dev/null https://shezw.github.io/panvara/getting-started/
curl -fsS -o /dev/null https://shezw.github.io/panvara/guides/
curl -fsS -o /dev/null https://shezw.github.io/panvara/reference/http-api
curl -fsS -o /dev/null https://shezw.github.io/panvara/releases/status
curl -fsS -o /dev/null https://shezw.github.io/panvara/contributing/
```

浏览器抽查还应确认：

- 首页 hero、样式和 `/panvara/` 前缀正确；
- 六个一级导航均可点击；
- 侧边栏、本地搜索和深色模式可用；
- “在 GitHub 上改进本页”指向 `main`；
- 已从公共边界移除的内部路径返回 404，而不是泄露旧页面。

## 日志与告警

- 构建日志：Actions 运行中的 `Build documentation` job。
- 部署日志：Actions 运行中的 `Deploy documentation` job。
- 部署记录：仓库 **Environments → github-pages**。
- 当前没有独立的外部可用性探针；GitHub Actions 失败通知是最低告警能力。
- 建议把 PR 的 `Build documentation` 配置为 `main` 的必需检查，并为 workflow 失败启用仓库通知。

排障时记录 run URL、run number、head SHA、失败 step 和首次失败时间。不要通过反复重跑掩盖可复现的构建错误。

## 常见故障

### 页面返回 404

确认仓库 **Settings → Pages → Build and deployment → Source** 为 **GitHub Actions**，再确认最近一次 `main` 部署成功。

### 页面存在但没有样式

检查运行日志中的构建环境是否包含：

```text
PANVARA_DOCS_BASE=/panvara/
```

### PR 或功能分支试图部署

这是安全边界故障。立即确认：

- `push.branches` 只有 `main`；
- `pull_request.branches` 只有 `main`；
- `deploy.if` 是本手册记录的双重条件；
- `github-pages` environment 只允许 `main`。

修复 workflow 后，在功能分支推送相关变更，确认不会出现新的 deployment，或 `deploy` 明确为 skipped。

## 回滚

文档内容以 Git 提交为事实来源。线上内容错误时：

1. 在 `main` 对问题提交执行 `git revert`，不要改写已推送历史。
2. 推送 revert 提交，触发新的受保护发布。
3. 等待 build 和 deploy 成功。
4. 重跑六个导航健康检查，并复查导致回滚的页面。

如果新提交构建失败，上一次成功版本会继续在线；先修复失败原因，不需要操作线上制品。
