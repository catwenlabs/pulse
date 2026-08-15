import { createContext, FormEvent, useCallback, useContext, useEffect, useRef, useState, type DragEvent, type ReactNode, type SetStateAction } from 'react'
import { QueryClientProvider, useInfiniteQuery, useMutation, useQuery, useQueryClient, type InfiniteData } from '@tanstack/react-query'
import { Link, useNavigate } from '@tanstack/react-router'
import {
  BookOpen,
  Bookmark,
  CheckCheck,
  ChevronDown,
  Clock3,
  FolderClosed,
  Inbox,
  Menu,
  MessageSquareText,
  MoreHorizontal,
  Plus,
  RefreshCw,
  Rss,
  Search,
  Settings,
  Star,
  Sparkles,
  X,
  type LucideIcon,
} from 'lucide-react'

import * as api from './api'
import { DigestPage, DigestHistoryPanel, StoryDetailPage } from './AISummarization'
import type { CreateSourceInput, Entry, Folder, PreviewResult, Source, SourceHealth, SourceKind, Story, StoryPatch } from './api'
import { EntryReader } from './components/EntryReader'
import { ConversationHistoryPage } from './components/ConversationHistoryPage'
import { SelectionChatSurface } from './components/SelectionChatSurface'
import { SelectionToolsSettings } from './components/SelectionToolsSettings'
import { StoryListItem, type StoryListItemChange } from './components/StoryListItem'
import { isActiveStorySummary, isStoredStorySummary, StorySummaryCard } from './components/storySummary'
import { Button, buttonVariants } from './components/ui/button'
import { Dialog, DialogClose, DialogContent, DialogDescription, DialogTitle, SheetContent } from './components/ui/dialog'
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuTrigger } from './components/ui/dropdown-menu'
import { Input } from './components/ui/input'
import { Select } from './components/ui/select'
import { Textarea } from './components/ui/textarea'
import { Toaster } from './components/ui/sonner'
import { compactTime, HighlightText, scrollWithin } from './lib/readerFormat'
import { cn } from './lib/utils'
import { createQueryClient, queryKeys } from './query'
import { useLibraryRealtime, type LibraryRealtimeSignal, type RealtimeConnectionState } from './realtime'
import { toast } from 'sonner'
import './styles.css'

export type View = 'sources' | 'inbox' | 'starred' | 'later' | 'ai' | 'story' | 'settings' | 'ai-conversations' | 'tools'
export type ToolKey = 'ai-conversations' | 'sources' | 'settings'
type SaveRequest = { url: string; title: string }
type ReaderEntry = Entry & {
  display_title: string
  note: string
  read_at?: string
  starred_at?: string
  hidden_at?: string
  later_at?: string
}

type ReaderStory = Pick<Story, 'id' | 'display_title' | 'note' | 'tags' | 'entry_count' | 'source_count' | 'read_at' | 'starred_at' | 'hidden_at' | 'later_at'> & {
  representative: Entry
  entries?: Entry[]
}
type ReaderPage = api.StoryPage | api.SourceEntryPage
type NavigationDragItem =
  | { scope: 'folders'; id: string }
  | { scope: 'root-sources'; id: string }
  | { scope: 'folder-sources'; folderID: string; id: string }
const EMPTY_SOURCES: Source[] = []
const EMPTY_FOLDERS: Folder[] = []
type NavigationFocusContextValue = {
  requestMobileMenuFocus: () => void
  consumeMobileMenuFocus: () => boolean
}
const NavigationFocusContext = createContext<NavigationFocusContextValue>({
  requestMobileMenuFocus: () => {},
  consumeMobileMenuFocus: () => false,
})

function projectReaderEntry(item: Entry, owner: Pick<ReaderStory, 'display_title' | 'note' | 'read_at' | 'starred_at' | 'hidden_at' | 'later_at'>): ReaderEntry {
  return {
    ...item,
    display_title: owner.display_title ?? '',
    note: owner.note ?? '',
    read_at: owner.read_at,
    starred_at: owner.starred_at,
    hidden_at: owner.hidden_at,
    later_at: owner.later_at,
  }
}

function orderReaderEntries(items: ReaderEntry[]): ReaderEntry[] {
  return [
    ...items.filter((item) => !item.read_at),
    ...items.filter((item) => Boolean(item.read_at)),
  ]
}

function readerStoryFromStory(item: Story): ReaderStory {
  return item
}

function readerStoryFromSourceEntry(item: api.SourceEntry): ReaderStory {
  return {
    id: item.story.id,
    display_title: item.story.display_title,
    note: item.story.note,
    tags: item.story.tags,
    representative: item.entry,
    entry_count: item.story.entry_count,
    source_count: item.story.source_count,
    read_at: item.story.read_at,
    starred_at: item.story.starred_at,
    hidden_at: item.story.hidden_at,
    later_at: item.story.later_at,
  }
}

function navItemClass(active: boolean, className?: string) {
  return cn(
    'flex min-h-10 cursor-pointer items-center gap-3 rounded-lg px-3 text-sm font-medium leading-5 text-muted-foreground no-underline transition-colors hover:bg-sidebar-accent hover:text-sidebar-accent-foreground',
    active && 'bg-sidebar-accent text-sidebar-accent-foreground',
    className,
  )
}

// Compact icon-only navigation item for the secondary (bottom) rail group.
function iconNavItemClass(active: boolean, className?: string) {
  return cn(
    'grid size-10 w-full cursor-pointer place-items-center rounded-lg text-muted-foreground no-underline transition-colors hover:bg-sidebar-accent hover:text-sidebar-accent-foreground',
    active && 'bg-sidebar-accent text-sidebar-accent-foreground',
    className,
  )
}

// Reveals an icon-only nav item's name on hover (and keyboard focus) with a
// styled tooltip to the right of the rail.
function IconNavTooltip({ label, children }: { label: string; children: ReactNode }) {
  return (
    <span className="group relative grid size-10 w-full place-items-center">
      {children}
      <span
        role="tooltip"
        className="pointer-events-none absolute left-full top-1/2 z-50 ml-2 -translate-y-1/2 whitespace-nowrap rounded-md bg-foreground px-2 py-1 text-xs font-medium text-background opacity-0 shadow-md transition-opacity duration-150 group-hover:opacity-100 group-focus-within:opacity-100"
      >
        {label}
      </span>
    </span>
  )
}

// Shared middle-column panel (the rail companion) used by the subscription
// tree, the settings menu, and the digest history. Keeping the three-column
// layout in one component makes the convention apply consistently to every
// view instead of duplicating the grid classes per panel.
function RailPanel({ labelledBy, children }: { labelledBy?: string; children: ReactNode }) {
  return (
    <section
      aria-labelledby={labelledBy}
      className="mt-6 flex min-h-0 flex-1 flex-col overflow-hidden px-2 max-md:order-2 max-md:mt-0 md:col-start-2 md:row-span-3 md:row-start-1 md:mt-0 md:border-l md:bg-[#f2f0e9] md:px-3 md:py-5"
    >
      {children}
    </section>
  )
}

// Shared chrome for the stream views (the reader for inbox/starred/later and
// the reading-notes view): a sticky app header (title + count + actions) above
// a scrollable card content area. Reader and AnnotationsPage render through
// this so every secondary view keeps one consistent layout instead of each
// re-implementing the header and scroll region.
function StreamShell({
  title,
  count,
  actions,
  banners,
  floating,
  contentLabel = '文章列表',
  contentRef,
  mobile,
  mobileMenuButtonRef,
  mobileNavigationOpen,
  onOpenMobileNavigation,
  children,
}: {
  title: string
  count?: string
  actions?: ReactNode
  banners?: ReactNode
  floating?: ReactNode
  contentLabel?: string
  contentRef?: { current: HTMLElement | null }
  mobile: boolean
  mobileMenuButtonRef: { current: HTMLButtonElement | null }
  mobileNavigationOpen: boolean
  onOpenMobileNavigation: () => void
  children: ReactNode
}) {
  return (
    <div className="relative grid h-full min-h-0 w-full grid-rows-[auto_minmax(0,1fr)] overflow-hidden">
      <header className={cn(
        'z-[3] flex min-h-16 items-center justify-between gap-6 border-b bg-card/95 px-5 py-2 shadow-[0_1px_3px_rgba(42,48,58,.04)]',
        mobile && 'max-md:grid max-md:grid-cols-[40px_minmax(0,1fr)_auto] max-md:gap-1.5 max-md:min-h-[calc(52px+env(safe-area-inset-top))] max-md:px-3.5 max-md:pt-[env(safe-area-inset-top)]',
      )}>
        {mobile && (
          <Button
            unstyled
            className="grid size-10 cursor-pointer content-center place-items-center rounded-lg border-0 bg-transparent px-[9px] text-foreground hover:bg-accent focus-visible:bg-accent focus-visible:outline-none"
            ref={mobileMenuButtonRef}
            aria-label="打开导航"
            aria-controls="mobile-navigation"
            aria-expanded={mobileNavigationOpen}
            onClick={onOpenMobileNavigation}
          >
            <Menu className="size-5" aria-hidden="true" />
          </Button>
        )}
        <div className={cn(
          'flex min-w-0 items-baseline gap-3',
          mobile ? 'max-md:min-w-0' : 'max-md:hidden',
        )}>
          <h1 className="m-0 truncate text-lg font-[650] leading-[1.1] max-md:text-[15px]">{title}</h1>
          {!mobile && count && <span className="flex-none text-xs text-muted-foreground">{count}</span>}
        </div>
        <div className={cn(
          'flex w-[min(680px,68%)] items-center justify-end gap-2.5',
          mobile && 'max-md:col-start-3 max-md:w-auto max-md:gap-0.5',
        )}>
          {actions}
        </div>
      </header>
      {banners}
      {floating}
      <section
        aria-label={contentLabel}
        ref={contentRef}
        className="min-h-0 overflow-x-hidden overflow-y-auto overscroll-contain border-0 bg-card shadow-none [scrollbar-gutter:stable] max-md:[-webkit-overflow-scrolling:touch] max-md:[scrollbar-gutter:auto]"
      >
        {children}
      </section>
    </div>
  )
}

