/*
   Panvara
   scripts/check-docs.mjs    2026-07-28
    ______     __  __     ______     ______     __     __
   /\  ___\   /\ \_\ \   /\  ___\   /\___  \   /\ \  _ \ \
   \ \___  \  \ \  __ \  \ \  __\   \/_/  /__  \ \ \/ ".\ \
    \/\_____\  \ \_\ \_\  \ \_____\   /\_____\  \ \__/".~\_\
     \/_____/   \/_/\/_/   \/_____/   \/_____/   \/_/   \/_/.com

   @link    : https://github.com/shezw/panvara
   @author  : shezw
   @email   : hello@shezw.com
*/

import fs from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..");
const failures = [];
const snapshotRevision = "cfea044bfee90b7b8d62de79e42d2503258d7781";

const requiredPages = [
  "docs/index.md",
  "docs/getting-started/index.md",
  "docs/guides/index.md",
  "docs/guides/concepts.md",
  "docs/guides/server.md",
  "docs/guides/appmodule.md",
  "docs/guides/crm-leads.md",
  "docs/guides/records.md",
  "docs/guides/access-preview.md",
  "docs/guides/model-change-preview.md",
  "docs/reference/http-api.md",
  "docs/reference/configuration.md",
  "docs/reference/commands.md",
  "docs/reference/troubleshooting.md",
  "docs/releases/status.md",
  "docs/releases/compatibility.md",
  "docs/contributing/index.md",
];

const expectedNavigation = [
  { text: "概览", link: "/" },
  { text: "快速开始", link: "/getting-started/" },
  { text: "使用指南", link: "/guides/" },
  { text: "API 与配置", link: "/reference/http-api" },
  { text: "版本与兼容", link: "/releases/status" },
  { text: "参与贡献", link: "/contributing/" },
];

function absolute(relativePath) {
  return path.join(root, relativePath);
}

function requirePath(relativePath, kind = "路径") {
  if (!fs.existsSync(absolute(relativePath))) {
    failures.push(`${kind}不存在: ${relativePath}`);
    return false;
  }
  return true;
}

function requireFragments(relativePath, fragments) {
  if (!requirePath(relativePath, "内容门禁文件")) {
    return;
  }
  const content = fs.readFileSync(absolute(relativePath), "utf8");
  for (const fragment of fragments) {
    if (!content.includes(fragment)) {
      failures.push(`${relativePath} 缺少公开边界声明: ${fragment}`);
    }
  }
}

function listMarkdownFiles(directory) {
  const files = [];
  for (const entry of fs.readdirSync(directory, { withFileTypes: true })) {
    if (entry.name === ".vitepress") {
      continue;
    }
    const entryPath = path.join(directory, entry.name);
    if (entry.isDirectory()) {
      files.push(...listMarkdownFiles(entryPath));
    } else if (entry.isFile() && entry.name.endsWith(".md")) {
      files.push(path.relative(root, entryPath).split(path.sep).join("/"));
    }
  }
  return files.sort();
}

for (const page of requiredPages) {
  requirePath(page, "公开文档");
}

const allowedPages = new Set(requiredPages);
const actualPages = listMarkdownFiles(absolute("docs"));
for (const page of actualPages) {
  if (!allowedPages.has(page)) {
    failures.push(`非公开 Markdown 仍位于 docs: ${page}`);
  }
}

for (const publicFile of ["README.md", ...requiredPages]) {
  if (!fs.existsSync(absolute(publicFile))) {
    continue;
  }
  const content = fs.readFileSync(absolute(publicFile), "utf8");
  for (const retiredClaim of [
    "当前正式发行版",
    "当前正式发行口径",
    "正式发行的 **Current Distribution**",
  ]) {
    if (content.includes(retiredClaim)) {
      failures.push(`${publicFile} 虚构了尚不存在的 GitHub Release: ${retiredClaim}`);
    }
  }
}

if (fs.existsSync(absolute("docs/.agent"))) {
  failures.push("docs/.agent 不得进入 VitePress 源目录");
}

