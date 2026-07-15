/*
   Panvara
   scripts/check-docs.mjs    2026-07-15
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

const requiredPages = [
  "docs/index.md",
  "docs/getting-started/index.md",
  "docs/getting-started/prerequisites.md",
  "docs/getting-started/local-environment.md",
  "docs/getting-started/build-and-lite.md",
  "docs/getting-started/crm-leads-acceptance.md",
  "docs/getting-started/revision-registry-acceptance.md",
  "docs/getting-started/troubleshooting.md",
  "docs/reference/commands.md",
  "docs/reference/configuration.md",
  "docs/contributing/documentation.md",
  "docs/contributing/module-guide-template.md",
  "docs/adr/0001-module-data-revision-identities.md",
  "docs/adr/0002-immutable-revision-registry.md",
];

const moduleHeadings = [
  "## 用途",
  "## 当前状态",
  "## 前置条件",
  "## 最小示例",
  "## 配置",
  "## 验收",
  "## 常见问题",
  "## 当前限制",
  "## 兼容与升级",
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

for (const page of requiredPages) {
  requirePath(page, "基础文档");
}

const manifestPath = "docs/_meta/modules.json";
if (!requirePath(manifestPath, "模块文档清单")) {
  finish();
}

let manifest;
try {
  manifest = JSON.parse(fs.readFileSync(absolute(manifestPath), "utf8"));
} catch (error) {
  failures.push(`模块文档清单不是有效 JSON: ${error.message}`);
  finish();
}

if (manifest.schemaVersion !== 1) {
  failures.push(`modules.json schemaVersion 必须为 1，实际为 ${manifest.schemaVersion}`);
}

if (!Array.isArray(manifest.modules) || manifest.modules.length === 0) {
  failures.push("modules.json 至少需要一个 modules 条目");
}

const ids = new Set();
const guides = new Set();
for (const module of manifest.modules ?? []) {
  const label = module.id || "<missing-id>";
  if (!module.id || ids.has(module.id)) {
    failures.push(`模块 ID 缺失或重复: ${label}`);
  }
  ids.add(module.id);

  if (!module.guide || guides.has(module.guide)) {
    failures.push(`模块 ${label} 的 guide 缺失或重复`);
  }
  guides.add(module.guide);

  if (!Array.isArray(module.sourcePaths) || module.sourcePaths.length === 0) {
    failures.push(`模块 ${label} 没有 sourcePaths`);
  }
  for (const sourcePath of module.sourcePaths ?? []) {
    requirePath(sourcePath, `模块 ${label} 的源码路径`);
  }

  if (!Array.isArray(module.acceptance) || module.acceptance.length === 0) {
    failures.push(`模块 ${label} 没有可执行的 acceptance 命令`);
  }

  if (!module.guide || !requirePath(module.guide, `模块 ${label} 的指南`)) {
    continue;
  }
  const guide = fs.readFileSync(absolute(module.guide), "utf8");
  for (const heading of moduleHeadings) {
    if (!guide.includes(heading)) {
      failures.push(`模块 ${label} 的指南缺少章节: ${heading}`);
    }
  }
}

const exampleDirectory = absolute("examples/modules");
const exampleSources = fs.existsSync(exampleDirectory)
  ? fs
      .readdirSync(exampleDirectory)
      .filter((name) => /\.(json|ya?ml)$/i.test(name))
      .map((name) => path.posix.join("examples/modules", name))
      .sort()
  : [];
const mappedSources = new Set(manifest.appModuleSources ?? []);
for (const source of exampleSources) {
  if (!mappedSources.has(source)) {
    failures.push(`示例 AppModule 尚未登记对应指南: ${source}`);
  }
}
for (const source of mappedSources) {
  requirePath(source, "已登记的示例 AppModule");
}

finish();

function finish() {
  if (failures.length > 0) {
    console.error("Panvara 文档约束检查失败:");
    for (const failure of failures) {
      console.error(`- ${failure}`);
    }
    process.exit(1);
  }
  console.log(
    `Panvara 文档约束检查通过：${manifest.modules.length} 个模块指南，${exampleSources.length} 个示例 AppModule。`,
  );
  process.exit(0);
}
