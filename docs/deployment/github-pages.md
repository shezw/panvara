<!--
    Panvara
    docs/deployment/github-pages.md    2026-07-17
     ______     __  __     ______     ______     __     __
    /\  ___\   /\ \_\ \   /\  ___\   /\___  \   /\ \  _ \ \
    \ \___  \  \ \  __ \  \ \  __\   \/_/  /__  \ \ \/ ".\ \
     \/\_____\  \ \_\ \_\  \ \_____\   /\_____\  \ \__/".~\_\
      \/_____/   \/_/\/_/   \/_____/   \/_____/   \/_/   \/_/.com

    @link    : https://github.com/shezw/panvara
    @author  : shezw
    @email   : hello@shezw.com
-->

# 部署文档到 GitHub Pages

Panvara 的公开文档站地址是：

<https://shezw.github.io/panvara/>

仓库使用 GitHub Actions 自动检查、构建并发布 VitePress 文档。正常发布不需要手工提交 `docs/.vitepress/dist`，也不需要维护 `gh-pages` 分支。

## 适用场景

- 第一次为仓库启用公开文档站。
- 修改 `docs/` 后确认文档已经自动上线。
- 排查 GitHub Actions 成功但页面打不开、样式丢失或链接跳错的问题。
- 回滚一版有问题的文档。

## 第一次启用

仓库管理员只需要执行一次：

1. 打开仓库的 **Settings**。
2. 在左侧选择 **Pages**。
3. 在 **Build and deployment** 的 **Source** 中选择 **GitHub Actions**。

完成后，推送文档相关文件会触发 `documentation-pages` 工作流。

## 发布流程

当前 alpha 开发阶段，工作流接受以下来源：

- `codex/alpha3b-draft-plan`：当前完整文档所在的开发分支。
- `main`：文档合入默认分支后的正式发布入口。

工作流只在文档、文档依赖、检查脚本或自身配置变化时自动运行，也可以在 GitHub 的 **Actions → documentation-pages → Run workflow** 中手工触发。

每次发布会依次完成：

1. 安装锁定版本的 Node.js 依赖。
2. 检查模块指南、示例和内部链接契约。
3. 使用 `/panvara/` 站点路径构建 VitePress。
4. 上传静态制品。
5. 部署到 `github-pages` 环境。

## 本地预检

在仓库根目录执行：

```sh
make docs-setup
PANVARA_DOCS_BASE=/panvara/ \
PANVARA_DOCS_SOURCE_BRANCH="$(git branch --show-current)" \
npm run docs:check
```

成功标志是终端出现 `build complete`，且命令退出码为 `0`。

本地日常阅读仍可使用：

```sh
make docs-serve
```

此时访问 `http://127.0.0.1:5173`，不需要手工添加 `/panvara/`。

## 线上验收

Actions 页面中 `Build documentation` 和 `Deploy documentation` 都显示绿色后，等待几十秒，再检查：

```sh
curl -fsS -o /dev/null https://shezw.github.io/panvara/
curl -fsS -o /dev/null https://shezw.github.io/panvara/getting-started/
curl -fsS -o /dev/null https://shezw.github.io/panvara/modules/
curl -fsS -o /dev/null https://shezw.github.io/panvara/arch
```

四条命令都没有输出并返回 `0` 即为通过。浏览器还应确认：

- 首页样式正常，不是纯文字页面。
- 顶部导航、侧边栏和本地搜索可以使用。
- 架构页 Mermaid 图能够显示。
- 页面地址始终保留 `/panvara/` 前缀。

## 常见问题

### 站点显示 404

先确认仓库 Settings 中 Pages 的 Source 已设置为 **GitHub Actions**，再检查最新 `documentation-pages` 运行是否成功。

### 页面打开但没有样式

这通常表示生产构建没有使用 `/panvara/` 基路径。工作流必须设置：

```text
PANVARA_DOCS_BASE=/panvara/
```

### “在 GitHub 上改进本页”指向错误分支

发布工作流会把当前发布分支写入 `PANVARA_DOCS_SOURCE_BRANCH`。本地构建未设置该变量时，链接默认指向 `main`。

### 自定义域名

当前配置面向 `shezw.github.io/panvara/`。以后绑定独立域名时，应将生产 `PANVARA_DOCS_BASE` 改为 `/`，并重新执行部署。

## 回滚

文档站不保存独立的手工状态，内容始终来自 Git 提交。发现问题时：

1. 对有问题的提交执行 `git revert`，不要改写已经推送的历史。
2. 推送回滚提交。
3. 等待 `documentation-pages` 再次成功。
4. 重新执行线上验收命令。

构建失败不会发布半成品；上一次成功版本会继续在线。