const configPath = "docs/.vitepress/config.mts";
if (requirePath(configPath, "VitePress 配置")) {
  const config = fs.readFileSync(absolute(configPath), "utf8");
  const navigationMatch = config.match(
    /const publicNavigation = \[([\s\S]*?)\];/,
  );

  if (!navigationMatch) {
    failures.push("VitePress 配置缺少 publicNavigation");
  } else {
    const actualNavigation = [
      ...navigationMatch[1].matchAll(
        /\{\s*text:\s*"([^"]+)",\s*link:\s*"([^"]+)"\s*\}/g,
      ),
    ].map((match) => ({ text: match[1], link: match[2] }));

    if (JSON.stringify(actualNavigation) !== JSON.stringify(expectedNavigation)) {
      failures.push(
        `一级导航必须按约定保留 6 项，实际为: ${JSON.stringify(actualNavigation)}`,
      );
    }
  }

  if (!config.includes("nav: publicNavigation")) {
    failures.push("VitePress themeConfig.nav 必须使用 publicNavigation");
  }

  for (const excludedSource of [
    '".agent/**"',
    '"**/.agent/**"',
    '"_meta/**"',
    '"_templates/**"',
  ]) {
    if (!config.includes(excludedSource)) {
      failures.push(`VitePress srcExclude 缺少: ${excludedSource}`);
    }
  }

  if (!config.includes("sitemap:") || !config.includes("hostname: docsSiteUrl")) {
    failures.push("VitePress 必须生成受公开页面边界约束的 sitemap");
  }

  for (const retiredRoute of [
    '"/adr/',
    '"/modules/',
    '"/roadmap/',
    '"/deployment/',
    '"/architecture-review',
    '"/core-v0"',
    '"/development"',
    '"/testing"',
  ]) {
    if (config.includes(retiredRoute)) {
      failures.push(`VitePress 导航仍暴露旧内部路由: ${retiredRoute}`);
    }
  }
}

requireFragments("README.md", [
  "v0.1.0-alpha.2",
  "当前二进制内建 Distribution 标识",
  "当前尚无对应 GitHub Release",
  snapshotRevision,
  "5–10 分钟启动 Lite",
  "make doctor",
  "make build",
  "make run",
  "docs/getting-started/index.md",
  "docs/guides/index.md",
  "docs/reference/http-api.md",
  "docs/releases/status.md",
  "docs/contributing/index.md",
]);

requireFragments("CONTRIBUTING.md", [
  "docs/contributing/index.md",
  "docs/guides/concepts.md",
  "docs/guides/",
  "docs/reference/",
  "docs/releases/",
  "docs/index.md",
  "docs/getting-started/index.md",
  "README",
]);
if (requirePath("CONTRIBUTING.md", "贡献指南")) {
  const contributing = fs.readFileSync(absolute("CONTRIBUTING.md"), "utf8");
  for (const retiredPath of [
    "docs/development.md",
    "docs/arch.md",
    "docs/contributing/documentation.md",
    "docs/modules/",
    "docs/_meta/modules.json",
    ".agent",
  ]) {
    if (contributing.includes(retiredPath)) {
      failures.push(`CONTRIBUTING.md 仍暴露旧内部路径: ${retiredPath}`);
    }
  }
}

requireFragments("docs/getting-started/index.md", [
  "v0.1.0-alpha.2",
  snapshotRevision,
  `git switch --detach ${snapshotRevision}`,
]);
requireFragments("docs/guides/appmodule.md", [
  `https://github.com/shezw/panvara/blob/${snapshotRevision}/examples/modules/crm-leads.yaml`,
]);
if (requirePath("docs/guides/appmodule.md", "AppModule 指南")) {
  const appModuleGuide = fs.readFileSync(
    absolute("docs/guides/appmodule.md"),
    "utf8",
  );
  if (appModuleGuide.includes("blob/main/examples/modules/crm-leads.yaml")) {
    failures.push("AppModule 示例链接不得漂移到 main，必须固定源码快照");
  }
}

requireFragments("docs/releases/status.md", [
  "Current Distribution",
  "Source Preview",
  "Planned / Unavailable",
  "v0.1.0-alpha.2",
  "尚无对应 GitHub Release",
  "Compatible Activate",
]);
requireFragments("docs/guides/access-preview.md", [
  "Source Preview",
  "v0.1.0-alpha.2",
]);
requireFragments("docs/guides/model-change-preview.md", [
  "Source Preview",
  "v0.1.0-alpha.2",
  'ACTIVATE_URL="$RELEASES_URL/$RELEASE_ID/activate"',
  ".data_schema_changed == false",
  ".record_namespace_revision == $namespace",
  "重启 Server**不会回退**",
]);
requireFragments("docs/reference/http-api.md", [
  "GET /api/admin/core/v1alpha1/modules/{module}/active",
  "POST /api/admin/core/v1alpha1/modules/{module}/releases/{release}/activate",
  "activation_conflict",
  "not_activatable",
]);

if (failures.length > 0) {
  console.error("Panvara 公开文档边界检查失败:");
  for (const failure of failures) {
    console.error(`- ${failure}`);
  }
  process.exit(1);
}

console.log(
  `Panvara 公开文档边界检查通过：${requiredPages.length} 个公开页面，${expectedNavigation.length} 个一级导航。`,
);