function sameNavigationScope(left: NavigationDragItem, right: NavigationDragItem) {
  if (left.scope !== right.scope) return false
  return left.scope !== 'folder-sources' || right.scope !== 'folder-sources' || left.folderID === right.folderID
}

function moveNavigationItem(ids: string[], draggedID: string, targetID: string) {
  if (draggedID === targetID) return ids
  const remaining = ids.filter((id) => id !== draggedID)
  const targetIndex = remaining.indexOf(targetID)
  if (targetIndex < 0) return ids
  remaining.splice(targetIndex, 0, draggedID)
  return remaining
}

function reorderItems<T extends { id: string }>(items: T[], ids: string[]) {
  const byID = new Map(items.map((item) => [item.id, item]))
  return ids.flatMap((id) => {
    const item = byID.get(id)
    return item ? [item] : []
  })
}

function sourceHealthRefreshKey(items: Source[]) {
  return items.map((item) => JSON.stringify(item)).sort().join('\u0000')
}

function NavigationDropIndicator({ active }: { active: boolean }) {
  if (!active) return null
  return (
    <div
      role="separator"
      aria-label="放置于此"
      aria-orientation="horizontal"
      className="pointer-events-none mx-1 my-1 h-0 border-t-2 border-primary shadow-[0_0_0_1px_hsl(var(--primary)/.12)]"
    />
  )
}

function UnreadBadge({ count, className }: { count: number; className?: string }) {
  if (!count) return null
  return (
    <span className={cn('shrink-0 text-xs font-semibold tabular-nums text-muted-foreground', className)}>
      {count}
    </span>
  )
}

export function AppProvider({ children }: { children: ReactNode }) {
  const [queryClient] = useState(createQueryClient)
  const focusOnNextRoute = useRef(false)
  const requestMobileMenuFocus = useCallback(() => {
    focusOnNextRoute.current = true
  }, [])
  const consumeMobileMenuFocus = useCallback(() => {
    const shouldFocus = focusOnNextRoute.current
    focusOnNextRoute.current = false
    return shouldFocus
  }, [])
  return (
    <QueryClientProvider client={queryClient}>
      <NavigationFocusContext.Provider value={{ requestMobileMenuFocus, consumeMobileMenuFocus }}>
        {children}
      </NavigationFocusContext.Provider>
    </QueryClientProvider>
  )
}

export function App() {
  return (
    <AppProvider>
      <AppContent view="inbox" sourceID="" />
    </AppProvider>
  )
}

