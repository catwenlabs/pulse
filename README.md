<div align="center">

# ⚡ Pulse

### Your self-hosted information pulse.

把 RSS、API、网页、Webhook 和本地文件汇聚成一个可搜索、可自动整理的个人阅读中枢。

[![CI](https://github.com/wenpengfei/pulse/actions/workflows/ci.yml/badge.svg)](https://github.com/wenpengfei/pulse/actions/workflows/ci.yml)
[![GHCR](https://img.shields.io/badge/GHCR-pulse-2496ED?logo=docker&logoColor=white)](https://github.com/users/wenpengfei/packages/container/package/pulse)
![Go](https://img.shields.io/badge/Go-1.25-00ADD8?logo=go&logoColor=white)
![React](https://img.shields.io/badge/React-19-61DAFB?logo=react&logoColor=black)
![PostgreSQL](https://img.shields.io/badge/PostgreSQL-17-4169E1?logo=postgresql&logoColor=white)

</div>

---

Pulse 是面向单用户、本机或局域网部署的信息阅读中枢。它将不同来源的内容统一为可搜索、可过滤、可阅读的 Entry，并使用结构化规则自动整理你的信息流。所有状态保存在自己的 PostgreSQL 中。

## Why Pulse?

- 📡 **统一摄取** — RSS、API、网页、Webhook、手工条目和本地文件进入同一条可靠管道。
- 🧭 **高效阅读** — 紧凑信息流、来源筛选、原位展开、全文搜索和批量已读。
- 🪄 **自动整理** — 使用结构化规则添加标签、收藏、隐藏、稍后读或发送通知。
- 🛡️ **数据自有** — 单用户、自托管；来源凭据加密存储，主密钥始终位于数据库之外。
- 🧱 **简单部署** — 一个 Pulse 镜像加一个 PostgreSQL，即可运行完整系统。

## Features

### Ingest anything

| 来源 | 能力 |
| --- | --- |
| RSS / Atom / JSON Feed | 条件请求、富文本、Media RSS 和图片 Enclosure |
| JSON API | 字段映射，以及页码、下一页 URL 和游标分页 |
| Static HTML | 单文档或列表提取、CSS Selector 映射 |
| Webhook | 每个 Source 独立密钥、幂等接收 |
| Manual Source | 手工保存链接和内容 |
| Local files | 从只读白名单目录摄取 Markdown 和 HTML |

### Read on your terms

- 统一收件箱、Folder、标签和持久化 View
- 已读、收藏、隐藏、稍后读、显示标题和个人笔记
- 安全保留标题、段落、列表、引用、代码、表格、链接和图片
- PostgreSQL 全文搜索与模糊搜索
- Source 归档时保留历史 Entry 和阅读状态
- OPML、脱敏配置 JSON 和单篇 Markdown 导出

## Quick Start

要求安装 Docker 与 Docker Compose。

```sh
git clone https://github.com/wenpengfei/pulse.git
cd pulse

export PULSE_MASTER_KEY="$(openssl rand -base64 32)"
docker compose up --build -d
```

打开 [http://localhost:8080](http://localhost:8080)，健康检查位于 [`/healthz`](http://localhost:8080/healthz)。

> [!IMPORTANT]
> 请将 `PULSE_MASTER_KEY` 保存在服务器的私密环境文件或 Secret 管理器中。丢失该密钥后，将无法解密已保存的来源认证 Header。

未配置主密钥时仍可使用无凭据来源，但 Pulse 会拒绝保存包含 Token、Cookie、密码或认证 Header 的配置。默认 Compose 仅运行 Pulse 和 PostgreSQL；同一 Pulse 镜像同时承担 Web、Scheduler、Acquisition Worker 和 Effect Worker 角色。

### Source 是什么

Source 是一项持久化的信息来源配置，描述“内容从哪里来”以及“Pulse 应该怎样取得内容”。Source 通常对应一个会持续产生或接收多条内容的入口，并不等于一篇文章或一个待收藏网页。

例如，一个 RSS Source 对应一个 Feed，之后发布的新文章都会进入同一个 Source；一个 Manual Source 则类似网页收藏夹，可以接收来自不同网站的任意多个网页。使用 Bookmarklet 保存十个不同网页时，不需要创建十个 Source，只需要把它们保存到同一个 Manual Source，它们会成为十个独立的 Entry。

### 支持的 Source 类型

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

当前网页端的“添加信息源”向导直接支持 RSS、JSON API 和 Static HTML，并会在保存前执行测试与预览。Manual Source 可以在首次使用 Bookmarklet 时由保存确认页创建；Book Annotations Source 可以在“阅读笔记”首次导入时创建。Webhook、File 以及更复杂的配置可以通过 HTTP API 创建；接口契约见 [`api/openapi.yaml`](api/openapi.yaml)。

### 首次使用

根据目标选择其中一条路径：

#### 订阅持续更新的内容

1. 打开 `http://localhost:8080`，选择“添加信息源”。
2. 选择 RSS、JSON API 或 Static HTML。
3. 填写便于识别的 Source 名称，以及对应的 Feed、API 或网页入口地址。
4. JSON API 需要配置字段映射；Static HTML 需要配置页面模式和 CSS Selector。
5. 选择“测试与预览”，确认 Pulse 能取得内容且标题、链接等字段正确。
6. 保存并启用 Source。之后 Pulse 会持续执行 Acquisition，并把取得的内容统一为阅读流中的 Entry。

如果要订阅另一个 Feed、API 数据集或独立 HTML 入口，再为它创建新的 Source；同一入口产生的每篇内容不需要单独创建 Source。

#### 手工收藏当前网页

1. 打开左侧导航底部的“安装保存书签”。
2. 按下面的 Mac 或 iPhone 步骤安装 Bookmarklet。
3. 在要收藏的网页上运行“保存到 Pulse”。
4. 首次保存时创建“网页收藏”Manual Source；以后继续选择同一个 Source。
5. Pulse 在后台抓取并提取网页正文；每个网页会成为该 Manual Source 下的一条独立 Entry。

### 使用 Bookmarklet 保存网页

Pulse 提供类似 Instapaper 的轻量网页收藏入口，无需安装浏览器扩展。Mac 和 iPhone Chrome 使用同一段 Bookmarklet 代码。

在 Mac Chrome 上：

1. 在 Pulse 左侧导航底部选择“安装保存书签”，复制完整 Bookmarklet 代码。
2. 新建名为“保存到 Pulse”的书签，并将代码粘贴到书签地址。
3. 浏览网页时点击书签栏中的“保存到 Pulse”。

在 iPhone Chrome 上：

1. 打开 Pulse 的“安装保存书签”弹窗并复制完整代码。
2. 先将任意网页添加到 Chrome 书签。
3. 长按刚创建的书签并选择“编辑”，名称改为“保存到 Pulse”。
4. 将书签 URL 替换为复制的完整代码并保存。
5. 打开要收藏的网页，再从 Chrome 书签中点击“保存到 Pulse”。

随后在打开的 Pulse 页面中确认或修改标题、URL 和目标 Manual Source，然后选择“保存网页”。

首次使用时，如果还没有 Manual Source，可以在确认页面创建“网页收藏”并立即保存。已暂停的 Manual Source 被选中后会自动重新启用。保存请求进入统一摄取管道，后端通过受控 HTTP Client 获取页面，使用 Readability 提取主要正文并清理不安全 HTML，完成后会作为可在 Pulse 内阅读的 Entry 出现在阅读流中。标题、作者、摘要和发布时间会在页面能够提供时一并补全。

如果目标站点需要登录、主要依赖 JavaScript 渲染、拒绝服务器访问，或者没有可识别的文章正文，Pulse 仍会保留标题和原始 URL，不会因为正文提取失败而丢失收藏。当前保存的是清理后的文章正文快照，不是包含脚本、样式和所有资源的完整网页归档；外部图片仍可能依赖原站可访问。

Bookmarklet 仅接受 `http://` 和 `https://` 网页。它会绑定安装时所使用的 Pulse 地址；如果之后更换域名、端口或部署路径，请从新地址重新安装。浏览器必须能够访问该 Pulse 实例，因此局域网或本机地址无法从网络外部直接使用。

### 导入阅读批注

“阅读笔记”用于保存 Apple Books、Kindle 和其他阅读器产生的高亮与批注。一个阅读平台通常只需要一个 Book Annotations Source；每条高亮会成为该 Source 下独立且可搜索的 Entry，Pulse 按书籍自动分组。

1. 在左侧导航选择“阅读笔记”。
2. 选择“导入批注”。
3. 选择来源平台，填写书名、高亮原文，以及可选的作者、章节、位置、颜色和原始批注。
4. 选择“加入导入队列”。首次导入会自动创建对应的 Book Annotations Source。
5. Worker 完成 Acquisition 后，重新进入“阅读笔记”即可按书查看。

相同平台、书籍和位置的批注重复导入时会更新现有 Entry；没有稳定位置时，Pulse 使用书籍与高亮文本指纹去重。来自阅读器的原始批注与用户后来在 Pulse 中追加的 Note 分开保存，重新导入不会覆盖 Pulse Note。

当前界面提供结构化单条导入，HTTP API 支持一次提交最多 500 条 Annotation。Apple Books 分享文本和 Kindle `My Clippings.txt` 的批量解析需要基于真实、脱敏的导出样本验证格式后接入，避免依赖未经验证的私有格式。

如果服务没有就绪：

```sh
docker compose ps
docker compose logs --tail=200 pulse
curl --fail --show-error http://localhost:8080/healthz
```

## How It Works

```mermaid
flowchart LR
    S["RSS · API · Web · Webhook · Files"] --> A["Unified Ingestion"]
    A --> N["Normalize & Deduplicate"]
    N --> R["Rules"]
    R --> DB[("PostgreSQL")]
    DB --> UI["Reader · Search · Views"]
```

Pulse 采用模块化单体架构，PostgreSQL 同时承载领域数据、任务队列、Lease、Checkpoint 和 Effect Outbox。每次成功摄取会原子提交 Entry、规则结果、Effect 与 Checkpoint，失败不会破坏已有数据或推进外部进度。

阅读完整的[架构说明](docs/architecture.md)、[领域语言](CONTEXT.md)与[架构决策记录](docs/adr/)。

## Container Image

CI 会在 `main` 更新后将镜像发布到 GitHub Container Registry：

```sh
docker pull ghcr.io/wenpengfei/pulse:main
```

可用标签包括浮动的 `main`，以及与提交对应的不可变 `sha-<commit>`。生产环境建议使用 SHA 标签或镜像 digest。

## Deployment

当前 `compose.yaml` 从仓库根目录的 `Dockerfile` 构建 Pulse，适合单机 Linux 服务器和局域网部署。

### First deployment

```sh
git clone https://github.com/wenpengfei/pulse.git
cd pulse

# 只在首次部署时生成，并在仓库之外持久保存。
export PULSE_MASTER_KEY="$(openssl rand -base64 32)"

docker compose pull postgres
docker compose up --build -d
curl --retry 20 --retry-delay 2 --retry-connrefused \
  --fail --show-error http://localhost:8080/healthz
```

生产环境必须通过服务管理器、Secret 管理器或仓库外的只读环境文件注入 `PULSE_MASTER_KEY`。不要把密钥写入 README、Compose、提交记录或仓库内未忽略的文件。

### Upgrade

```sh
make backup
git pull --ff-only
docker compose build --pull pulse
docker compose up -d --remove-orphans
curl --retry 20 --retry-delay 2 --retry-connrefused \
  --fail --show-error http://localhost:8080/healthz
```

升级前备份，升级后检查健康状态、Source 列表和 Entry 数量。应用启动时会自动应用尚未执行的数据库迁移。

### Operations

```sh
# 查看状态
docker compose ps

# 跟踪应用日志
docker compose logs -f --tail=200 pulse

# 重启应用，不重启数据库
docker compose restart pulse

# 停止完整应用并保留数据库卷
docker compose down
```

> [!WARNING]
> 不要执行 `docker compose down -v`。`-v` 会删除包含 Pulse 数据的 PostgreSQL 命名卷。

## Backup & Restore

创建并校验 PostgreSQL 逻辑备份：

```sh
make backup
make backup-verify FILE=backups/pulse-YYYYMMDDTHHMMSSZ.sql.gz
```

默认保留 14 天，可通过 `PULSE_BACKUP_RETENTION_DAYS` 修改。Linux 服务器可以使用系统 cron 定期运行 `make backup`，并将失败输出交给现有告警系统。

恢复前先停止 Pulse 写入，并确认备份通过 `gzip -t`：

```sh
docker compose stop pulse
gzip -dc backups/pulse-YYYYMMDDTHHMMSSZ.sql.gz |
  docker compose exec -T postgres psql --username=pulse --dbname=pulse
docker compose start pulse
```

恢复属于覆盖性操作。执行前应保留当前数据库备份，完成后检查 `/healthz`、Source 列表和 Entry 数量。

## Export

```sh
curl -O http://localhost:8080/api/v1/opml/export
make export-config
make export-entry ID=ENTRY_UUID
```

配置导出包含 Source、Rule 和 View，但认证 Header、Token、Cookie、密码和 Webhook 密钥会被脱敏。

## Local Development

一键启动 PostgreSQL、Go 后端和带 HMR 的 Vite 开发服务器：

```sh
make dev
```

首次运行且 `web/node_modules` 不存在时会自动安装前端依赖。访问
[http://localhost:5173](http://localhost:5173)，按 `Ctrl+C` 停止本次启动的服务。

也可以分别启动各个组件。只在 Docker 中启动 PostgreSQL，并将其暴露到本机 `54321`：

```sh
make dev-db-up
```

在本机运行 Go 后端：

```sh
make dev-api
```

首次安装前端依赖，然后启动带有 HMR 的 Vite 开发服务器：

```sh
make dev-web-install
make dev-web
```

访问 [http://localhost:5173](http://localhost:5173)。开发服务器会将 `/api` 和 `/healthz` 请求代理到本机 `8080` 端口。

停止本地开发数据库：

```sh
make dev-db-down
```

查看 PostgreSQL 日志可运行 `make dev-db-logs`。本地数据库地址和导入目录具有安全默认值，也可以显式覆盖：

```sh
make dev-api \
  DEV_DATABASE_URL='postgres://pulse:pulse@127.0.0.1:54321/pulse?sslmode=disable' \
  DEV_IMPORT_ROOTS='./imports'
```

### Environment variables

| 变量 | 容器默认值 | 用途 |
| --- | --- | --- |
| `PULSE_DATABASE_URL` | `postgres://pulse:pulse@postgres:5432/pulse?sslmode=disable` | PostgreSQL 连接地址 |
| `PULSE_HTTP_ADDR` | `127.0.0.1:8080`（容器内覆盖为 `:8080`） | HTTP 监听地址 |
| `PULSE_ROLES` | `web,scheduler,worker,effect-worker` | 当前进程启用的运行角色 |
| `PULSE_MASTER_KEY` | 空 | Base64 编码的 32 字节来源凭据主密钥 |
| `PULSE_WEB_DIR` | `/web` | 已构建前端静态文件目录 |
| `PULSE_IMPORT_ROOTS` | `/data/imports` | File Source 允许读取的路径列表 |
| `PULSE_EMBEDDING_PROVIDER` | `disabled` | Story 语义聚合 Provider；可设为 `ollama` |
| `PULSE_EMBEDDING_BASE_URL` | `http://127.0.0.1:11434` | Embedding 服务基础地址，不包含 `/api/embed` |
| `PULSE_EMBEDDING_MODEL` | `qwen3-embedding` | Embedding 模型名称 |

本机使用 Vite 时，浏览器访问 `5173`，Go 后端只需在 `8080` 提供 API；正式镜像则由 Go 服务直接提供 `/web` 中的前端静态资源。

Story 聚合在未启用 embedding 时仍使用 URL、标题和正文指纹。启用本地 Ollama：

```sh
ollama pull qwen3-embedding
export PULSE_EMBEDDING_PROVIDER=ollama
export PULSE_EMBEDDING_BASE_URL=http://127.0.0.1:11434
export PULSE_EMBEDDING_MODEL=qwen3-embedding
```

Pulse 在 Ollama 不可用时自动使用传统文本算法；Entry 摄取和 Checkpoint 不受影响。
容器内的 `127.0.0.1` 指向 Pulse 容器本身，Compose 部署时应把
`PULSE_EMBEDDING_BASE_URL` 设置为可从 Pulse 容器访问的 Ollama 服务地址。

Pulse 当前不提供内置用户认证，并且阅读批注可能包含敏感内容。直接运行程序和默认 Compose 端口映射都只对本机开放；容器内部仍监听 `:8080`。不要将该端口直接暴露到公网；需要让 iPhone 等其他设备访问时，应通过可信 VPN 或带认证的反向代理开放，而不是直接改成公网监听。

参与开发或使用 AI 编码 Agent 前，请阅读 [AGENTS.md](AGENTS.md)。其中包含仓库结构、架构边界、验证矩阵和数据安全规则。

## Verification

```sh
make test
make test-race
make vet
make compose-config
```

RSS 浏览器端到端流程需要先启动完整应用，再运行：

```sh
make e2e
```

GitHub Actions 会在 Pull Request 和 `main` 推送时执行后端、前端与容器验证；只有全部通过后才发布 GHCR 镜像。

## Documentation

- [OpenAPI contract](api/openapi.yaml)
- [Architecture](docs/architecture.md)
- [Domain language](CONTEXT.md)
- [Architecture decisions](docs/adr/)
- [GitHub Container Registry](https://github.com/users/wenpengfei/packages/container/package/pulse)

---

<div align="center">

Built with Go, React and PostgreSQL.

</div>
