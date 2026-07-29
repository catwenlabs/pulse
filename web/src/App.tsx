import { FormEvent, useEffect, useRef, useState } from 'react'
import {
  BookOpen,
  Bookmark,
  CheckCheck,
  ChevronDown,
  Clock3,
  FolderClosed,
  Inbox,
  Menu,
  MoreHorizontal,
  Plus,
  RefreshCw,
  Rss,
  Search,
  Star,
  X,
  type LucideIcon,
} from 'lucide-react'

import * as api from './api'
import type { AnnotationInput, CreateSourceInput, Entry, EntryPatch, Folder, PreviewResult, Source, SourceHealth, SourceKind } from './api'
import { Button, buttonVariants } from './components/ui/button'
import { Dialog, DialogClose, DialogContent, DialogDescription, DialogTitle, SheetContent } from './components/ui/dialog'
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuTrigger } from './components/ui/dropdown-menu'
import { Input } from './components/ui/input'
import { Select } from './components/ui/select'
import { Textarea } from './components/ui/textarea'
import { cn } from './lib/utils'
import './styles.css'

type Notice = { tone: 'success' | 'error'; message: string }
type View = 'sources' | 'inbox' | 'starred' | 'later' | 'annotations'
type SaveRequest = { url: string; title: string }

function navItemClass(active: boolean, className?: string) {
  return cn(
    'flex min-h-9 items-center gap-3 rounded-md px-3 text-sm font-medium text-muted-foreground no-underline transition-colors hover:bg-sidebar-accent hover:text-sidebar-accent-foreground',
    active && 'bg-sidebar-accent text-sidebar-accent-foreground',
    className,
  )
}

