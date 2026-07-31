# Sources

> 返回 [README](../README.md)。

Source 是一项持久化的信息来源配置，描述“内容从哪里来”以及“Pulse 应该怎样取得内容”。Source 通常对应一个会持续产生或接收多条内容的入口，并不等于一篇文章或一个待收藏网页。

例如，一个 RSS Source 对应一个 Feed，之后发布的新文章都会进入同一个 Source；一个 Manual Source 则类似网页收藏夹，可以接收来自不同网站的任意多个网页。使用 Bookmarklet 保存十个不同网页时，不需要创建十个 Source，只需要把它们保存到同一个 Manual Source，它们会成为十个独立的 Entry。

## 支持的 Source 类型

| 类型 | Source 中填写或绑定的入口 | 适用场景 | 是否需要为每篇内容创建 Source |
| --- | --- | --- | --- |
| RSS / Atom / JSON Feed (`rss`) | Feed URL，例如 `https://example.com/feed.xml` | 订阅博客、新闻站和播客等标准 Feed；Pulse 会持续检查新增或更新的条目 | 不需要；通常每个 Feed 创建一个 Source |
| JSON API (`json-api`) | 返回内容列表的 HTTP API URL | 从结构化 JSON 接口摄取内容；支持字段映射及页码、下一页 URL、游标分页 | 不需要；通常每个 API 数据集或查询创建一个 Source |
| Static HTML (`html`) | 公开网页 URL | 从没有 Feed/API 的网页提取内容；列表模式可从一个页面提取多条内容，单文档模式用于持续关注一个页面 | 列表页面不需要；单文档模式通常每个需要独立跟踪的页面创建一个 Source |
| Webhook (`webhook`) | Pulse 为该 Source 提供的 Webhook 接收地址和独立密钥 | 由外部系统主动向 Pulse 推送内容 | 不需要；通常每个外部系统或推送用途创建一个 Source |
| Manual Source (`manual`) | 一个收藏集合标识，不是待收藏网页 URL | 通过 Bookmarklet 或 API 手工保存任意网站的网页和内容 | 不需要；一个“网页收藏”Source 可以保存任意多个网页 |
| Local files (`file`) | `PULSE_IMPORT_ROOTS` 白名单内的 Markdown 或 HTML 文件路径 | 将本机或挂载目录中的单个文档摄取为 Entry | 当前每个被跟踪的文件创建一个 Source |
| Book Annotations (`annotations`) | Apple Books、Kindle 或其他阅读器的批注集合 | 保存高亮原文、书籍、章节、位置、颜色和来源笔记；每条 Annotation 成为一条 Entry | 不需要；通常每个阅读平台创建一个 Source |

HTTP Source 目前只接受 `http://` 或 `https://` 地址。File Source 只允许读取 `PULSE_IMPORT_ROOTS` 下的 `.md`、`.markdown`、`.html` 或 `.htm` 文件。

## 创建方式

当前网页端的“添加信息源”向导直接支持 RSS、JSON API 和 Static HTML，并会在保存前执行测试与预览。Manual Source 可以在首次使用 Bookmarklet 时由保存确认页创建；Book Annotations Source 可以在“阅读笔记”首次导入时创建。Webhook、File 以及更复杂的配置可以通过 HTTP API 创建；接口契约见 [`api/openapi.yaml`](../api/openapi.yaml)。
