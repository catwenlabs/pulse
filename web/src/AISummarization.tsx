import { useEffect, useMemo, useRef, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { ArrowLeft, BookOpen, CheckCircle2, ChevronDown, FileText, Loader2, RefreshCw, Sparkles, Star, Tag } from 'lucide-react'

import * as api from './api'
import type { Digest, DigestPriority, DigestStory, DigestTheme, Entry } from './api'
import { EntryReader } from './components/EntryReader'
import { StoryListItem, type StoryListItemChange, type StoryMergeCandidate } from './components/StoryListItem'
import { isActiveStorySummary, isStoredStorySummary, statusLabels, StorySummaryCard } from './components/storySummary'
import { Button } from './components/ui/button'
import { queryKeys } from './query'

const activeJobStatuses = new Set(['pending', 'running', 'retry', 'queued'])

// Context threaded through every place a digest references a Story, so each
// reference renders the same self-contained, expandable list item as the
// homepage reader (no navigation). Expansion is tracked per reference occurrence
// (not per story id) so the same Story referenced in several sections expands
// independently; the getStory fetch is still deduped by story id.
interface DigestReferenceContext {
  digestId: string
  mergeCandidates: StoryMergeCandidate[]
  expandedKeys: Set<string>
  toggleExpanded: (referenceKey: string, open: boolean) => void
  onChanged: (change: StoryListItemChange) => void
  onRemoved: (storyId: string) => void
  onError: (message: string) => void
}

export function DigestPage({ digestID = '', onSelectDigest }: { digestID?: string; onSelectDigest?: (digestID: string) => void }) {
  const queryClient = useQueryClient()
  const [markedDigestID, setMarkedDigestID] = useState('')
  // The catch-up scope is fixed to sane defaults: every unread Story, oldest
  // first. When the backlog exceeds the safety limit the create request caps
  // itself at that limit, so generation never needs a scope dialog.
  const defaultScope = useMemo<api.DigestScope>(() => ({ order: 'oldest' }), [])
  const digestsQuery = useQuery({
    queryKey: queryKeys.digests,
    queryFn: () => api.listDigests(),
    refetchInterval: (query) => query.state.data?.some((digest) => isActiveStatus(digest.status)) ? 1500 : false,
  })
  const digests = digestsQuery.data ?? []
  const effectiveDigestID = digestID || digests[0]?.id || ''
  const previewQuery = useQuery({
    queryKey: queryKeys.digestPreview(defaultScope),
    queryFn: () => api.previewDigest(defaultScope),
    placeholderData: (previousData) => previousData,
  })
  const selectedDigestQuery = useQuery({
    queryKey: queryKeys.digest(effectiveDigestID),
    queryFn: () => api.getDigest(effectiveDigestID),
    enabled: Boolean(effectiveDigestID),
    placeholderData: (previousData) => previousData,
    refetchInterval: (query) => isActiveStatus(query.state.data?.status) ? 1500 : false,
  })
  const createMutation = useMutation({
    mutationFn: api.createDigest,
    onSuccess: (job) => {
      onSelectDigest?.(job.target_id)
      void queryClient.invalidateQueries({ queryKey: queryKeys.digests })
    },
  })
  const markReadMutation = useMutation({
    mutationFn: ({ digestID }: { digestID: string }) => api.markDigestRead(digestID),
    onSuccess: (_result, variables) => {
      void queryClient.invalidateQueries({ queryKey: queryKeys.readerRoot })
      void queryClient.invalidateQueries({ queryKey: queryKeys.sources })
      void queryClient.invalidateQueries({ queryKey: queryKeys.storyRoot })
      setMarkedDigestID(variables.digestID)
    },
  })
  const pageRef = useRef<HTMLDivElement>(null)
  // Switching digests swaps the whole result card, so reset the app shell's
  // main scroll region to the top; otherwise the new digest opens wherever
  // the previous one was left. Keyed on the effective selection so clicking
  // the already-open digest doesn't jump. (scrollTo is optional-called because
  // jsdom does not implement element scrolling.)
  useEffect(() => {
    if (!effectiveDigestID) return
    pageRef.current?.closest('main')?.scrollTo?.({ top: 0 })
  }, [effectiveDigestID])

  const selectedDigest = selectedDigestQuery.data
  const preview = previewQuery.data
  const cappedScope = useMemo<api.DigestScope>(() => removeEmpty({
    max_stories: preview?.matching_stories_truncated ? preview.safety_limit : undefined,
    order: 'oldest' as const,
  }), [preview?.matching_stories_truncated, preview?.safety_limit])
  const previewReady = preview !== undefined
    && preview.matching_stories > 0
    && (preview.can_queue || preview.matching_stories_truncated)
  const digestActionLabel = createMutation.isPending
    ? '正在排队…'
    : previewQuery.isPending || !preview
      ? '正在检查…'
      : previewQuery.error
        ? '重试检查'
        : preview.matching_stories === 0
          ? '没有未读 Story'
          : '生成AI追更'
  const digestActionDisabled = previewQuery.isPending
    || (previewQuery.error ? false : createMutation.isPending || !previewReady)

  async function createDigest() {
    try {
      await createMutation.mutateAsync(cappedScope)
    } catch {
      // The failure is rendered under the header from createMutation.error.
    }
  }

  return (
    <div className="ai-page" ref={pageRef}>
      <header className="ai-page-header">
        <div className="ai-page-header-copy">
          <p className="ai-eyebrow ai-eyebrow-with-icon"><Sparkles size={14} aria-hidden="true" />AI CATCH-UP</p>
          <h1>AI 追更</h1>
          <p className={`ai-page-subtitle ${previewQuery.error ? 'ai-page-subtitle-error' : ''}`} aria-live="polite" role={previewQuery.error ? 'alert' : undefined}>
            {formatDigestSubtitle(preview, previewQuery.isPending, previewQuery.error)}
          </p>
        </div>
        <div className="ai-header-actions">
          <Button
            className="ai-header-action min-w-[148px] cursor-pointer"
            disabled={digestActionDisabled}
            onClick={() => {
              if (previewQuery.error) {
                void previewQuery.refetch()
              } else {
                void createDigest()
              }
            }}
          >
            {createMutation.isPending || previewQuery.isPending
              ? <Loader2 className="size-4 animate-spin motion-reduce:animate-none" aria-hidden="true" />
              : previewQuery.error
                ? <RefreshCw className="size-4" aria-hidden="true" />
                : <Sparkles className="size-4" aria-hidden="true" />}
            <span>{digestActionLabel}</span>
          </Button>
        </div>
      </header>

      {createMutation.isError && (
        <p className="ai-form-error mt-3" role="alert">{createMutation.error.message}</p>
      )}

        <DigestResult
          digest={selectedDigest}
          loading={selectedDigestQuery.isPending && Boolean(effectiveDigestID)}
          refreshing={selectedDigestQuery.isFetching && selectedDigestQuery.isPlaceholderData}
          error={selectedDigestQuery.error}
          onRetry={() => void selectedDigestQuery.refetch()}
          onMarkRead={(digestID) => markReadMutation.mutate({ digestID })}
          markReadPending={markReadMutation.isPending && markReadMutation.variables?.digestID === selectedDigest?.id}
          markReadDone={markedDigestID === selectedDigest?.id}
          markReadError={markReadMutation.variables?.digestID === selectedDigest?.id && markReadMutation.error instanceof Error ? markReadMutation.error : null}
        />
    </div>
  )
}

// Renders the digest history list inside the shared middle rail panel, so the
// AI catch-up view follows the same three-column layout as the reader and the
// settings menu. Selection is lifted to the parent (URL or App state); the
// default highlights the most recent digest.
export function DigestHistoryPanel({ digestID, onSelect }: { digestID: string; onSelect: (digestID: string) => void }) {
  const digestsQuery = useQuery({
    queryKey: queryKeys.digests,
    queryFn: () => api.listDigests(),
    refetchInterval: (query) => query.state.data?.some((digest) => isActiveStatus(digest.status)) ? 1500 : false,
  })
  const digests = digestsQuery.data ?? []
  const effectiveID = digestID || digests[0]?.id || ''
  return (
    <>
      <div className="ai-panel-heading">
        <p className="m-0 text-sm font-semibold text-foreground md:text-base" id="digest-history-label">历史追更</p>
        <span className="ai-panel-heading-count"><strong>{digests.length}</strong> 份</span>
      </div>
      {digestsQuery.isPending && <p className="ai-state">正在加载历史记录…</p>}
      {digestsQuery.error && (
        <div className="ai-state ai-error">
          <p>{digestsQuery.error.message}</p>
          <Button variant="secondary" size="sm" onClick={() => void digestsQuery.refetch()}>重试加载</Button>
        </div>
      )}
      {!digestsQuery.isPending && !digestsQuery.error && digests.length === 0 && (
        <div className="ai-empty-state">
          <span className="ai-empty-icon" aria-hidden="true"><Sparkles size={20} /></span>
          <p>还没有追更摘要。生成一份，稍后可以回来查看。</p>
        </div>
      )}
      <div className="ai-history-list ai-history-scroll-list">
        {digests.map((digest) => (
          <button
            className={`ai-history-panel-item ${effectiveID === digest.id ? 'is-selected' : ''}`}
            key={digest.id}
            type="button"
            aria-pressed={effectiveID === digest.id}
            onClick={() => onSelect(digest.id)}
          >
            <span className="ai-history-panel-item-main">
              <span className={`ai-status-dot ai-status-${digest.status}`} aria-hidden="true" />
              <span className="ai-history-panel-item-title">{digest.story_count} 个未读 Story</span>
              <span className="ai-history-panel-item-status">{statusLabels[digest.status] ?? digest.status}</span>
            </span>
            <span className="ai-history-panel-item-meta">{formatScopeRange(digest) || '全部未读'} · {formatDate(digest.created_at)}</span>
          </button>
        ))}
      </div>
    </>
  )
}

function formatDigestSubtitle(preview: api.DigestPreview | undefined, loading: boolean, error: Error | null) {
  const behaviorNote = '生成本身不会标记任何 Story 为已读。'
  if (loading && !preview) return `正在检查当前未读 Story 数量。${behaviorNote}`
  if (error && !preview) return `暂时无法获取当前未读 Story 数量。${behaviorNote}`
  if (!preview) return behaviorNote
  if (preview.matching_stories_truncated) {
    return `当前未读 Story 超过 ${preview.safety_limit} 条，生成时将自动只处理最早的 ${preview.safety_limit} 条。${behaviorNote}`
  }
  return `当前有 ${preview.matching_stories} 条未读 Story。${behaviorNote}`
}

function formatScopeRange(scope: api.DigestScope) {
  if (scope.start_at && scope.end_at) return `（${formatDate(scope.start_at)} 至 ${formatDate(scope.end_at)}）`
  if (scope.start_at) return `（${formatDate(scope.start_at)} 之后）`
  if (scope.end_at) return `（${formatDate(scope.end_at)} 之前）`
  return ''
}

function DigestResult({
  digest,
  loading,
  refreshing = false,
  error,
  onRetry,
  onMarkRead,
  markReadPending = false,
  markReadDone = false,
  markReadError = null,
}: {
  digest?: Digest
  loading: boolean
  refreshing?: boolean
  error: Error | null
  onRetry?: () => void
  onMarkRead?: (digestID: string) => void
  markReadPending?: boolean
  markReadDone?: boolean
  markReadError?: Error | null
}) {
  if (loading) {
    return (
      <section className="ai-result-card" aria-live="polite" role="status">
        <div className="ai-loading-state">
          <span className="ai-loading-icon" aria-hidden="true"><Loader2 size={22} className="animate-spin motion-reduce:animate-none" /></span>
          <strong>正在加载追更摘要</strong>
          <p>正在取回标题级速览结果，请稍候。</p>
        </div>
      </section>
    )
  }
  if (error) {
    return (
      <section className="ai-result-card" role="alert">
        <div className="ai-loading-state ai-error">
          <strong>暂时无法加载这份追更</strong>
          <p>{error.message}</p>
          {onRetry && <Button variant="secondary" size="sm" onClick={onRetry}>重试加载</Button>}
        </div>
      </section>
    )
  }
  if (!digest) {
    return (
      <section className="ai-result-card">
        <div className="ai-empty-result">
          <span className="ai-empty-icon" aria-hidden="true"><FileText size={20} /></span>
          <strong>选择一份历史追更</strong>
          <p>标题级整理结果会显示在这里。</p>
        </div>
      </section>
    )
  }

  const stories = digest.stories ?? []
  const storyByID = new Map(stories.map((story) => [story.story_id, story]))
  const processing = isActiveStatus(digest.status)
  const readableStories = stories.filter((story) => story.available && story.story_id)
  const canMarkRead = !refreshing
    && !processing
    && (digest.status === 'completed' || digest.status === 'partial')
    && readableStories.length > 0

  const [expandedKeys, setExpandedKeys] = useState<Set<string>>(() => new Set())
  const [storyNotice, setStoryNotice] = useState('')
  const queryClient = useQueryClient()
  const mergeCandidates = readableStories.map((story) => ({
    storyId: story.story_id,
    title: story.title || '无标题',
  }))
  const referenceContext: DigestReferenceContext = {
    digestId: digest.id,
    mergeCandidates,
    expandedKeys,
    toggleExpanded: (referenceKey, open) => setExpandedKeys((current) => {
      if (open ? current.has(referenceKey) : !current.has(referenceKey)) return current
      const next = new Set(current)
      if (open) next.add(referenceKey); else next.delete(referenceKey)
      return next
    }),
    onChanged: (change) => {
      // Skip the auto mark-read-on-expand (a read-only patch) so expanding a row
      // doesn't reload the whole digest card; any other change refreshes it so
      // availability and counts stay in sync.
      if (change.kind === 'patched' && Object.keys(change.patch).length === 1 && change.patch.read !== undefined) return
      void queryClient.invalidateQueries({ queryKey: queryKeys.digest(digest.id) })
      void queryClient.invalidateQueries({ queryKey: queryKeys.digests })
    },
    onRemoved: () => {
      void queryClient.invalidateQueries({ queryKey: queryKeys.digest(digest.id) })
      void queryClient.invalidateQueries({ queryKey: queryKeys.digests })
    },
    onError: (message) => setStoryNotice(message),
  }
  return (
    <section className="ai-result-card" aria-labelledby="digest-result-title" aria-live={processing || refreshing ? 'polite' : undefined}>
      {refreshing && (
        <span className="ai-result-refresh" role="status">
          <Loader2 size={13} className="animate-spin motion-reduce:animate-none" aria-hidden="true" />
          正在更新摘要…
        </span>
      )}
      <header className="ai-card-heading ai-result-heading">
        <div className="ai-card-heading-title">
          <span className="ai-card-icon ai-card-icon-result" aria-hidden="true"><FileText size={18} /></span>
          <div>
            <p className="ai-eyebrow">CATCH-UP DIGEST</p>
            <h2 id="digest-result-title">{digest.story_count} 个未读 Story</h2>
          </div>
        </div>
        <span className={`ai-status ai-status-${digest.status}`}>{statusLabels[digest.status] ?? digest.status}</span>
      </header>
      <div className="ai-result-meta">
        <span>{formatDate(digest.created_at)}</span>
        <span>{digest.provider || 'OpenAI-compatible'}{digest.model ? ` / ${digest.model}` : ''}</span>
      </div>
      {processing && <p className="ai-processing-note"><Loader2 size={15} className="animate-spin motion-reduce:animate-none" aria-hidden="true" />正在整理标题级速览，完成后会自动更新。</p>}
      {digest.error && <p className="ai-error-box" role="alert">{digest.error}</p>}
      {storyNotice && <p className="ai-result-action-error" role="alert">{storyNotice}</p>}
      {canMarkRead && (
        <>
          <div className="ai-result-actions">
            <p className="ai-result-actions-copy">摘要中的 {readableStories.length} 个 Story 可以统一处理阅读状态。</p>
            <Button
              variant="secondary"
              size="sm"
              disabled={markReadPending || markReadDone}
              onClick={() => onMarkRead?.(digest.id)}
            >
              {markReadPending
                ? <Loader2 className="size-4 animate-spin motion-reduce:animate-none" aria-hidden="true" />
                : <CheckCircle2 className="size-4" aria-hidden="true" />}
              <span>{markReadPending ? '正在标记已读…' : markReadDone ? '相关 Story 已标为已读' : `将 ${readableStories.length} 个 Story 标记为已读`}</span>
            </Button>
          </div>
          {markReadError && <p className="ai-result-action-error" role="alert">{markReadError.message}</p>}
        </>
      )}
      {digest.overview && (
        <div className="ai-overview">
          <span className="ai-overview-label">AI 速览</span>
          <p>{digest.overview}</p>
        </div>
      )}
      {digest.priorities && digest.priorities.length > 0 && (
        <DigestPriorities priorities={digest.priorities} storyByID={storyByID} referenceContext={referenceContext} />
      )}
      {digest.themes && digest.themes.length > 0 && (
        <DigestThemes themes={digest.themes} storyByID={storyByID} referenceContext={referenceContext} />
      )}
      <section className="ai-source-section" aria-labelledby="digest-sources-title">
        <div className="ai-subsection-heading">
          <div>
            <p className="ai-eyebrow">STORY INDEX</p>
            <h3 id="digest-sources-title">来源 Story</h3>
          </div>
          <span>{stories.length} 个</span>
        </div>
        <div className="ai-source-list">
          {stories.map((story) => <StoryReference key={story.story_id} story={story} referenceKey={`index:${story.story_id}`} referenceContext={referenceContext} />)}
        </div>
      </section>
      {digest.omissions && digest.omissions.length > 0 && (
        <section className="ai-source-section" aria-labelledby="digest-omissions-title">
          <div className="ai-subsection-heading">
            <div>
              <p className="ai-eyebrow">NOT HIGHLIGHTED</p>
              <h3 id="digest-omissions-title">未被重点引用</h3>
            </div>
            <span>{digest.omissions.length} 个</span>
          </div>
          <div className="ai-source-list">
            {digest.omissions.map((item) => {
              const snapshot = item.story_id ? storyByID.get(item.story_id) : undefined
              return (
                <StoryReference
                  key={`omission-${item.label}-${item.story_id || item.title}`}
                  story={{
                    label: item.label,
                    story_id: item.story_id || '',
                    title: item.title,
                    entry_count: snapshot?.entry_count ?? 0,
                    source_count: snapshot?.source_count ?? 0,
                    available: snapshot?.available ?? false,
                    source_title: snapshot?.source_title,
                    sort_time: snapshot?.sort_time,
                  }}
                  referenceKey={`omission:${item.label}:${item.story_id || item.title}`}
                  referenceContext={referenceContext}
                />
              )
            })}
          </div>
        </section>
      )}
    </section>
  )
}

function DigestPriorities({ priorities, storyByID, referenceContext }: { priorities: DigestPriority[]; storyByID: Map<string, DigestStory>; referenceContext: DigestReferenceContext }) {
  return (
    <section className="ai-section" aria-labelledby="digest-priorities-title">
      <div className="ai-subsection-heading">
        <div>
          <p className="ai-eyebrow">READ NEXT</p>
          <h3 id="digest-priorities-title">建议优先阅读</h3>
        </div>
        <span>{priorities.length} 项</span>
      </div>
      <div className="ai-priority-list">
        {priorities.map((priority) => (
          <article className="ai-priority-item" key={`${priority.rank}-${priority.title}`}>
            <div className="ai-priority-title">
              <span>{priority.rank}</span>
              <strong>{priority.title}</strong>
            </div>
            <p>{priority.reason}</p>
            <StoryReferenceList ids={priority.story_ids} storyByID={storyByID} origin={`priority:${priority.rank}`} referenceContext={referenceContext} />
          </article>
        ))}
      </div>
    </section>
  )
}

function DigestThemes({ themes, storyByID, referenceContext }: { themes: DigestTheme[]; storyByID: Map<string, DigestStory>; referenceContext: DigestReferenceContext }) {
  return (
    <section className="ai-section" aria-labelledby="digest-themes-title">
      <div className="ai-subsection-heading">
        <div>
          <p className="ai-eyebrow">THEMES</p>
          <h3 id="digest-themes-title">主题归类</h3>
        </div>
        <span>{themes.length} 组</span>
      </div>
      <div className="ai-theme-list">
        {themes.map((theme) => (
          <article key={theme.title}>
            <strong>{theme.title}</strong>
            <p>{theme.summary}</p>
            <StoryReferenceList ids={theme.story_ids} storyByID={storyByID} origin={`theme:${theme.title}`} referenceContext={referenceContext} />
          </article>
        ))}
      </div>
    </section>
  )
}

function StoryReferenceList({ ids, storyByID, origin, referenceContext }: { ids: string[]; storyByID: Map<string, DigestStory>; origin: string; referenceContext: DigestReferenceContext }) {
  return <div className="ai-reference-list">{ids.map((id) => {
    const story = storyByID.get(id)
    return story
      ? <StoryReference key={id} story={story} referenceKey={`${origin}:${id}`} referenceContext={referenceContext} />
      : <a href={`/stories/${id}`} key={id}>查看 Story</a>
  })}</div>
}

function StoryReference({ story, referenceKey, referenceContext }: { story: DigestStory; referenceKey: string; referenceContext: DigestReferenceContext }) {
	if (!story.story_id || !story.available) {
		return <span className="ai-reference ai-reference-unavailable">{story.label} · {story.title || '来源 Story 已不可用'}</span>
	}
  return (
    <StoryListItem
      storyId={story.story_id}
      row={{
        label: story.label,
        title: story.title || '无标题',
        // Only multi-source stories carry distinct source info; a single source's
        // source_title is the article title itself, so showing it would duplicate
        // the title column.
        sourceLabel: story.source_count > 1 ? `${story.source_count} 个来源` : '',
        timestamp: story.sort_time ?? '',
        read: false,
      }}
      expanded={referenceContext.expandedKeys.has(referenceKey)}
      onExpandedChange={(open) => referenceContext.toggleExpanded(referenceKey, open)}
      mergeCandidates={referenceContext.mergeCandidates}
      onChanged={referenceContext.onChanged}
      onRemoved={() => referenceContext.onRemoved(story.story_id)}
      onError={referenceContext.onError}
    />
  )
}

export function StoryDetailPage({ storyID }: { storyID: string }) {
  const queryClient = useQueryClient()
  const storyQuery = useQuery({
    queryKey: queryKeys.story(storyID),
    queryFn: () => api.getStory(storyID),
    refetchInterval: (query) => isActiveStorySummary(query.state.data?.ai_summary) ? 1500 : false,
  })
  const requestMutation = useMutation({
    mutationFn: () => api.requestStorySummary(storyID),
    onSuccess: () => void queryClient.invalidateQueries({ queryKey: queryKeys.story(storyID) }),
  })

  if (storyQuery.isPending) return <div className="ai-page ai-state-page"><p className="ai-state">正在加载 Story…</p></div>
  if (storyQuery.error) return <div className="ai-page ai-state-page"><p className="ai-state ai-error">{storyQuery.error.message}</p></div>
  const story = storyQuery.data
  if (!story) return null
  const entries = story.entries && story.entries.length > 0 ? story.entries : [story.representative]
  const summary = story.ai_summary
  const title = story.display_title || story.representative.source_title || '无标题'
  const storyDate = story.first_published_at || story.representative.published_at || story.representative.discovered_at
  const summaryPending = requestMutation.isPending || isActiveStorySummary(summary)
  return (
    <div className="ai-page story-detail-page">
      <div className="story-detail-shell">
        <a className="story-back-link" href="/">
          <ArrowLeft size={15} aria-hidden="true" />
          <span>返回阅读库</span>
        </a>
        <header className="story-hero">
          <div className="story-hero-copy">
            <div className="story-kicker">
              <span className="story-kicker-icon" aria-hidden="true"><BookOpen size={20} /></span>
              <div>
                <p className="ai-eyebrow">STORY DETAIL</p>
                <span>聚合阅读对象</span>
              </div>
            </div>
            <h1>{title}</h1>
            <div className="story-meta-row" aria-label="Story 信息">
              <span className={`story-read-state ${story.read_at ? 'is-read' : 'is-unread'}`}>
                <span className="story-read-dot" aria-hidden="true" />
                {story.read_at ? '已读' : '未读'}
              </span>
              <span>{entries.length} 个 Entry</span>
              <span>{story.source_count} 个来源</span>
              <span>{formatDate(storyDate)}</span>
              {story.starred_at && <span className="story-meta-highlight"><Star size={13} fill="currentColor" aria-hidden="true" />已收藏</span>}
            </div>
            {story.tags && story.tags.length > 0 && (
              <div className="story-tag-list" aria-label="Story 标签">
                <Tag size={14} aria-hidden="true" />
                {story.tags.map((tag) => <span key={tag.id}>{tag.name}</span>)}
              </div>
            )}
          </div>
          <aside className="story-ai-action" aria-label="Story AI 摘要">
            <div className="story-ai-action-heading">
              <div className="story-ai-action-title">
                <span className="story-ai-action-icon" aria-hidden="true"><Sparkles size={17} /></span>
                <div>
                  <p className="ai-eyebrow">AI ASSIST</p>
                  <h2>快速理解这个 Story</h2>
                </div>
              </div>
              <span className="story-summary-request-status">{summary ? statusLabels[summary.status] ?? summary.status : '尚未生成'}</span>
            </div>
            <p>只读取这个 Story 的内容生成摘要，不会改变阅读状态。</p>
            <Button
              className="story-ai-action-button"
              disabled={summaryPending}
              onClick={() => void requestMutation.mutateAsync()}
            >
              {summaryPending
                ? <Loader2 className="size-4 animate-spin motion-reduce:animate-none" aria-hidden="true" />
                : <Sparkles className="size-4" aria-hidden="true" />}
              <span>{summaryPending ? '正在生成…' : isStoredStorySummary(summary) ? '重新生成AI摘要' : '生成AI摘要'}</span>
            </Button>
          </aside>
        </header>
        {requestMutation.error && <p className="ai-form-error story-request-error" role="alert">{requestMutation.error.message}</p>}
        {story.note && (
          <section className="story-note" aria-label="Story 笔记">
            <span className="story-note-label">NOTE</span>
            <p>{story.note}</p>
          </section>
        )}
        <StorySummaryCard summary={summary} />
        <section className="story-entry-list" aria-labelledby="story-entries-title">
          <header className="story-section-heading">
            <div>
              <p className="ai-eyebrow">SOURCE ENTRIES</p>
              <h2 id="story-entries-title">来源 Entries</h2>
              <p>每个 Entry 保留独立的来源、摘要和正文，适合按来源核对细节。</p>
            </div>
            <span className="story-section-count">{entries.length} 个</span>
          </header>
          <div className="story-entry-stack">
            {entries.map((entry, index) => <EntryCard entry={entry} index={index} key={entry.id} />)}
          </div>
        </section>
      </div>
    </div>
  )
}

function EntryCard({ entry, index }: { entry: Entry; index: number }) {
  return (
    <article className="story-entry-card" id={`entry-${entry.id}`}>
      <details className="story-entry-reader">
        <summary className="story-entry-heading">
          <div className="story-entry-title-wrap">
            <span className="story-entry-number" aria-hidden="true">E{String(index + 1).padStart(2, '0')}</span>
            <div className="story-entry-heading-copy">
              <h3>{entry.source_title || '无标题'}</h3>
              <p className="story-entry-meta">{entry.author || '未知作者'} · {formatDate(entry.published_at || entry.discovered_at)}</p>
            </div>
          </div>
          <ChevronDown className="story-entry-chevron" aria-hidden="true" />
        </summary>
        <div className="story-entry-body">
          <EntryReader
            entry={entry}
            className="story-entry-prose"
            title={entry.source_title || '无标题'}
            empty={<p className="story-entry-empty">这个 Entry 没有摘要，打开原文查看完整内容。</p>}
          />
        </div>
      </details>
    </article>
  )
}

function isActiveStatus(status?: string) {
  return Boolean(status && activeJobStatuses.has(status))
}

function formatDate(value?: string) {
  if (!value) return '刚刚'
  return new Intl.DateTimeFormat('zh-CN', { month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit' }).format(new Date(value))
}

function removeEmpty<T extends object>(value: T): T {
  return Object.fromEntries(Object.entries(value).filter(([, item]) => item !== undefined && item !== '')) as T
}
