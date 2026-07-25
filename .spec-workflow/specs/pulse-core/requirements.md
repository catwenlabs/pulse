# Pulse Core Requirements

## Introduction

本规格定义 Pulse 第一版的核心摄取纵向切片及其后续增量。系统必须先证明 Source、Acquisition、Driver、Entry 和 PostgreSQL 事务可以可靠协作，再扩展具体来源、规则和阅读界面。

## Requirements

### Requirement 1: Source lifecycle

**User Story:** 作为用户，我希望创建和管理 Source，以便每个独立信息来源具有自己的配置、状态和诊断。

#### Acceptance Criteria

1. WHEN 用户提交有效 Source 配置 THEN 系统 SHALL 创建唯一 Source 并返回标识。
2. WHEN `driver_kind + normalized_locator` 已存在 THEN 系统 SHALL 拒绝重复 Source。
3. WHEN Source 被暂停 THEN Scheduler SHALL 不再创建定时 Acquisition。
4. WHEN Source 被删除 THEN 系统 SHALL 归档 Source 并默认保留已有 Entry。
5. WHEN Source 配置被修改 THEN 新配置 SHALL 只影响后续 Acquisition。

### Requirement 2: Unified acquisition

**User Story:** 作为用户，我希望拉取、推送、导入和文件变化走同一处理管道，以便不同来源具有一致的可靠性。

#### Acceptance Criteria

1. WHEN 任意 Trigger 到达 THEN 系统 SHALL 持久化 Acquisition Command。
2. WHEN Worker 领取任务 THEN PostgreSQL SHALL 使用 Lease 防止拉取型 Source 重复执行。
3. WHEN Acquisition 重试 THEN 系统 SHALL 不创建重复 Entry 或重复 Effect。
4. WHEN Pipeline 失败 THEN 系统 SHALL 不前移 Checkpoint。
5. WHEN Pipeline 成功 THEN Entry、规则结果、Effect 和 Checkpoint SHALL 在同一事务提交。

### Requirement 3: Driver contract

**User Story:** 作为开发者，我希望通过 Driver 添加信息源类型，以便不修改 Entry、规则、搜索或阅读模块。

#### Acceptance Criteria

1. WHEN Driver 接收 Source、Trigger、Payload 和 Checkpoint THEN 它 SHALL 返回 Candidate、建议 Checkpoint 和诊断。
2. Driver SHALL NOT 直接修改 Entry、Rule、Reader 或 Checkpoint 存储。
3. WHEN Driver 校验配置失败 THEN 系统 SHALL 返回可定位到字段的错误。
4. WHEN 新 Driver 通过契约测试 THEN 它 SHALL 可注册而无需修改 Pipeline。

### Requirement 4: Entry identity and update

**User Story:** 作为用户，我希望系统区分新内容和已有内容更新，以免产生重复文章。

#### Acceptance Criteria

1. WHEN Candidate 有稳定外部 ID THEN 系统 SHALL 优先使用 `source_id + external_id`。
2. WHEN外部 ID 缺失且有永久链接 THEN 系统 SHALL 使用规范化链接。
3. WHEN两者均缺失 THEN Collection Source SHALL 使用用户选择的字段组合。
4. WHEN身份相同且内容变化 THEN 系统 SHALL 更新来源字段并保留阅读状态、手动标签、显示标题和笔记。
5. 系统 SHALL NOT 保存 Entry 历史正文版本。
6. WHEN 用户删除 Entry THEN 系统 SHALL 创建 Tombstone 防止其再次摄取。

### Requirement 5: Initial source types

**User Story:** 作为用户，我希望接入常见信息源，以便把分散内容汇集到 Pulse。

#### Acceptance Criteria

1. 系统 SHALL 支持 RSS、Atom 和 JSON Feed。
2. 系统 SHALL 支持带声明式字段映射和分页的 JSON API。
3. 系统 SHALL 支持单文档和列表形式的静态 HTML。
4. 系统 SHALL 支持绑定 Source 与独立密钥的 JSON Webhook。
5. 系统 SHALL 支持用户显式创建的 Manual Source。
6. 系统 SHALL 支持白名单目录中的 Markdown 和 HTML File Source。

### Requirement 6: Configuration wizard

**User Story:** 作为用户，我希望通过界面测试和映射自定义 Source，以便无需编写配置文件。

#### Acceptance Criteria

