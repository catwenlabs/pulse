import { useEffect, useMemo, useState, type FormEvent } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'

import * as api from './api'
import type { Digest, DigestPriority, DigestStory, DigestTheme, Entry, Story, StoryAISummary } from './api'
import { Button } from './components/ui/button'
import { Input } from './components/ui/input'
import { queryKeys } from './query'

const activeJobStatuses = new Set(['pending', 'running', 'retry', 'queued'])

const statusLabels: Record<string, string> = {
  not_requested: '尚未生成',
  queued: '排队中',
  pending: '排队中',
  running: '生成中',
  retry: '等待重试',
  completed: '已完成',
  partial: '部分完成',
  failed: '生成失败',
  dead: '已停止',
  stale: '内容已变化',
  unavailable: 'AI 不可用',
}

export function DigestPage() {
  const queryClient = useQueryClient()
  const [selectedDigestID, setSelectedDigestID] = useState('')
  const [maxStories, setMaxStories] = useState('')
  const [startAt, setStartAt] = useState('')
  const [endAt, setEndAt] = useState('')
  const [formError, setFormError] = useState('')
  const parsedMaxStories = maxStories.trim() ? Number(maxStories) : undefined
  const maxStoriesValid = parsedMaxStories === undefined
    || (Number.isInteger(parsedMaxStories) && parsedMaxStories >= 1)
  const draftScope = useMemo<api.DigestScope>(() => removeEmpty({
    start_at: localDateTimeToISOString(startAt),
    end_at: localDateTimeToISOString(endAt),
    max_stories: parsedMaxStories,
  }), [endAt, parsedMaxStories, startAt])
  const digestsQuery = useQuery({
    queryKey: queryKeys.digests,
    queryFn: () => api.listDigests(),
  })
  const previewQuery = useQuery({
    queryKey: queryKeys.digestPreview({ startAt, endAt, maxStories }),
    queryFn: () => api.previewDigest(draftScope),
    enabled: maxStoriesValid,
  })
  const selectedDigestQuery = useQuery({
    queryKey: queryKeys.digest(selectedDigestID),
    queryFn: () => api.getDigest(selectedDigestID),
    enabled: Boolean(selectedDigestID),
    refetchInterval: (query) => isActiveStatus(query.state.data?.status) ? 1500 : false,
  })
  const createMutation = useMutation({
    mutationFn: api.createDigest,
    onSuccess: (job) => {
      setSelectedDigestID(job.target_id)
      void queryClient.invalidateQueries({ queryKey: queryKeys.digests })
    },
  })

  const digests = digestsQuery.data ?? []
  const selectedDigest = selectedDigestQuery.data
  const selectedFromHistory = selectedDigestID || digests[0]?.id || ''
  const preview = previewQuery.data
  const previewReady = maxStoriesValid
    && !previewQuery.isFetching
    && !previewQuery.error
    && preview !== undefined
    && digestScopesEqual(preview.scope, draftScope)
    && preview.matching_stories > 0
    && preview.can_queue

  useEffect(() => {
    if (!selectedDigestID && digests[0]) setSelectedDigestID(digests[0].id)
  }, [digests, selectedDigestID])

  async function createDigest(event: FormEvent) {
    event.preventDefault()
    setFormError('')
    if (!maxStoriesValid) {
      setFormError('数量必须是正整数')
      return
    }
    if (!previewReady) {
      setFormError(preview?.matching_stories === 0
        ? '当前范围没有可处理的 Story'
        : preview?.can_queue === false
          ? '请先缩小范围或指定最多 Story 数'
          : '正在检查处理范围，请稍后再试')
      return
    }
    try {
      await createMutation.mutateAsync(draftScope)
    } catch (cause) {
      setFormError(cause instanceof Error ? cause.message : '无法创建追更摘要')
    }
  }

  return (
    <div className="ai-page">
      <header className="ai-page-header">
        <div>
          <p className="ai-eyebrow">AI CATCH-UP</p>
          <h1>AI 追更</h1>
          <p>把一段时间积累的未读 Story 整理成标题级速览。生成时不会标记任何 Story 为已读。</p>
        </div>
        <span className="ai-title-only-badge">仅发送标题和必要元数据</span>
      </header>

      <section className="ai-create-card" aria-labelledby="create-digest-title">
        <div>
          <p className="ai-eyebrow">NEW DIGEST</p>
          <h2 id="create-digest-title">生成一份未读追更</h2>
          <p>默认覆盖全部未读、未隐藏 Story。积压超过安全上限时，请先缩小时间范围或指定数量。</p>
        </div>
        <form onSubmit={(event) => void createDigest(event)}>
          <div className="ai-form-grid">
            <label>
              <span>最早时间（可选）</span>
              <Input type="datetime-local" value={startAt} onChange={(event) => setStartAt(event.target.value)} />
            </label>
            <label>
              <span>最晚时间（可选）</span>
              <Input type="datetime-local" value={endAt} onChange={(event) => setEndAt(event.target.value)} />
            </label>
            <label>
              <span>最多 Story（可选）</span>
              <Input inputMode="numeric" min={1} placeholder="默认安全上限" value={maxStories} onChange={(event) => setMaxStories(event.target.value)} />
            </label>
          </div>
          {formError && <p className="ai-form-error" role="alert">{formError}</p>}
          {createMutation.isError && !formError && <p className="ai-form-error" role="alert">{createMutation.error.message}</p>}
          {previewQuery.error && <p className="ai-form-error" role="alert">{previewQuery.error.message}</p>}
          {previewQuery.data && (
            <p className="ai-scope-preview">
              {formatScopePreview(previewQuery.data)}
            </p>
          )}
          <Button type="submit" disabled={createMutation.isPending || (maxStoriesValid && !previewReady)}>
            {createMutation.isPending
              ? '正在排队…'
              : !maxStoriesValid && maxStories.trim()
                ? '数量必须是正整数'
                : previewQuery.isFetching || !preview
                  ? '正在检查范围…'
                  : preview.matching_stories === 0
                ? '没有可处理的 Story'
                : !preview.can_queue
                  ? '请先缩小范围'
                  : '生成追更摘要'}
          </Button>
        </form>
      </section>

      <div className="ai-history-layout">
        <section className="ai-history-card" aria-labelledby="digest-history-title">
          <div className="ai-card-heading">
            <div>
              <p className="ai-eyebrow">HISTORY</p>
              <h2 id="digest-history-title">历史追更</h2>
            </div>
            <span>{digests.length} 份</span>
          </div>
          {digestsQuery.isPending && <p className="ai-state">正在加载历史记录…</p>}
          {digestsQuery.error && <p className="ai-state ai-error">{digestsQuery.error.message}</p>}
          {!digestsQuery.isPending && !digestsQuery.error && digests.length === 0 && (
            <p className="ai-state">还没有追更摘要。生成一份，稍后可以回来查看。</p>
          )}
          <div className="ai-history-list">
            {digests.map((digest) => (
              <button
                className={`ai-history-item ${selectedFromHistory === digest.id ? 'is-selected' : ''}`}
                key={digest.id}
                type="button"
                onClick={() => setSelectedDigestID(digest.id)}
              >
                <span className="ai-history-item-title">{digest.story_count} 个未读 Story</span>
                <span className="ai-history-item-meta">
                  {statusLabels[digest.status] ?? digest.status} · {formatScopeRange(digest) || '全部未读'} · {formatDate(digest.created_at)}
                </span>
              </button>
            ))}
          </div>
        </section>

        <DigestResult digest={selectedDigest} loading={selectedDigestQuery.isPending && Boolean(selectedDigestID)} error={selectedDigestQuery.error} />
      </div>
    </div>
  )
}

