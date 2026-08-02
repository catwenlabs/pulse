# Pulse

Pulse 是单用户的信息摄取与阅读中枢。它从不同类型的信息源取得内容，将内容统一为可搜索、可过滤、可阅读的条目。

## Language

**Source**:
一项持久化的信息来源配置，描述内容来自哪里以及应使用哪种 Driver。
未归入 Folder 的 Source 在 root 导航中保持独立的用户展示顺序。
_Avoid_: Feed、Subscription、Channel

**Driver**:
理解某类 Source 配置和外部协议，并把一次摄取请求转换为候选内容的实现。
_Avoid_: Fetcher、Connector、Plugin

**Trigger**:
发起一次摄取的原因，包括定时到期、外部推送、文件变化、手工刷新和导入。
_Avoid_: Source Type、Fetch Type

**Acquisition**:
从一个 Source 取得一批候选内容的单次尝试。
_Avoid_: Sync、Crawl、Fetch

**Candidate**:
Driver 取得但尚未完成统一化和去重的内容。
_Avoid_: Raw Entry、Article

**Entry**:
完成统一化的单个 Source 内容记录，是来源归属、规则匹配和 Story 聚合的基础对象；Entry 保留各来源的独立内容和出处，并作为按 Source 浏览时的列表对象。
_Avoid_: Item、Post、Article

**Story**:
由一条或多条 Entry 组成的阅读对象，是 Reader 中组织、统计和阅读状态操作的最小单元；聚合阅读列表展示 Story，按 Source 浏览展示该 Source 的 Entry，但 Entry 的阅读状态由所属 Story 决定；包括按 Source 筛选的未读数量在内，所有 Reader 统计均按 Story 去重；聚合搜索检查所有成员 Entry 并将命中的 Story 只返回一次。用户维护的显示标题、Note 和标签属于 Story，来源标题和内容仍属于各 Entry。后加入已有 Story 的 Entry 继承该 Story 的阅读状态，不会使已读 Story 重新变为未读；两个 Story 合并时保留任一侧已经发生的阅读状态操作。每个 Entry 必须且只能属于一个 Story，单独出现的 Entry 组成单 Entry Story。
_Avoid_: Duplicate Entry、Merged Entry、Topic

**Annotation**:
用户在外部阅读器中产生的一条阅读批注，由高亮原文、个人批注、书籍身份和阅读位置组成；摄取后成为一个 Entry，来源批注与用户在 Pulse 中为 Story 追加的 Note 分开保存。
_Avoid_: Highlight Entry、Book Note

**Checkpoint**:
Source 在成功摄取后保存的外部进度位置，例如游标、ETag、文件偏移或页面指纹。
_Avoid_: Cursor、Sync State

**Identity Key**:
在同一个 Source 内稳定识别一条内容的值，用于区分新 Entry 和已有 Entry 的更新。
_Avoid_: Database ID、Fingerprint

**Tombstone**:
Entry 被用户删除后保留的最小身份记录，用于阻止来源再次返回时让它复活。
_Avoid_: Deleted Entry、Archive

**Rule**:
Entry 进入阅读库时求值的条件与动作定义；阅读状态、用户标签等 Reader 动作作用于命中 Entry 所属的 Story，Entry 级动作必须具有独立且明确的语义。
_Avoid_: Filter、Automation

**View**:
可命名并重复使用的持久化 Reader 查询，只选择阅读对象而不修改对象。
_Avoid_: Smart Folder、Rule

**Folder**:
用于组织 Source 的一级集合；Folder 列表和每个 Folder 内的 Source 都有独立的用户展示顺序，一个 Source 可以属于多个 Folder 且在不同 Folder 中可以有不同位置。
_Avoid_: Category、View

**Effect**:
规则产生的、需要在数据库事务之外执行的动作，例如通知或 Webhook。
_Avoid_: Action、Side Effect