export function App() {
  const [sources, setSources] = useState<Source[]>([])
  const [folders, setFolders] = useState<Folder[]>([])
  const [expandedFolders, setExpandedFolders] = useState<Set<string>>(() => new Set())
  const [loading, setLoading] = useState(true)
  const [loadError, setLoadError] = useState('')
  const [showCreate, setShowCreate] = useState(false)
  const [sourceToDelete, setSourceToDelete] = useState<Source | null>(null)
  const [deleting, setDeleting] = useState(false)
  const [notice, setNotice] = useState<Notice | null>(null)
  const [activeView, setActiveView] = useState<View>('inbox')
  const [selectedSourceID, setSelectedSourceID] = useState('')
  const [health, setHealth] = useState<Record<string, SourceHealth>>({})
  const [mobileNavigationOpen, setMobileNavigationOpen] = useState(false)
  const [showBookmarklet, setShowBookmarklet] = useState(false)
  const [saveRequest, setSaveRequest] = useState<SaveRequest | null>(() => readSaveRequest())
  const isMobile = useMediaQuery('(max-width: 767px)')
  const mobileMenuButtonRef = useRef<HTMLButtonElement>(null)
  const mobileDrawerCloseRef = useRef<HTMLButtonElement>(null)
  const bookmarkletButtonRef = useRef<HTMLButtonElement>(null)

  async function load() {
    setLoading(true)
    setLoadError('')
    try {
      const [loaded, loadedFolders] = await Promise.all([
        api.listSources(),
        api.listFolders().catch(() => []),
      ])
      setSources(loaded)
      setFolders(loadedFolders)
      setExpandedFolders((current) => new Set([
        ...current,
        ...loadedFolders.map((folder) => folder.id),
      ]))
      const snapshots = await Promise.all(loaded.map(async (item) => {
        try {
          return [item.id, await api.getSourceHealth(item.id)] as const
        } catch {
          return null
        }
      }))
      setHealth(Object.fromEntries(snapshots.filter((item): item is readonly [string, SourceHealth] => item !== null)))
    } catch (error) {
      setLoadError(error instanceof Error ? error.message : '加载信息源失败')
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    void load()
  }, [])

  useEffect(() => {
    const update = () => setSaveRequest(readSaveRequest())
    window.addEventListener('hashchange', update)
    return () => window.removeEventListener('hashchange', update)
  }, [])

  useEffect(() => {
    if (!saveRequest || !window.location.hash.startsWith('#save?')) return
    window.history.replaceState(null, '', `${window.location.pathname}${window.location.search}`)
  }, [saveRequest])

  async function handleCreate(input: CreateSourceInput) {
    const created = await api.createSource(input)
    setSources((current) => [...current, created].sort((a, b) => a.name.localeCompare(b.name)))
    setShowCreate(false)
    setNotice({ tone: 'success', message: `已添加 ${created.name}` })
  }

  async function handleRun(source: Source) {
    try {
      await api.runSource(source.id)
      setNotice({ tone: 'success', message: '抓取任务已进入队列' })
    } catch (error) {
      setNotice({
        tone: 'error',
        message: error instanceof Error ? error.message : '无法创建抓取任务',
      })
    }
  }

  async function handleToggle(source: Source) {
    try {
      const updated = await api.setSourceEnabled(source.id, !source.enabled)
      setSources((current) => current.map((item) => item.id === updated.id ? updated : item))
      setNotice({
        tone: 'success',
        message: updated.enabled ? `已恢复 ${updated.name}` : `已暂停 ${updated.name}`,
      })
    } catch (error) {
      setNotice({
        tone: 'error',
        message: error instanceof Error ? error.message : '无法更新信息源',
      })
    }
  }

  async function handleArchive(source: Source) {
    setDeleting(true)
    try {
      await api.archiveSource(source.id)
      setSources((current) => current.filter((item) => item.id !== source.id))
      setHealth((current) => Object.fromEntries(
        Object.entries(current).filter(([id]) => id !== source.id),
      ))
      setSelectedSourceID((current) => current === source.id ? '' : current)
      setFolders(await api.listFolders().catch(() => folders))
      setSourceToDelete(null)
      setNotice({ tone: 'success', message: `已删除 ${source.name}` })
    } catch (error) {
      setNotice({
        tone: 'error',
        message: error instanceof Error ? error.message : '无法删除信息源',
      })
    } finally {
      setDeleting(false)
    }
  }

  function showStream(view: Exclude<View, 'sources'>, sourceID = '') {
    setActiveView(view)
    setSelectedSourceID(sourceID)
    closeMobileNavigation()
  }

  function closeMobileNavigation(restoreFocus = true) {
    if (!mobileNavigationOpen) return
    setMobileNavigationOpen(false)
    if (restoreFocus) {
      window.requestAnimationFrame(() => mobileMenuButtonRef.current?.focus())
    }
  }

  const activeSourceName = sources.find((source) => source.id === selectedSourceID)?.name
  const assignedSourceIDs = new Set(folders.flatMap((folder) => folder.source_ids))
  const rootSources = sources.filter((source) => !assignedSourceIDs.has(source.id))
  const mobileTitle = activeView === 'sources'
    ? '信息源'
    : activeSourceName || (
      activeView === 'starred'
        ? '收藏'
        : activeView === 'later'
          ? '稍后阅读'
          : activeView === 'annotations'
            ? '阅读笔记'
            : '全部文章'
    )

  if (saveRequest) {
    return (
      <SavePage
        key={`${saveRequest.url}\u0000${saveRequest.title}`}
        request={saveRequest}
        sources={sources}
        loading={loading}
        loadError={loadError}
        onSourceCreated={(created) => setSources((current) => [...current, created])}
        onSourceUpdated={(updated) => setSources((current) => (
          current.map((source) => source.id === updated.id ? updated : source)
        ))}
        onClose={() => {
          window.history.replaceState(null, '', `${window.location.pathname}${window.location.search}`)
          setSaveRequest(null)
        }}
      />
    )
  }

  return (
    <Dialog
      modal={isMobile}
      open={isMobile && mobileNavigationOpen}
      onOpenChange={(open) => {
        if (open) setMobileNavigationOpen(true)
        else closeMobileNavigation()
      }}
    >
    <div className="grid h-dvh min-h-0 grid-cols-[256px_minmax(0,1fr)] overflow-hidden max-md:block max-md:w-full">
      <SheetContent
        persistent={!isMobile}
        onOpenAutoFocus={(event) => {
          event.preventDefault()
          mobileDrawerCloseRef.current?.focus()
        }}
        onCloseAutoFocus={(event) => {
          event.preventDefault()
          mobileMenuButtonRef.current?.focus()
        }}
      >
      <aside
        className="fixed inset-y-0 left-0 z-30 flex w-64 flex-col border-r bg-sidebar px-3 py-4 text-sidebar-foreground transition-transform md:translate-x-0 max-md:w-[min(86vw,20rem)] max-md:-translate-x-full max-md:shadow-xl data-[state=open]:translate-x-0"
        id="mobile-navigation"
        role="navigation"
        aria-label="移动导航抽屉"
        aria-hidden={isMobile ? !mobileNavigationOpen : undefined}
        inert={isMobile && !mobileNavigationOpen ? true : undefined}
      >
        <div className="flex items-center justify-between px-1.5 pb-[18px] max-md:px-1">
          <a className="flex items-center gap-2.5 font-serif text-[23px] font-semibold text-foreground no-underline" href="/" aria-label="Pulse 首页" onClick={() => showStream('inbox')}>
            <span className="grid size-[34px] place-items-center rounded-[10px_10px_10px_3px] bg-primary font-sans text-lg font-bold text-white" aria-hidden="true">P</span>
            <span>Pulse</span>
          </a>
          <div className="flex items-center gap-1.5">
            <Button unstyled className="grid size-[31px] cursor-pointer place-items-center rounded-[7px] border-0 bg-primary text-lg text-white hover:bg-primary-hover" aria-label="添加信息源" onClick={() => {
              closeMobileNavigation(false)
              setShowCreate(true)
            }}>
              <Plus className="size-4" aria-hidden="true" /><span className="sr-only">添加信息源</span>
            </Button>
            {isMobile && (
              <Button unstyled
                className="hidden size-[34px] cursor-pointer place-items-center rounded-lg border-0 bg-white/50 p-0 text-2xl text-[#66635b] max-md:grid"
                ref={mobileDrawerCloseRef}
                aria-label="关闭导航"
                onClick={() => closeMobileNavigation()}
              >
                <X className="size-5" aria-hidden="true" />
              </Button>
            )}
          </div>
        </div>

        <nav className="grid gap-1 overflow-visible" aria-label="主导航">
          <a className={navItemClass(activeView === 'inbox' && !selectedSourceID)} href="#inbox" onClick={() => showStream('inbox')}><NavIcon name="inbox" />全部文章</a>
          <a className={navItemClass(activeView === 'starred')} href="#starred" onClick={() => showStream('starred')}><NavIcon name="star" />收藏</a>
          <a className={navItemClass(activeView === 'later')} href="#later" onClick={() => showStream('later')}><NavIcon name="clock" />稍后阅读</a>
          <a className={navItemClass(activeView === 'annotations')} href="#annotations" onClick={() => showStream('annotations')}><NavIcon name="book" />阅读笔记</a>
        </nav>

        <section className="mt-[22px] flex min-h-0 flex-1 flex-col px-[7px]" aria-labelledby="source-tree-label">
          <div className="mb-2 flex items-center justify-between text-[10px] text-[#9a978d]">
            <p className="m-0 text-[11px] font-semibold uppercase tracking-wide text-muted-foreground" id="source-tree-label">订阅</p>
            <span>{sources.length}</span>
          </div>
          {loading && <p className="border-0 bg-transparent px-2 py-1 text-left text-xs text-muted-foreground">正在同步信息源…</p>}
          {!loading && loadError && <Button unstyled className="border-0 bg-transparent px-2 py-1 text-left text-xs text-destructive" onClick={() => void load()}>重试加载</Button>}
          <div className="min-h-0 overflow-y-auto">
            {folders.map((folder) => (
              <div key={folder.id}>
                <Button
                  unstyled
                  className="flex min-h-9 w-full items-center gap-2 rounded-md px-2 text-sm font-medium text-sidebar-foreground hover:bg-sidebar-accent hover:text-sidebar-accent-foreground"
                  aria-label={`${folder.name}，${folder.source_count} 个订阅源`}
                  aria-expanded={expandedFolders.has(folder.id)}
                  onClick={() => setExpandedFolders((current) => {
                    const next = new Set(current)
                    if (next.has(folder.id)) next.delete(folder.id)
                    else next.add(folder.id)
                    return next
                  })}
                >
                  <ChevronDown className={cn('size-4 shrink-0 transition-transform', !expandedFolders.has(folder.id) && '-rotate-90')} aria-hidden="true" />
                  <FolderClosed className="size-4 shrink-0" aria-hidden="true" />
                  <span className="min-w-0 flex-1 truncate text-left">{folder.name}</span>
                  <span className="text-xs font-normal text-muted-foreground">{folder.source_count}</span>
                </Button>
                {expandedFolders.has(folder.id) && (
                  <div className="ml-4 border-l pl-2">
                    {folder.source_ids.map((sourceID) => {
                      const source = sources.find((candidate) => candidate.id === sourceID)
                      if (!source) return null
                      return (
                        <Button
                          unstyled
                          className={navItemClass(selectedSourceID === source.id && activeView === 'inbox', 'w-full min-h-8 py-1 text-xs')}
                          key={`${folder.id}-${source.id}`}
                          onClick={() => showStream('inbox', source.id)}
                        >
                          <span className={cn('size-2 rounded-full bg-muted-foreground/40', source.enabled && 'bg-emerald-500')} />
                          <span className="truncate">{source.name}</span>
                        </Button>
                      )
                    })}
                  </div>
                )}
              </div>
            ))}
            {rootSources.map((source) => (
              <Button unstyled
                className={navItemClass(selectedSourceID === source.id && activeView === 'inbox', 'w-full min-h-8 py-1 text-xs')}
                key={source.id}
                onClick={() => showStream('inbox', source.id)}
              >
                <span className={cn('size-2 rounded-full bg-muted-foreground/40', source.enabled && 'bg-emerald-500')} />
                <span>{source.name}</span>
              </Button>
            ))}
          </div>
        </section>

        <div className="mt-3 grid gap-2 border-t border-[#d8d4ca] px-[7px] pt-2.5 text-[10px] text-[#77746c]">
          <Button unstyled className={navItemClass(false, 'w-full')} ref={bookmarkletButtonRef} onClick={() => {
            closeMobileNavigation(false)
            setShowBookmarklet(true)
          }}>
            <NavIcon name="bookmark" />安装保存书签
          </Button>
          <Button unstyled className={navItemClass(activeView === 'sources', 'w-full')} onClick={() => {
            setActiveView('sources')
            closeMobileNavigation()
          }}>
            <NavIcon name="source" />管理信息源
          </Button>
          <span className="flex items-center gap-2 px-3 py-1"><span className="size-2 rounded-full bg-success shadow-[0_0_0_3px_rgba(93,148,108,.13)]" />本地服务已连接</span>
        </div>
      </aside>
      </SheetContent>

      <main
        className={cn(
          'col-start-2 h-dvh min-w-0 overflow-y-auto bg-background p-0',
          activeView !== 'sources' && 'overflow-hidden max-md:grid max-md:grid-rows-[auto_minmax(0,1fr)]',
        )}
        inert={isMobile && mobileNavigationOpen ? true : undefined}
      >
        {isMobile && (
          <header className="max-md:z-[7] max-md:grid max-md:min-h-[calc(52px+env(safe-area-inset-top))] max-md:grid-cols-[40px_minmax(0,1fr)] max-md:items-center max-md:gap-2 max-md:border-b max-md:bg-card/95 max-md:px-3.5 max-md:pt-[env(safe-area-inset-top)] max-md:backdrop-blur-xl">
            <Button unstyled
              className="grid size-10 cursor-pointer content-center gap-1 rounded-lg border-0 bg-transparent px-[9px] text-foreground hover:bg-accent focus-visible:bg-accent focus-visible:outline-none"
              ref={mobileMenuButtonRef}
              aria-label="打开导航"
              aria-controls="mobile-navigation"
              aria-expanded={mobileNavigationOpen}
              onClick={() => setMobileNavigationOpen(true)}
            >
              <Menu className="size-5" aria-hidden="true" />
            </Button>
            <h1>{mobileTitle}</h1>
          </header>
        )}
        {notice && (
          <div className={cn(
            'fixed right-4 top-4 z-50 flex w-[min(26rem,calc(100vw-2rem))] items-center justify-between rounded-lg border bg-background px-4 py-3 text-sm shadow-lg',
            notice.tone === 'success' ? 'border-emerald-200 text-emerald-700' : 'border-destructive/30 text-destructive',
          )} role="status">
            {notice.message}
            <Button unstyled aria-label="关闭提示" onClick={() => setNotice(null)}><X className="size-4" aria-hidden="true" /></Button>
          </div>
        )}
        {activeView === 'sources' ? (
          <>
        <header className="mb-[38px] flex items-end justify-between gap-6 max-md:flex-col max-md:items-start">
          <div>
            <p className="text-xs font-semibold uppercase tracking-widest text-muted-foreground">LIBRARY</p>
            <h1>信息源</h1>
            <p className="mb-0 mt-3 text-sm text-muted-foreground">管理 Pulse 持续关注的 RSS、API、网页与推送来源。</p>
          </div>
          <div className="flex gap-2.5 max-md:w-full">
            <a className={buttonVariants({ variant: 'secondary', className: 'max-md:flex-1' })} href="/api/v1/opml/export">导出 OPML</a>
            <Button className="max-md:flex-1" onClick={() => setShowCreate(true)}>
              <Plus className="size-4" aria-hidden="true" /> 添加信息源
            </Button>
          </div>
        </header>

        <section className="overflow-hidden rounded-[13px] border bg-card shadow-[0_10px_30px_rgba(72,66,51,.04)] max-md:mx-4 max-md:mb-6" aria-labelledby="source-heading">
          <div className="flex items-center justify-between border-b px-[22px] py-5">
            <div>
              <h2 id="source-heading">全部信息源</h2>
              <span>{sources.length} 个来源</span>
            </div>
            <Button unstyled className="grid size-9 place-items-center rounded-md text-muted-foreground hover:bg-accent hover:text-foreground" aria-label="重新载入" onClick={() => void load()}><RefreshCw className="size-4" aria-hidden="true" /></Button>
          </div>

          {loading && <div className="grid min-h-[250px] place-items-center gap-[7px] text-center text-[13px] text-[#8b887f]">正在同步信息源…</div>}
          {!loading && loadError && (
            <div className="grid min-h-[250px] place-items-center gap-[7px] text-center text-[13px] text-[#8b887f] text-destructive">
              <strong>暂时无法加载</strong>
              <span>{loadError}</span>
              <Button unstyled className="cursor-pointer border-0 bg-transparent text-primary-hover underline" onClick={() => void load()}>重试</Button>
            </div>
          )}
          {!loading && !loadError && sources.length === 0 && (
            <div className="grid min-h-[250px] place-items-center gap-[7px] text-center text-[13px] text-[#8b887f]">
              <strong>还没有信息源</strong>
              <span>添加一个 RSS Feed，开始构建你的阅读中枢。</span>
            </div>
          )}
          {!loading && !loadError && sources.length > 0 && (
            <div >
              {sources.map((source) => (
                <article className="grid min-h-[78px] grid-cols-[42px_minmax(0,1fr)_auto_minmax(0,260px)_auto] items-center gap-3.5 border-b border-[#ebe8e0] px-5 py-[13px] last:border-b-0 hover:bg-[#f8f6f0] max-md:grid-cols-[42px_minmax(0,1fr)_auto]" key={source.id}>
                  <div className="grid size-10 place-items-center rounded-[10px] bg-[#f3ded5] font-serif text-lg font-semibold text-[#a1482d]" aria-hidden="true">{source.name.slice(0, 1).toUpperCase()}</div>
                  <div className="min-w-0">
                    <div className="flex items-center gap-2">
                      <h3>{source.name}</h3>
                      <span className="rounded bg-[#eee6dd] px-1.5 py-0.5 text-[8px] font-bold tracking-[.08em] text-[#8b624f]">{source.kind.toUpperCase()}</span>
                    </div>
                    <p>{source.locator}</p>
                  </div>
                  <div className="flex items-center gap-[7px] text-[11px] text-[#77746c] max-md:hidden">
                    <span className={cn('size-2 rounded-full bg-muted-foreground/40', source.enabled && 'bg-emerald-500')} />
                    {source.enabled ? '已启用' : '已暂停'}
                  </div>
                  {health[source.id] && (
                    <div className="grid min-w-[180px] gap-0.5 text-[10px] text-[#77746c] max-md:hidden" aria-label={`${source.name} 抓取诊断`}>
                      <span>状态 {health[source.id].status}</span>
                      {health[source.id].last_requested_at && <span>最近 {formatTime(health[source.id].last_requested_at!)}</span>}
                      <span>{health[source.id].candidate_count} 候选 · {health[source.id].new_count} 新增 · {health[source.id].updated_count} 更新</span>
                      <span>{health[source.id].duration_milliseconds} ms · 连续失败 {health[source.id].consecutive_failures}</span>
                      {health[source.id].next_scheduled_at && <span>下次 {formatTime(health[source.id].next_scheduled_at!)}</span>}
                      {health[source.id].last_error && <em>{health[source.id].last_error}</em>}
                    </div>
                  )}
                  <div className="flex items-center justify-end gap-[3px]">
                    <Button unstyled
                      className="cursor-pointer border-0 bg-transparent text-[11px] text-primary-hover"
                      aria-label={`${source.enabled ? '暂停' : '恢复'} ${source.name}`}
                      onClick={() => void handleToggle(source)}
                    >
                      {source.enabled ? '暂停' : '恢复'}
                    </Button>
                    <Button unstyled
                      className="size-[34px] rounded-[7px] hover:bg-[#eeeae2] hover:text-primary-hover disabled:cursor-not-allowed disabled:opacity-30"
                      aria-label={`刷新 ${source.name}`}
                      disabled={!source.enabled}
                      onClick={() => void handleRun(source)}
                    >
                      <RefreshCw className="size-4" aria-hidden="true" />
                    </Button>
                    <Button unstyled
                      className="min-h-[30px] cursor-pointer rounded-md border-0 bg-transparent px-[7px] text-[10px] text-[#9a3f2c] hover:bg-[#f3ded5]"
                      aria-label={`删除 ${source.name}`}
                      onClick={() => setSourceToDelete(source)}
                    >
                      删除
                    </Button>
                  </div>
                </article>
              ))}
            </div>
          )}
        </section>
          </>
        ) : activeView === 'annotations' ? (
          <AnnotationsPage
            sources={sources}
            onSourceCreated={(created) => setSources((current) => [...current, created])}
            onSourceUpdated={(updated) => setSources((current) => (
              current.map((source) => source.id === updated.id ? updated : source)
            ))}
          />
        ) : (
          <Reader
            view={activeView}
            sourceID={selectedSourceID}
            sourceName={activeSourceName}
            sources={sources}
            mobile={isMobile}
          />
        )}
      </main>

      {showCreate && (
        <CreateSourceDialog
          onClose={() => setShowCreate(false)}
          onCreate={handleCreate}
        />
      )}
      {sourceToDelete && (
        <DeleteSourceDialog
          source={sourceToDelete}
          deleting={deleting}
          onCancel={() => setSourceToDelete(null)}
          onConfirm={() => void handleArchive(sourceToDelete)}
        />
      )}
      {showBookmarklet && (
        <BookmarkletDialog
          onClose={() => setShowBookmarklet(false)}
          returnFocusElement={isMobile ? mobileMenuButtonRef.current : bookmarkletButtonRef.current}
        />
      )}
    </div>
    </Dialog>
  )
}

