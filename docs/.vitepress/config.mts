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

const docsBase = process.env.PANVARA_DOCS_BASE ?? "/";
const docsSourceBranch = process.env.PANVARA_DOCS_SOURCE_BRANCH ?? "main";
const docsSiteUrl =
  process.env.PANVARA_DOCS_SITE_URL ?? "https://shezw.github.io/panvara/";

const publicNavigation = [
  { text: "概览", link: "/" },
  { text: "快速开始", link: "/getting-started/" },
  { text: "使用指南", link: "/guides/" },
  { text: "API 与配置", link: "/reference/http-api" },
  { text: "版本与兼容", link: "/releases/status" },
  { text: "参与贡献", link: "/contributing/" },
];

export default withMermaid(
  defineConfig({
    base: docsBase,
    lang: "zh-CN",
    title: "Panvara",
    titleTemplate: ":title · Panvara",
    description: "数据模型驱动的可组合全栈框架",
    cleanUrls: true,
    lastUpdated: true,
    srcExclude: [
      ".agent/**",
      "**/.agent/**",
      "_meta/**",
      "_templates/**",
    ],
    sitemap: {
      hostname: docsSiteUrl,
    },
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
      nav: publicNavigation,
      sidebar: {
        "/getting-started/": [
          {
            text: "快速开始",
            items: [
              { text: "10 分钟启动 Lite", link: "/getting-started/" },
            ],
          },
        ],
        "/guides/": [
          {
            text: "Current Distribution",
            items: [
              { text: "使用指南", link: "/guides/" },
              { text: "认识 Panvara", link: "/guides/concepts" },
              { text: "运行 Server", link: "/guides/server" },
              { text: "定义 AppModule", link: "/guides/appmodule" },
              { text: "CRM Leads", link: "/guides/crm-leads" },
              { text: "操作 Record", link: "/guides/records" },
            ],
          },
          {
            text: "Source Preview",
            items: [
              { text: "访问管理预览", link: "/guides/access-preview" },
              { text: "模型变更预览", link: "/guides/model-change-preview" },
            ],
          },
        ],
        "/reference/": [
          {
            text: "API 与配置",
            items: [
              { text: "HTTP API", link: "/reference/http-api" },
              { text: "配置参考", link: "/reference/configuration" },
              { text: "命令参考", link: "/reference/commands" },
              { text: "故障排查", link: "/reference/troubleshooting" },
            ],
          },
        ],
        "/releases/": [
          {
            text: "版本与兼容",
            items: [
              { text: "能力状态", link: "/releases/status" },
              { text: "数据与升级", link: "/releases/compatibility" },
            ],
          },
        ],
        "/contributing/": [
          {
            text: "参与贡献",
            items: [
              { text: "贡献指南", link: "/contributing/" },
            ],
          },
        ],
      },
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
        pattern: `https://github.com/shezw/panvara/edit/${docsSourceBranch}/docs/:path`,
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
