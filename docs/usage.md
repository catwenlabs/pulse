# 使用指南

> 返回 [README](../README.md)。

## 首次使用

根据目标选择其中一条路径。

### 订阅持续更新的内容

1. 打开 `http://localhost:8080`，选择“添加信息源”。
2. 选择 RSS、JSON API 或 Static HTML。
3. 填写便于识别的 Source 名称，以及对应的 Feed、API 或网页入口地址。
4. JSON API 需要配置字段映射；Static HTML 需要配置页面模式和 CSS Selector。
5. 选择“测试与预览”，确认 Pulse 能取得内容且标题、链接等字段正确。
6. 保存并启用 Source。之后 Pulse 会持续执行 Acquisition，并把取得的内容统一为阅读流中的 Entry。

如果要订阅另一个 Feed、API 数据集或独立 HTML 入口，再为它创建新的 Source；同一入口产生的每篇内容不需要单独创建 Source。Source 的详细说明见 [Sources](./sources.md)。

### 手工收藏当前网页

1. 打开左侧导航底部的“安装保存书签”。
2. 按下面的 Mac 或 iPhone 步骤安装 Bookmarklet。
3. 在要收藏的网页上运行“保存到 Pulse”。
4. 首次保存时创建“网页收藏”Manual Source；以后继续选择同一个 Source。
5. Pulse 在后台抓取并提取网页正文；每个网页会成为该 Manual Source 下的一条独立 Entry。

## 使用 Bookmarklet 保存网页

Pulse 提供类似 Instapaper 的轻量网页收藏入口，无需安装浏览器扩展。Mac 和 iPhone Chrome 使用同一段 Bookmarklet 代码。

**Mac Chrome：**

1. 在 Pulse 左侧导航底部选择“安装保存书签”，复制完整 Bookmarklet 代码。
2. 新建名为“保存到 Pulse”的书签，并将代码粘贴到书签地址。
3. 浏览网页时点击书签栏中的“保存到 Pulse”。

**iPhone Chrome：**

1. 打开 Pulse 的“安装保存书签”弹窗并复制完整代码。
2. 先将任意网页添加到 Chrome 书签。
3. 长按刚创建的书签并选择“编辑”，名称改为“保存到 Pulse”。
4. 将书签 URL 替换为复制的完整代码并保存。
5. 打开要收藏的网页，再从 Chrome 书签中点击“保存到 Pulse”。

随后在打开的 Pulse 页面中确认或修改标题、URL 和目标 Manual Source，然后选择“保存网页”。

首次使用时，如果还没有 Manual Source，可以在确认页面创建“网页收藏”并立即保存。已暂停的 Manual Source 被选中后会自动重新启用。保存请求进入统一摄取管道，后端通过受控 HTTP Client 获取页面，使用 Readability 提取主要正文并清理不安全 HTML，完成后会作为可在 Pulse 内阅读的 Entry 出现在阅读流中。标题、作者、摘要和发布时间会在页面能够提供时一并补全。

如果目标站点需要登录、主要依赖 JavaScript 渲染、拒绝服务器访问，或者没有可识别的文章正文，Pulse 仍会保留标题和原始 URL，不会因为正文提取失败而丢失收藏。当前保存的是清理后的文章正文快照，不是包含脚本、样式和所有资源的完整网页归档；外部图片仍可能依赖原站可访问。

Bookmarklet 仅接受 `http://` 和 `https://` 网页。它会绑定安装时所使用的 Pulse 地址；如果之后更换域名、端口或部署路径，请从新地址重新安装。浏览器必须能够访问该 Pulse 实例，因此局域网或本机地址无法从网络外部直接使用。

## 导入阅读批注

“阅读笔记”用于保存 Apple Books、Kindle 和其他阅读器产生的高亮与批注。一个阅读平台通常只需要一个 Book Annotations Source；每条高亮会成为该 Source 下独立且可搜索的 Entry，Pulse 按书籍自动分组。

1. 在左侧导航选择“阅读笔记”。
2. 选择“导入批注”。
3. 选择来源平台，填写书名、高亮原文，以及可选的作者、章节、位置、颜色和原始批注。
4. 选择“加入导入队列”。首次导入会自动创建对应的 Book Annotations Source。
5. Worker 完成 Acquisition 后，重新进入“阅读笔记”即可按书查看。

相同平台、书籍和位置的批注重复导入时会更新现有 Entry；没有稳定位置时，Pulse 使用书籍与高亮文本指纹去重。来自阅读器的原始批注与用户后来在 Pulse 中追加的 Note 分开保存，重新导入不会覆盖 Pulse Note。

当前界面提供结构化单条导入，HTTP API 支持一次提交最多 500 条 Annotation。Apple Books 分享文本和 Kindle `My Clippings.txt` 的批量解析需要基于真实、脱敏的导出样本验证格式后接入，避免依赖未经验证的私有格式。

## 排查服务状态

如果服务没有就绪：

```sh
docker compose ps
docker compose logs --tail=200 pulse
curl --fail --show-error http://localhost:8080/healthz
```