function AnnotationsPage({
  sources,
  onSourceCreated,
  onSourceUpdated,
}: {
  sources: Source[]
  onSourceCreated: (source: Source) => void
  onSourceUpdated: (source: Source) => void
}) {
  const annotationSources = sources.filter((source) => source.kind === 'annotations')
  const [entries, setEntries] = useState<Entry[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [showImport, setShowImport] = useState(false)
  const [expandedBooks, setExpandedBooks] = useState<Set<string>>(() => new Set())
  const [importing, setImporting] = useState(false)
  const [importMessage, setImportMessage] = useState('')
  const [form, setForm] = useState<AnnotationInput>({
    provider: 'apple-books',
    book_title: '',
    book_author: '',
    chapter: '',
    location: '',
    highlight_color: 'yellow',
    highlight: '',
    note: '',
  })

  useEffect(() => {
    let active = true
    async function loadAnnotations() {
      setLoading(true)
      setError('')
      try {
        const batches = await Promise.all(annotationSources.map(async (source) => {
          const result: Entry[] = []
          let offset = 0
          for (;;) {
            const page = await api.listEntries({ sourceId: source.id, limit: 200, offset })
            result.push(...page)
            if (page.length < 200) return result
            offset += page.length
          }
        }))
        if (active) setEntries(batches.flat().filter((entry) => entry.annotation))
      } catch (cause) {
        if (active) setError(cause instanceof Error ? cause.message : '无法加载阅读笔记')
      } finally {
        if (active) setLoading(false)
      }
    }
    void loadAnnotations()
    return () => {
      active = false
    }
  }, [sources])

  const books = Array.from(entries.reduce((groups, item) => {
    const detail = item.annotation!
    const key = annotationBookKey(detail)
    const group = groups.get(key)
    if (group) group.entries.push(item)
    else groups.set(key, { detail, entries: [item] })
    return groups
  }, new Map<string, { detail: NonNullable<Entry['annotation']>; entries: Entry[] }>()).values())

  async function submit(event: FormEvent) {
    event.preventDefault()
    setImporting(true)
    setImportMessage('')
    try {
      let target = annotationSources.find((source) => source.locator.startsWith(form.provider))
      if (!target) {
        target = await api.createSource({
          name: form.provider === 'kindle' ? 'Kindle 批注' : 'Apple Books 批注',
          kind: 'annotations',
          locator: `${form.provider}-${Date.now().toString(36)}-${Math.random().toString(36).slice(2, 8)}`,
        })
        onSourceCreated(target)
      } else if (!target.enabled) {
        target = await api.setSourceEnabled(target.id, true)
        onSourceUpdated(target)
      }
      await api.importAnnotations(target.id, [form])
      setImportMessage('批注已加入导入队列')
      setForm((current) => ({ ...current, chapter: '', location: '', highlight: '', note: '' }))
    } catch (cause) {
      setImportMessage(cause instanceof Error ? cause.message : '导入批注失败')
    } finally {
      setImporting(false)
    }
  }

  return (
    <div className="min-h-0 overflow-y-auto bg-[#f4f1ea] px-[clamp(24px,5vw,72px)] pb-[72px] pt-[42px] max-md:px-3.5 max-md:pb-10 max-md:pt-5">
      <header className="mb-[38px] flex items-end justify-between gap-6 max-md:flex-col max-md:items-start mx-auto mb-7 max-md:gap-4">
        <div>
          <p className="text-xs font-semibold uppercase tracking-widest text-muted-foreground">READING NOTES</p>
          <h1>阅读笔记</h1>
          <p className="mb-0 mt-3 text-sm text-muted-foreground">集中保存 Apple Books、Kindle 和其他阅读器中的高亮与批注。</p>
        </div>
        <Button onClick={() => setShowImport((current) => !current)}>
          {showImport ? '收起导入' : '导入批注'}
        </Button>
      </header>

      {showImport && (
        <section className="mx-auto mb-7 grid max-w-[1040px] grid-cols-[minmax(180px,.7fr)_minmax(320px,1.3fr)] gap-8 rounded-[14px] border bg-white p-6 shadow-[0_10px_32px_rgba(51,46,36,.06)] max-md:grid-cols-1 max-md:gap-4 max-md:p-[18px]" aria-labelledby="annotation-import-title">
          <div>
            <p className="text-xs font-semibold uppercase tracking-widest text-muted-foreground">NEW ANNOTATION</p>
            <h2 id="annotation-import-title">添加一条阅读批注</h2>
            <p>第一版支持结构化手工导入；Apple Books 与 Kindle 批量格式将在取得真实导出样本后接入。</p>
          </div>
          <form onSubmit={(event) => void submit(event)}>
            <div className="grid grid-cols-2 gap-3 max-md:grid-cols-1">
              <label>
                <span>来源平台</span>
                <Select value={form.provider} onChange={(event) => setForm({ ...form, provider: event.target.value })}>
                  <option value="apple-books">Apple Books</option>
                  <option value="kindle">Kindle</option>
                  <option value="other">其他</option>
                </Select>
              </label>
              <label>
                <span>书名</span>
                <Input required value={form.book_title} onChange={(event) => setForm({ ...form, book_title: event.target.value })} />
              </label>
              <label>
                <span>作者</span>
                <Input value={form.book_author} onChange={(event) => setForm({ ...form, book_author: event.target.value })} />
              </label>
              <label>
                <span>章节</span>
                <Input value={form.chapter} onChange={(event) => setForm({ ...form, chapter: event.target.value })} />
              </label>
              <label>
                <span>位置</span>
                <Input value={form.location} onChange={(event) => setForm({ ...form, location: event.target.value })} />
              </label>
              <label>
                <span>高亮颜色</span>
                <Select value={form.highlight_color} onChange={(event) => setForm({ ...form, highlight_color: event.target.value })}>
                  <option value="yellow">黄色</option>
                  <option value="green">绿色</option>
                  <option value="blue">蓝色</option>
                  <option value="pink">粉色</option>
                  <option value="">未指定</option>
                </Select>
              </label>
            </div>
            <label>
              <span>高亮原文</span>
              <Textarea required value={form.highlight} onChange={(event) => setForm({ ...form, highlight: event.target.value })} />
            </label>
            <label>
              <span>原始批注</span>
              <Textarea value={form.note} onChange={(event) => setForm({ ...form, note: event.target.value })} />
            </label>
            {importMessage && <p className="m-0 text-xs text-[#41634a]" role="status">{importMessage}</p>}
            <Button type="submit" disabled={importing}>
              {importing ? '正在导入…' : '加入导入队列'}
            </Button>
          </form>
        </section>
      )}

      {loading && <div className="p-[30px] text-center text-xs text-muted-foreground">正在加载阅读笔记…</div>}
      {!loading && error && <div className="p-[30px] text-center text-xs text-muted-foreground">{error}</div>}
      {!loading && !error && books.length === 0 && (
        <div className="mx-auto grid max-w-[1040px] justify-items-center gap-[7px] rounded-[14px] border border-dashed border-[#d5d0c5] bg-white/50 px-6 py-16 text-muted-foreground">
          <strong>还没有阅读批注</strong>
          <span>导入第一条高亮后，Pulse 会按书籍自动整理。</span>
        </div>
      )}
      {!loading && !error && books.length > 0 && (
        <section className="mx-auto grid max-w-[1040px] gap-[18px]" aria-label="书籍批注">
          {books.map(({ detail, entries: bookEntries }) => (
            <article className="rounded-[14px] border bg-white p-6 shadow-[0_8px_28px_rgba(51,46,36,.05)] max-md:p-[18px]" key={annotationBookKey(detail)}>
              <div className="flex items-start justify-between gap-5 border-b border-[#ece8df] pb-[18px]">
                <div>
                  <span>{detail.provider === 'apple-books' ? 'APPLE BOOKS' : detail.provider.toUpperCase()}</span>
                  <h2>{detail.book_title}</h2>
                  {detail.book_author && <p>{detail.book_author}</p>}
                </div>
                <strong>{bookEntries.length} 条批注</strong>
              </div>
              <div className="grid gap-3 pt-[18px]">
                {(expandedBooks.has(annotationBookKey(detail)) ? bookEntries : bookEntries.slice(0, 3)).map((item) => (
                  <blockquote key={item.id}>
                    <p>{item.summary}</p>
                    {item.annotation?.annotation_note && <footer>{item.annotation.annotation_note}</footer>}
                    <small>{[item.annotation?.chapter, item.annotation?.location].filter(Boolean).join(' · ')}</small>
                  </blockquote>
                ))}
                {bookEntries.length > 3 && (
                  <Button unstyled
                    className="justify-self-start rounded-[7px] border border-[#ddd7cb] bg-white px-2.5 py-[7px] text-[11px] text-[#625d53] hover:border-[#c9c1b2] hover:bg-[#f8f5ef]"
                    onClick={() => setExpandedBooks((current) => {
                      const next = new Set(current)
                      const key = annotationBookKey(detail)
                      if (next.has(key)) next.delete(key)
                      else next.add(key)
                      return next
                    })}
                  >
                    {expandedBooks.has(annotationBookKey(detail))
                      ? '收起批注'
                      : `展开全部 ${bookEntries.length} 条`}
                  </Button>
                )}
              </div>
            </article>
          ))}
        </section>
      )}
    </div>
  )
}

function annotationBookKey(detail: NonNullable<Entry['annotation']>): string {
  return `${detail.provider}\u0000${detail.book_identity || detail.book_title}\u0000${detail.book_author}`
}

function readSaveRequest(): SaveRequest | null {
  if (!window.location.hash.startsWith('#save?')) return null
  const parameters = new URLSearchParams(window.location.hash.slice('#save?'.length))
  return {
    url: parameters.get('url') || '',
    title: parameters.get('title') || '',
  }
}

function normalizeSavedURL(value: string): string {
  let parsed: URL
  try {
    parsed = new URL(value.trim())
  } catch {
    throw new Error('请输入有效的网页地址')
  }
  if (parsed.protocol !== 'http:' && parsed.protocol !== 'https:') {
    throw new Error('只支持 HTTP 或 HTTPS 网页地址')
  }
  return parsed.href
}

function SavePage({
  request,
  sources,
  loading,
  loadError,
  onSourceCreated,
  onSourceUpdated,
  onClose,
}: {
  request: SaveRequest
  sources: Source[]
  loading: boolean
  loadError: string
  onSourceCreated: (source: Source) => void
  onSourceUpdated: (source: Source) => void
  onClose: () => void
}) {
  const manualSources = sources.filter((source) => source.kind === 'manual')
  const [url, setURL] = useState(request.url)
  const [title, setTitle] = useState(request.title)
  const [sourceID, setSourceID] = useState('')
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState('')
  const [success, setSuccess] = useState('')

  useEffect(() => {
    if (manualSources.some((source) => source.id === sourceID)) return
    setSourceID(manualSources[0]?.id || '')
  }, [manualSources, sourceID])

  async function save(targetSourceID: string, message: string) {
    const normalizedURL = normalizeSavedURL(url)
    if (!title.trim()) throw new Error('请输入标题')
    const targetSource = manualSources.find((source) => source.id === targetSourceID)
    if (targetSource && !targetSource.enabled) {
      onSourceUpdated(await api.setSourceEnabled(targetSource.id, true))
    }
    await api.createManualEntry(targetSourceID, {
      url: normalizedURL,
      title: title.trim(),
    })
    setSuccess(message)
  }

  async function submit(event: FormEvent) {
    event.preventDefault()
    setError('')
    setSuccess('')
    setSaving(true)
    try {
      const targetSourceID = sourceID || manualSources[0]?.id
      if (!targetSourceID) throw new Error('请先创建或选择一个 Manual Source')
      await save(targetSourceID, '已加入保存队列，Pulse 将在后台提取正文')
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : '保存网页失败')
    } finally {
      setSaving(false)
    }
  }

  async function createSourceAndSave() {
    setError('')
    setSuccess('')
    setSaving(true)
    try {
      const normalizedURL = normalizeSavedURL(url)
      if (!title.trim()) throw new Error('请输入标题')
      const created = await api.createSource({
        name: '网页收藏',
        kind: 'manual',
        locator: `reading-list-${Date.now().toString(36)}-${Math.random().toString(36).slice(2, 8)}`,
      })
      onSourceCreated(created)
      await api.createManualEntry(created.id, {
        url: normalizedURL,
        title: title.trim(),
      })
      setSourceID(created.id)
      setSuccess('已创建“网页收藏”并加入保存队列，Pulse 将在后台提取正文')
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : '创建收藏 Source 失败')
    } finally {
      setSaving(false)
    }
  }

  return (
    <main className="grid min-h-dvh w-full place-items-center overflow-y-auto bg-[radial-gradient(circle_at_top,#fff_0,#f3f1eb_58%)] p-6 max-md:place-items-start max-md:p-3">
      <section className="w-[min(520px,100%)] rounded-2xl border border-input bg-card p-[34px] shadow-[0_22px_70px_rgba(48,43,34,.12)] max-md:min-h-[calc(100dvh-24px)] max-md:rounded-xl max-md:px-[18px] max-md:py-6" aria-labelledby="save-page-title">
        <div className="mb-[30px] flex items-center gap-[9px] font-serif text-xl font-semibold max-md:mb-6"><span className="grid size-[34px] place-items-center rounded-[10px_10px_10px_3px] bg-primary font-sans text-lg font-bold text-white" aria-hidden="true">P</span>Pulse</div>
        <p className="text-xs font-semibold uppercase tracking-widest text-muted-foreground">READ LATER</p>
        <h1 id="save-page-title">保存到 Pulse</h1>
        <p className="mb-[25px] mt-[9px] text-[13px] leading-relaxed text-muted-foreground">确认网页信息后，将它加入你的阅读列表。</p>
        <form onSubmit={(event) => void submit(event)}>
          <label>
            <span>网页地址</span>
            <Input
              autoFocus
              required
              type="url"
              maxLength={2048}
              value={url}
              onChange={(event) => setURL(event.target.value)}
            />
          </label>
          <label>
            <span>标题</span>
            <Input required maxLength={500} value={title} onChange={(event) => setTitle(event.target.value)} />
          </label>
          {manualSources.length > 0 && (
            <label>
              <span>保存到</span>
              <Select value={sourceID} onChange={(event) => setSourceID(event.target.value)}>
                {manualSources.map((source) => (
                  <option key={source.id} value={source.id}>
                    {source.name}{source.enabled ? '' : '（已暂停，将自动恢复）'}
                  </option>
                ))}
              </Select>
            </label>
          )}
          {loading && <p className="m-0 text-xs text-muted-foreground">正在加载 Manual Source…</p>}
          {!loading && loadError && <p className="m-0 text-xs" role="alert">{loadError}</p>}
          {!loading && !loadError && manualSources.length === 0 && (
            <div className="grid gap-1 rounded-lg bg-[#f1ede5] p-[13px] text-xs leading-normal text-[#6d6257]">
              <strong>还没有 Manual Source</strong>
              <span>Pulse 会在你确认后创建“网页收藏”，不会创建隐藏 Source。</span>
            </div>
          )}
          {error && <p className="m-0 text-xs" role="alert">{error}</p>}
          {success && <p className="m-0 rounded-lg bg-[#e3efe5] px-[13px] py-[11px] text-xs text-[#345d40]" role="status">{success}</p>}
          <div className="mt-[7px] flex justify-end gap-[9px] max-md:flex-wrap">
            <Button variant="secondary" type="button" onClick={onClose}>关闭</Button>
            {manualSources.length > 0 ? (
              <Button type="submit" disabled={saving || loading}>
                {saving ? '正在保存…' : '保存网页'}
              </Button>
            ) : (
              <Button
                type="button"
                disabled={saving || loading || Boolean(loadError)}
                onClick={() => void createSourceAndSave()}
              >
                {saving ? '正在创建…' : '创建“网页收藏”并保存'}
              </Button>
            )}
          </div>
        </form>
      </section>
    </main>
  )
}

