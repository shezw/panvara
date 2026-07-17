/*
   Panvara
   docs/.vitepress/config.mts    2026-07-15
    ______     __  __     ______     ______     __     __
   /\  ___\   /\ \_\ \   /\  ___\   /\___  \   /\ \  _ \ \
   \ \___  \  \ \  __ \  \ \  __\   \/_/  /__  \ \ \/ ".\ \
    \/\_____\  \ \_\ \_\  \ \_____\   /\_____\  \ \__/".~\_\
     \/_____/   \/_/\/_/   \/_____/   \/_____/   \/_/   \/_/.com

   @link    : https://github.com/shezw/panvara
   @author  : shezw
   @email   : hello@shezw.com
*/

import { defineConfig } from "vitepress";
import { withMermaid } from "vitepress-plugin-mermaid";

export default withMermaid(
  defineConfig({
    lang: "zh-CN",
    title: "Panvara",
    titleTemplate: ":title · Panvara",
    description: "数据模型驱动的可组合全栈框架",
    cleanUrls: true,
    lastUpdated: true,
    srcExclude: ["_meta/**", "_templates/**"],
    head: [
      ["meta", { name: "theme-color", content: "#635bff" }],
      ["meta", { name: "viewport", content: "width=device-width, initial-scale=1" }],
    ],
    markdown: {
      lineNumbers: true,
    },
    mermaid: {
      securityLevel: "strict",
    },
    themeConfig: {
      siteTitle: "Panvara",
      nav: [
        { text: "开始", link: "/getting-started/" },
        { text: "模块", link: "/modules/" },
        { text: "参考", link: "/reference/commands" },
        { text: "架构", link: "/arch" },
        {
          text: "v0.1.0-alpha.2",
          items: [
            { text: "当前版本边界", link: "/core-v0" },
            { text: "测试体系", link: "/testing" },
          ],
        },
      ],
      sidebar: [
        {
          text: "开始",
          items: [
            { text: "使用与验收 Guideline", link: "/getting-started/" },
            { text: "安装开发工具", link: "/getting-started/prerequisites" },
            { text: "创建本地环境", link: "/getting-started/local-environment" },
            { text: "编译并运行 Lite", link: "/getting-started/build-and-lite" },
            { text: "CRM Leads 完整验收", link: "/getting-started/crm-leads-acceptance" },
            { text: "Revision Registry 验收", link: "/getting-started/revision-registry-acceptance" },
            { text: "Draft 与 Plan 验收", link: "/getting-started/draft-plan-acceptance" },
            { text: "故障排查", link: "/getting-started/troubleshooting" },
          ],
        },
        {
          text: "模块指南",
          items: [
            { text: "模块总览", link: "/modules/" },
            { text: "CRM Leads 示例", link: "/modules/crm-leads" },
            { text: "AppModule", link: "/modules/appmodule" },
            { text: "Revision Registry", link: "/modules/revision-registry" },
            { text: "Draft 与 Change Plan", link: "/modules/draft-planning" },
            { text: "Record Runtime", link: "/modules/record-runtime" },
            { text: "HTTP API", link: "/modules/http-api" },
            { text: "运行模式", link: "/modules/runtime-profiles" },
            { text: "项目上下文", link: "/modules/project-context" },
          ],
        },
        {
          text: "参考",
          items: [
            { text: "命令参考", link: "/reference/commands" },
            { text: "配置参考", link: "/reference/configuration" },
          ],
        },
        {
          text: "参与开发",
          items: [
            { text: "开发环境", link: "/development" },
            { text: "文档同步规范", link: "/contributing/documentation" },
            { text: "模块指南模板", link: "/contributing/module-guide-template" },
            { text: "验证测试框架", link: "/testing" },
            { text: "总体架构", link: "/arch" },
            { text: "Core v0 路线", link: "/core-v0" },
            { text: "ADR-0001 身份拆分", link: "/adr/0001-module-data-revision-identities" },
            { text: "ADR-0002 不可变 Registry", link: "/adr/0002-immutable-revision-registry" },
            { text: "ADR-0003 Draft 与 Plan", link: "/adr/0003-draft-validation-change-plan" },
          ],
        },
      ],
      search: {
        provider: "local",
        options: {
          locales: {
            root: {
              translations: {
                button: {
                  buttonText: "搜索文档",
                  buttonAriaLabel: "搜索文档",
                },
                modal: {
                  noResultsText: "没有找到相关内容",
                  resetButtonTitle: "清除搜索",
                  footer: {
                    selectText: "选择",
                    navigateText: "切换",
                    closeText: "关闭",
                  },
                },
              },
            },
          },
        },
      },
      outline: {
        level: [2, 3],
        label: "本页内容",
      },
      editLink: {
        pattern: "https://github.com/shezw/panvara/edit/main/docs/:path",
        text: "在 GitHub 上改进本页",
      },
      lastUpdatedText: "最后更新",
      docFooter: {
        prev: "上一页",
        next: "下一页",
      },
      darkModeSwitchLabel: "外观",
      lightModeSwitchTitle: "切换到浅色模式",
      darkModeSwitchTitle: "切换到深色模式",
      sidebarMenuLabel: "目录",
      returnToTopLabel: "返回顶部",
      socialLinks: [{ icon: "github", link: "https://github.com/shezw/panvara" }],
      footer: {
        message: "以 MIT License 发布",
        copyright: "Copyright © 2026 Panvara",
      },
    },
  }),
);
