# 部署与运维

> 返回 [README](../README.md)。

仓库根目录的 `compose.yaml` 从 `Dockerfile` 构建 Pulse，适合单机 Linux 服务器和局域网部署。仅拉取预构建镜像的快速上手见 [README · Quick Start](../README.md#quick-start)。

## 首次部署

```sh
git clone https://github.com/catwenlabs/pulse.git
cd pulse

# 只在首次部署时生成，并在仓库之外持久保存。
export PULSE_MASTER_KEY="$(openssl rand -base64 32)"

docker compose pull postgres
docker compose up --build -d
curl --retry 20 --retry-delay 2 --retry-connrefused \
  --fail --show-error http://localhost:8080/healthz
```

生产环境必须通过服务管理器、Secret 管理器或仓库外的只读环境文件注入 `PULSE_MASTER_KEY`。不要把密钥写入 README、Compose、提交记录或仓库内未忽略的文件。

## 升级

```sh
make backup
git pull --ff-only
docker compose build --pull pulse
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

### Story 语义聚合（可选）

Story 聚合在未启用 embedding 时仍使用 URL、标题和正文指纹。启用本地 Ollama：

```sh
ollama pull qwen3-embedding
export PULSE_EMBEDDING_PROVIDER=ollama
export PULSE_EMBEDDING_BASE_URL=http://127.0.0.1:11434
export PULSE_EMBEDDING_MODEL=qwen3-embedding
```

Pulse 在 Ollama 不可用时自动使用传统文本算法；Entry 摄取和 Checkpoint 不受影响。容器内的 `127.0.0.1` 指向 Pulse 容器本身，Compose 部署时应把 `PULSE_EMBEDDING_BASE_URL` 设置为可从 Pulse 容器访问的 Ollama 服务地址。

### 安全注意事项

Pulse 当前不提供内置用户认证，并且阅读批注可能包含敏感内容。直接运行程序和默认 Compose 端口映射都只对本机开放；容器内部仍监听 `:8080`。不要将该端口直接暴露到公网；需要让 iPhone 等其他设备访问时，应通过可信 VPN 或带认证的反向代理开放，而不是直接改成公网监听。