function BookmarkletDialog({
  onClose,
  returnFocusElement,
}: {
  onClose: () => void
  returnFocusElement: HTMLElement | null
}) {
  const saveTarget = `${window.location.origin}${window.location.pathname}#save?`
  const bookmarklet = `javascript:(()=>{const p=new URLSearchParams({url:location.href,title:document.title});window.open(${JSON.stringify(saveTarget)}+p.toString(),'_blank','popup,width=520,height=680,noopener');void 0})()`

  useEffect(() => {
    return () => returnFocusElement?.focus()
  }, [returnFocusElement])

  return (
    <Dialog open onOpenChange={(open) => !open && onClose()}>
      <DialogContent className="w-[min(560px,100%)]">
        <DialogClose className="absolute right-4 top-4 grid size-8 cursor-pointer place-items-center rounded-md text-muted-foreground hover:bg-accent hover:text-foreground" aria-label="关闭"><X className="size-4" aria-hidden="true" /></DialogClose>
        <p className="text-xs font-semibold uppercase tracking-widest text-muted-foreground">BOOKMARKLET</p>
        <DialogTitle>安装“保存到 Pulse”</DialogTitle>
        <DialogDescription className="mb-[25px] mt-[9px] text-[13px] leading-normal text-muted-foreground">新建一个浏览器书签，把下面整段代码粘贴到书签的地址栏。</DialogDescription>
        <label className="grid gap-[7px] text-xs font-semibold text-[#514f48]">
          <span>Bookmarklet 代码</span>
          <Textarea autoFocus readOnly value={bookmarklet} onFocus={(event) => event.currentTarget.select()} />
        </label>
        <div className="mt-[18px] grid grid-cols-2 gap-3 max-md:grid-cols-1">
          <section>
            <h3>Mac Chrome</h3>
            <ol className="m-0 grid gap-[7px] pl-5 text-xs leading-normal text-muted-foreground">
              <li>复制上面的完整代码。</li>
              <li>新建书签，名称填写“保存到 Pulse”。</li>
              <li>将代码粘贴到书签地址，然后在任意文章页点击它。</li>
            </ol>
          </section>
          <section>
            <h3>iPhone Chrome</h3>
            <ol className="m-0 grid gap-[7px] pl-5 text-xs leading-normal text-muted-foreground">
              <li>先把任意网页添加到 Chrome 书签。</li>
              <li>长按刚创建的书签，选择“编辑”，名称改为“保存到 Pulse”。</li>
              <li>把 URL 替换为上面的完整代码并保存。</li>
              <li>打开要收藏的网页，再从书签中点“保存到 Pulse”。</li>
            </ol>
          </section>
        </div>
        <div className="mt-[7px] flex justify-end gap-[9px] max-md:flex-wrap">
          <Button onClick={onClose}>完成</Button>
        </div>
      </DialogContent>
    </Dialog>
  )
}