export function AppContent({ view, sourceID: selectedSourceID, storyID = '' }: { view: View; sourceID: string; storyID?: string }) {
  const queryClient = useQueryClient()
  const navigate = useNavigate()
  const { requestMobileMenuFocus, consumeMobileMenuFocus } = useContext(NavigationFocusContext)
  const { connectionState, signal } = useLibraryRealtime(queryClient)
  const sourcesQuery = useQuery({
    queryKey: queryKeys.sources,
    queryFn: api.listSources,
    refetchOnMount: false,
  })
  const foldersQuery = useQuery({
    queryKey: queryKeys.folders,
    queryFn: api.listFolders,
  })
  const sources = sourcesQuery.data ?? EMPTY_SOURCES
  const folders = foldersQuery.data ?? EMPTY_FOLDERS
  const healthRefreshKey = sourceHealthRefreshKey(sources)
  const setSources = (updater: SetStateAction<Source[]>) => {
    queryClient.setQueryData<Source[]>(queryKeys.sources, (current) => (
      typeof updater === 'function' ? updater(current ?? []) : updater
    ))
  }
  const setFolders = (updater: SetStateAction<Folder[]>) => {
    queryClient.setQueryData<Folder[]>(queryKeys.folders, (current) => (
      typeof updater === 'function' ? updater(current ?? []) : updater
    ))
  }
  const [expandedFolders, setExpandedFolders] = useState<Set<string>>(() => new Set())
  const loading = sourcesQuery.isPending
  const loadError = sourcesQuery.error instanceof Error
    ? sourcesQuery.error.message
    : foldersQuery.error instanceof Error
      ? foldersQuery.error.message
      : ''
  const [showCreate, setShowCreate] = useState(false)
  const [sourceToDelete, setSourceToDelete] = useState<Source | null>(null)
  const [sourceToEdit, setSourceToEdit] = useState<Source | null>(null)
  const [sourceToOrganize, setSourceToOrganize] = useState<Source | null>(null)
  const [deleting, setDeleting] = useState(false)
  const [health, setHealth] = useState<Record<string, SourceHealth>>({})
  const [serviceConnected, setServiceConnected] = useState<boolean | null>(null)
  const [mobileNavigationOpen, setMobileNavigationOpen] = useState(false)
  const [showBookmarklet, setShowBookmarklet] = useState(false)
  const [tool, setTool] = useState<ToolKey>('settings')
  const [selectedDigestID, setSelectedDigestID] = useState('')
  const [saveRequest, setSaveRequest] = useState<SaveRequest | null>(() => readSaveRequest())
  const [navigationDropTarget, setNavigationDropTarget] = useState<NavigationDragItem | null>(null)
  const isMobile = useMediaQuery('(max-width: 767px)')
  const mobileMenuButtonRef = useRef<HTMLButtonElement>(null)
  const mobileDrawerCloseRef = useRef<HTMLButtonElement>(null)
  const bookmarkletButtonRef = useRef<HTMLButtonElement>(null)
  const navigationDragItemRef = useRef<NavigationDragItem | null>(null)

  async function load() {
    await Promise.all([sourcesQuery.refetch(), foldersQuery.refetch()])
  }

  async function refreshSources() {
    try {
      await sourcesQuery.refetch()
    } catch {
      // unread counts refresh on the next navigation if this fails
    }
  }

  function beginNavigationDrag(item: NavigationDragItem, event: DragEvent<HTMLElement>) {
    navigationDragItemRef.current = item
    setNavigationDropTarget(null)
    if (event.dataTransfer) {
      event.dataTransfer.effectAllowed = 'move'
      event.dataTransfer.setData('text/plain', item.id)
    }
  }

  function allowNavigationDrop(item: NavigationDragItem, event: DragEvent<HTMLElement>) {
    const dragged = navigationDragItemRef.current
    if (!dragged || !sameNavigationScope(dragged, item)) return
    if (dragged.id === item.id) {
      setNavigationDropTarget(null)
      return
    }
    event.preventDefault()
    setNavigationDropTarget(item)
    if (event.dataTransfer) event.dataTransfer.dropEffect = 'move'
  }

  async function handleNavigationDrop(item: NavigationDragItem, event: DragEvent<HTMLElement>) {
    event.preventDefault()
    const dragged = navigationDragItemRef.current
    navigationDragItemRef.current = null
    setNavigationDropTarget(null)
    if (!dragged || !sameNavigationScope(dragged, item) || dragged.id === item.id) return

    try {
      if (item.scope === 'folders' && dragged.scope === 'folders') {
        const ids = moveNavigationItem(folders.map((folder) => folder.id), dragged.id, item.id)
        const previous = folders
        setFolders(reorderItems(folders, ids))
        try {
          await api.reorderFolders(ids)
        } catch (error) {
          setFolders(previous)
          throw error
        }
        return
      }

      if (item.scope === 'root-sources' && dragged.scope === 'root-sources') {
        const ids = moveNavigationItem(rootSources.map((source) => source.id), dragged.id, item.id)
        const previous = sources
        const nextRootSources = reorderItems(rootSources, ids)
        let nextRootIndex = 0
        const assigned = new Set(folders.flatMap((folder) => folder.source_ids))
        setSources(sources.map((source) => {
          if (assigned.has(source.id)) return source
          const next = nextRootSources[nextRootIndex]
          nextRootIndex += 1
          return next ?? source
        }))
        try {
          await api.reorderRootSources(ids)
        } catch (error) {
          setSources(previous)
          throw error
        }
        return
      }

      if (item.scope === 'folder-sources' && dragged.scope === 'folder-sources') {
        const folder = folders.find((candidate) => candidate.id === item.folderID)
        if (!folder) return
        const ids = moveNavigationItem(folder.source_ids, dragged.id, item.id)
        const previous = folders
        setFolders(folders.map((candidate) => candidate.id === folder.id
          ? { ...candidate, source_ids: ids }
          : candidate))
        try {
          await api.reorderFolderSources(folder.id, ids)
        } catch (error) {
          setFolders(previous)
          throw error
        }
      }
    } catch (error) {
      toast.error(error instanceof Error ? error.message : '无法保存导航顺序', { duration: 6000 })
    }
  }

  function endNavigationDrag() {
    navigationDragItemRef.current = null
    setNavigationDropTarget(null)
  }

  function isNavigationDropTarget(item: NavigationDragItem) {
    return navigationDropTarget !== null
      && sameNavigationScope(navigationDropTarget, item)
      && navigationDropTarget.id === item.id
  }

  useEffect(() => {
    setExpandedFolders((current) => new Set([
      ...current,
      ...folders.map((folder) => folder.id),
    ]))
  }, [folders])

  useEffect(() => {
    if (!consumeMobileMenuFocus()) return
    window.setTimeout(() => {
      document.querySelector<HTMLButtonElement>('button[aria-label="打开导航"]')?.focus()
    }, 0)
  }, [consumeMobileMenuFocus, selectedSourceID, view])

  useEffect(() => {
    let active = true
    const refreshHealth = async () => {
      const snapshots = await Promise.all(sources.map(async (item) => {
        try {
          return [item.id, await api.getSourceHealth(item.id)] as const
        } catch {
          return null
        }
      }))
      if (active) {
        setHealth(Object.fromEntries(snapshots.filter((item): item is readonly [string, SourceHealth] => item !== null)))
      }
    }
    void refreshHealth()
    return () => {
      active = false
    }
  }, [healthRefreshKey])

  useEffect(() => {
    let active = true
    let activeController: AbortController | null = null
    let nextCheckID: number | undefined
    const updateServiceHealth = async () => {
      const controller = new AbortController()
      activeController = controller
      const timeoutID = window.setTimeout(() => controller.abort(), 8_000)
      const connected = await api.checkServiceHealth(controller.signal)
      window.clearTimeout(timeoutID)
      if (!active) return
      activeController = null
      setServiceConnected(connected)
      nextCheckID = window.setTimeout(() => void updateServiceHealth(), 30_000)
    }
    void updateServiceHealth()
    return () => {
      active = false
      activeController?.abort()
      if (nextCheckID !== undefined) window.clearTimeout(nextCheckID)
    }
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

  async function handleCreate(input: CreateSourceInput, selectedFolderIDs: Set<string>) {
    const created = await api.createSource(input)
    setSources((current) => [...current, created])
    const folderIDs = [...selectedFolderIDs]
    const results = await Promise.allSettled(
      folderIDs.map((folderID) => api.addSourceToFolder(folderID, created.id)),
    )
    const assignedFolderIDs = new Set(folderIDs.filter((_, index) => results[index].status === 'fulfilled'))
    if (selectedFolderIDs.size > 0) {
      setFolders((current) => current.map((folder) => {
        if (!assignedFolderIDs.has(folder.id) || folder.source_ids.includes(created.id)) return folder
        return {
          ...folder,
          source_count: folder.source_count + 1,
          source_ids: [...folder.source_ids, created.id],
        }
      }))
      const refreshed = await api.listFolders().catch(() => null)
      if (refreshed) setFolders(refreshed)
      setExpandedFolders((current) => new Set([...current, ...selectedFolderIDs]))
    }
    setShowCreate(false)
    if (results.some((result) => result.status === 'rejected')) {
      toast.warning(`已添加 ${created.name}，但部分文件夹归类失败`, { duration: 6000 })
    } else {
      toast.success(`已添加 ${created.name}`, { duration: 3500 })
    }
  }

  async function handleRun(source: Source) {
    try {
      await api.runSource(source.id)
      toast.success('抓取任务已进入队列', { duration: 3500 })
    } catch (error) {
      toast.error(error instanceof Error ? error.message : '无法创建抓取任务', { duration: 6000 })
    }
  }

  async function handleToggle(source: Source) {
    try {
      const updated = await api.setSourceEnabled(source.id, !source.enabled)
      setSources((current) => current.map((item) => item.id === updated.id ? updated : item))
      toast.success(updated.enabled ? `已恢复 ${updated.name}` : `已暂停 ${updated.name}`, { duration: 3500 })
    } catch (error) {
      toast.error(error instanceof Error ? error.message : '无法更新信息源', { duration: 6000 })
    }
  }

  async function handleEdit(source: Source, name: string, locator: string) {
    const updated = await api.updateSource(source.id, { name, locator })
    setSources((current) => current
      .map((item) => item.id === updated.id ? updated : item))
    setSourceToEdit(null)
    toast.success(`已更新 ${updated.name}`, { duration: 3500 })
  }

  async function handleArchive(source: Source) {
    setDeleting(true)
    try {
      await api.archiveSource(source.id)
      setSources((current) => current.filter((item) => item.id !== source.id))
      setHealth((current) => Object.fromEntries(
        Object.entries(current).filter(([id]) => id !== source.id),
      ))
      if (selectedSourceID === source.id) void navigate({ to: '/' })
      setFolders(await api.listFolders().catch(() => folders))
      setSourceToDelete(null)
      toast.success(`已删除 ${source.name}`, { duration: 3500 })
    } catch (error) {
      toast.error(error instanceof Error ? error.message : '无法删除信息源', { duration: 6000 })
    } finally {
      setDeleting(false)
    }
  }

  async function handleOrganize(source: Source, selectedFolderIDs: Set<string>, newFolderName: string) {
    const currentFolderIDs = new Set(
      folders.filter((folder) => folder.source_ids.includes(source.id)).map((folder) => folder.id),
    )
    try {
      if (newFolderName.trim()) {
        const createdFolder = await api.createFolder(newFolderName.trim())
        selectedFolderIDs.add(createdFolder.id)
      }
      // Additions happen before removals so a failed request does not unexpectedly
      // leave the Source without any of its requested Folder memberships.
      await Promise.all([...selectedFolderIDs]
        .filter((folderID) => !currentFolderIDs.has(folderID))
        .map((folderID) => api.addSourceToFolder(folderID, source.id)))
      await Promise.all([...currentFolderIDs]
        .filter((folderID) => !selectedFolderIDs.has(folderID))
        .map((folderID) => api.removeSourceFromFolder(folderID, source.id)))
      const refreshed = await api.listFolders()
      setFolders(refreshed)
      setExpandedFolders((current) => new Set([
        ...current,
        ...selectedFolderIDs,
      ]))
      setSourceToOrganize((current) => current?.id === source.id ? null : current)
      toast.success(`已整理 ${source.name}`, { duration: 3500 })
    } catch (cause) {
      // These APIs are individually atomic. Reconcile after a partial failure and
      // close the stale form so retrying cannot create the same Folder twice.
      const refreshed = await api.listFolders().catch(() => null)
      if (refreshed) setFolders(refreshed)
      setSourceToOrganize((current) => current?.id === source.id ? null : current)
      toast.error(cause instanceof Error ? cause.message : '无法整理信息源', { duration: 6000 })
    }
  }

  function showStream(view: Exclude<View, 'sources' | 'ai' | 'story' | 'settings' | 'ai-conversations'>, sourceID = '') {
    if (view === 'starred') void navigate({ to: '/starred' })
    else if (view === 'later') void navigate({ to: '/later' })
    else if (sourceID) void navigate({ to: '/sources/$sourceID', params: { sourceID } })
    else void navigate({ to: '/' })
    closeMobileNavigation()
  }

  function closeMobileNavigation(restoreFocus = true) {
    if (!mobileNavigationOpen) return
    setMobileNavigationOpen(false)
    if (restoreFocus) {
      if (isMobile) requestMobileMenuFocus()
      window.setTimeout(() => {
        const menuButton = mobileMenuButtonRef.current
          ?? document.querySelector<HTMLButtonElement>('button[aria-label="打开导航"]')
        menuButton?.focus()
      }, 0)
    }
  }

  const activeView = view
  const isReaderView = activeView === 'inbox' || activeView === 'starred' || activeView === 'later'
  const isToolsView = activeView === 'tools'
  const showSourceTree = activeView !== 'tools' && activeView !== 'ai' && (isMobile || activeView === 'inbox')
  const showToolsMenu = activeView === 'tools' && !isMobile
  const showDigestsHistory = activeView === 'ai'
  const showMiddlePanel = showSourceTree || showToolsMenu || showDigestsHistory
  const activeSourceName = sources.find((source) => source.id === selectedSourceID)?.name
  const assignedSourceIDs = new Set(folders.flatMap((folder) => folder.source_ids))
  const rootSources = sources.filter((source) => !assignedSourceIDs.has(source.id))
  const totalUnread = sources.reduce((sum, source) => sum + (source.unread_count || 0), 0)
  useEffect(() => {
    document.title = totalUnread > 0 ? `(${totalUnread}) Pulse` : 'Pulse'
    return () => {
      document.title = 'Pulse'
    }
  }, [totalUnread])
  const mobileTitle = activeView === 'sources'
    ? '信息源'
    : activeSourceName || (
      activeView === 'starred'
        ? '收藏'
        : activeView === 'later'
          ? '稍后阅读'
          : activeView === 'ai'
              ? 'AI 追更'
              : activeView === 'story'
                ? 'Story'
                : activeView === 'tools'
                  ? '设置'
                  : activeView === 'settings'
                    ? '划词工具'
                    : activeView === 'ai-conversations'
                      ? 'AI 对话'
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
    <Toaster />
    <div className={cn(
      'grid h-dvh min-h-0 overflow-hidden max-md:block max-md:w-full',
      showMiddlePanel ? 'grid-cols-[288px_minmax(0,1fr)]' : 'grid-cols-[72px_minmax(0,1fr)]',
    )}>
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
        className={cn(
          'fixed inset-y-0 left-0 z-30 flex w-56 flex-col border-r bg-sidebar px-3 pb-0 pt-[max(0.25rem,env(safe-area-inset-top))] text-sidebar-foreground transition-[width,transform] motion-reduce:transition-none md:grid md:grid-rows-[auto_minmax(0,1fr)_auto] md:p-0 md:translate-x-0 max-md:w-[min(86vw,20rem)] max-md:-translate-x-full max-md:shadow-xl data-[state=open]:translate-x-0',
          showMiddlePanel ? 'md:w-72 md:grid-cols-[72px_216px]' : 'md:w-[72px] md:grid-cols-[72px]',
        )}
        id="mobile-navigation"
        role="navigation"
        aria-label="移动导航抽屉"
        aria-hidden={isMobile ? !mobileNavigationOpen : undefined}
        inert={isMobile && !mobileNavigationOpen ? true : undefined}
      >
        <div className="flex items-center justify-between px-1.5 pb-1 md:col-start-1 md:row-start-1 md:flex-col md:gap-3 md:px-2 md:pb-2 md:pt-4 max-md:px-1">
          <Link className="flex min-h-11 items-center gap-2 font-serif text-xl font-semibold leading-none text-foreground no-underline md:min-h-0 md:gap-3 md:text-2xl" to="/" aria-label="Pulse 首页" onClick={() => closeMobileNavigation(false)}>
            <span className="grid size-8 shrink-0 place-items-center rounded-[9px_9px_9px_3px] bg-primary font-sans text-sm font-bold text-white md:size-10 md:rounded-[12px_12px_12px_4px] md:text-lg" aria-hidden="true">P</span>
            <span className="md:hidden">Pulse</span>
          </Link>
          <div className="flex items-center gap-1.5">
            {isMobile && (
              <Button unstyled className="group grid size-11 cursor-pointer place-items-center rounded-lg border-0 bg-transparent text-white" aria-label="添加信息源" onClick={() => {
                closeMobileNavigation(false)
                setShowCreate(true)
              }}>
                <span className="grid size-7 place-items-center rounded-md bg-primary transition-colors group-hover:bg-primary-hover">
                  <Plus className="size-3.5" aria-hidden="true" />
                </span>
                <span className="sr-only">添加信息源</span>
              </Button>
            )}
            {isMobile && (
              <Button unstyled
                className="group hidden size-11 cursor-pointer place-items-center rounded-lg border-0 bg-transparent p-0 text-[#66635b] max-md:grid"
                ref={mobileDrawerCloseRef}
                aria-label="关闭导航"
                onClick={() => closeMobileNavigation()}
              >
                <span className="grid size-7 place-items-center rounded-md bg-white/50 transition-colors group-hover:bg-white/80">
                  <X className="size-3.5" aria-hidden="true" />
                </span>
              </Button>
            )}
          </div>
        </div>

        {showSourceTree && <RailPanel labelledBy="source-tree-label">
          <div className="mb-3 flex items-center justify-between px-1 text-xs text-muted-foreground">
            <p className="m-0 text-sm font-semibold text-foreground md:text-base" id="source-tree-label">订阅源</p>
            <span>{sources.length}</span>
          </div>
          <div className="mb-3 border-b border-border/70 pb-3">
            <Link
              className={navItemClass(activeView === 'inbox' && !selectedSourceID, 'min-h-10 max-md:min-h-11')}
              to="/"
              onClick={() => closeMobileNavigation()}
            >
              <NavIcon name="inbox" />全部文章
              <UnreadBadge count={totalUnread} className="ml-auto" />
            </Link>
          </div>
          {loading && <p className="border-0 bg-transparent px-2 py-1 text-left text-xs text-muted-foreground">正在同步信息源…</p>}
          {!loading && loadError && <Button unstyled className="border-0 bg-transparent px-2 py-1 text-left text-xs text-destructive" onClick={() => void load()}>重试加载</Button>}
          <div className="min-h-0 flex-1 overflow-y-auto overscroll-contain">
            {folders.map((folder) => (
              <div key={folder.id}>
                <NavigationDropIndicator active={isNavigationDropTarget({ scope: 'folders', id: folder.id })} />
                <Button
                  unstyled
                  draggable
                  className="flex min-h-9 w-full cursor-grab items-center gap-2 rounded-md px-2 text-sm font-medium text-sidebar-foreground hover:bg-sidebar-accent hover:text-sidebar-accent-foreground active:cursor-grabbing max-md:min-h-11"
                  aria-label={`${folder.name}，${folder.source_count} 个订阅源`}
                  aria-expanded={expandedFolders.has(folder.id)}
                  onDragStart={(event) => beginNavigationDrag({ scope: 'folders', id: folder.id }, event)}
                  onDragOver={(event) => allowNavigationDrop({ scope: 'folders', id: folder.id }, event)}
                  onDrop={(event) => void handleNavigationDrop({ scope: 'folders', id: folder.id }, event)}
                  onDragEnd={endNavigationDrag}
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
                        <div key={`${folder.id}-${source.id}`}>
                          <NavigationDropIndicator active={isNavigationDropTarget({ scope: 'folder-sources', folderID: folder.id, id: source.id })} />
                          <Button
                            unstyled
                            draggable
                            className={navItemClass(selectedSourceID === source.id && activeView === 'inbox', 'w-full cursor-grab py-1 text-sm active:cursor-grabbing max-md:min-h-11')}
                            title={source.name}
                            onDragStart={(event) => beginNavigationDrag({ scope: 'folder-sources', folderID: folder.id, id: source.id }, event)}
                            onDragOver={(event) => allowNavigationDrop({ scope: 'folder-sources', folderID: folder.id, id: source.id }, event)}
                            onDrop={(event) => void handleNavigationDrop({ scope: 'folder-sources', folderID: folder.id, id: source.id }, event)}
                            onDragEnd={endNavigationDrag}
                            onClick={() => showStream('inbox', source.id)}
                          >
                            <span className={cn('size-2 shrink-0 rounded-full bg-muted-foreground/40', source.enabled && 'bg-emerald-500')} />
                            <span className="min-w-0 flex-1 truncate text-left">{source.name}</span>
                            <UnreadBadge count={source.unread_count} />
                          </Button>
                        </div>
                      )
                    })}
                  </div>
                )}
              </div>
            ))}
            {rootSources.map((source) => (
              <div key={source.id}>
                <NavigationDropIndicator active={isNavigationDropTarget({ scope: 'root-sources', id: source.id })} />
                <Button
                  unstyled
                  draggable
                  className={navItemClass(selectedSourceID === source.id && activeView === 'inbox', 'w-full cursor-grab py-1 text-sm active:cursor-grabbing max-md:min-h-11')}
                  title={source.name}
                  onDragStart={(event) => beginNavigationDrag({ scope: 'root-sources', id: source.id }, event)}
                  onDragOver={(event) => allowNavigationDrop({ scope: 'root-sources', id: source.id }, event)}
                  onDrop={(event) => void handleNavigationDrop({ scope: 'root-sources', id: source.id }, event)}
                  onDragEnd={endNavigationDrag}
                  onClick={() => showStream('inbox', source.id)}
                >
                  <span className={cn('size-2 shrink-0 rounded-full bg-muted-foreground/40', source.enabled && 'bg-emerald-500')} />
                  <span className="min-w-0 flex-1 truncate text-left">{source.name}</span>
                  <UnreadBadge count={source.unread_count} />
                </Button>
              </div>
            ))}
          </div>
        </RailPanel>}

        {showToolsMenu && <RailPanel labelledBy="tools-menu-label">
          <div className="mb-3 flex items-center justify-between px-1 text-xs text-muted-foreground">
            <p className="m-0 text-sm font-semibold text-foreground md:text-base" id="tools-menu-label">设置</p>
          </div>
          <div className="mb-3 border-b border-border/70 pb-3">
            <Button unstyled className={navItemClass(tool === 'ai-conversations', 'w-full justify-start')} aria-current={tool === 'ai-conversations' ? 'page' : undefined} onClick={() => setTool('ai-conversations')}>
              <NavIcon name="chat" />AI 对话
            </Button>
            <Button unstyled className={navItemClass(tool === 'sources', 'w-full justify-start')} aria-current={tool === 'sources' ? 'page' : undefined} onClick={() => setTool('sources')}>
              <NavIcon name="source" />管理信息源
            </Button>
            <Button unstyled className={navItemClass(tool === 'settings', 'w-full justify-start')} aria-current={tool === 'settings' ? 'page' : undefined} onClick={() => setTool('settings')}>
              <NavIcon name="settings" />划词工具设置
            </Button>
            <Button unstyled ref={bookmarkletButtonRef} className={navItemClass(false, 'w-full justify-start')} onClick={() => setShowBookmarklet(true)}>
              <NavIcon name="bookmark" />安装保存书签
            </Button>
          </div>
        </RailPanel>}

        {showDigestsHistory && <RailPanel labelledBy="digest-history-label">
          <DigestHistoryPanel digestID={selectedDigestID} onSelect={(digestID) => {
            setSelectedDigestID(digestID)
            if (isMobile) closeMobileNavigation()
          }} />
        </RailPanel>}

        {isMobile ? (
          <div className={cn(
            'order-3 border-t border-[#d8d4ca] px-2 pt-2 pb-[max(0.75rem,env(safe-area-inset-bottom))] text-xs text-muted-foreground',
            !showSourceTree && 'max-md:mt-auto',
          )}>
            <DropdownMenu>
              <DropdownMenuTrigger asChild>
                <Button unstyled className={navItemClass(activeView !== 'inbox', 'min-h-11 w-full justify-between gap-3 border border-border/80 bg-card px-2.5 shadow-sm hover:border-primary/25')} aria-label="更多导航">
                  <span className="flex items-center gap-2.5">
                    <span className="grid size-7 place-items-center rounded-md bg-primary/10 text-primary">
                      <MoreHorizontal className="size-4" aria-hidden="true" />
                    </span>
                    <span>更多</span>
                  </span>
                  <ChevronDown className="size-3.5 text-muted-foreground/70" aria-hidden="true" />
                </Button>
              </DropdownMenuTrigger>
              <DropdownMenuContent side="top" align="start" className="w-[min(17rem,calc(86vw-2rem))]">
                <DropdownMenuItem className={cn('min-h-11 gap-3', activeView === 'starred' && 'bg-accent font-semibold text-primary')} aria-current={activeView === 'starred' ? 'page' : undefined} onSelect={() => showStream('starred')}><NavIcon name="star" />收藏</DropdownMenuItem>
                <DropdownMenuItem className={cn('min-h-11 gap-3', activeView === 'later' && 'bg-accent font-semibold text-primary')} aria-current={activeView === 'later' ? 'page' : undefined} onSelect={() => showStream('later')}><NavIcon name="clock" />稍后阅读</DropdownMenuItem>
                <DropdownMenuItem className={cn('min-h-11 gap-3', activeView === 'ai' && 'bg-accent font-semibold text-primary')} aria-current={activeView === 'ai' ? 'page' : undefined} onSelect={() => {
                  void navigate({ to: '/digests' })
                  closeMobileNavigation()
                }}><NavIcon name="ai" />AI 追更</DropdownMenuItem>
                <DropdownMenuItem className={cn('min-h-11 gap-3', activeView === 'tools' && 'bg-accent font-semibold text-primary')} aria-current={activeView === 'tools' ? 'page' : undefined} onSelect={() => {
                  void navigate({ to: '/tools' })
                  closeMobileNavigation()
                }}><NavIcon name="settings" />设置</DropdownMenuItem>
                <div className="mt-1 flex min-h-11 items-center gap-3 border-t px-3 pt-1 text-xs text-muted-foreground" role="status">
                  <span
                    className={cn(
                      'size-2 shrink-0 rounded-full',
                      serviceConnected === null && 'animate-pulse bg-muted-foreground/40',
                      serviceConnected === true && 'bg-success shadow-[0_0_0_3px_rgba(93,148,108,.13)]',
                      serviceConnected === false && 'bg-destructive shadow-[0_0_0_3px_hsl(var(--destructive)/.12)]',
                    )}
                    aria-hidden="true"
                  />
                  {serviceConnected === null ? '正在检查本地服务…' : serviceConnected ? '本地服务已连接' : '本地服务不可用'}
                </div>
              </DropdownMenuContent>
            </DropdownMenu>
          </div>
        ) : (
          <>
            <nav className="grid min-h-0 content-start gap-1 overflow-y-auto px-2 py-3 md:col-start-1 md:row-start-2" aria-label="主导航">
              <Link className={navItemClass(activeView === 'ai', 'min-h-[54px] flex-col justify-center gap-1 px-1 text-[10px] leading-none')} to="/digests" onClick={() => closeMobileNavigation()}><NavIcon name="ai" />AI 追更</Link>
              <Button unstyled className={navItemClass(false, 'min-h-[54px] w-full flex-col justify-center gap-1 px-1 text-center text-[10px] leading-none')} aria-label="添加信息源" onClick={() => {
                closeMobileNavigation(false)
                setShowCreate(true)
              }}>
                <Plus className="size-4 shrink-0" strokeWidth={1.8} aria-hidden="true" />
                <span>添加</span>
              </Button>
            </nav>
            <div className="m-0 grid gap-2 border-t border-[#d8d4ca] px-2 pb-4 pt-2 text-xs leading-5 text-muted-foreground md:col-start-1 md:row-start-3">
              <IconNavTooltip label="收藏">
                <Link className={iconNavItemClass(activeView === 'starred')} to="/starred" aria-label="收藏" onClick={() => closeMobileNavigation()}><NavIcon name="star" /></Link>
              </IconNavTooltip>
              <IconNavTooltip label="稍后阅读">
                <Link className={iconNavItemClass(activeView === 'later')} to="/later" aria-label="稍后阅读" onClick={() => closeMobileNavigation()}><NavIcon name="clock" /></Link>
              </IconNavTooltip>
              <IconNavTooltip label="阅读笔记">
              </IconNavTooltip>
              <IconNavTooltip label="设置">
                <Button unstyled className={iconNavItemClass(activeView === 'tools')} aria-label="设置" onClick={() => void navigate({ to: '/tools' })}>
                  <NavIcon name="settings" />
                </Button>
              </IconNavTooltip>
              <span
                className="flex min-h-11 items-center justify-center px-0 py-1"
                title={serviceConnected === null ? '正在检查本地服务' : serviceConnected ? '本地服务已连接' : '本地服务不可用'}
                role="status"
              >
                <span
                  className={cn(
                    'size-2 rounded-full',
                    serviceConnected === null && 'animate-pulse bg-muted-foreground/40',
                    serviceConnected === true && 'bg-success shadow-[0_0_0_3px_rgba(93,148,108,.13)]',
                      serviceConnected === false && 'bg-destructive shadow-[0_0_0_3px_hsl(var(--destructive)/.12)]',
                  )}
                  aria-hidden="true"
                />
                <span className="sr-only">{serviceConnected === null ? '正在检查本地服务' : serviceConnected ? '本地服务已连接' : '本地服务不可用'}</span>
              </span>
            </div>
          </>
        )}
      </aside>
      </SheetContent>

      <main
        className={cn(
          'col-start-2 h-dvh min-w-0 overflow-y-auto overscroll-contain bg-background p-0 [scrollbar-gutter:stable]',
          activeView !== 'sources' && activeView !== 'ai' && activeView !== 'story' && activeView !== 'settings' && activeView !== 'ai-conversations' && activeView !== 'tools' && 'overflow-hidden',
          !isReaderView && activeView !== 'sources' && activeView !== 'ai' && activeView !== 'story' && activeView !== 'settings' && activeView !== 'ai-conversations' && activeView !== 'tools' && 'max-md:grid max-md:grid-rows-[auto_minmax(0,1fr)]',
          isReaderView && 'max-md:block',
          (activeView === 'ai' || activeView === 'story') && 'max-md:block',
          activeView === 'sources' && 'px-[clamp(24px,4vw,56px)] py-9 max-md:px-0 max-md:py-0',
          (activeView === 'settings' || activeView === 'ai-conversations' || activeView === 'tools') && 'px-[clamp(24px,4vw,56px)] py-9 max-md:px-4 max-md:py-6',
        )}
        inert={isMobile && mobileNavigationOpen ? true : undefined}
      >
        {isMobile && !isReaderView && (
          <header className="max-md:sticky max-md:top-0 max-md:z-[7] max-md:grid max-md:min-h-[calc(52px+env(safe-area-inset-top))] max-md:grid-cols-[40px_minmax(0,1fr)] max-md:items-center max-md:gap-2 max-md:border-b max-md:bg-card/95 max-md:px-3.5 max-md:pt-[env(safe-area-inset-top)] max-md:backdrop-blur-xl">
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
        {isMobile && activeView === 'tools' && (
          <nav className="mx-auto mb-6 flex w-full max-w-[1280px] items-center gap-2 overflow-x-auto px-1 md:hidden" aria-label="工具导航">
            <Button unstyled className={cn('min-h-10 shrink-0 rounded-full px-4 text-sm font-medium', tool === 'ai-conversations' ? 'bg-primary text-primary-foreground' : 'bg-muted text-muted-foreground hover:bg-accent')} aria-current={tool === 'ai-conversations' ? 'page' : undefined} onClick={() => setTool('ai-conversations')}>AI 对话</Button>
            <Button unstyled className={cn('min-h-10 shrink-0 rounded-full px-4 text-sm font-medium', tool === 'sources' ? 'bg-primary text-primary-foreground' : 'bg-muted text-muted-foreground hover:bg-accent')} aria-current={tool === 'sources' ? 'page' : undefined} onClick={() => setTool('sources')}>管理信息源</Button>
            <Button unstyled className={cn('min-h-10 shrink-0 rounded-full px-4 text-sm font-medium', tool === 'settings' ? 'bg-primary text-primary-foreground' : 'bg-muted text-muted-foreground hover:bg-accent')} aria-current={tool === 'settings' ? 'page' : undefined} onClick={() => setTool('settings')}>划词工具设置</Button>
            <Button unstyled className="min-h-10 shrink-0 rounded-full bg-muted px-4 text-sm font-medium text-muted-foreground hover:bg-accent" onClick={() => setShowBookmarklet(true)}>安装保存书签</Button>
          </nav>
        )}
        {activeView === 'sources' || (activeView === 'tools' && tool === 'sources') ? (
          <>
        <header className="mx-auto mb-8 flex w-full max-w-[1280px] items-end justify-between gap-6 max-md:mb-6 max-md:flex-col max-md:items-stretch max-md:px-4 max-md:pt-6">
          <div>
            <p className="mb-2 text-xs font-semibold uppercase tracking-[0.16em] text-primary">LIBRARY</p>
            <h1 className="m-0 font-serif text-4xl font-semibold tracking-tight text-foreground max-md:text-3xl">信息源</h1>
            <p className="mb-0 mt-2 text-sm leading-6 text-muted-foreground">管理 Pulse 持续关注的 RSS、API、网页与推送来源。</p>
          </div>
          <div className="flex shrink-0 gap-2.5 max-md:w-full">
            <a className={buttonVariants({ variant: 'secondary', className: 'max-md:flex-1' })} href="/api/v1/opml/export">导出 OPML</a>
            <Button className="max-md:flex-1" onClick={() => setShowCreate(true)}>
              <Plus className="size-4" aria-hidden="true" /> 添加信息源
            </Button>
          </div>
        </header>

        <section className="mx-auto w-full max-w-[1280px] overflow-hidden rounded-xl border bg-card shadow-sm max-md:mb-6 max-md:w-[calc(100%-2rem)]" aria-labelledby="source-heading">
          <div className="flex items-center justify-between border-b bg-muted/20 px-5 py-4">
            <div>
              <h2 className="m-0 text-base font-semibold text-foreground" id="source-heading">全部信息源</h2>
              <span className="mt-1 block text-xs text-muted-foreground">{sources.length} 个来源</span>
            </div>
            <Button variant="ghost" size="icon" aria-label="重新载入" onClick={() => void load()}><RefreshCw className="size-4" aria-hidden="true" /></Button>
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
            <div>
              {sources.map((source) => (
                <article className="grid min-h-[88px] grid-cols-[42px_minmax(180px,1fr)_110px_minmax(180px,240px)_auto] items-center gap-4 border-b px-5 py-4 last:border-b-0 hover:bg-muted/25 max-lg:grid-cols-[42px_minmax(180px,1fr)_110px_auto] max-md:grid-cols-[40px_minmax(0,1fr)] max-md:gap-x-3 max-md:px-4" key={source.id}>
                  <div className="grid size-10 place-items-center rounded-lg bg-primary/10 font-serif text-lg font-semibold text-primary" aria-hidden="true">{source.name.slice(0, 1).toUpperCase()}</div>
                  <div className="min-w-0">
                    <div className="flex min-w-0 items-center gap-2">
                      <h3 className="m-0 truncate text-sm font-semibold text-foreground">{source.name}</h3>
                      <span className="shrink-0 rounded-md bg-muted px-2 py-0.5 text-[10px] font-semibold uppercase tracking-wide text-muted-foreground">{source.kind}</span>
                    </div>
                    <p className="mb-0 mt-1 truncate text-xs text-muted-foreground" title={source.locator}>{source.locator}</p>
                  </div>
                  <div className="flex items-center gap-2 text-xs text-muted-foreground max-md:col-start-2 max-md:row-start-2">
                    <span className={cn('size-2 rounded-full bg-muted-foreground/40', source.enabled && 'bg-emerald-500')} />
                    {source.enabled ? '已启用' : '已暂停'}
                  </div>
                  {health[source.id] && (
                    <div className="grid min-w-[180px] gap-0.5 text-[11px] leading-4 text-muted-foreground max-lg:hidden" aria-label={`${source.name} 抓取诊断`}>
                      <span>状态 {health[source.id].status}</span>
                      {health[source.id].last_requested_at && <span>最近 {formatTime(health[source.id].last_requested_at!)}</span>}
                      <span>{health[source.id].candidate_count} 候选 · {health[source.id].new_count} 新增 · {health[source.id].updated_count} 更新</span>
                      <span>{health[source.id].duration_milliseconds} ms · 连续失败 {health[source.id].consecutive_failures}</span>
                      {health[source.id].next_scheduled_at && <span>下次 {formatTime(health[source.id].next_scheduled_at!)}</span>}
                      {health[source.id].last_error && <em>{health[source.id].last_error}</em>}
                    </div>
                  )}
                  <div className="flex items-center justify-end gap-1 max-md:col-span-2 max-md:mt-1 max-md:justify-start">
                    <Button variant="ghost" size="sm"
                      aria-label={`编辑 ${source.name}`}
                      onClick={() => setSourceToEdit(source)}
                    >
                      编辑
                    </Button>
                    <Button variant="ghost" size="sm"
                      aria-label={`整理 ${source.name} 到文件夹`}
                      onClick={() => setSourceToOrganize(source)}
                    >
                      整理
                    </Button>
                    <Button variant="ghost" size="sm"
                      aria-label={`${source.enabled ? '暂停' : '恢复'} ${source.name}`}
                      onClick={() => void handleToggle(source)}
                    >
                      {source.enabled ? '暂停' : '恢复'}
                    </Button>
                    <Button variant="ghost" size="icon"
                      aria-label={`刷新 ${source.name}`}
                      disabled={!source.enabled}
                      onClick={() => void handleRun(source)}
                    >
                      <RefreshCw className="size-4" aria-hidden="true" />
                    </Button>
                    <Button variant="ghost" size="sm"
                      className="text-destructive hover:bg-destructive/10 hover:text-destructive"
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
        ) : activeView === 'tools' ? (
          <div className="mx-auto w-full max-w-[1280px]">
            {tool === 'ai-conversations' ? <ConversationHistoryPage /> : <SelectionToolsSettings />}
          </div>
        ) : activeView === 'ai' ? (
          <DigestPage digestID={selectedDigestID} onSelectDigest={setSelectedDigestID} />
        ) : activeView === 'story' ? (
          <StoryDetailPage storyID={storyID} />
        ) : activeView === 'settings' ? (
          <div className="mx-auto w-full max-w-[1280px]">
            <SelectionToolsSettings />
          </div>
        ) : activeView === 'ai-conversations' ? (
          <div className="mx-auto w-full max-w-[1280px]">
            <ConversationHistoryPage />
          </div>
        ) : (
          <Reader
            view={activeView}
            sourceID={selectedSourceID}
            sourceName={activeSourceName}
            sources={sources}
            mobile={isMobile}
            mobileMenuButtonRef={mobileMenuButtonRef}
            mobileNavigationOpen={mobileNavigationOpen}
            onOpenMobileNavigation={() => setMobileNavigationOpen(true)}
            refreshSources={refreshSources}
            realtimeSignal={signal}
            realtimeConnectionState={connectionState}
          />
        )}
      </main>

      {showCreate && (
        <CreateSourceDialog
          folders={folders}
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
      {sourceToEdit && (
        <EditSourceDialog
          source={sourceToEdit}
          onClose={() => setSourceToEdit(null)}
          onSave={(name, locator) => handleEdit(sourceToEdit, name, locator)}
        />
      )}
      {sourceToOrganize && (
        <OrganizeSourceDialog
          source={sourceToOrganize}
          folders={folders}
          onClose={() => setSourceToOrganize(null)}
          onSave={(selectedFolderIDs, newFolderName) => handleOrganize(sourceToOrganize, selectedFolderIDs, newFolderName)}
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
      <section className="w-[min(520px,100%)] rounded-2xl border border-input bg-card p-9 shadow-[0_22px_70px_rgba(48,43,34,.12)] max-md:min-h-[calc(100dvh-24px)] max-md:rounded-xl max-md:p-5" aria-labelledby="save-page-title">
        <div className="mb-8 flex items-center gap-3 font-serif text-2xl font-semibold max-md:mb-6"><span className="grid size-10 place-items-center rounded-[12px_12px_12px_4px] bg-primary font-sans text-lg font-bold text-white" aria-hidden="true">P</span>Pulse</div>
        <p className="text-xs font-semibold uppercase tracking-[0.14em] text-muted-foreground">READ LATER</p>
        <h1 id="save-page-title">保存到 Pulse</h1>
        <p className="mb-6 mt-2 text-base leading-6 text-muted-foreground">确认网页信息后，将它加入你的阅读列表。</p>
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

function EditSourceDialog({
  source,
  onClose,
  onSave,
}: {
  source: Source
  onClose: () => void
  onSave: (name: string, locator: string) => Promise<void>
}) {
  const [name, setName] = useState(source.name)
  const [locator, setLocator] = useState(source.locator)
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState('')
  const locatorLabel = source.kind === 'rss'
    ? 'Feed 地址'
    : source.kind === 'json-api'
      ? 'API 地址'
      : source.kind === 'html'
        ? '网页地址'
        : '位置'

  async function submit(event: FormEvent) {
    event.preventDefault()
    setSaving(true)
    setError('')
    try {
      await onSave(name.trim(), locator.trim())
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : '更新信息源失败')
      setSaving(false)
    }
  }

  return (
    <Dialog open onOpenChange={(open) => !open && onClose()}>
      <DialogContent>
        <DialogClose className="absolute right-4 top-4 grid size-8 cursor-pointer place-items-center rounded-md text-muted-foreground hover:bg-accent hover:text-foreground" aria-label="关闭">
          <X className="size-4" aria-hidden="true" />
        </DialogClose>
        <p className="text-xs font-semibold uppercase tracking-widest text-muted-foreground">EDIT SOURCE</p>
        <DialogTitle>编辑信息源</DialogTitle>
        <DialogDescription className="mb-5 mt-2 text-sm leading-6 text-muted-foreground">
          修改名称会立即更新文章列表中的来源显示。来源类型和高级配置保持不变。
        </DialogDescription>
        <form className="grid gap-4" onSubmit={(event) => void submit(event)}>
          <label>
            <span>名称</span>
            <Input autoFocus required value={name} onChange={(event) => setName(event.target.value)} />
          </label>
          <label>
            <span>{locatorLabel}</span>
            <Input
              required
              type={source.kind === 'rss' || source.kind === 'json-api' || source.kind === 'html' ? 'url' : 'text'}
              value={locator}
              onChange={(event) => setLocator(event.target.value)}
            />
          </label>
          {error && <p className="m-0 text-sm text-destructive" role="alert">{error}</p>}
          <div className="mt-2 flex justify-end gap-2">
            <Button type="button" variant="ghost" onClick={onClose}>取消</Button>
            <Button type="submit" disabled={saving}>{saving ? '保存中…' : '保存修改'}</Button>
          </div>
        </form>
      </DialogContent>
    </Dialog>
  )
}

function FolderPicker({
  folders,
  selectedFolderIDs,
  onChange,
  disabled = false,
  legend,
}: {
  folders: Folder[]
  selectedFolderIDs: Set<string>
  onChange: (folderIDs: Set<string>) => void
  disabled?: boolean
  legend: string
}) {
  return (
    <fieldset className="grid gap-2 border-0 p-0">
      <legend className="mb-2 text-sm font-semibold">{legend}</legend>
      {folders.map((folder) => {
        const selected = selectedFolderIDs.has(folder.id)
        return (
          <label
            className={cn(
              'min-h-11 cursor-pointer grid-cols-[auto_auto_minmax(0,1fr)_auto] items-center !gap-3 rounded-lg border bg-card px-3 !font-normal transition-colors hover:bg-muted/40',
              selected && 'border-primary/50 bg-primary/5',
              disabled && 'cursor-not-allowed opacity-50',
            )}
            key={folder.id}
          >
            <input
              className="size-4 accent-primary"
              type="checkbox"
              checked={selected}
              disabled={disabled}
              onChange={(event) => {
                const next = new Set(selectedFolderIDs)
                if (event.target.checked) next.add(folder.id)
                else next.delete(folder.id)
                onChange(next)
              }}
            />
            <FolderClosed className={cn('size-4 text-muted-foreground', selected && 'text-primary')} aria-hidden="true" />
            <span className="min-w-0 flex-1 truncate">{folder.name}</span>
            <span className="text-xs text-muted-foreground">{folder.source_count}</span>
          </label>
        )
      })}
    </fieldset>
  )
}

function OrganizeSourceDialog({
  source,
  folders,
  onClose,
  onSave,
}: {
  source: Source
  folders: Folder[]
  onClose: () => void
  onSave: (selectedFolderIDs: Set<string>, newFolderName: string) => Promise<void>
}) {
  const [selectedFolderIDs, setSelectedFolderIDs] = useState<Set<string>>(() => new Set(
    folders.filter((folder) => folder.source_ids.includes(source.id)).map((folder) => folder.id),
  ))
  const [newFolderName, setNewFolderName] = useState('')
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState('')

  async function submit(event: FormEvent) {
    event.preventDefault()
    setSaving(true)
    setError('')
    try {
      await onSave(new Set(selectedFolderIDs), newFolderName)
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : '无法整理信息源')
      setSaving(false)
    }
  }

  return (
    <Dialog open onOpenChange={(open) => !open && !saving && onClose()}>
      <DialogContent>
        <DialogTitle>整理到文件夹</DialogTitle>
        <DialogDescription className="mb-5 mt-2 text-sm leading-6 text-muted-foreground">
          “{source.name}”可以同时出现在多个文件夹中。
        </DialogDescription>
        <form onSubmit={(event) => void submit(event)}>
          {folders.length === 0 ? (
            <div>
              <p className="mb-2 text-sm font-semibold">已有文件夹</p>
              <p className="m-0 rounded-lg border border-dashed p-4 text-sm text-muted-foreground">还没有文件夹，请在下方创建一个。</p>
            </div>
          ) : (
            <FolderPicker
              folders={folders}
              selectedFolderIDs={selectedFolderIDs}
              onChange={setSelectedFolderIDs}
              disabled={saving}
              legend="已有文件夹"
            />
          )}
          <label>
            <span>新建并加入文件夹（可选）</span>
            <Input
              value={newFolderName}
              onChange={(event) => setNewFolderName(event.target.value)}
              placeholder="例如：技术"
            />
          </label>
          <p className="m-0 text-xs text-muted-foreground">不选择任何文件夹会把该信息源移到根目录。</p>
          {error && <p className="m-0" role="alert">{error}</p>}
          <div className="flex justify-end gap-2">
            <DialogClose asChild><Button type="button" variant="secondary" disabled={saving}>取消</Button></DialogClose>
            <Button type="submit" disabled={saving}>{saving ? '正在保存…' : '保存'}</Button>
          </div>
        </form>
      </DialogContent>
    </Dialog>
  )
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
  mobileMenuButtonRef,
  mobileNavigationOpen,
  onOpenMobileNavigation,
  refreshSources,
  realtimeSignal,
  realtimeConnectionState,
}: {
  view: Exclude<View, 'sources' | 'ai' | 'story' | 'settings' | 'ai-conversations'>
  sourceID: string
  sourceName?: string
  sources: Source[]
  mobile: boolean
  mobileMenuButtonRef: { current: HTMLButtonElement | null }
  mobileNavigationOpen: boolean
  onOpenMobileNavigation: () => void
  refreshSources: () => void
  realtimeSignal: LibraryRealtimeSignal | null
  realtimeConnectionState: RealtimeConnectionState
}) {
  const navigate = useNavigate()
  const [entries, setEntries] = useState<ReaderEntry[]>([])
  const [storiesByEntry, setStoriesByEntry] = useState<Record<string, ReaderStory>>({})
  const [expandedEntryId, setExpandedEntryId] = useState<string | null>(null)
  const [search, setSearch] = useState('')
  const [debouncedSearch, setDebouncedSearch] = useState('')
  const [mobileSearchOpen, setMobileSearchOpen] = useState(false)
  const [markingAllRead, setMarkingAllRead] = useState(false)
  const [readerNotice, setReaderNotice] = useState('')
  const [pendingNewCount, setPendingNewCount] = useState(0)
  const [highlightedEntryIDs, setHighlightedEntryIDs] = useState<Set<string>>(() => new Set())
  const entryStreamElement = useRef<HTMLElement | null>(null)
  const readerNoticeTimeout = useRef<number | undefined>(undefined)
  const readerNoticeVersion = useRef(0)
  const knownStoryIDs = useRef<Set<string>>(new Set())
  const pendingStoryIDs = useRef<Set<string>>(new Set())
  const hasRenderedServerData = useRef(false)
  const pageSize = 50
  const queryClient = useQueryClient()
  const state = view === 'inbox' ? 'inbox' : view
  const readerKey = queryKeys.reader({
    q: debouncedSearch,
    state,
    sourceId: sourceID || undefined,
    view,
    limit: pageSize,
  })
  const readerQuery = useInfiniteQuery<ReaderPage, Error, InfiniteData<ReaderPage, string | undefined>, typeof readerKey, string | undefined>({
    queryKey: readerKey,
    initialPageParam: undefined as string | undefined,
    refetchOnMount: 'always',
    queryFn: ({ pageParam }) => {
      const query = {
        q: debouncedSearch,
        state,
        sourceId: sourceID || undefined,
        limit: pageSize,
        cursor: pageParam,
      }
      return sourceID
        ? api.listSourceEntries(sourceID, query)
        : api.listStories(query)
    },
    getNextPageParam: (lastPage) => lastPage.next_cursor || undefined,
  })
  const serverPages = readerQuery.data?.pages ?? []
  const lastPage = serverPages[serverPages.length - 1]
  const totalCount = lastPage
    ? ('entries' in lastPage ? lastPage.total_entries : lastPage.total_stories)
    : 0
  const loading = readerQuery.isPending
  const loadingMore = readerQuery.isFetchingNextPage
  const hasMore = Boolean(readerQuery.hasNextPage)
  const error = readerQuery.error instanceof Error ? readerQuery.error.message : ''

  function clearReaderNoticeTimer() {
    if (readerNoticeTimeout.current !== undefined) {
      window.clearTimeout(readerNoticeTimeout.current)
      readerNoticeTimeout.current = undefined
    }
    readerNoticeVersion.current += 1
  }

  function clearReaderNotice() {
    clearReaderNoticeTimer()
    setReaderNotice('')
  }

  function showReaderNotice(message: string, duration?: number) {
    clearReaderNoticeTimer()
    setReaderNotice(message)
    if (duration === undefined) return
    const version = readerNoticeVersion.current
    readerNoticeTimeout.current = window.setTimeout(() => {
      if (readerNoticeVersion.current !== version) return
      readerNoticeTimeout.current = undefined
      setReaderNotice('')
    }, duration)
  }

  useEffect(() => () => {
    if (readerNoticeTimeout.current !== undefined) window.clearTimeout(readerNoticeTimeout.current)
  }, [])

  useEffect(() => {
    const timeout = window.setTimeout(() => setDebouncedSearch(search.trim()), 250)
    return () => window.clearTimeout(timeout)
  }, [search])

  useEffect(() => {
    setMobileSearchOpen(false)
  }, [sourceID, view])

  useEffect(() => {
    setPendingNewCount(0)
    setHighlightedEntryIDs(new Set())
    knownStoryIDs.current = new Set()
    pendingStoryIDs.current = new Set()
    hasRenderedServerData.current = false
    setEntries([])
    setStoriesByEntry({})
    setExpandedEntryId(null)
    clearReaderNotice()
    entryStreamElement.current?.scrollTo({ top: 0 })
  }, [debouncedSearch, sourceID, view])

  useEffect(() => {
    if (!readerQuery.data) return
    const stories = readerQuery.data.pages.flatMap((page) => (
      'entries' in page
        ? page.entries.map(readerStoryFromSourceEntry)
        : page.stories.map(readerStoryFromStory)
    ))
    const items = stories.map((item) => projectReaderEntry(item.representative, item))
    const storyIDs = new Set(stories.map((item) => item.id))
    const added = hasRenderedServerData.current
      ? stories.filter((item) => !knownStoryIDs.current.has(item.id)).map((item) => item.representative.id)
      : []
    hasRenderedServerData.current = true
    knownStoryIDs.current = storyIDs
    if (added.length > 0) {
      setHighlightedEntryIDs((current) => new Set([...current, ...added]))
    }
    setEntries(orderReaderEntries(items))
    setStoriesByEntry(Object.fromEntries(stories.map((item) => [item.representative.id, item])))
    setExpandedEntryId((current) => current && items.some((item) => item.id === current) ? current : null)
  }, [readerQuery.data])

  async function loadMore() {
    try {
      await readerQuery.fetchNextPage()
    } catch (cause) {
      showReaderNotice(cause instanceof Error ? cause.message : '加载更多文章失败')
    }
  }

  function canAutoRefresh() {
    const stream = entryStreamElement.current
    return document.visibilityState === 'visible' &&
      expandedEntryId === null &&
      (stream === null || stream.scrollTop <= 80)
  }

  async function probeForNewStories() {
    const query = {
      q: debouncedSearch,
      state,
      sourceId: sourceID || undefined,
      limit: pageSize,
    }
    const probeKey = [...readerKey, 'probe'] as const
    const page = await queryClient.fetchQuery<ReaderPage>({
      queryKey: probeKey,
      queryFn: () => sourceID ? api.listSourceEntries(sourceID, query) : api.listStories(query),
      staleTime: 0,
    })
    const stories = 'entries' in page
      ? page.entries.map(readerStoryFromSourceEntry)
      : page.stories.map(readerStoryFromStory)
    pendingStoryIDs.current = new Set(
      stories
        .filter((story) => !knownStoryIDs.current.has(story.id))
        .map((story) => story.id),
    )
    setPendingNewCount(pendingStoryIDs.current.size)
  }

  useEffect(() => {
    if (!realtimeSignal || realtimeSignal.kind === 'reconnected' && realtimeConnectionState === 'connecting') return
    if (sourceID && realtimeSignal.sourceId && sourceID !== realtimeSignal.sourceId) return
    const reconcile = async () => {
      try {
        if (canAutoRefresh()) {
          pendingStoryIDs.current = new Set()
          setPendingNewCount(0)
          await readerQuery.refetch()
          return
        }
        await probeForNewStories()
      } catch {
        // SSE is an enhancement. A later event, visibility restoration, or
        // reconnect will try the HTTP reconciliation again.
      }
    }
    void reconcile()
  }, [realtimeSignal?.id])

  async function acceptNewContent() {
    pendingStoryIDs.current = new Set()
    setPendingNewCount(0)
    try {
      await readerQuery.refetch()
      entryStreamElement.current?.scrollTo({ top: 0, behavior: 'smooth' })
    } catch {
      showReaderNotice('加载新内容失败，请稍后重试')
    }
  }

  // Per-story mutations run inside StoryListItem and report back here so the
  // reader list (keyed by representative entry id) stays in sync.
  function applyStoryUpdate(updatedStory: ReaderStory) {
    setStoriesByEntry((current) => Object.fromEntries(
      Object.entries(current).map(([entryID, story]) => [
        entryID,
        story.id === updatedStory.id ? updatedStory : story,
      ]),
    ))
    setEntries((current) => current.map((candidate) => {
      const owner = storiesByEntry[candidate.id]
      return owner?.id === updatedStory.id ? projectReaderEntry(candidate, updatedStory) : candidate
    }))
  }

  function handleStoryChanged(change: StoryListItemChange) {
    if (change.kind === 'patched' || change.kind === 'representative' || change.kind === 'split') {
      applyStoryUpdate(change.story)
      if (change.kind === 'patched' && change.patch.read !== undefined) {
        void refreshSources()
      }
      if (change.kind === 'representative') {
        showReaderNotice('已更新默认来源')
      }
    }
  }

  function handleStoryRemoved(_storyId: string, entryId: string) {
    setEntries((current) => current.filter((candidate) => candidate.id !== entryId))
    setStoriesByEntry((current) => {
      const next = { ...current }
      delete next[entryId]
      return next
    })
    setExpandedEntryId((current) => current === entryId ? null : current)
  }

  async function markAllRead() {
    setMarkingAllRead(true)
    clearReaderNotice()
    try {
      const result = await api.markStoriesRead({ sourceId: sourceID || undefined })
      const readAt = new Date().toISOString()
      setEntries((current) => current.map((item) => item.read_at ? item : { ...item, read_at: readAt }))
      setStoriesByEntry((current) => Object.fromEntries(
        Object.entries(current).map(([entryID, story]) => [
          entryID,
          story.read_at ? story : { ...story, read_at: readAt },
        ]),
      ))
      void queryClient.invalidateQueries({ queryKey: ['story'] })
      showReaderNotice(
        result.updated_count > 0 ? `已将 ${result.updated_count} 篇文章标记为已读` : '没有未读文章',
        3500,
      )
      void refreshSources()
    } catch (cause) {
      showReaderNotice(cause instanceof Error ? cause.message : '全部标记为已读失败')
    } finally {
      setMarkingAllRead(false)
    }
  }

  const title = sourceName || (view === 'starred' ? '收藏' : view === 'later' ? '稍后阅读' : '全部文章')
  const sourceNames = Object.fromEntries(sources.map((source) => [source.id, source.name]))
  const orderedEntries = entries
  const mergeCandidates = orderedEntries
    .map((candidate) => storiesByEntry[candidate.id])
    .filter((story): story is ReaderStory => Boolean(story))
    .map((story) => ({
      storyId: story.id,
      title: story.display_title || story.representative.source_title || '无标题',
      displayTitle: story.display_title,
      note: story.note,
    }))
  return (
    <StreamShell
      title={title}
      count={!mobile ? (loading ? '正在更新…' : `${totalCount} 篇`) : undefined}
      mobile={mobile}
      mobileMenuButtonRef={mobileMenuButtonRef}
      mobileNavigationOpen={mobileNavigationOpen}
      onOpenMobileNavigation={onOpenMobileNavigation}
      actions={<>
          <Button
            variant="secondary"
            size="sm"
            className={cn(
              'h-9 shrink-0 border-border bg-background text-foreground shadow-sm',
              mobile && 'max-md:size-9 max-md:px-0',
            )}
            disabled={markingAllRead}
            aria-label={sourceName ? `将 ${sourceName} 全部标记为已读` : '将全部文章标记为已读'}
            onClick={() => void markAllRead()}
          >
            <CheckCheck className="size-4" aria-hidden="true" />
            <span className={mobile ? 'sr-only' : 'max-md:hidden'}>{markingAllRead ? '正在标记…' : '全部标记为已读'}</span>
          </Button>
          {mobile ? (
            mobileSearchOpen ? (
              <div className="flex items-center gap-0.5">
                <div className="relative">
                  <Search className="pointer-events-none absolute left-2 top-1/2 z-[1] size-4 -translate-y-1/2 text-muted-foreground" aria-hidden="true" />
                  <Input
                    autoFocus
                    className="h-9 w-[min(42vw,11rem)] border-border bg-background pl-8 pr-2 text-sm text-foreground shadow-sm focus-visible:ring-2 focus-visible:ring-ring/20"
                    value={search}
                    onChange={(event) => setSearch(event.target.value)}
                    onKeyDown={(event) => {
                      if (event.key === 'Escape') setMobileSearchOpen(false)
                    }}
                    aria-label="搜索文章"
                    placeholder="搜索文章"
                  />
                </div>
                <Button
                  unstyled
                  className="grid size-9 cursor-pointer place-items-center rounded-md border-0 bg-transparent text-muted-foreground hover:bg-accent hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
                  aria-label="关闭搜索"
                  onClick={() => setMobileSearchOpen(false)}
                >
                  <X className="size-4" aria-hidden="true" />
                </Button>
              </div>
            ) : (
              <Button
                unstyled
                className="grid size-9 cursor-pointer place-items-center rounded-md border-0 bg-transparent text-muted-foreground hover:bg-accent hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
                aria-label="打开搜索"
                onClick={() => setMobileSearchOpen(true)}
              >
                <Search className="size-[18px]" aria-hidden="true" />
              </Button>
            )
          ) : (
            <label className="relative w-[min(390px,55%)]">
              <span className="sr-only">搜索文章</span>
              <Search className="pointer-events-none absolute left-2.5 top-1/2 z-[1] size-[22px] -translate-y-1/2 text-muted-foreground" strokeWidth={2.25} aria-hidden="true" />
              <Input
                className="h-9 w-full border-border bg-background pl-10 pr-3 text-sm text-foreground shadow-sm focus-visible:ring-2 focus-visible:ring-ring/20"
                value={search}
                onChange={(event) => setSearch(event.target.value)}
                placeholder="搜索文章"
              />
            </label>
          )}
      </>}
      banners={<>
        {pendingNewCount > 0 && (
          <div className="border-b bg-amber-50 px-4 py-2 text-sm text-amber-950" role="status" aria-live="polite">
            <Button
              unstyled
              className="w-full cursor-pointer text-left font-medium hover:underline focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
              aria-label={`加载 ${pendingNewCount} 条新内容`}
              onClick={() => void acceptNewContent()}
            >
              有 {pendingNewCount} 条新内容
            </Button>
          </div>
        )}
        {realtimeConnectionState === 'degraded' && (
          <div className="border-b bg-muted/60 px-4 py-1.5 text-xs text-muted-foreground" aria-live="polite">
            实时更新已断开，正在重连…
          </div>
        )}
      </>}
      floating={readerNotice ? <div className="absolute right-4 top-[72px] z-[5] rounded-lg border border-[#dce1e5] bg-white/95 px-3 py-2 text-sm text-[#55606b] shadow-[0_8px_24px_rgba(45,51,60,.1)]" role="status">{readerNotice}</div> : undefined}
      contentRef={entryStreamElement}
    >
          {loading && <p className="p-8 text-center text-sm text-muted-foreground">正在加载文章…</p>}
          {error && <p className="p-8 text-center text-sm text-muted-foreground text-destructive">{error}</p>}
          {!loading && !error && entries.length === 0 && <p className="p-8 text-center text-sm text-muted-foreground">这里还没有文章。</p>}
          {orderedEntries.map((item) => {
            const story = storiesByEntry[item.id]
            return (
              <StoryListItem
                key={item.id}
                storyId={story?.id ?? ''}
                rowEntryId={item.id}
                expanded={expandedEntryId === item.id}
                onExpandedChange={(open) => setExpandedEntryId(open ? item.id : null)}
                scrollContainerRef={entryStreamElement}
                sources={sources}
                mergeCandidates={mergeCandidates}
                initialStory={story ? {
                  id: story.id,
                  display_title: story.display_title,
                  note: story.note,
                  tags: story.tags,
                  representative: item,
                  entries: [item],
                  entry_count: story.entry_count,
                  source_count: story.source_count,
                  read_at: story.read_at,
                  starred_at: story.starred_at,
                  hidden_at: story.hidden_at,
                  later_at: story.later_at,
                } : undefined}
                row={{
                  title: item.display_title || item.source_title || '无标题',
                  sourceLabel: story && story.entry_count > 1
                    ? (story.source_count > 1 ? `${story.source_count} 个来源` : `${story.entry_count} 个版本`)
                    : (sourceNames[item.source_id] || item.author || '未知来源'),
                  timestamp: item.discovered_at,
                  read: Boolean(item.read_at),
                  highlightQuery: debouncedSearch,
                  emphasized: highlightedEntryIDs.has(item.id),
                }}
                onChanged={handleStoryChanged}
                onRemoved={handleStoryRemoved}
                onError={showReaderNotice}
              />
            )
          })}
          {!loading && !error && hasMore && (
            <div className="flex min-h-[80dvh] justify-center border-t px-4 py-5">
              <Button variant="secondary" disabled={loadingMore} onClick={() => void loadMore()}>
                {loadingMore ? '正在加载…' : '加载更多'}
              </Button>
            </div>
          )}
          {!loading && !error && !hasMore && entries.length > 0 && (
            <div className="min-h-[80dvh]" aria-hidden="true" />
          )}
    </StreamShell>
  )
}

function CreateSourceDialog({
  folders,
  onClose,
  onCreate,
}: {
  folders: Folder[]
  onClose: () => void
  onCreate: (input: CreateSourceInput, selectedFolderIDs: Set<string>) => Promise<void>
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
  const [selectedFolderIDs, setSelectedFolderIDs] = useState<Set<string>>(() => new Set())

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
      await onCreate(input(), new Set(selectedFolderIDs))
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
          {folders.length > 0 && (
            <FolderPicker
              folders={folders}
              selectedFolderIDs={selectedFolderIDs}
              onChange={setSelectedFolderIDs}
              disabled={submitting || testing}
              legend="添加到文件夹（可选）"
            />
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
    ai: Sparkles,
    chat: MessageSquareText,
    settings: Settings,
  }
  const Icon = icons[name] ?? Inbox
  return <Icon className="size-4 shrink-0" strokeWidth={1.8} aria-hidden="true" />
}