function formatScopePreview(preview: api.DigestPreview) {
  const range = formatScopeRange(preview.scope)
  if (preview.matching_stories === 0) {
    return `当前范围${range}没有未读 Story。`
  }
  if (!preview.can_queue) {
    return `当前范围${range}匹配超过 ${preview.safety_limit} 个 Story，请缩小时间范围或指定最多 Story 数。`
  }
  const matching = preview.matching_stories_truncated
    ? `至少 ${preview.matching_stories}`
    : `${preview.matching_stories}`
  return `当前范围${range}匹配 ${matching} 个未读 Story，本次将处理 ${preview.selected_stories} 个。`
}

function formatScopeRange(scope: api.DigestScope) {
  if (scope.start_at && scope.end_at) return `（${formatDate(scope.start_at)} 至 ${formatDate(scope.end_at)}）`
  if (scope.start_at) return `（${formatDate(scope.start_at)} 之后）`
  if (scope.end_at) return `（${formatDate(scope.end_at)} 之前）`
  return ''
}

function digestScopesEqual(left: api.DigestScope, right: api.DigestScope) {
  return (left.start_at ?? '') === (right.start_at ?? '')
    && (left.end_at ?? '') === (right.end_at ?? '')
    && (left.max_stories ?? 0) === (right.max_stories ?? 0)
}