function useMediaQuery(query: string): boolean {
  const [matches, setMatches] = useState(() => (
    typeof window !== 'undefined' && typeof window.matchMedia === 'function'
      ? window.matchMedia(query).matches
      : false
  ))

  useEffect(() => {
    if (typeof window.matchMedia !== 'function') return
    const media = window.matchMedia(query)
    const update = () => setMatches(media.matches)
    update()
    media.addEventListener('change', update)
    return () => media.removeEventListener('change', update)
  }, [query])

  return matches
}

function DeleteSourceDialog({
  source,
  deleting,
  onCancel,
  onConfirm,
}: {
  source: Source
  deleting: boolean
  onCancel: () => void
  onConfirm: () => void
}) {
  return (
    <Dialog open onOpenChange={(open) => !open && !deleting && onCancel()}>
      <DialogContent
        className="w-[min(440px,100%)]"
        onEscapeKeyDown={(event) => deleting && event.preventDefault()}
        onPointerDownOutside={(event) => deleting && event.preventDefault()}
      >
        <p className="text-xs font-semibold uppercase tracking-widest text-muted-foreground">ARCHIVE SOURCE</p>
        <DialogTitle>删除信息源？</DialogTitle>
        <DialogDescription className="mb-[25px] mt-[9px] text-[13px] leading-normal text-muted-foreground">
          “{source.name}”将停止抓取并从订阅列表中移除。
        </DialogDescription>
        <p className="-mt-2.5 mb-6 rounded-lg bg-[#f1ede5] px-[13px] py-3 text-xs leading-relaxed text-[#6d6257]">已经抓取的文章、收藏和笔记都会保留。</p>
        <div className="mt-[7px] flex justify-end gap-[9px] max-md:flex-wrap">
          <Button variant="secondary" disabled={deleting} onClick={onCancel} autoFocus>取消删除</Button>
          <Button
            variant="destructive"
            disabled={deleting}
            aria-label={`确认删除 ${source.name}`}
            onClick={onConfirm}
          >
            {deleting ? '正在删除…' : '确认删除'}
          </Button>
        </div>
      </DialogContent>
    </Dialog>
  )
}

