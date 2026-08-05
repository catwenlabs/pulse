import { useEffect, useMemo, useState, type FormEvent } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { ArrowLeft, ArrowUpRight, BookOpen, CalendarDays, CheckCircle2, Clock3, ExternalLink, FileText, Info, Loader2, RefreshCw, Sparkles, Star, Tag, X } from 'lucide-react'

import * as api from './api'
import type { Digest, DigestPriority, DigestStory, DigestTheme, Entry, Story, StoryAISummary } from './api'
import { Button } from './components/ui/button'
import { Dialog, DialogClose, DialogContent, DialogDescription, DialogTitle } from './components/ui/dialog'
import { Input } from './components/ui/input'
import { sanitizeEntryHTML } from './lib/sanitizeEntryHTML'
import { queryKeys } from './query'

const activeJobStatuses = new Set(['pending', 'running', 'retry', 'queued'])
const digestScrollTopGap = 24

export function isStoredStorySummary(summary?: StoryAISummary) {
  return Boolean(summary && summary.status !== 'not_requested')
}

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
  const [markedDigestID, setMarkedDigestID] = useState('')
  const [scopeDialogOpen, setScopeDialogOpen] = useState(false)
  const [scopePrompted, setScopePrompted] = useState(false)
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
    refetchInterval: (query) => query.state.data?.some((digest) => isActiveStatus(digest.status)) ? 1500 : false,
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
    placeholderData: (previousData) => previousData,
    refetchInterval: (query) => isActiveStatus(query.state.data?.status) ? 1500 : false,
  })
  const createMutation = useMutation({
    mutationFn: api.createDigest,
    onSuccess: (job) => {
      setSelectedDigestID(job.target_id)
      void queryClient.invalidateQueries({ queryKey: queryKeys.digests })
    },
  })
  const markReadMutation = useMutation({
    mutationFn: ({ storyIDs }: { digestID: string; storyIDs: string[] }) => api.markStoriesRead({ storyIDs }),
    onSuccess: (_result, variables) => {
      void queryClient.invalidateQueries({ queryKey: queryKeys.readerRoot })
      void queryClient.invalidateQueries({ queryKey: queryKeys.sources })
      void queryClient.invalidateQueries({ queryKey: queryKeys.storyRoot })
      setMarkedDigestID(variables.digestID)
    },
  })

  const digests = digestsQuery.data ?? []
  const selectedDigest = selectedDigestQuery.data
  const selectedFromHistory = selectedDigestID || digests[0]?.id || ''
  const preview = previewQuery.data
  const scopeSelectionRequired = Boolean(preview?.matching_stories_truncated)
  const scopeDialogRequired = scopeSelectionRequired || scopePrompted
  const previewReady = maxStoriesValid
    && !previewQuery.isFetching
    && !previewQuery.error
    && preview !== undefined
    && digestScopesEqual(preview.scope, draftScope)
    && preview.matching_stories > 0
    && preview.can_queue
  const digestActionLabel = createMutation.isPending
    ? '正在排队…'
    : previewQuery.isPending || !preview
      ? '正在检查…'
      : previewQuery.error
        ? '重试检查'
        : scopeDialogRequired
          ? '设置追更范围'
          : preview.matching_stories === 0
            ? '没有未读 Story'
            : '生成追更摘要'
  const digestActionDisabled = previewQuery.isPending
    || (previewQuery.error
      ? false
      : scopeDialogRequired
        ? previewQuery.isFetching
        : createMutation.isPending || !previewReady)

  useEffect(() => {
    if (!selectedDigestID && digests[0]) setSelectedDigestID(digests[0].id)
  }, [digests, selectedDigestID])

  useEffect(() => {
    if (scopeSelectionRequired && !scopePrompted) {
      if (!maxStories.trim()) setMaxStories(String(preview?.safety_limit ?? 100))
      setScopePrompted(true)
      setScopeDialogOpen(true)
    }
  }, [maxStories, preview?.safety_limit, scopePrompted, scopeSelectionRequired])

  async function createDigest(event?: FormEvent) {
    event?.preventDefault()
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
      setScopeDialogOpen(false)
    } catch (cause) {
      setFormError(cause instanceof Error ? cause.message : '无法创建追更摘要')
    }
  }

  function selectDigest(digestID: string, source: HTMLButtonElement) {
    if (selectedDigestID === digestID) return
    const scrollContainer = source.closest<HTMLElement>('main')
    if (scrollContainer) {
      const historyLayout = source.closest<HTMLElement>('.ai-history-layout')
      const layoutRect = historyLayout?.getBoundingClientRect()
      const containerRect = scrollContainer.getBoundingClientRect()
      const targetTop = layoutRect
        ? Math.max(0, scrollContainer.scrollTop + layoutRect.top - containerRect.top - digestScrollTopGap)
        : 0
      scrollContainer.scrollTo({ top: targetTop, left: 0, behavior: 'smooth' })
    } else {
      window.scrollTo({ top: 0, left: 0, behavior: 'smooth' })
    }
    setSelectedDigestID(digestID)
  }

  return (
    <div className="ai-page">
      <header className="ai-page-header">
        <div className="ai-page-header-copy">
          <p className="ai-eyebrow ai-eyebrow-with-icon"><Sparkles size={14} aria-hidden="true" />AI CATCH-UP</p>
          <h1>AI 追更</h1>
          <p className={`ai-page-subtitle ${previewQuery.error ? 'ai-page-subtitle-error' : ''}`} aria-live="polite" role={previewQuery.error ? 'alert' : undefined}>
            {formatDigestSubtitle(preview, previewQuery.isPending, previewQuery.error)}
          </p>
        </div>
        <Button
          className="ai-header-action min-w-[148px] cursor-pointer"
          disabled={digestActionDisabled}
          onClick={() => {
            if (previewQuery.error) {
              void previewQuery.refetch()
            } else if (scopeDialogRequired) {
              setScopeDialogOpen(true)
            } else {
              void createDigest()
            }
          }}
        >
          {createMutation.isPending || previewQuery.isPending
            ? <Loader2 className="size-4 animate-spin motion-reduce:animate-none" aria-hidden="true" />
            : previewQuery.error
              ? <RefreshCw className="size-4" aria-hidden="true" />
              : scopeDialogRequired
                ? <CalendarDays className="size-4" aria-hidden="true" />
                : <Sparkles className="size-4" aria-hidden="true" />}
          <span>{digestActionLabel}</span>
        </Button>
      </header>

      <Dialog
        open={scopeDialogOpen}
        onOpenChange={(open) => {
          setScopeDialogOpen(open)
          if (open) setFormError('')
        }}
      >
        <DialogContent className="ai-scope-dialog">
          <DialogClose className="absolute right-4 top-4 grid size-8 cursor-pointer place-items-center rounded-md text-muted-foreground hover:bg-accent hover:text-foreground" aria-label="关闭">
            <X className="size-4" aria-hidden="true" />
          </DialogClose>
          <DialogTitle>设置追更范围</DialogTitle>
          <DialogDescription>
            当前未读 Story 超过 {preview?.safety_limit ?? 100} 个的安全上限。缩小时间范围或指定最多 Story 数后再生成。
          </DialogDescription>
          <form onSubmit={(event) => void createDigest(event)}>
            <fieldset className="ai-scope-fieldset">
              <legend>追更窗口</legend>
              <div className="ai-form-grid">
                <label>
                  <span><CalendarDays size={14} aria-hidden="true" />最早时间（可选）</span>
                  <Input type="datetime-local" value={startAt} onChange={(event) => setStartAt(event.target.value)} />
                </label>
                <label>
                  <span><CalendarDays size={14} aria-hidden="true" />最晚时间（可选）</span>
                  <Input type="datetime-local" value={endAt} onChange={(event) => setEndAt(event.target.value)} />
                </label>
                <label>
                  <span><FileText size={14} aria-hidden="true" />最多 Story（可选）</span>
                  <Input inputMode="numeric" min={1} placeholder="默认安全上限" value={maxStories} onChange={(event) => setMaxStories(event.target.value)} />
                </label>
              </div>
            </fieldset>
            {formError && <p className="ai-form-error" role="alert">{formError}</p>}
            {createMutation.isError && !formError && <p className="ai-form-error" role="alert">{createMutation.error.message}</p>}
            {previewQuery.error && <p className="ai-form-error" role="alert">{previewQuery.error.message}</p>}
            {previewQuery.data && (
              <p className="ai-scope-preview" aria-live="polite">
                <Info size={15} aria-hidden="true" />
                <span>{formatScopePreview(previewQuery.data)}</span>
              </p>
            )}
            <div className="ai-dialog-actions">
              <DialogClose asChild><Button type="button" variant="secondary">取消</Button></DialogClose>
              <Button type="submit" className="cursor-pointer" disabled={createMutation.isPending || (maxStoriesValid && !previewReady)}>
                {createMutation.isPending ? <Loader2 className="size-4 animate-spin motion-reduce:animate-none" aria-hidden="true" /> : <Sparkles className="size-4" aria-hidden="true" />}
                <span>{createMutation.isPending
                  ? '正在排队…'
                  : !maxStoriesValid && maxStories.trim()
                    ? '数量必须是正整数'
                    : previewQuery.isFetching || !preview
                      ? '正在检查范围…'
                      : preview.matching_stories === 0
                        ? '没有可处理的 Story'
                        : !preview.can_queue
                          ? '请先缩小范围'
                          : '生成追更摘要'}</span>
              </Button>
            </div>
          </form>
        </DialogContent>
      </Dialog>

      <div className="ai-history-layout">
        <section className="ai-history-card" aria-labelledby="digest-history-title">
          <div className="ai-card-heading ai-history-heading">
            <div className="ai-card-heading-title">
              <span className="ai-card-icon" aria-hidden="true"><Clock3 size={18} /></span>
              <div>
                <p className="ai-eyebrow">HISTORY</p>
                <h2 id="digest-history-title">历史追更</h2>
              </div>
            </div>
            <span className="ai-history-count"><strong>{digests.length}</strong> 份</span>
          </div>
          <p className="ai-card-helper">选择一份摘要，在右侧查看标题级整理结果。</p>
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
          <div className="ai-history-list">
            {digests.map((digest) => (
              <button
                className={`ai-history-item ${selectedFromHistory === digest.id ? 'is-selected' : ''}`}
                key={digest.id}
                type="button"
                aria-pressed={selectedFromHistory === digest.id}
                onClick={(event) => selectDigest(digest.id, event.currentTarget)}
              >
                <span className="ai-history-item-main">
                  <span className="ai-history-item-title">{digest.story_count} 个未读 Story</span>
                  <span className={`ai-status ai-status-${digest.status}`}>{statusLabels[digest.status] ?? digest.status}</span>
                </span>
                <span className="ai-history-item-meta">{formatScopeRange(digest) || '全部未读'} · {formatDate(digest.created_at)}</span>
                <ArrowUpRight className="ai-history-item-arrow" size={16} aria-hidden="true" />
              </button>
            ))}
          </div>
        </section>

        <DigestResult
          digest={selectedDigest}
          loading={selectedDigestQuery.isPending && Boolean(selectedDigestID)}
          refreshing={selectedDigestQuery.isFetching && selectedDigestQuery.isPlaceholderData}
          error={selectedDigestQuery.error}
          onRetry={() => void selectedDigestQuery.refetch()}
          onMarkRead={(digestID, storyIDs) => markReadMutation.mutate({ digestID, storyIDs })}
          markReadPending={markReadMutation.isPending && markReadMutation.variables?.digestID === selectedDigest?.id}
          markReadDone={markedDigestID === selectedDigest?.id}
          markReadError={markReadMutation.variables?.digestID === selectedDigest?.id && markReadMutation.error instanceof Error ? markReadMutation.error : null}
        />
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

function formatDigestSubtitle(preview: api.DigestPreview | undefined, loading: boolean, error: Error | null) {
  const behaviorNote = '生成本身不会标记任何 Story 为已读。'
  if (loading && !preview) return `正在检查当前未读 Story 数量。${behaviorNote}`
  if (error && !preview) return `暂时无法获取当前未读 Story 数量。${behaviorNote}`
  if (!preview) return behaviorNote
  if (preview.matching_stories_truncated) {
    return `当前未读 Story 超过 ${preview.safety_limit} 条。${behaviorNote}`
  }
  return `当前有 ${preview.matching_stories} 条未读 Story。${behaviorNote}`
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
  onMarkRead?: (digestID: string, storyIDs: string[]) => void
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
      {canMarkRead && (
        <>
          <div className="ai-result-actions">
            <p className="ai-result-actions-copy">摘要中的 {readableStories.length} 个 Story 可以统一处理阅读状态。</p>
            <Button
              variant="secondary"
              size="sm"
              disabled={markReadPending || markReadDone}
              onClick={() => onMarkRead?.(digest.id, readableStories.map((story) => story.story_id))}
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
        <DigestPriorities priorities={digest.priorities} storyByID={storyByID} />
      )}
      {digest.themes && digest.themes.length > 0 && (
        <DigestThemes themes={digest.themes} storyByID={storyByID} />
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
          {stories.map((story) => <StoryReference key={story.story_id} story={story} />)}
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
      <span className="ai-reference-label">{story.label}</span>
      <strong>{story.title || '无标题'}</strong>
      {story.entry_count > 0 && <small>{story.entry_count} 个 Entry · {story.source_count} 个来源</small>}
      <ArrowUpRight className="ai-reference-arrow" size={15} aria-hidden="true" />
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
  const entries = story.entries && story.entries.length > 0 ? story.entries : [story.representative]
  const summary = story.ai_summary
  const title = story.display_title || story.representative.source_title || '无标题'
  const storyDate = story.first_published_at || story.representative.published_at || story.representative.discovered_at
  const summaryPending = requestMutation.isPending || storySummaryIsActive(summary)
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

export function StorySummaryCard({
  summary,
  loading = false,
  loadError = null,
  onRetry,
}: {
  summary?: StoryAISummary
  loading?: boolean
  loadError?: Error | null
  onRetry?: () => void
}) {
  if (loading) {
    return (
      <section className="ai-summary-card story-summary-card" aria-live="polite" role="status">
        <div className="ai-loading-state">
          <span className="ai-loading-icon" aria-hidden="true"><Loader2 size={22} className="animate-spin motion-reduce:animate-none" /></span>
          <strong>正在加载 AI 摘要</strong>
          <p>正在读取这个 Story 的摘要状态，请稍候。</p>
        </div>
      </section>
    )
  }
  if (loadError) {
    return (
      <section className="ai-summary-card story-summary-card" role="alert">
        <div className="ai-loading-state ai-error">
          <strong>暂时无法加载 AI 摘要</strong>
          <p>{loadError.message}</p>
          {onRetry && <Button variant="secondary" size="sm" onClick={onRetry}>重试加载</Button>}
        </div>
      </section>
    )
  }
  if (!summary || summary.status === 'not_requested') {
    return (
      <section className="ai-summary-card story-summary-card pb-6" aria-labelledby="story-summary-title">
        <header className="ai-card-heading story-summary-heading">
          <div className="ai-card-heading-title">
            <span className="ai-card-icon" aria-hidden="true"><FileText size={18} /></span>
            <div>
              <p className="ai-eyebrow">AI STORY SUMMARY</p>
              <h2 id="story-summary-title">内容摘要</h2>
            </div>
          </div>
          <span className="story-summary-request-status">按需生成</span>
        </header>
        <p className="story-summary-empty-copy">还没有摘要。点击「生成AI摘要」后，AI 才会读取这个 Story 的内容。</p>
      </section>
    )
  }
  return (
    <section className="ai-summary-card story-summary-card pb-6" aria-labelledby="story-summary-title">
      <header className="ai-card-heading story-summary-heading">
        <div className="ai-card-heading-title">
          <span className="ai-card-icon" aria-hidden="true"><FileText size={18} /></span>
          <div>
            <p className="ai-eyebrow">AI STORY SUMMARY</p>
            <h2 id="story-summary-title">内容摘要</h2>
          </div>
        </div>
        <span className={`ai-status ai-status-${summary.status}`}>{statusLabels[summary.status] ?? summary.status}</span>
      </header>
      {summary.error && <p className="ai-error-box">{summary.error}</p>}
      {storySummaryIsActive(summary) && (
        <p className="story-summary-processing" role="status">
          <Loader2 size={15} className="animate-spin motion-reduce:animate-none" aria-hidden="true" />
          摘要正在生成，完成后会自动更新。
        </p>
      )}
      {summary.overview && (
        <div className="ai-overview">
          <span className="ai-overview-label">AI 速览</span>
          <p>{summary.overview}</p>
        </div>
      )}
      {summary.key_points && summary.key_points.length > 0 && (
        <section className="story-summary-section" aria-labelledby="story-key-points-title">
          <div className="story-summary-section-heading">
            <h3 id="story-key-points-title">重点提要</h3>
            <span>{summary.key_points.length} 条</span>
          </div>
          <ul className="ai-key-point-list">{summary.key_points.map((point) => <li key={point}>{point}</li>)}</ul>
        </section>
      )}
      {summary.sources && summary.sources.length > 0 && (
        <section className="ai-source-section story-summary-sources" aria-labelledby="story-summary-sources-title">
          <div className="ai-subsection-heading">
            <div>
              <p className="ai-eyebrow">REFERENCES</p>
              <h3 id="story-summary-sources-title">来源说明</h3>
            </div>
            <span>{summary.sources.length} 个</span>
          </div>
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

function EntryCard({ entry, index }: { entry: Entry; index: number }) {
  const contentHTML = entry.content_html?.trim()
    ? sanitizeEntryHTML(entry.content_html, entry.canonical_url)
    : ''
  return (
    <article className="story-entry-card" id={`entry-${entry.id}`}>
      <header className="story-entry-heading">
        <div className="story-entry-title-wrap">
          <span className="story-entry-number" aria-hidden="true">E{String(index + 1).padStart(2, '0')}</span>
          <div className="story-entry-heading-copy">
            <p className="story-entry-kicker">SOURCE ENTRY</p>
            <h3>{entry.source_title || '无标题'}</h3>
            <p className="story-entry-meta">{entry.author || '未知作者'} · {formatDate(entry.published_at || entry.discovered_at)}</p>
          </div>
        </div>
        {entry.canonical_url && (
          <a href={entry.canonical_url} rel="noreferrer" target="_blank">
            <ExternalLink size={14} aria-hidden="true" />
            <span>打开原文</span>
          </a>
        )}
      </header>
      <div className="story-entry-body">
        {entry.summary && <p className="story-entry-summary">{entry.summary}</p>}
        {contentHTML ? (
          <details className="story-entry-reader" open={index === 0}>
            <summary className="story-entry-reader-toggle">
              <span>阅读正文</span>
              <span>展开 / 收起</span>
            </summary>
            <div className="entry-prose story-entry-prose" dangerouslySetInnerHTML={{ __html: contentHTML }} />
          </details>
        ) : !entry.summary ? (
          <p className="story-entry-empty">这个 Entry 没有摘要，打开原文查看完整内容。</p>
        ) : null}
      </div>
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