function DigestResult({ digest, loading, error }: { digest?: Digest; loading: boolean; error: Error | null }) {
  if (loading) return <section className="ai-result-card"><p className="ai-state">正在加载追更摘要…</p></section>
  if (error) return <section className="ai-result-card"><p className="ai-state ai-error">{error.message}</p></section>
  if (!digest) return <section className="ai-result-card"><p className="ai-state">选择一份历史追更，查看结构化结果。</p></section>

  const stories = digest.stories ?? []
  const storyByID = new Map(stories.map((story) => [story.story_id, story]))
  return (
    <section className="ai-result-card" aria-labelledby="digest-result-title">
      <header className="ai-card-heading">
        <div>
          <p className="ai-eyebrow">CATCH-UP DIGEST</p>
          <h2 id="digest-result-title">{digest.story_count} 个未读 Story</h2>
        </div>
        <span className={`ai-status ai-status-${digest.status}`}>{statusLabels[digest.status] ?? digest.status}</span>
      </header>
      <p className="ai-result-meta">{formatDate(digest.created_at)} · {digest.provider || 'OpenAI-compatible'}{digest.model ? ` / ${digest.model}` : ''}</p>
      {digest.error && <p className="ai-error-box">{digest.error}</p>}
      {digest.overview && <p className="ai-overview">{digest.overview}</p>}
      {digest.priorities && digest.priorities.length > 0 && (
        <DigestPriorities priorities={digest.priorities} storyByID={storyByID} />
      )}
      {digest.themes && digest.themes.length > 0 && (
        <DigestThemes themes={digest.themes} storyByID={storyByID} />
      )}
      <section className="ai-source-section" aria-labelledby="digest-sources-title">
        <h3 id="digest-sources-title">来源 Story</h3>
        <div className="ai-source-list">
          {stories.map((story) => <StoryReference key={story.story_id} story={story} />)}
        </div>
      </section>
      {digest.omissions && digest.omissions.length > 0 && (
        <section className="ai-source-section">
          <h3>未被重点引用</h3>
          <ul className="ai-omission-list">
            {digest.omissions.map((item) => {
              const snapshot = item.story_id ? storyByID.get(item.story_id) : undefined
              return <li key={`${item.label}-${item.story_id || item.title}`}><StoryReference story={{
                label: item.label,
                story_id: item.story_id || '',
                title: item.title,
                entry_count: snapshot?.entry_count ?? 0,
                source_count: snapshot?.source_count ?? 0,
                available: snapshot?.available ?? false,
              }} />：{item.reason}</li>
            })}
          </ul>
        </section>
      )}
    </section>
  )
}

function DigestPriorities({ priorities, storyByID }: { priorities: DigestPriority[]; storyByID: Map<string, DigestStory> }) {
  return (
    <section className="ai-section" aria-labelledby="digest-priorities-title">
      <h3 id="digest-priorities-title">建议优先阅读</h3>
      <div className="ai-priority-list">
        {priorities.map((priority) => (
          <article className="ai-priority-item" key={`${priority.rank}-${priority.title}`}>
            <strong>{priority.rank}. {priority.title}</strong>
            <p>{priority.reason}</p>
            <StoryReferenceList ids={priority.story_ids} storyByID={storyByID} />
          </article>
        ))}
      </div>
    </section>
  )
}

function DigestThemes({ themes, storyByID }: { themes: DigestTheme[]; storyByID: Map<string, DigestStory> }) {
  return (
    <section className="ai-section" aria-labelledby="digest-themes-title">
      <h3 id="digest-themes-title">主题归类</h3>
      <div className="ai-theme-list">
        {themes.map((theme) => (
          <article key={theme.title}>
            <strong>{theme.title}</strong>
            <p>{theme.summary}</p>
            <StoryReferenceList ids={theme.story_ids} storyByID={storyByID} />
          </article>
        ))}
      </div>
    </section>
  )
}

function StoryReferenceList({ ids, storyByID }: { ids: string[]; storyByID: Map<string, DigestStory> }) {
  return <div className="ai-reference-list">{ids.map((id) => {
    const story = storyByID.get(id)
    return story ? <StoryReference key={id} story={story} /> : <a href={`/stories/${id}`} key={id}>查看 Story</a>
  })}</div>
}

function StoryReference({ story }: { story: DigestStory }) {
	if (!story.story_id || !story.available) {
		return <span className="ai-reference ai-reference-unavailable">{story.label} · {story.title || '来源 Story 已不可用'}</span>
	}
  return (
    <a className="ai-reference" href={`/stories/${story.story_id}`}>
      <span>{story.label}</span>
      <strong>{story.title || '无标题'}</strong>
      {story.entry_count > 0 && <small>{story.entry_count} 个 Entry · {story.source_count} 个来源</small>}
    </a>
  )
}