function formatTime(value: string): string {
  return new Intl.DateTimeFormat('zh-CN', {
    month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit',
  }).format(new Date(value))
}

function Reader({
  view,
  sourceID,
  sourceName,
  sources,
  mobile,
}: {
  view: Exclude<View, 'sources'>
  sourceID: string
  sourceName?: string
  sources: Source[]
  mobile: boolean
}) {
  const [entries, setEntries] = useState<Entry[]>([])
  const [selected, setSelected] = useState<Entry | null>(null)
  const [actionMenuOpen, setActionMenuOpen] = useState(false)
  const [notesOpen, setNotesOpen] = useState(false)
  const [search, setSearch] = useState('')
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [markingAllRead, setMarkingAllRead] = useState(false)
  const [readerNotice, setReaderNotice] = useState('')
  const selectedEntryElement = useRef<HTMLElement | null>(null)
  const readingAreaToScroll = useRef('')

  useEffect(() => {
    let cancelled = false
    setLoading(true)
    setError('')
    const state = view === 'inbox' ? 'inbox' : view
    void api.listEntries({ q: search, state, sourceId: sourceID || undefined }).then((items) => {
      if (cancelled) return
      setEntries(items)
      setSelected((current) => items.find((item) => item.id === current?.id) ?? null)
    }).catch((cause: unknown) => {
      if (!cancelled) setError(cause instanceof Error ? cause.message : '加载文章失败')
    }).finally(() => {
      if (!cancelled) setLoading(false)
    })
    return () => {
      cancelled = true
    }
  }, [search, sourceID, view])

  useEffect(() => {
    if (!selected) return
    function handleKeyDown(event: KeyboardEvent) {
      if (event.key !== 'Escape') return
      const element = selectedEntryElement.current
      setSelected(null)
      setActionMenuOpen(false)
      setNotesOpen(false)
      readingAreaToScroll.current = ''
      window.requestAnimationFrame(() => {
        element?.scrollIntoView({ behavior: 'smooth', block: 'start' })
        selectedEntryElement.current = null
      })
    }
    document.addEventListener('keydown', handleKeyDown)
    return () => document.removeEventListener('keydown', handleKeyDown)
  }, [selected])

  async function patch(item: Entry, change: EntryPatch) {
    const updated = await api.updateEntry(item.id, change)
    setEntries((current) => current.map((candidate) => candidate.id === updated.id ? updated : candidate))
    setSelected((current) => current?.id === updated.id ? updated : current)
  }

  function toggleEntry(item: Entry, element: HTMLElement) {
    if (selected?.id === item.id) {
      setSelected(null)
      setActionMenuOpen(false)
      setNotesOpen(false)
      selectedEntryElement.current = null
      readingAreaToScroll.current = ''
      return
    }

    selectedEntryElement.current = element
    readingAreaToScroll.current = item.id
    setSelected(item)
    setActionMenuOpen(false)
    setNotesOpen(false)
    if (!item.read_at) {
      void patch(item, { read: true })
    }
  }

  async function markAllRead() {
    setMarkingAllRead(true)
    setReaderNotice('')
    try {
      const result = await api.markEntriesRead(sourceID || undefined)
      const readAt = new Date().toISOString()
      setEntries((current) => current.map((item) => item.read_at ? item : { ...item, read_at: readAt }))
      setSelected((current) => current && !current.read_at ? { ...current, read_at: readAt } : current)
      setReaderNotice(result.updated_count > 0 ? `已将 ${result.updated_count} 篇文章标记为已读` : '没有未读文章')
    } catch (cause) {
      setReaderNotice(cause instanceof Error ? cause.message : '全部标记为已读失败')
    } finally {
      setMarkingAllRead(false)
    }
  }

  const title = sourceName || (view === 'starred' ? '收藏' : view === 'later' ? '稍后阅读' : '全部文章')
  const sourceNames = Object.fromEntries(sources.map((source) => [source.id, source.name]))
  return (
    <div className="relative grid h-full min-h-0 w-full grid-rows-[auto_minmax(0,1fr)] overflow-hidden max-md:h-auto">
      <header className="z-[3] flex min-h-[58px] items-center justify-between gap-6 border-b bg-card/95 py-2 pl-[18px] pr-3.5 shadow-[0_1px_3px_rgba(42,48,58,.04)] max-md:static max-md:min-h-[52px] max-md:px-3">
        <div className="flex min-w-0 items-baseline gap-[9px] max-md:hidden" aria-hidden={mobile || undefined}>
          <h1>{title}</h1>
          <span className="flex-none text-[10px] text-muted-foreground">{loading ? '正在更新…' : `${entries.length} 篇`}</span>
        </div>
        <div className="flex w-[min(680px,68%)] items-center justify-end gap-2.5 max-md:w-full max-md:justify-stretch">
          <Button
            variant="secondary"
            size="sm"
            className="h-9 shrink-0 border-border bg-background text-foreground shadow-sm max-md:size-9 max-md:px-0"
            disabled={markingAllRead}
            aria-label={sourceName ? `将 ${sourceName} 全部标记为已读` : '将全部文章标记为已读'}
            onClick={() => void markAllRead()}
          >
            <CheckCheck className="size-4" aria-hidden="true" />
            <span className="max-md:hidden">{markingAllRead ? '正在标记…' : '全部标记为已读'}</span>
          </Button>
          <label className="relative w-[min(390px,55%)] max-md:w-auto max-md:flex-1">
            <span className="sr-only">搜索文章</span>
            <Search className="pointer-events-none absolute left-2.5 top-1/2 z-[1] size-[22px] -translate-y-1/2 text-muted-foreground" strokeWidth={2.25} aria-hidden="true" />
            <Input
              className="h-9 w-full border-border bg-background pl-10 pr-3 text-sm text-foreground shadow-sm focus-visible:ring-2 focus-visible:ring-ring/20"
              value={search}
              onChange={(event) => setSearch(event.target.value)}
              placeholder="搜索文章"
            />
          </label>
        </div>
      </header>
      {readerNotice && <div className="absolute right-4 top-[66px] z-[5] rounded-[7px] border border-[#dce1e5] bg-white/95 px-[11px] py-2 text-[10px] text-[#55606b] shadow-[0_8px_24px_rgba(45,51,60,.1)]" role="status">{readerNotice}</div>}
      <section className="min-h-0 overflow-x-hidden overflow-y-auto overscroll-contain border-0 bg-card shadow-none [scrollbar-gutter:stable] max-md:[-webkit-overflow-scrolling:touch] max-md:[scrollbar-gutter:auto]" aria-label="文章列表">
          {loading && <p className="p-[30px] text-center text-xs text-muted-foreground">正在加载文章…</p>}
          {error && <p className="p-[30px] text-center text-xs text-muted-foreground text-destructive">{error}</p>}
          {!loading && !error && entries.length === 0 && <p className="p-[30px] text-center text-xs text-muted-foreground">这里还没有文章。</p>}
          {entries.map((item) => (
            <article
              data-entry-row={item.id}
              className={cn(
                'border-b last:border-b-0',
                selected?.id === item.id && 'bg-card shadow-[inset_3px_0_hsl(var(--border))]',
              )}
              key={item.id}
            >
              <Button unstyled
                className={cn(
                  'grid min-h-9 w-full cursor-pointer grid-cols-[8px_minmax(110px,15%)_minmax(220px,1fr)_54px_16px] items-center gap-1.5 border-0 bg-card px-2 text-left hover:bg-muted/60',
                  'max-md:min-h-12 max-md:grid-cols-[8px_minmax(0,1fr)_48px_14px] max-md:grid-rows-[auto_auto] max-md:gap-x-1.5 max-md:gap-y-0 max-md:px-2 max-md:py-0.5',
                  item.read_at && 'text-muted-foreground [&_strong]:font-normal',
                )}
                aria-expanded={selected?.id === item.id}
                onClick={(event) => toggleEntry(item, event.currentTarget.closest('article')!)}
              >
                <span className={cn('size-1.5 rounded-full bg-primary max-md:row-span-2', item.read_at && 'border border-muted-foreground bg-transparent')} aria-hidden="true" />
                <span className="text-xs font-[550] text-[#66717d] max-md:col-start-2 max-md:row-start-1 max-md:text-[11px]">{sourceNames[item.source_id] || item.author || '未知来源'}</span>
                <strong>{item.display_title || item.source_title || '无标题'}</strong>
                <time dateTime={item.discovered_at}>{compactTime(item.discovered_at)}</time>
                <ChevronDown className={cn('size-4 text-muted-foreground transition-transform max-md:col-start-4 max-md:row-span-2 max-md:self-center', selected?.id === item.id && 'rotate-180')} aria-hidden="true" />
              </Button>
              {selected?.id === item.id && (
                <div
                  data-entry-detail={item.id}
                  className="min-h-[calc(100vh-100px)] border-t border-[#e8e9eb] bg-card px-[clamp(28px,8vw,120px)] pb-16 max-md:px-[18px] max-md:pb-[38px] max-md:pt-[18px]"
                  ref={(element) => {
                    if (!element || readingAreaToScroll.current !== item.id) return
                    readingAreaToScroll.current = ''
                    window.requestAnimationFrame(() => {
                      element.scrollIntoView({ behavior: 'smooth', block: 'start' })
                    })
                  }}
                >
                  <div className="mx-auto max-w-[820px]">
                    <div className="mb-7 flex min-h-12 items-center justify-between border-b border-[#eeeae2] text-xs text-[#8c887f] max-md:mb-[22px]">
                      <span>{selected.author || sourceNames[selected.source_id] || '未知来源'}</span>
                      <div className="flex items-center gap-3">
                        {selected.canonical_url && (
                          <a href={selected.canonical_url} target="_blank" rel="noreferrer">查看原文 ↗</a>
                        )}
                        <DropdownMenu open={actionMenuOpen} onOpenChange={setActionMenuOpen}>
                          <DropdownMenuTrigger asChild>
                            <Button unstyled className="grid h-7 w-8 cursor-pointer place-items-center rounded-md text-muted-foreground hover:bg-accent hover:text-foreground" aria-label="更多操作"><MoreHorizontal className="size-4" aria-hidden="true" /></Button>
                          </DropdownMenuTrigger>
                          <DropdownMenuContent>
                            <DropdownMenuItem onSelect={() => void patch(selected, { read: false })}>标记未读</DropdownMenuItem>
                            <DropdownMenuItem onSelect={() => void patch(selected, { starred: !selected.starred_at })}>
                              {selected.starred_at ? '取消收藏' : '收藏文章'}
                            </DropdownMenuItem>
                            <DropdownMenuItem onSelect={() => void patch(selected, { later: !selected.later_at })}>
                              {selected.later_at ? '移出稍后阅读' : '稍后阅读'}
                            </DropdownMenuItem>
                            <DropdownMenuItem onSelect={() => setNotesOpen((open) => !open)}>
                              编辑标题与笔记
                            </DropdownMenuItem>
                          </DropdownMenuContent>
                        </DropdownMenu>
                      </div>
                    </div>
                    <h2>{selected.display_title || selected.source_title || '无标题'}</h2>
                    <div
                      className="entry-prose"
                      dangerouslySetInnerHTML={{
                        __html: sanitizeEntryHTML(
                          selected.content_html || selected.summary || '',
                          selected.canonical_url,
                        ),
                      }}
                    />
                    {notesOpen && <div className="mt-[38px] grid max-w-[720px] gap-3 border-t pt-6">
                      <label>
                        <span>显示标题</span>
                        <Input
                          value={selected.display_title}
                          onChange={(event) => setSelected({ ...selected, display_title: event.target.value })}
                          placeholder={selected.source_title}
                        />
                      </label>
                      <label>
                        <span>笔记</span>
                        <Textarea
                          value={selected.note}
                          onChange={(event) => setSelected({ ...selected, note: event.target.value })}
                          placeholder="记录你的想法…"
                        />
                      </label>
                      <Button variant="secondary" onClick={() => void patch(selected, {
                        display_title: selected.display_title,
                        note: selected.note,
                      })}>
                        保存标题与笔记
                      </Button>
                    </div>}
                  </div>
                </div>
              )}
            </article>
          ))}
      </section>
    </div>
  )
}

