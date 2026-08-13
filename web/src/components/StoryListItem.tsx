import { useEffect, useRef, useState, type RefObject } from 'react'
import { useNavigate } from '@tanstack/react-router'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { ChevronDown, MoreHorizontal, Sparkles, X } from 'lucide-react'

import * as api from '../api'
import type { Entry, Source, Story, StoryPatch } from '../api'
import { EntryReader } from './EntryReader'
import { SelectionChatSurface } from './SelectionChatSurface'
import { isActiveStorySummary, isStoredStorySummary, StorySummaryCard } from './storySummary'
import { Button } from './ui/button'
import { Dialog, DialogClose, DialogContent, DialogDescription, DialogTitle } from './ui/dialog'
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuTrigger } from './ui/dropdown-menu'
import { Input } from './ui/input'
import { Textarea } from './ui/textarea'
import { compactTime, HighlightText, nearestScrollContainer, scrollWithin } from '../lib/readerFormat'
import { cn } from '../lib/utils'
import { queryKeys } from '../query'

export interface StoryListItemRow {
  title: string
  sourceLabel: string
  timestamp: string
  read: boolean
  highlightQuery?: string
  emphasized?: boolean
  label?: string
}

export interface StoryMergeCandidate {
  storyId: string
  title: string
  displayTitle?: string
  note?: string
}

export type StoryListItemChange =
  | { kind: 'patched'; story: Story; patch: StoryPatch }
  | { kind: 'representative'; story: Story }
  | { kind: 'split'; story: Story; removedEntryId: string }
  | { kind: 'summary-requested'; storyId: string }

export interface StoryListItemProps {
  storyId: string
  row: StoryListItemRow
  expanded: boolean
  onExpandedChange: (expanded: boolean, rowElement: HTMLElement | null) => void
  // The entry id the parent uses to key/reconcile this row. The reader keys
  // rows by the representative entry id, which can diverge from the story's
  // current representative after a set-representative; deletion and removal
  // must target this id so the correct row is updated. Defaults to the
  // story's representative entry id (fine for story-keyed parents).
  rowEntryId?: string
  // Optional Story snapshot the parent already has (e.g. from a list response).
  // Seeds the expansion so it renders immediately on open instead of waiting
  // for the getStory fetch; the fetch still runs to enrich multi-source
  // entries and the AI summary.
  initialStory?: Story
  mergeCandidates?: StoryMergeCandidate[]
  sources?: Source[]
  markReadOnExpand?: boolean
  scrollContainerRef?: RefObject<HTMLElement | null>
  onChanged?: (change: StoryListItemChange) => void
  onRemoved?: (storyId: string, entryId: string) => void
  onError?: (message: string) => void
}

// Project story-level metadata onto an entry so the expansion can render a
// single active entry with the story's title/note/read state, mirroring the
// reader's projectReaderEntry.
function projectEntry(entry: Entry, story: Story): Entry & { display_title: string; note: string } {
  return {
    ...entry,
    display_title: story.display_title ?? '',
    note: story.note ?? '',
  }
}

