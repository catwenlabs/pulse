<div align="center">

# ⚡ Pulse

### Your self-hosted information pulse.

把 RSS、API、网页、Webhook 和本地文件汇聚成一个可搜索、可自动整理的个人阅读中枢。

[![CI](https://github.com/catwenlabs/pulse/actions/workflows/ci.yml/badge.svg)](https://github.com/catwenlabs/pulse/actions/workflows/ci.yml)
[![GHCR](https://img.shields.io/badge/GHCR-pulse-2496ED?logo=docker&logoColor=white)](https://github.com/users/catwenlabs/packages/container/package/pulse)
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

- **Ingest anything** — RSS/Atom/JSON Feed、JSON API、Static HTML、Webhook、手工条目、本地文件、阅读批注，统一为 Entry。
- **Read on your terms** — 收件箱、Folder、标签、持久化 View；已读、收藏、隐藏、稍后读、笔记；PostgreSQL 全文与模糊搜索；OPML / 脱敏配置 / Markdown 导出。
- **Cluster across sources** — 同一新闻的多个来源自动聚合成一条 Story；跨源合并需启用 Ollama embedding（默认关闭），配置见[部署与运维](docs/operations.md#story-语义聚合可选)。
- **AI catch-up** — 用户主动生成 StorySummary 或标题级未读追更 Digest；支持 OpenAI-compatible Provider、DeepSeek、OpenRouter、Qwen 与 Ollama，结果保留可回到来源 Story 的引用。

## Quick Start

```sh
git clone https://github.com/catwenlabs/pulse.git
cd pulse
export PULSE_MASTER_KEY="$(openssl rand -base64 32)"   # 首次生成，妥善保存
docker compose up -d
curl --fail http://localhost:8080/healthz
```

打开 [http://localhost:8080](http://localhost:8080)，健康检查位于 [`/healthz`](http://localhost:8080/healthz)。部署定义见仓库中的 [`compose.yaml`](compose.yaml)；镜像标签与生产建议见[部署与运维](docs/operations.md)。

> [!IMPORTANT]
> `PULSE_MASTER_KEY` 用于加密来源凭据，请保存在服务器的私密环境文件或 Secret 管理器中——**丢失后无法解密已保存的认证信息**。未配置时仍可使用无凭据来源，但 Pulse 会拒绝保存 Token、Cookie、密码或认证 Header。

> [!WARNING]
> Pulse 当前**无内置用户认证**，默认端口仅绑定 `127.0.0.1`。**不要直接暴露到公网**；需要远程访问请走可信 VPN 或带认证的反向代理。

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

## Concepts

**Source** 是一项持久化的信息来源配置（一个 Feed、一个 API、一个网页入口或一个收藏集合），而不是一篇文章。来源类型与创建方式详见 [Sources](docs/sources.md)。

## Usage

- 订阅 RSS / API / 网页，或手工收藏网页、导入阅读批注 —— 详见[使用指南](docs/usage.md)。
- 网页收藏通过 Bookmarklet 完成，无需浏览器扩展。

## Configuration

关键环境变量（完整列表与 Story 语义聚合配置见[部署与运维](docs/operations.md#配置环境变量)）：

| 变量 | 默认值 | 用途 |
| --- | --- | --- |
| `PULSE_DATABASE_URL` | `postgres://pulse:pulse@postgres:5432/pulse?sslmode=disable` | PostgreSQL 连接地址 |
| `PULSE_HTTP_ADDR` | `127.0.0.1:8080`（容器内需设为 `:8080`） | HTTP 监听地址 |
| `PULSE_MASTER_KEY` | 空 | 来源凭据主密钥（Base64，32 字节） |
| `PULSE_ROLES` | `web,scheduler,worker,effect-worker` | 进程启用的运行角色 |
| `PULSE_EMBEDDING_PROVIDER` | `disabled` | Story 语义聚合；可设为 `ollama` |
| `PULSE_AI_PROVIDER` | `disabled` | 全局 AI Provider；可设为 `openai-compatible` 或 `ollama` |
| `PULSE_AI_MAX_ACTIVE_JOBS` | `4` | 全局 AI Provider 的排队、运行和重试 Job 上限 |

## Local Development

一键启动 PostgreSQL、Go 后端和带 HMR 的 Vite 前端：

```sh
make dev
```

访问 [http://localhost:5173](http://localhost:5173)，`/api` 与 `/healthz` 会代理到本机 `8080`。也可分别启动：`make dev-db-up`、`make dev-api`、`make dev-web`（首次需 `make dev-web-install`）；停止开发库用 `make dev-db-down`。

参与开发或使用 AI 编码 Agent 前，请阅读 [AGENTS.md](AGENTS.md)。

## Deployment & Operations

部署、升级、备份恢复、导出与完整配置详见[部署与运维](docs/operations.md)。升级示例：

```sh
make backup
git pull --ff-only
docker compose pull pulse
docker compose up -d --remove-orphans
```

## Verification

```sh
make test
make test-race
make vet
make compose-config
make e2e   # RSS 浏览器端到端，需先启动完整应用
```

GitHub Actions 会在 Pull Request 和 `main` 推送时执行后端、前端与容器验证；全部通过后才发布 GHCR 镜像。

## Documentation

- [架构说明](docs/architecture.md) · [领域语言](CONTEXT.md) · [架构决策记录](docs/adr/)
- [Sources](docs/sources.md) · [使用指南](docs/usage.md) · [部署与运维](docs/operations.md)
- [OpenAPI 契约](api/openapi.yaml)
- [GitHub Container Registry](https://github.com/users/catwenlabs/packages/container/package/pulse)

---

<div align="center">

Built with Go, React and PostgreSQL.

</div>