function compactTime(value: string): string {
  const date = new Date(value)
  const today = new Date()
  if (date.toDateString() === today.toDateString()) {
    return new Intl.DateTimeFormat('zh-CN', { hour: '2-digit', minute: '2-digit' }).format(date)
  }
  return new Intl.DateTimeFormat('zh-CN', { month: '2-digit', day: '2-digit' }).format(date)
}

function sanitizeEntryHTML(value: string, baseURL?: string): string {
  const document = new DOMParser().parseFromString(value, 'text/html')
  const blocked = 'script,style,iframe,object,embed,form,input,button,textarea,select,link,meta,base,svg,math'
  document.body.querySelectorAll(blocked).forEach((element) => element.remove())

  const allowed = new Set([
    'A', 'B', 'BLOCKQUOTE', 'BR', 'CODE', 'DEL', 'DIV', 'EM', 'FIGCAPTION', 'FIGURE',
    'H1', 'H2', 'H3', 'H4', 'H5', 'H6', 'HR', 'I', 'IMG', 'LI', 'OL', 'P', 'PRE',
    'S', 'SPAN', 'STRONG', 'SUB', 'SUP', 'TABLE', 'TBODY', 'TD', 'TFOOT', 'TH',
    'THEAD', 'TR', 'U', 'UL',
  ])

  Array.from(document.body.querySelectorAll('*')).forEach((element) => {
    if (!allowed.has(element.tagName)) {
      element.replaceWith(...Array.from(element.childNodes))
      return
    }

    const href = element.tagName === 'A' ? safeContentURL(element.getAttribute('href'), baseURL) : ''
    const src = element.tagName === 'IMG' ? safeContentURL(element.getAttribute('src'), baseURL) : ''
    const alt = element.tagName === 'IMG' ? element.getAttribute('alt') || '' : ''
    element.getAttributeNames().forEach((name) => element.removeAttribute(name))

    if (element.tagName === 'A' && href) {
      element.setAttribute('href', href)
      element.setAttribute('target', '_blank')
      element.setAttribute('rel', 'noopener noreferrer')
    }
    if (element.tagName === 'IMG' && src) {
      element.setAttribute('src', src)
      element.setAttribute('alt', alt)
      element.setAttribute('loading', 'lazy')
      element.setAttribute('decoding', 'async')
      element.setAttribute('referrerpolicy', 'no-referrer')
    } else if (element.tagName === 'IMG') {
      element.remove()
    }
  })

  return document.body.innerHTML
}

function safeContentURL(value: string | null, baseURL?: string): string {
  if (!value) return ''
  try {
    const url = new URL(value, baseURL || window.location.origin)
    return url.protocol === 'http:' || url.protocol === 'https:' ? url.href : ''
  } catch {
    return ''
  }
}