export function StoryListItem({
  storyId,
  row,
  expanded,
  onExpandedChange,
  rowEntryId,
  initialStory,
  mergeCandidates,
  sources,
  markReadOnExpand = true,
  scrollContainerRef,
  onChanged,
  onRemoved,
  onError,
}: StoryListItemProps) {
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const [actionMenuOpen, setActionMenuOpen] = useState(false)
  const [notesOpen, setNotesOpen] = useState(false)
  const [mergePickerOpen, setMergePickerOpen] = useState(false)
  const [activeEntryId, setActiveEntryId] = useState<string | null>(null)
  const [splitRequest, setSplitRequest] = useState<{ entryID: string; options: api.SplitOptions } | null>(null)
  const [mergeResolution, setMergeResolution] = useState<{ target: StoryMergeCandidate; displayTitle: string; note: string } | null>(null)
  const [deleteConfirmation, setDeleteConfirmation] = useState<api.DeletionConfirmation | null>(null)
  const [deleteDialogOpen, setDeleteDialogOpen] = useState(false)
  const [inlineSummaryRequested, setInlineSummaryRequested] = useState(false)
  const [titleDraft, setTitleDraft] = useState('')
  const [noteDraft, setNoteDraft] = useState('')
  const rowElement = useRef<HTMLElement | null>(null)
  const pendingScroll = useRef(false)
  const markedRead = useRef(false)

  const storyQuery = useQuery({
    queryKey: queryKeys.story(storyId),
    queryFn: () => api.getStory(storyId),
    enabled: expanded,
    refetchInterval: (query) => isActiveStorySummary(query.state.data?.ai_summary) ? 1500 : false,
  })
  const story = storyQuery.data

  const summaryMutation = useMutation({
    mutationFn: () => api.requestStorySummary(storyId),
    onSuccess: () => void queryClient.invalidateQueries({ queryKey: queryKeys.story(storyId) }),
  })

  // Seed the story cache from the parent's snapshot so the expansion renders
  // immediately; mark it stale (updatedAt: 0) so getStory still refetches to
  // enrich multi-source entries and the AI summary.
  useEffect(() => {
    if (!storyId || !initialStory) return
    const cached = queryClient.getQueryData<Story>(queryKeys.story(storyId))
    if (!cached) queryClient.setQueryData<Story>(queryKeys.story(storyId), initialStory, { updatedAt: 0 })
  }, [storyId, initialStory, queryClient])

  const sourceNames = Object.fromEntries((sources ?? []).map((source) => [source.id, source.name]))
  const candidates = (mergeCandidates ?? []).filter((candidate) => candidate.storyId !== storyId)

  function reportError(cause: unknown, fallback: string) {
    onError?.(cause instanceof Error ? cause.message : fallback)
  }

  // Merge a mutation response over the cached story, preserving entries when
  // the response omits them.
  function writeStory(updated: Story) {
    queryClient.setQueryData<Story>(queryKeys.story(storyId), (current) => (
      current ? { ...current, ...updated, entries: updated.entries ?? current.entries } : updated
    ))
  }

  async function applyPatch(patch: StoryPatch) {
    try {
      const updated = await api.updateStory(storyId, patch)
      writeStory(updated)
      onChanged?.({ kind: 'patched', story: updated, patch })
    } catch (cause) {
      reportError(cause, '更新 Story 失败')
    }
  }

  function requestSummary() {
    setInlineSummaryRequested(true)
    summaryMutation.mutate()
    onChanged?.({ kind: 'summary-requested', storyId })
  }

  async function setRepresentative(entryId: string) {
    try {
      const updated = await api.setStoryRepresentative(storyId, entryId)
      writeStory(updated)
      onChanged?.({ kind: 'representative', story: updated })
    } catch (cause) {
      reportError(cause, '设置默认来源失败')
    }
  }

  async function splitEntry(entryId: string, options: api.SplitOptions) {
    if (!story?.entries) return
    try {
      await api.splitStory(storyId, entryId, options)
      const remaining = story.entries.filter((candidate) => candidate.id !== entryId)
      const updated: Story = {
        ...story,
        entries: remaining,
        entry_count: remaining.length,
        source_count: new Set(remaining.map((candidate) => candidate.source_id)).size,
      }
      writeStory(updated)
      setSplitRequest(null)
      onChanged?.({ kind: 'split', story: updated, removedEntryId: entryId })
    } catch (cause) {
      reportError(cause, '拆分报道失败')
    }
  }

  async function mergeInto(target: StoryMergeCandidate, options: { display_title?: string; note?: string } = {}) {
    if (!story) return
    try {
      await api.mergeStory(storyId, target.storyId, options)
      setMergePickerOpen(false)
      setMergeResolution(null)
      onRemoved?.(storyId, rowEntryId ?? story.representative.id)
      onExpandedChange(false, rowElement.current)
    } catch (cause) {
      if (cause instanceof api.APIError && cause.status === 409) {
        setMergePickerOpen(false)
        setMergeResolution({
          target,
          displayTitle: story.display_title || target.displayTitle || '',
          note: story.note || target.note || '',
        })
        return
      }
      reportError(cause, '合并 Story 失败')
    }
  }

  async function confirmDelete() {
    if (!story) return
    const entryId = rowEntryId ?? story.representative.id
    try {
      await api.deleteEntry(entryId, Boolean(deleteConfirmation))
      setDeleteDialogOpen(false)
      setDeleteConfirmation(null)
      onRemoved?.(storyId, entryId)
      onExpandedChange(false, rowElement.current)
    } catch (cause) {
      if (cause instanceof api.APIError && cause.status === 409) {
        setDeleteConfirmation(cause.problem)
        return
      }
      reportError(cause, '删除来源内容失败')
    }
  }

  function open() {
    pendingScroll.current = true
    setActionMenuOpen(false)
    setNotesOpen(false)
    setMergePickerOpen(false)
    setActiveEntryId(null)
    onExpandedChange(true, rowElement.current)
  }

  function close() {
    const trigger = rowElement.current?.querySelector<HTMLElement>('button[aria-expanded]') ?? null
    // Move focus to the row trigger before the panel unmounts, otherwise the
    // browser jump-scrolls when the focused close button is removed.
    trigger?.focus({ preventScroll: true })
    setActionMenuOpen(false)
    setNotesOpen(false)
    setMergePickerOpen(false)
    setActiveEntryId(null)
    onExpandedChange(false, rowElement.current)
    window.requestAnimationFrame(() => {
      if (rowElement.current) scrollWithin(scrollContainerRef?.current ?? nearestScrollContainer(rowElement.current), rowElement.current)
    })
  }

  // Reset transient state and seed the notes draft when a story is opened.
  useEffect(() => {
    if (!expanded) {
      setInlineSummaryRequested(false)
      markedRead.current = false
    }
  }, [expanded])

  // Marking read is driven by the expanded prop (parent-controlled), not the
  // click handler, so it fires regardless of how the row was opened.
  useEffect(() => {
    if (expanded && markReadOnExpand && !row.read && !markedRead.current) {
      markedRead.current = true
      void applyPatch({ read: true })
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [expanded, markReadOnExpand, row.read])

  useEffect(() => {
    if (story) {
      setTitleDraft(story.display_title ?? '')
      setNoteDraft(story.note ?? '')
    }
    // Only reseed when a different story loads, not on every poll/refetch.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [story?.id])

  useEffect(() => {
    if (!expanded) return
    function handleKeyDown(event: KeyboardEvent) {
      if (event.key !== 'Escape') return
      close()
    }
    document.addEventListener('keydown', handleKeyDown)
    return () => document.removeEventListener('keydown', handleKeyDown)
  })

  // The body entry defaults to the parent's snapshot (the list entry, whose
  // content is stable and never clobbered by the enrichment fetch); the
  // enrichment fetch's entries are only used for an explicit multi-source
  // switch. This mirrors the Reader: the article body always comes from the
  // list, getStory only adds siblings + the AI summary.
  const anchorEntry = initialStory?.representative
  const activeEntry = story
    ? projectEntry(
        (activeEntryId ? story.entries?.find((entry) => entry.id === activeEntryId) : undefined)
          ?? anchorEntry
          ?? story.representative,
        story,
      )
    : null
  const inlineSummary = story?.ai_summary
  const summaryRequestPending = summaryMutation.isPending || isActiveStorySummary(inlineSummary)
  const hasStoredStorySummary = isStoredStorySummary(inlineSummary)
  const summaryActionLabel = summaryRequestPending
    ? '正在生成…'
    : hasStoredStorySummary
      ? '重新生成AI摘要'
      : '生成AI摘要'
  const inlineSummaryVisible = Boolean(expanded && (inlineSummaryRequested || hasStoredStorySummary || storyQuery.error))
  const displayedInlineSummary = inlineSummary
    ?? (summaryRequestPending ? { story_id: storyId, status: 'queued' as const } : undefined)

  const rowId = rowEntryId ?? storyId
  return (
    <article
      data-entry-row={rowId}
      className={cn(
        'border-b last:border-b-0',
        expanded && 'bg-card shadow-[inset_3px_0_hsl(var(--border))]',
        row.emphasized && 'new-content-highlight',
      )}
      ref={(element) => { rowElement.current = element }}
    >
      <Button unstyled
        className={cn(
          'grid min-h-9 w-full cursor-pointer grid-cols-[8px_minmax(110px,15%)_minmax(220px,1fr)_54px_16px] items-center gap-1.5 border-0 bg-card px-2 text-left hover:bg-muted/60',
          'max-md:min-h-11 max-md:grid-cols-[8px_minmax(0,1fr)_48px_14px] max-md:grid-rows-1 max-md:gap-x-1.5 max-md:px-2 max-md:py-0.5',
          row.read && 'text-muted-foreground [&_strong]:font-normal',
        )}
        aria-expanded={expanded}
        onClick={() => (expanded ? close() : open())}
      >
        <span className={cn('size-1.5 rounded-full bg-primary', row.read && 'border border-muted-foreground bg-transparent')} aria-hidden="true" />
        <span className="truncate text-sm font-medium text-[#66717d] max-md:hidden">
          {row.label && <span className="ai-reference-label">{row.label} </span>}
          <HighlightText text={row.sourceLabel} query={row.highlightQuery ?? ''} />
        </span>
        <strong className="min-w-0 truncate text-base font-semibold leading-6 max-md:col-start-2">
          <HighlightText text={row.title} query={row.highlightQuery ?? ''} />
        </strong>
        <time className="text-right text-xs tabular-nums text-muted-foreground max-md:col-start-3" dateTime={row.timestamp}>{row.timestamp ? compactTime(row.timestamp) : ''}</time>
        <ChevronDown className={cn('size-4 text-muted-foreground transition-transform max-md:col-start-4', expanded && 'rotate-180')} aria-hidden="true" />
      </Button>
      {expanded && (
        <div
          data-entry-detail={rowId}
          className="min-h-full border-t border-[#e8e9eb] bg-card px-[clamp(24px,8vw,120px)] pb-16 max-md:px-5 max-md:pb-10 max-md:pt-0"
          ref={(element) => {
            if (!element || !pendingScroll.current) return
            pendingScroll.current = false
            window.requestAnimationFrame(() => {
              scrollWithin(scrollContainerRef?.current ?? nearestScrollContainer(element), element)
            })
          }}
        >
          <div className="mx-auto max-w-[72ch]">
            {!story && (
              <p className="py-8 text-center text-sm text-muted-foreground">正在加载 Story…</p>
            )}
            {story && activeEntry && (
              <>
                <h2 className="mb-4 mt-1 text-xl font-bold leading-snug">
                  {activeEntry.canonical_url ? (
                    <a
                      className="text-foreground underline-offset-4 hover:underline"
                      href={activeEntry.canonical_url}
                      target="_blank"
                      rel="noreferrer"
                      title="查看原文"
                    >
                      {activeEntry.display_title || activeEntry.source_title || '无标题'}
                    </a>
                  ) : (
                    activeEntry.display_title || activeEntry.source_title || '无标题'
                  )}
                </h2>
                {activeEntry.display_title && activeEntry.source_title && activeEntry.display_title !== activeEntry.source_title && (
                  <p className="-mt-2 mb-4 text-sm text-muted-foreground">来源标题：{activeEntry.source_title}</p>
                )}
                <div className="mb-5 flex min-h-12 flex-wrap items-center justify-between gap-2 border-b border-[#eeeae2] text-sm text-muted-foreground max-md:mb-3">
                  <span className="min-w-0 truncate">{activeEntry.author || sourceNames[activeEntry.source_id] || activeEntry.source_title || '未知来源'}</span>
                  <div className="flex min-w-0 shrink-0 items-center gap-1">
                    <Button
                      unstyled
                      className="inline-flex min-h-9 min-w-0 shrink-0 cursor-pointer items-center gap-1.5 rounded-md border-0 bg-transparent px-1.5 text-xs font-medium text-muted-foreground hover:text-primary hover:underline hover:underline-offset-2 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2"
                      aria-label={summaryActionLabel}
                      disabled={summaryRequestPending || storyQuery.isPending}
                      onClick={requestSummary}
                    >
                      <Sparkles className="size-4 shrink-0" aria-hidden="true" />
                      <span>{summaryActionLabel}</span>
                    </Button>
                    <DropdownMenu open={actionMenuOpen} onOpenChange={setActionMenuOpen}>
                      <DropdownMenuTrigger asChild>
                        <Button unstyled className="grid size-10 cursor-pointer place-items-center rounded-md text-muted-foreground hover:bg-accent hover:text-foreground" aria-label="更多操作"><MoreHorizontal className="size-4" aria-hidden="true" /></Button>
                      </DropdownMenuTrigger>
                      <DropdownMenuContent>
                        <DropdownMenuItem onSelect={() => void applyPatch({ read: false })}>标记未读</DropdownMenuItem>
                        <DropdownMenuItem onSelect={() => void applyPatch({ starred: !story.starred_at })}>
                          {story.starred_at ? '取消收藏' : '收藏文章'}
                        </DropdownMenuItem>
                        <DropdownMenuItem onSelect={() => void applyPatch({ later: !story.later_at })}>
                          {story.later_at ? '移出稍后阅读' : '稍后阅读'}
                        </DropdownMenuItem>
                        <DropdownMenuItem onSelect={() => setNotesOpen((openNote) => !openNote)}>
                          编辑标题与笔记
                        </DropdownMenuItem>
                        <DropdownMenuItem onSelect={() => void navigate({
                          to: '/stories/$storyID',
                          params: { storyID: storyId },
                        })}>
                          查看 Story 与 AI 摘要
                        </DropdownMenuItem>
                        {candidates.length > 0 && (
                          <DropdownMenuItem onSelect={() => setMergePickerOpen(true)}>合并到其他 Story</DropdownMenuItem>
                        )}
                        <DropdownMenuItem onSelect={() => setDeleteDialogOpen(true)}>永久删除来源内容</DropdownMenuItem>
                      </DropdownMenuContent>
                    </DropdownMenu>
                    <Button
                      unstyled
                      className="grid size-10 cursor-pointer place-items-center rounded-md text-muted-foreground hover:bg-accent hover:text-foreground"
                      aria-label="关闭文章"
                      onClick={close}
                    >
                      <X className="size-5" aria-hidden="true" />
                    </Button>
                  </div>
                </div>
                {inlineSummaryVisible && (
                  <div className="reader-inline-summary">
                    <StorySummaryCard
                      summary={displayedInlineSummary}
                      loading={storyQuery.isPending && !displayedInlineSummary}
                      loadError={storyQuery.error instanceof Error ? storyQuery.error : null}
                      onRetry={() => void storyQuery.refetch()}
                    />
                    {summaryMutation.isError && (
                      <p className="story-request-error" role="alert">{summaryMutation.error.message}</p>
                    )}
                  </div>
                )}
                {(story.entries?.length ?? 0) > 1 && (
                  <div className="mb-5 flex flex-wrap items-center gap-2 text-sm text-muted-foreground">
                    <span>同一则新闻 · {story.entries!.length} 个来源：</span>
                    {story.entries!.map((sourceEntry) => {
                      const sourceName = sourceNames[sourceEntry.source_id] || sourceEntry.author || sourceEntry.source_title || '未知来源'
                      const isActive = sourceEntry.id === activeEntry.id
                      return (
                        <span className="inline-flex items-center gap-1" key={sourceEntry.id}>
                          <Button
                            unstyled
                            className={cn(
                              'cursor-pointer',
                              isActive ? 'font-semibold text-foreground underline' : 'text-primary hover:underline',
                            )}
                            aria-pressed={isActive}
                            aria-label={`切换到来源 ${sourceName}`}
                            onClick={() => setActiveEntryId(sourceEntry.id)}
                          >
                            {sourceName}
                          </Button>
                          {sourceEntry.canonical_url && (
                            <a
                              className="text-muted-foreground hover:text-foreground"
                              href={sourceEntry.canonical_url}
                              rel="noreferrer"
                              target="_blank"
                              aria-label={`在新标签打开 ${sourceName} 原文`}
                            >
                              ↗
                            </a>
                          )}
                          {sourceEntry.id !== story.representative.id && (
                            <Button
                              unstyled
                              className="text-xs text-muted-foreground hover:text-foreground"
                              aria-label={`设为默认来源 ${sourceName}`}
                              onClick={() => void setRepresentative(sourceEntry.id)}
                            >
                              设为默认
                            </Button>
                          )}
                          {sourceEntry.id !== (rowEntryId ?? story.representative.id) && (
                            <Button
                              unstyled
                              className="text-xs text-muted-foreground hover:text-foreground"
                              aria-label={`分开 ${sourceEntry.source_title || '来源'}`}
                              onClick={() => setSplitRequest({ entryID: sourceEntry.id, options: {} })}
                            >
                              分开
                            </Button>
                          )}
                        </span>
                      )
                    })}
                  </div>
                )}
                {mergePickerOpen && candidates.length > 0 && (
                  <div className="mb-5 flex flex-wrap items-center gap-2 text-sm text-muted-foreground">
                    {candidates.map((target) => (
                      <Button
                        key={target.storyId}
                        unstyled
                        className="rounded border border-border px-2 py-0.5 text-xs hover:bg-accent"
                        onClick={() => void mergeInto(target)}
                      >
                        合并到：{target.title}
                      </Button>
                    ))}
                    <Button
                      unstyled
                      className="text-xs text-muted-foreground hover:text-foreground"
                      aria-label="取消合并"
                      onClick={() => setMergePickerOpen(false)}
                    >
                      取消
                    </Button>
                  </div>
                )}
                <SelectionChatSurface label="文章正文">
                  <EntryReader entry={activeEntry} />
                </SelectionChatSurface>
                {notesOpen && <div className="mt-10 grid max-w-[68ch] gap-4 border-t pt-6">
                  <label>
                    <span>显示标题</span>
                    <Input
                      value={titleDraft}
                      onChange={(event) => setTitleDraft(event.target.value)}
                      placeholder={story.representative.source_title}
                    />
                  </label>
                  <label>
                    <span>笔记</span>
                    <Textarea
                      value={noteDraft}
                      onChange={(event) => setNoteDraft(event.target.value)}
                      placeholder="记录你的想法…"
                    />
                  </label>
                  <Button variant="secondary" onClick={() => void applyPatch({
                    display_title: titleDraft,
                    note: noteDraft,
                  })}>
                    保存标题与笔记
                  </Button>
                </div>}
              </>
            )}
          </div>
        </div>
      )}
      <Dialog open={splitRequest !== null} onOpenChange={(openState) => !openState && setSplitRequest(null)}>
        {splitRequest && (
          <DialogContent>
            <DialogTitle>拆分来源内容？</DialogTitle>
            <DialogDescription className="mb-5 mt-2 text-sm leading-6 text-muted-foreground">
              新 Story 会继承当前阅读状态；标题、笔记和标签默认保留在原 Story。需要移动或复制的元数据请在这里明确选择。
            </DialogDescription>
            <div className="grid gap-3 text-sm">
              {([
                ['copy_display_title', '复制显示标题'],
                ['move_display_title', '移动显示标题'],
                ['copy_note', '复制笔记'],
                ['move_note', '移动笔记'],
                ['copy_tags', '复制标签'],
                ['move_tags', '移动标签'],
              ] as const).map(([option, optionLabel]) => (
                <label className="flex items-center gap-2" key={option}>
                  <input
                    type="checkbox"
                    checked={Boolean(splitRequest.options[option])}
                    onChange={(event) => setSplitRequest((current) => {
                      if (!current) return current
                      const options = { ...current.options, [option]: event.target.checked }
                      if (event.target.checked) {
                        const opposite: Partial<Record<keyof api.SplitOptions, keyof api.SplitOptions>> = {
                          copy_display_title: 'move_display_title',
                          move_display_title: 'copy_display_title',
                          copy_note: 'move_note',
                          move_note: 'copy_note',
                          copy_tags: 'move_tags',
                          move_tags: 'copy_tags',
                        }
                        const other = opposite[option]
                        if (other) options[other] = false
                      }
                      return { ...current, options }
                    })}
                  />
                  {optionLabel}
                </label>
              ))}
            </div>
            <div className="mt-6 flex justify-end gap-2">
              <DialogClose asChild><Button variant="secondary">取消</Button></DialogClose>
              <Button onClick={() => void splitEntry(splitRequest.entryID, splitRequest.options)}>确认拆分</Button>
            </div>
          </DialogContent>
        )}
      </Dialog>
      <Dialog open={mergeResolution !== null} onOpenChange={(openState) => !openState && setMergeResolution(null)}>
        {mergeResolution && (
          <DialogContent>
            <DialogTitle>解决 Story 元数据冲突</DialogTitle>
            <DialogDescription className="mb-5 mt-2 text-sm leading-6 text-muted-foreground">
              两个 Story 的自定义元数据不同。请明确选择合并后保留的标题和笔记。
            </DialogDescription>
            <div className="grid gap-4">
              <label>
                <span>合并后的显示标题</span>
                <Input
                  value={mergeResolution.displayTitle}
                  onChange={(event) => setMergeResolution((current) => current ? { ...current, displayTitle: event.target.value } : current)}
                />
              </label>
              <label>
                <span>合并后的笔记</span>
                <Textarea
                  value={mergeResolution.note}
                  onChange={(event) => setMergeResolution((current) => current ? { ...current, note: event.target.value } : current)}
                />
              </label>
            </div>
            <div className="mt-6 flex justify-end gap-2">
              <DialogClose asChild><Button variant="secondary">取消</Button></DialogClose>
              <Button onClick={() => void mergeInto(mergeResolution.target, {
                display_title: mergeResolution.displayTitle,
                note: mergeResolution.note,
              })}>确认合并</Button>
            </div>
          </DialogContent>
        )}
      </Dialog>
      <Dialog open={deleteDialogOpen} onOpenChange={(openState) => {
        setDeleteDialogOpen(openState)
        if (!openState) setDeleteConfirmation(null)
      }}>
        <DialogContent>
          <DialogTitle>{deleteConfirmation ? '确认删除最后一条来源内容？' : '永久删除来源内容？'}</DialogTitle>
          <DialogDescription className="mb-5 mt-2 text-sm leading-6 text-muted-foreground">
            {deleteConfirmation
              ? `这会同时删除 Story「${deleteConfirmation.display_title || '无标题'}」及其笔记；此操作不可撤销。`
              : '这会写入 Tombstone，防止来源内容被再次摄取恢复。普通阅读移除请使用隐藏。'}
          </DialogDescription>
          {deleteConfirmation?.note && (
            <p className="mb-4 rounded-md bg-muted px-3 py-2 text-sm">Story 笔记：{deleteConfirmation.note}</p>
          )}
          <div className="flex justify-end gap-2">
            <DialogClose asChild><Button variant="secondary">取消</Button></DialogClose>
            <Button variant="destructive" onClick={() => void confirmDelete()}>确认永久删除</Button>
          </div>
        </DialogContent>
      </Dialog>
    </article>
  )
}
