# Pulse

Pulse 是面向单用户、仅供本机或局域网访问的阅读中枢。它将 RSS/Atom/JSON Feed、JSON API、静态 HTML、Webhook、手动条目和只读目录文件统一摄取到 PostgreSQL，并提供阅读、搜索、组织、规则和诊断界面。

## Docker 部署

首次启动前生成位于数据库之外的 32 字节主密钥：

```sh
export PULSE_MASTER_KEY="$(openssl rand -base64 32)"
docker compose up --build -d
```

请把 `PULSE_MASTER_KEY` 保存到服务器的私密环境文件或 Secret 管理器；丢失它将无法解密已保存的来源认证 Header。未配置主密钥时仍可使用无凭据来源，但 Pulse 会拒绝保存包含 Token、Cookie、密码或认证 Header 的配置。

首页为 `http://服务器局域网地址:8080/`，健康检查为 `/healthz`。默认 Compose 只启动 Pulse 和 PostgreSQL；同一 Pulse 镜像同时运行 Web、Scheduler、Acquisition Worker 和 Effect Worker。`imports/` 以只读方式挂载到 `/data/imports`。

## 备份与恢复

立即创建并校验 PostgreSQL 逻辑备份：

```sh
make backup
make backup-verify FILE=backups/pulse-YYYYMMDDTHHMMSSZ.sql.gz
```

默认保留 14 天，可用 `PULSE_BACKUP_RETENTION_DAYS` 修改。恢复前先停止 Pulse 写入，再导入一个已通过 `gzip -t` 的文件：

Linux 服务器可用系统 cron 每日执行，例如 `0 3 * * * cd /opt/pulse && make backup`。cron 应以能够访问 Docker 的专用运维账号运行，并把失败输出交给服务器现有告警系统。

```sh
docker compose stop pulse
gzip -dc backups/pulse-YYYYMMDDTHHMMSSZ.sql.gz |
  docker compose exec -T postgres psql --username=pulse --dbname=pulse
docker compose start pulse
```

恢复属于覆盖性操作，应先保留当前数据库备份，并在恢复后检查 `/healthz`、Source 列表和 Entry 数量。

## 导出

```sh
curl -O http://localhost:8080/api/v1/opml/export
make export-config
make export-entry ID=ENTRY_UUID
```

配置导出包含 Source、Rule 和 View，但认证 Header、Token、Cookie、密码和 Webhook 密钥会被脱敏。单篇 Entry 可导出为 Markdown。

## 本地开发与验证

只启动 PostgreSQL 并将其暴露到本机 `54321` 端口：

```sh
docker compose -f compose.yaml -f compose.dev.yaml up -d postgres
```

本机运行 Go 后端时，将数据库地址设置为：

```sh
PULSE_DATABASE_URL='postgres://pulse:pulse@127.0.0.1:54321/pulse?sslmode=disable' make run
```

前端开发服务器运行于 `http://localhost:5173`，并将 API 请求代理到本机后端：

```sh
cd web
npm run dev
```

完整验证命令：

```sh
make test
make test-race
make vet
docker compose config --quiet
```

API 契约位于 `api/openapi.yaml`，架构说明位于 `docs/architecture.md`。
