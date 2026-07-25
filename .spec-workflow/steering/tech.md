# Technology Stack

## Project Type

单用户 B/S 应用，运行在普通 Linux 服务器并通过 Docker Compose 部署，仅面向本机和局域网。

## Core Technologies

- 后端：Go
- 前端：TypeScript、React
- 数据库：PostgreSQL 17
- 接口：REST、OpenAPI
- 实时更新：Server-Sent Events
- 部署：Docker Compose

## Application Architecture

模块化单体。一个镜像支持 Web、Scheduler、Acquisition Worker 和 Effect Worker 角色；默认单容器启用全部角色。需要 JavaScript 的 Browser Worker 后续作为可选容器加入。

PostgreSQL 是第一版唯一有状态基础设施，同时承载领域数据、任务队列、Lease、Checkpoint、Effect Outbox 和搜索。第一版不引入 Redis 或独立搜索引擎。

## Data and Search

- 迁移使用显式版本 SQL。
- 英文全文检索使用 `tsvector`。
- 中文关键词与模糊匹配使用规范化文本和 `pg_trgm`。
- Source 凭据密文存储，主密钥通过 Docker Secret 或只读文件挂载。

## Testing and Quality

- Go 单元测试使用标准 `testing` 包和表驱动测试。
- PostgreSQL 集成测试验证事务、Lease、Checkpoint 和幂等约束。
- 前端单元测试使用 Vitest 与 Testing Library。
- 关键配置和阅读旅程使用 Playwright。
- 业务逻辑与公共接口目标覆盖率不低于 80%。
- 必须通过 `go test -race ./...`、`go vet ./...`、前端类型检查和 lint。

## Operations

- 自动运行数据库迁移。
- 每日 PostgreSQL 逻辑备份，保留 7 个日备份、4 个周备份和 6 个月备份。
- `/data/imports` 只读挂载，`/data/exports` 单独写入。
- 日志与诊断不得输出凭据、Webhook 密钥、Cookie 或认证 Header。
