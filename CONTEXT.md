# Pulse

Pulse 是单用户的信息摄取与阅读中枢。它从不同类型的信息源取得内容，将内容统一为可搜索、可过滤、可阅读的条目。

## Language

**Source**:
一项持久化的信息来源配置，描述内容来自哪里以及应使用哪种 Driver。
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
完成统一化并进入阅读库的内容记录，是搜索、规则和阅读操作的共同对象。
_Avoid_: Item、Post、Article

**Story**:
一个用于阅读和检索的新闻事件聚合，由一条或多条不同 Source 的 Entry 组成；每个 Entry 必须且只能属于一个 Story，单独出现的 Entry 组成单 Entry Story。
_Avoid_: Duplicate Entry、Merged Entry、Topic

**Annotation**:
用户在外部阅读器中产生的一条阅读批注，由高亮原文、个人批注、书籍身份和阅读位置组成；摄取后成为一个 Entry，来源批注与用户在 Pulse 中追加的 Note 分开保存。
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
Entry 进入阅读库时执行的条件与动作定义。
_Avoid_: Filter、Automation

**View**:
针对阅读库的持久化查询，只选择内容而不修改内容。
_Avoid_: Smart Folder、Rule

**Folder**:
用于组织 Source 的一级集合；一个 Source 可以属于多个 Folder。
_Avoid_: Category、View

**Effect**:
规则产生的、需要在数据库事务之外执行的动作，例如通知或 Webhook。
_Avoid_: Action、Side Effect
