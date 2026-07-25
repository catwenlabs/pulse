# Product Vision

## Product

Pulse 是部署在个人 Linux 服务器上的单用户信息阅读中枢。它把 RSS、API、静态网页、Webhook、手工收藏和本地文件统一转换为可搜索、可过滤、可阅读的 Entry。

## Primary User

在本机或局域网浏览器中使用 Pulse 的单个用户。第一阶段不包含账号、登录、权限、共享和协作。

## Core Value

- 在一个收件箱中持续接收不同来源的内容。
- 通过可视化向导配置自定义来源，不要求编写代码。
- 可靠地区分新内容与已有内容更新。
- 使用结构化规则自动分类、隐藏、收藏和通知。
- 保留个人阅读资料，不因来源删除或短暂异常丢失内容。

## First Release

第一版支持 RSS/Atom/JSON Feed、JSON API、静态 HTML、JSON Webhook、Manual Source 和 Markdown/HTML File Source。动态网页、邮件、PDF/EPUB、OAuth 平台、插件、AI 和原生 App 不在第一版范围内。

## Experience Principles

- 配置必须可测试、可预览、可诊断。
- 抓取失败不得破坏已有数据或前移 Checkpoint。
- Source 之间隔离；每个入口必须绑定显式创建的 Source。
- 默认部署简单：Pulse 与 PostgreSQL 两个容器即可运行。
