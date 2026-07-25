# Project Structure

## Directory Organization

```text
.
├── api/
│   └── openapi.yaml
├── cmd/
│   └── pulse/
├── internal/
│   ├── source/
│   ├── ingestion/
│   ├── drivers/
│   ├── entry/
│   ├── rule/
│   ├── effect/
│   ├── search/
│   ├── reader/
│   ├── storage/
│   └── transport/
├── migrations/
├── web/
│   └── src/
├── tests/
├── deploy/
├── docs/
└── .spec-workflow/
```

## Naming

- Go 包：短小写单词，例如 `source`、`entry`。
- Go 文件：小写蛇形命名；测试使用 `_test.go`。
- Go 导出类型和函数：PascalCase；非导出名称：camelCase。
- React 组件：PascalCase；Hook：`useXxx`；测试与组件同目录。
- 数据库：表名和列名使用小写蛇形复数/单数约定。

## Module Boundaries

- `source` 拥有 Source 生命周期和 Driver Registry。
- `ingestion` 拥有 Acquisition Command、Queue、Lease 和事务编排。
- `drivers` 只处理外部协议并返回 Candidate。
- `entry` 拥有标准化、身份计算、去重和 Tombstone。
- `rule` 只产生数据库状态变化和 Effect。
- `effect` 拥有 Outbox 领取、重试和投递。
- `reader` 拥有已读、收藏、标签、Folder 和 View。
- `storage` 提供 PostgreSQL Adapter，不包含领域决策。
- `transport` 将 HTTP/CLI 输入转换为领域调用，不包含业务规则。

依赖指向领域模块；领域模块不得依赖 HTTP 框架、PostgreSQL Driver、React 或 Chromium。

## Interface Rules

- 在消费方定义小接口，接受接口并返回具体类型。
- Driver 的稳定接口只有类型、校验和 Acquisition。
- 测试与调用方穿过同一个模块接口。
- 只有出现生产与测试或两个真实实现时才建立 Adapter seam。

## File Guidelines

- 文件围绕一个领域职责组织，不按 Controller/Service/Repository 机械分层。
- 公共行为由包测试覆盖，不直接测试私有实现。
- 函数优先保持单一控制流和显式错误包装。
- `context.Context` 始终为可能阻塞操作的第一个参数。