1. WHEN 配置 API Source THEN 用户 SHALL 能查看脱敏响应、选择列表、映射字段并预览 Entry。
2. WHEN 配置静态网页 THEN 用户 SHALL 能通过可视化点选生成 CSS Selector。
3. WHEN配置 Collection Source THEN 用户 SHALL 确认或覆盖 Identity Key。
4. WHEN 配置分页 THEN 系统 SHALL 支持页码、下一页 URL 和游标。
5. WHEN 首次摄取 THEN 默认导入最近 100 条，并允许选择其他范围。
6. WHEN 网页列表意外解析为 0 条 THEN 系统 SHALL 不提交 Checkpoint 并显示配置异常。

### Requirement 7: Rules

**User Story:** 作为用户，我希望用结构化规则自动整理 Entry。

#### Acceptance Criteria

1. 系统 SHALL 支持来源、标题、作者、正文、URL 和时间条件以及 AND、OR、NOT。
2. 系统 SHALL 支持标签、收藏、已读、隐藏、稍后读、站内通知和 Webhook 动作。
3. WHEN Entry 更新 THEN 系统 SHALL 重新求值规则。
4. WHEN规则不再匹配 THEN 系统 SHALL 撤销规则生成的派生标签，但不得撤销用户操作。
5. 通知和 Webhook SHALL 对同一 Rule Version 与 Entry 幂等。
6. 第一版 SHALL NOT 执行任意用户代码。

### Requirement 8: Reader and organization

**User Story:** 作为用户，我希望在统一收件箱中阅读并组织内容。

#### Acceptance Criteria

1. 新 Entry SHALL 默认进入统一收件箱并保持未读。
2. Folder SHALL 只组织 Source，且只支持一级；一个 Source MAY 属于多个 Folder。
3. 标签和 View SHALL 组织 Entry；一个 Entry MAY 出现在多个 View。
4. 用户 SHALL 能设置已读、收藏、隐藏、稍后读、显示标题和笔记。
5. Source 中缺失或明确删除的内容 SHALL 默认保留本地 Entry。
6. 桌面阅读界面 SHALL 在左侧持续展示一级 Folder 与 Source，在右侧展示紧凑 Entry 列表；选择 Source SHALL 只查询该 Source 的 Entry，选择 Entry SHALL 在列表原位展开正文和阅读操作。
7. Reader SHALL 安全保留 Feed 正文的标题、段落、列表、引用、代码、表格、链接和图片；RSS Driver SHALL 支持 `content:encoded`、Media RSS 图片和图片 Enclosure。

### Requirement 9: Diagnostics and security

**User Story:** 作为用户，我希望能在界面发现并修复抓取问题，同时避免来源凭据泄漏。

#### Acceptance Criteria

1. Source SHALL 展示最近抓取、下次计划、耗时、状态、Candidate/新增/更新数量和连续失败。
2. 失败响应 MAY 保存脱敏、限大小、最长 7 天的诊断快照。
3. 每个 Webhook Source SHALL 使用独立可轮换密钥，不提供匿名全局入口。
4. Source 凭据 SHALL 加密存储，主密钥 SHALL 位于数据库之外。
5. 网络 Driver SHALL 防御 SSRF、重定向绕过、响应膨胀、XXE 和不安全 HTML。
6. 管理界面第一版 SHALL 不包含登录或授权。

### Requirement 10: Deployment and operations

**User Story:** 作为用户，我希望通过 Docker Compose 稳定运行和备份 Pulse。

#### Acceptance Criteria

1. 默认部署 SHALL 只需要 Pulse 与 PostgreSQL。
2. 同一 Pulse 镜像 SHALL 支持 Web、Scheduler、Acquisition Worker 和 Effect Worker 角色。
3. 系统 SHALL 提供每日 PostgreSQL 逻辑备份和状态查看。
4. File Source SHALL 只能读取显式挂载的只读导入目录。
5. 系统 SHALL 提供 OPML、脱敏配置 JSON 和单篇 Markdown 导出。

## Non-Functional Requirements

### Reliability

- 所有异步命令和外部 Effect 必须幂等。
- 所有网络、解析和数据库操作必须支持超时与取消。
- 拉取型 Source 同时最多一个 Acquisition；不同 Source 可以并行。

### Performance

- Source 与 Entry 列表的普通请求目标 P95 小于 300ms。
- 单次 Acquisition 必须有最大页数、Entry 数、响应大小和总耗时限制。
- 第一版面向约 1,000 个 Source 和百万级 Entry。

### Security

- 网络访问必须经过统一受控 HTTP Client。
- 密钥、Cookie 和认证 Header 不得进入日志或普通响应。
- 文件访问必须限制在白名单挂载目录。

### Usability

- 自定义 Source 保存前必须能测试和预览。
- 失败信息必须指出 Source、阶段和可操作原因。
- Web 界面必须响应式并可作为 PWA 安装。