function CreateSourceDialog({
  onClose,
  onCreate,
}: {
  onClose: () => void
  onCreate: (input: CreateSourceInput) => Promise<void>
}) {
  const [name, setName] = useState('')
  const [locator, setLocator] = useState('')
  const [kind, setKind] = useState<SourceKind>('rss')
  const [itemsPath, setItemsPath] = useState('items')
  const [idField, setIDField] = useState('id')
  const [titleField, setTitleField] = useState('title')
  const [urlField, setURLField] = useState('url')
  const [paginationMode, setPaginationMode] = useState('none')
  const [paginationPath, setPaginationPath] = useState('')
  const [paginationParam, setPaginationParam] = useState('')
  const [htmlMode, setHTMLMode] = useState('collection')
  const [itemSelector, setItemSelector] = useState('article')
  const [titleSelector, setTitleSelector] = useState('h2')
  const [linkSelector, setLinkSelector] = useState('a')
  const [contentSelector, setContentSelector] = useState('')
  const [submitting, setSubmitting] = useState(false)
  const [testing, setTesting] = useState(false)
  const [error, setError] = useState('')
  const [preview, setPreview] = useState<PreviewResult | null>(null)

  function input(): CreateSourceInput {
    const base: CreateSourceInput = { name: name.trim(), kind, locator: locator.trim() }
    if (kind === 'html') {
      const fields: Record<string, Record<string, unknown>> = {
        title: { selector: titleSelector.trim() },
        url: { selector: linkSelector.trim(), attribute: 'href' },
      }
      if (contentSelector.trim()) {
        fields.content_html = { selector: contentSelector.trim(), html: true }
      }
      return {
        ...base,
        config: {
          mode: htmlMode,
          ...(htmlMode === 'collection' ? { item_selector: itemSelector.trim() } : {}),
          fields,
        },
      }
    }
    if (kind !== 'json-api') return base
    const fields: Record<string, string> = { id: idField.trim() }
    if (titleField.trim()) fields.title = titleField.trim()
    if (urlField.trim()) fields.url = urlField.trim()
    const pagination: Record<string, unknown> = { mode: paginationMode }
    if (paginationMode === 'page') {
      pagination.page_param = paginationParam.trim() || 'page'
      pagination.start = 1
    } else if (paginationMode === 'next') {
      pagination.next_path = paginationPath.trim()
    } else if (paginationMode === 'cursor') {
      pagination.cursor_path = paginationPath.trim()
      pagination.cursor_param = paginationParam.trim() || 'cursor'
    }
    return {
      ...base,
      config: { items_path: itemsPath.trim(), fields, pagination },
    }
  }

  async function testSource(event: FormEvent) {
    event.preventDefault()
    setTesting(true)
    setError('')
    try {
      setPreview(await api.previewSource(input()))
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : '测试信息源失败')
    } finally {
      setTesting(false)
    }
  }

  async function save() {
    setSubmitting(true)
    setError('')
    try {
      await onCreate(input())
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : '保存信息源失败')
      setSubmitting(false)
    }
  }

  return (
    <Dialog open onOpenChange={(open) => !open && onClose()}>
      <DialogContent>
        <DialogClose className="absolute right-4 top-4 grid size-8 cursor-pointer place-items-center rounded-md text-muted-foreground hover:bg-accent hover:text-foreground" aria-label="关闭"><X className="size-4" aria-hidden="true" /></DialogClose>
        <p className="text-xs font-semibold uppercase tracking-widest text-muted-foreground">NEW SOURCE</p>
        <DialogTitle>添加信息源</DialogTitle>
        <div className="mt-3.5 flex gap-2 text-[10px] font-bold text-[#99958b]" aria-label="配置步骤">
          <span className={cn('rounded-full bg-muted px-2.5 py-1 text-muted-foreground', !preview && 'bg-primary text-primary-foreground', preview && 'bg-emerald-100 text-emerald-700')}>1 配置</span>
          <span className={cn('rounded-full bg-muted px-2.5 py-1 text-muted-foreground', preview && 'bg-primary text-primary-foreground')}>2 预览并保存</span>
        </div>
        <DialogDescription className="mb-[25px] mt-[9px] text-[13px] leading-normal text-muted-foreground">先临时抓取并检查文章身份；确认前不会写入数据库。</DialogDescription>

        <form onSubmit={(event) => void testSource(event)}>
          <label>
            <span>来源类型</span>
            <Select
              value={kind}
              onChange={(event) => {
                setKind(event.target.value as SourceKind)
                setPreview(null)
              }}
            >
              <option value="rss">RSS / Atom / JSON Feed</option>
              <option value="json-api">JSON API</option>
              <option value="html">静态 HTML</option>
            </Select>
          </label>
          <label>
            <span>名称</span>
            <Input
              autoFocus
              required
              value={name}
              onChange={(event) => {
                setName(event.target.value)
                setPreview(null)
              }}
              placeholder="例如：技术博客"
            />
          </label>
          <label>
            <span>{kind === 'json-api' ? 'API 地址' : kind === 'html' ? '网页地址' : 'Feed 地址'}</span>
            <Input
              required
              type="url"
              value={locator}
              onChange={(event) => {
                setLocator(event.target.value)
                setPreview(null)
              }}
              placeholder={
                kind === 'json-api'
                  ? 'https://api.example.com/items'
                  : kind === 'html'
                    ? 'https://example.com/news'
                    : 'https://example.com/feed.xml'
              }
            />
          </label>
          {kind === 'json-api' && (
            <div className="grid gap-3.5 rounded-[9px] border border-[#e0dcd3] bg-[#f6f3ed] p-[15px]">
              <label>
                <span>列表路径</span>
                <Input required value={itemsPath} onChange={(event) => setItemsPath(event.target.value)} placeholder="data.items" />
              </label>
              <div className="grid grid-cols-3 gap-[9px] max-md:grid-cols-1">
                <label>
                  <span>ID 字段</span>
                  <Input required value={idField} onChange={(event) => setIDField(event.target.value)} placeholder="id" />
                </label>
                <label>
                  <span>标题字段</span>
                  <Input value={titleField} onChange={(event) => setTitleField(event.target.value)} placeholder="title" />
                </label>
                <label>
                  <span>URL 字段</span>
                  <Input value={urlField} onChange={(event) => setURLField(event.target.value)} placeholder="url" />
                </label>
              </div>
              <label>
                <span>分页方式</span>
                <Select value={paginationMode} onChange={(event) => setPaginationMode(event.target.value)}>
                  <option value="none">不分页</option>
                  <option value="page">页码参数</option>
                  <option value="next">下一页 URL</option>
                  <option value="cursor">游标</option>
                </Select>
              </label>
              {(paginationMode === 'next' || paginationMode === 'cursor') && (
                <label>
                  <span>{paginationMode === 'next' ? '下一页路径' : '游标路径'}</span>
                  <Input
                    required
                    value={paginationPath}
                    onChange={(event) => setPaginationPath(event.target.value)}
                    placeholder={paginationMode === 'next' ? 'paging.next' : 'paging.cursor'}
                  />
                </label>
              )}
              {(paginationMode === 'page' || paginationMode === 'cursor') && (
                <label>
                  <span>{paginationMode === 'page' ? '页码参数' : '游标参数'}</span>
                  <Input
                    value={paginationParam}
                    onChange={(event) => setPaginationParam(event.target.value)}
                    placeholder={paginationMode === 'page' ? 'page' : 'cursor'}
                  />
                </label>
              )}
            </div>
          )}
          {kind === 'html' && (
            <div className="grid gap-3.5 rounded-[9px] border border-[#e0dcd3] bg-[#f6f3ed] p-[15px]">
              <label>
                <span>页面模式</span>
                <Select value={htmlMode} onChange={(event) => setHTMLMode(event.target.value)}>
                  <option value="collection">列表页面</option>
                  <option value="single">单文档</option>
                </Select>
              </label>
              {htmlMode === 'collection' && (
                <label>
                  <span>条目选择器</span>
                  <Input required value={itemSelector} onChange={(event) => setItemSelector(event.target.value)} placeholder="article.card" />
                </label>
              )}
              <div className="grid grid-cols-3 gap-[9px] max-md:grid-cols-1">
                <label>
                  <span>标题选择器</span>
                  <Input required value={titleSelector} onChange={(event) => setTitleSelector(event.target.value)} placeholder="h2.title" />
                </label>
                <label>
                  <span>链接选择器</span>
                  <Input required value={linkSelector} onChange={(event) => setLinkSelector(event.target.value)} placeholder="a.permalink" />
                </label>
                <label>
                  <span>正文选择器</span>
                  <Input value={contentSelector} onChange={(event) => setContentSelector(event.target.value)} placeholder=".content" />
                </label>
              </div>
              <div className="flex flex-wrap gap-1.5">
                <span>条目 <code>{htmlMode === 'collection' ? itemSelector : 'document'}</code></span>
                <span>标题 <code>{titleSelector}</code></span>
                <span>链接 <code>{linkSelector}@href</code></span>
              </div>
            </div>
          )}
          {error && <p className="m-0 text-xs" role="alert">{error}</p>}
          {preview && (
            <div className="overflow-hidden rounded-[9px] border border-input bg-[#f7f5ef]">
              <div className="flex justify-between border-b border-[#dfdcd3] px-3 py-[11px] text-[11px] text-success">
                <strong>连接成功</strong>
                <span>发现 {preview.candidates.length} 条预览内容</span>
              </div>
              {kind === 'html' && (
                <div className="bg-accent px-3 py-[7px] text-[9px] font-bold tracking-[.08em] text-[#8d887e]">
                  选择器提取预览
                </div>
              )}
              <div className="max-h-[230px] overflow-y-auto">
                {preview.candidates.length === 0 && <p>Feed 当前没有可预览的文章。</p>}
                {preview.candidates.map((candidate, index) => (
                  <article key={`${candidate.identity_key}-${index}`}>
                    <strong>{candidate.title || '无标题'}</strong>
                    {candidate.url && <span>{candidate.url}</span>}
                    <code>{candidate.identity_key}</code>
                    {candidate.identity_warning && <em>{candidate.identity_warning}</em>}
                  </article>
                ))}
              </div>
            </div>
          )}
          <div className="mt-[7px] flex justify-end gap-[9px] max-md:flex-wrap">
            <Button variant="secondary" type="button" onClick={onClose}>取消</Button>
            {!preview && (
              <Button type="submit" disabled={testing}>
                {testing ? '正在测试…' : '测试并预览'}
              </Button>
            )}
            {preview && (
              <>
                <Button variant="secondary" type="button" onClick={() => setPreview(null)}>
                  返回修改
                </Button>
                <Button type="button" disabled={submitting} onClick={() => void save()}>
                  {submitting ? '正在保存…' : '保存并启用'}
                </Button>
              </>
            )}
          </div>
        </form>
      </DialogContent>
    </Dialog>
  )
}

function NavIcon({ name }: { name: string }) {
  const icons: Record<string, LucideIcon> = {
    inbox: Inbox,
    source: Rss,
    star: Star,
    clock: Clock3,
    bookmark: Bookmark,
    book: BookOpen,
  }
  const Icon = icons[name] ?? Inbox
  return <Icon className="size-4 shrink-0" strokeWidth={1.8} aria-hidden="true" />
}