export function StoryDetailPage({ storyID }: { storyID: string }) {
  const queryClient = useQueryClient()
  const storyQuery = useQuery({
    queryKey: queryKeys.story(storyID),
    queryFn: () => api.getStory(storyID),
    refetchInterval: (query) => storySummaryIsActive(query.state.data?.ai_summary) ? 1500 : false,
  })
  const requestMutation = useMutation({
    mutationFn: () => api.requestStorySummary(storyID),
    onSuccess: () => void queryClient.invalidateQueries({ queryKey: queryKeys.story(storyID) }),
  })

  if (storyQuery.isPending) return <div className="ai-page ai-state-page"><p className="ai-state">正在加载 Story…</p></div>
  if (storyQuery.error) return <div className="ai-page ai-state-page"><p className="ai-state ai-error">{storyQuery.error.message}</p></div>
  const story = storyQuery.data
  if (!story) return null
  const entries = story.entries ?? [story.representative]
  const summary = story.ai_summary
  return (
    <div className="ai-page story-detail-page">
      <header className="ai-page-header">
        <div>
          <p className="ai-eyebrow">STORY</p>
          <h1>{story.display_title || story.representative.source_title || '无标题'}</h1>
          <p>{entries.length} 个 Entry · {story.source_count} 个来源 · {story.read_at ? '已读' : '未读'}</p>
        </div>
        <Button
          disabled={requestMutation.isPending || storySummaryIsActive(summary)}
          onClick={() => void requestMutation.mutateAsync()}
        >
          {requestMutation.isPending || storySummaryIsActive(summary) ? '正在生成…' : summary?.status === 'stale' ? '重新生成摘要' : '生成 Story 摘要'}
        </Button>
      </header>
      {requestMutation.error && <p className="ai-form-error" role="alert">{requestMutation.error.message}</p>}
      <StorySummaryCard summary={summary} />
      <section className="story-entry-list" aria-labelledby="story-entries-title">
        <h2 id="story-entries-title">来源 Entries</h2>
        {entries.map((entry) => <EntryCard entry={entry} key={entry.id} />)}
      </section>
    </div>
  )
}

function StorySummaryCard({ summary }: { summary?: StoryAISummary }) {
  if (!summary || summary.status === 'not_requested') {
    return <section className="ai-summary-card"><p className="ai-state">还没有摘要。点击右上角按钮后，AI 才会读取这个 Story 的内容。</p></section>
  }
  return (
    <section className="ai-summary-card" aria-labelledby="story-summary-title">
      <header className="ai-card-heading">
        <div>
          <p className="ai-eyebrow">AI STORY SUMMARY</p>
          <h2 id="story-summary-title">内容摘要</h2>
        </div>
        <span className={`ai-status ai-status-${summary.status}`}>{statusLabels[summary.status] ?? summary.status}</span>
      </header>
      {summary.error && <p className="ai-error-box">{summary.error}</p>}
      {summary.overview && <p className="ai-overview">{summary.overview}</p>}
      {summary.key_points && summary.key_points.length > 0 && (
        <ul className="ai-key-point-list">{summary.key_points.map((point) => <li key={point}>{point}</li>)}</ul>
      )}
      {summary.sources && summary.sources.length > 0 && (
        <section className="ai-source-section">
          <h3>来源说明</h3>
          <div className="ai-summary-source-list">
            {summary.sources.map((source) => (
              <a className="ai-summary-source" href={`#entry-${source.entry_id}`} key={source.entry_id}>
                <strong>{source.label} · {source.title}</strong>
                <span>{source.note || '来源已纳入摘要'}</span>
              </a>
            ))}
          </div>
        </section>
      )}
    </section>
  )
}

function EntryCard({ entry }: { entry: Entry }) {
  return (
    <article className="story-entry-card" id={`entry-${entry.id}`}>
      <div className="story-entry-heading">
        <div>
          <h3>{entry.source_title || '无标题'}</h3>
          <p>{entry.author || '未知作者'} · {formatDate(entry.published_at || entry.discovered_at)}</p>
        </div>
        {entry.canonical_url && <a href={entry.canonical_url} rel="noreferrer" target="_blank">打开原文 ↗</a>}
      </div>
      <p>{entry.summary || '这个 Entry 没有摘要，打开原文查看完整内容。'}</p>
    </article>
  )
}

function isActiveStatus(status?: string) {
  return Boolean(status && activeJobStatuses.has(status))
}

function storySummaryIsActive(summary?: StoryAISummary) {
  return isActiveStatus(summary?.status)
}

function formatDate(value?: string) {
  if (!value) return '刚刚'
  return new Intl.DateTimeFormat('zh-CN', { month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit' }).format(new Date(value))
}

function localDateTimeToISOString(value: string) {
  return value ? new Date(value).toISOString() : undefined
}

function removeEmpty<T extends object>(value: T): T {
  return Object.fromEntries(Object.entries(value).filter(([, item]) => item !== undefined && item !== '')) as T
}
