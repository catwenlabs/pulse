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

### First-use flow

1. 打开 `http://localhost:8080`。
2. 选择“添加信息源”。
3. 输入名称和来源地址。
4. 先执行测试与预览，再保存并启用 Source。
5. Source 成功摄取后，在阅读流中查看 Entry。

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

只在 Docker 中启动 PostgreSQL，并将其暴露到本机 `54321`。该命令不会构建或启动 Pulse：

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
| `PULSE_HTTP_ADDR` | `:8080` | HTTP 监听地址 |
| `PULSE_ROLES` | `web,scheduler,worker,effect-worker` | 当前进程启用的运行角色 |
| `PULSE_MASTER_KEY` | 空 | Base64 编码的 32 字节来源凭据主密钥 |
| `PULSE_WEB_DIR` | `/web` | 已构建前端静态文件目录 |
| `PULSE_IMPORT_ROOTS` | `/data/imports` | File Source 允许读取的路径列表 |

本机使用 Vite 时，浏览器访问 `5173`，Go 后端只需在 `8080` 提供 API；正式镜像则由 Go 服务直接提供 `/web` 中的前端静态资源。

参与开发或使用 AI 编码 Agent 前，请阅读 [AGENTS.md](AGENTS.md)。其中包含仓库结构、架构边界、验证矩阵和数据安全规则。

## Verification

```sh
make test
make test-race
make vet
docker compose config --quiet
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
