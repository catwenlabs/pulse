# 部署与运维

> 返回 [README](../README.md)。

仓库根目录的 `compose.yaml` 拉取 GHCR 上的预构建镜像，适合单机 Linux 服务器和局域网部署。快速上手见 [README · Quick Start](../README.md#quick-start)。

## 首次部署

```sh
git clone https://github.com/catwenlabs/pulse.git
cd pulse

# .env.example 可提交；.env 已被 Git 忽略，只在本机保存真实配置。
cp .env.example .env
# 编辑 .env，至少设置 PULSE_MASTER_KEY；生产环境也可改用 Secret 管理器。

docker compose up -d
curl --retry 20 --retry-delay 2 --retry-connrefused \
  --fail --show-error http://localhost:8080/healthz
```

生产环境必须通过服务管理器、Secret 管理器或仓库外的只读环境文件注入 `PULSE_MASTER_KEY`。不要把密钥写入 README、Compose、提交记录或仓库内未忽略的文件。Compose 会读取根目录 `.env` 用于变量插值，但 `compose.yaml` 仍显式列出允许注入容器的 `PULSE_*` 变量。宿主机运行 `make dev` 时，Makefile 也只读取同一份 `.env`；由于开发 API 在宿主机运行，`dev-api` 会应用本机数据库、监听地址和导入目录的默认值，必要时可通过 `PULSE_*` Make 参数覆盖。

## 升级

```sh
make backup
git pull --ff-only
docker compose pull pulse
docker compose up -d --remove-orphans
curl --retry 20 --retry-delay 2 --retry-connrefused \
  --fail --show-error http://localhost:8080/healthz
```

升级前备份，升级后检查健康状态、Source 列表和 Entry 数量。应用启动时会自动应用尚未执行的数据库迁移。

## 日常操作

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

## 备份与恢复

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

## 导出

```sh
curl -O http://localhost:8080/api/v1/opml/export
make export-config
make export-entry ID=ENTRY_UUID
```

配置导出包含 Source、Rule 和 View，但认证 Header、Token、Cookie、密码和 Webhook 密钥会被脱敏。

## 配置（环境变量）

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
| `PULSE_AI_PROVIDER` | `disabled` | 全局 AI Provider；可设为 `openai-compatible` 或 `ollama`（两者都走同一适配器） |
| `PULSE_AI_BASE_URL` | `http://127.0.0.1:11434/v1` | OpenAI-compatible 服务基础地址；例如 OpenAI、DeepSeek、OpenRouter、Qwen 或 Ollama |
| `PULSE_AI_API_KEY` | 空 | Provider API Key；通过 Secret/环境注入，不会返回到 API 或写入日志 |
| `PULSE_AI_HEADERS_JSON` | 空 | 可选的额外 Header JSON 对象；同样按 Secret 处理 |
| `PULSE_AI_MODEL` | `qwen3:8b` | 模型名称 |
| `PULSE_AI_TIMEOUT` | `2m` | 单次 AI 请求超时 |
| `PULSE_AI_MAX_DIGEST_STORIES` | `100` | 默认 Catch-up Digest 的安全上限 |
| `PULSE_AI_MAX_ACTIVE_JOBS` | `4` | 全局 AI Provider 的排队、运行和重试 Job 上限 |

### Story 语义聚合（可选）

Story 聚合把同一新闻的多个来源条目合并成一条 Story。**未启用 embedding 时，跨源合并极少发生**——此时聚类只命中近乎一致的标题（72 小时内）、相同 URL 或相同正文指纹；不同来源对同一新闻的措辞差异通常匹配不上。想要真正的跨源去重，请启用本地 Ollama：

```sh
ollama pull qwen3-embedding          # 约 4.7 GB，首次调用有冷启动加载耗时
# 在 .env 中设置：
PULSE_EMBEDDING_PROVIDER=ollama
PULSE_EMBEDDING_BASE_URL=http://127.0.0.1:11434
PULSE_EMBEDDING_MODEL=qwen3-embedding
```

启用后，worker 角色每 30 秒运行一次聚类（用标题 + 正文前 500 字生成向量做相似度匹配）。开启 embedding **之前**已入库、从未生成过向量的旧条目也会被 worker 逐步回填并重新聚类；想立刻跑完可调用 `POST /api/v1/stories/recluster`（同步、逐条生成向量，量大耗时；应用日志会打印每轮 `Story recluster pass` 进度）。

Pulse 在 Ollama 不可用时自动回退到传统文本算法，Entry 摄取与 Checkpoint 不受影响。容器内的 `127.0.0.1` 指向 Pulse 容器本身，Compose 部署时应把 `PULSE_EMBEDDING_BASE_URL` 设置为可从 Pulse 容器访问的 Ollama 服务地址（macOS Docker Desktop 下可用 `http://host.docker.internal:11434`）。

### AI StorySummary 与 Catch-up Digest（可选）

AI 功能只在用户点击后执行，不会自动标记 Story 为已读。`StorySummary` 会读取一个 Story 的 Entry 内容，保存结构化概览、要点和来源说明；内容发生变化后，旧摘要会显示为过期。Catch-up Digest 默认按 Story 去重，选择未读且未隐藏的 Story，并把标题、来源数、Entry 数和时间等必要元数据固定成快照后发送给 AI，不发送 Entry 正文或 `Entry.Summary`。Digest 会持久化历史，结果中的每个 Story 引用都可以回到对应 Story 查看。

所有供应商共用一个 OpenAI-compatible Chat Completions 适配器。选择供应商只需要改变环境变量，例如：

```sh
# Ollama（本地，不需要 API Key）
# 在 .env 中设置：
PULSE_AI_PROVIDER=ollama
PULSE_AI_BASE_URL=http://host.docker.internal:11434/v1
PULSE_AI_MODEL=qwen3:8b

# DeepSeek / OpenRouter / Qwen / OpenAI 等
# 在 .env 中设置：
PULSE_AI_PROVIDER=openai-compatible
PULSE_AI_BASE_URL=https://api.deepseek.com
PULSE_AI_API_KEY=YOUR_API_KEY
PULSE_AI_MODEL=deepseek-v4-flash
```

启用后必须保留 `worker` 角色，AI Job 会使用 PostgreSQL 的持久化队列、Lease 和重试机制。AI 调用日志会记录完整的 Chat Completions 请求 JSON，便于排查 Provider 兼容性；请求中的 Story/Prompt 可能包含敏感内容，请只在受控的本地诊断环境查看。API Key、Authorization Header 和 AI Provider 响应正文不会被写入日志，也不要把 API Key 写入 Compose 文件或提交记录。

AI 请求使用受控 HTTP Client：公网 Provider 不会连接私有、回环或链路本地地址；本地 Ollama 只允许 `localhost`、`127.0.0.1`、`::1`、`host.docker.internal`、`host.containers.internal`、`gateway.docker.internal` 和 `ollama` 这些显式主机名。Provider URL 不支持内嵌用户凭据，重定向也会继续执行同样的安全检查。

### 安全注意事项

Pulse 当前不提供内置用户认证，并且阅读批注可能包含敏感内容。直接运行程序和默认 Compose 端口映射都只对本机开放；容器内部仍监听 `:8080`。不要将该端口直接暴露到公网；需要让 iPhone 等其他设备访问时，应通过可信 VPN 或带认证的反向代理开放，而不是直接改成公网监听。
