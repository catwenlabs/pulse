import { FormEvent, KeyboardEvent as ReactKeyboardEvent, useEffect, useRef, useState } from 'react'

import * as api from './api'
import type { AnnotationInput, CreateSourceInput, Entry, EntryPatch, Folder, PreviewResult, Source, SourceHealth, SourceKind } from './api'
import './styles.css'

type Notice = { tone: 'success' | 'error'; message: string }
type View = 'sources' | 'inbox' | 'starred' | 'later' | 'annotations'
type SaveRequest = { url: string; title: string }

export function App() {
  const [sources, setSources] = useState<Source[]>([])
  const [folders, setFolders] = useState<Folder[]>([])
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
  const isMobile = useMediaQuery('(max-width: 760px)')
  const mobileMenuButtonRef = useRef<HTMLButtonElement>(null)
  const mobileDrawerCloseRef = useRef<HTMLButtonElement>(null)
  const mobileDrawerRef = useRef<HTMLElement>(null)
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

  useEffect(() => {
    if (!isMobile || !mobileNavigationOpen) return
    mobileDrawerCloseRef.current?.focus()
    function closeOnEscape(event: KeyboardEvent) {
      if (event.key !== 'Escape') return
      closeMobileNavigation()
    }
    document.addEventListener('keydown', closeOnEscape)
    return () => document.removeEventListener('keydown', closeOnEscape)
  }, [isMobile, mobileNavigationOpen])

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

  function trapMobileNavigationFocus(event: React.KeyboardEvent<HTMLElement>) {
    if (!isMobile || !mobileNavigationOpen || event.key !== 'Tab') return
    const focusable = Array.from(mobileDrawerRef.current?.querySelectorAll<HTMLElement>(
      'a[href], button:not([disabled]), input:not([disabled]), select:not([disabled]), textarea:not([disabled]), [tabindex]:not([tabindex="-1"])',
    ) ?? [])
    if (focusable.length === 0) return
    const first = focusable[0]
    const last = focusable[focusable.length - 1]
    if (event.shiftKey && document.activeElement === first) {
      event.preventDefault()
      last.focus()
    } else if (!event.shiftKey && document.activeElement === last) {
      event.preventDefault()
      first.focus()
    }
  }

  const activeSourceName = sources.find((source) => source.id === selectedSourceID)?.name
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
    <div className={`app-shell ${mobileNavigationOpen ? 'mobile-navigation-open' : ''}`}>
      <aside
        className="sidebar"
        id="mobile-navigation"
        ref={mobileDrawerRef}
        role="navigation"
        aria-label="移动导航抽屉"
        aria-hidden={isMobile ? !mobileNavigationOpen : undefined}
        inert={isMobile && !mobileNavigationOpen ? true : undefined}
        onKeyDown={trapMobileNavigationFocus}
      >
        <div className="sidebar-brand-row">
          <a className="brand" href="/" aria-label="Pulse 首页" onClick={() => showStream('inbox')}>
            <span className="brand-mark" aria-hidden="true">P</span>
            <span>Pulse</span>
          </a>
          <div className="sidebar-header-actions">
            <button className="sidebar-add" aria-label="添加信息源" onClick={() => {
              closeMobileNavigation(false)
              setShowCreate(true)
            }}>
              <span aria-hidden="true">＋</span><span className="sr-only">添加信息源</span>
            </button>
            {isMobile && (
              <button
                className="sidebar-dismiss"
                ref={mobileDrawerCloseRef}
                aria-label="关闭导航"
                onClick={() => closeMobileNavigation()}
              >
                <span aria-hidden="true">×</span>
              </button>
            )}
          </div>
        </div>

        <nav className="main-nav" aria-label="主导航">
          <a className={activeView === 'inbox' && !selectedSourceID ? 'active' : ''} href="#inbox" onClick={() => showStream('inbox')}><NavIcon name="inbox" />全部文章</a>
          <a className={activeView === 'starred' ? 'active' : ''} href="#starred" onClick={() => showStream('starred')}><NavIcon name="star" />收藏</a>
          <a className={activeView === 'later' ? 'active' : ''} href="#later" onClick={() => showStream('later')}><NavIcon name="clock" />稍后阅读</a>
          <a className={activeView === 'annotations' ? 'active' : ''} href="#annotations" onClick={() => showStream('annotations')}><NavIcon name="book" />阅读笔记</a>
        </nav>

        <section className="sidebar-section folder-section" aria-labelledby="folder-label">
          <div className="sidebar-section-heading">
            <p className="section-label" id="folder-label">文件夹</p>
            <span>{folders.length}</span>
          </div>
          {folders.length === 0 && <p className="empty-folder">尚未创建文件夹</p>}
          <div className="folder-tree">
            {folders.map((folder) => (
              <div className="folder-row" key={folder.id}>
                <span aria-hidden="true">▾</span>
                <strong>{folder.name}</strong>
                <span>{folder.source_count}</span>
              </div>
            ))}
          </div>
        </section>

        <section className="sidebar-section subscription-section" aria-labelledby="subscription-label">
          <div className="sidebar-section-heading">
            <p className="section-label" id="subscription-label">订阅源</p>
            <span>{sources.length}</span>
          </div>
          {loading && <p className="sidebar-state">正在同步信息源…</p>}
          {!loading && loadError && <button className="sidebar-state error-state" onClick={() => void load()}>重试加载</button>}
          <div className="subscription-list">
            {sources.map((source) => (
              <button
                className={selectedSourceID === source.id && activeView === 'inbox' ? 'active' : ''}
                key={source.id}
                onClick={() => showStream('inbox', source.id)}
              >
                <span className={`subscription-dot ${source.enabled ? 'enabled' : ''}`} />
                <span>{source.name}</span>
              </button>
            ))}
          </div>
        </section>

        <div className="sidebar-footer">
          <button ref={bookmarkletButtonRef} onClick={() => {
            closeMobileNavigation(false)
            setShowBookmarklet(true)
          }}>
            <NavIcon name="bookmark" />安装保存书签
          </button>
          <button className={activeView === 'sources' ? 'active' : ''} onClick={() => {
            setActiveView('sources')
            closeMobileNavigation()
          }}>
            <NavIcon name="source" />管理信息源
          </button>
          <span><span className="status-dot" />本地服务已连接</span>
        </div>
      </aside>

      <main
        className={`main-content ${activeView === 'sources' ? 'source-main' : 'reader-main'}`}
        inert={isMobile && mobileNavigationOpen ? true : undefined}
      >
        {isMobile && (
          <header className="mobile-app-bar">
            <button
              className="mobile-menu-button"
              ref={mobileMenuButtonRef}
              aria-label="打开导航"
              aria-controls="mobile-navigation"
              aria-expanded={mobileNavigationOpen}
              onClick={() => setMobileNavigationOpen(true)}
            >
              <span aria-hidden="true" />
              <span aria-hidden="true" />
              <span aria-hidden="true" />
            </button>
            <h1>{mobileTitle}</h1>
          </header>
        )}
        {notice && (
          <div className={`notice ${notice.tone}`} role="status">
            {notice.message}
            <button aria-label="关闭提示" onClick={() => setNotice(null)}>×</button>
          </div>
        )}
        {activeView === 'sources' ? (
          <>
        <header className="page-header">
          <div>
            <p className="eyebrow">LIBRARY</p>
            <h1>信息源</h1>
            <p className="page-description">管理 Pulse 持续关注的 RSS、API、网页与推送来源。</p>
          </div>
          <div className="header-actions">
            <a className="button secondary" href="/api/v1/opml/export">导出 OPML</a>
            <button className="button primary" onClick={() => setShowCreate(true)}>
              <span aria-hidden="true">＋</span> 添加信息源
            </button>
          </div>
        </header>

        <section className="source-panel" aria-labelledby="source-heading">
          <div className="panel-heading">
            <div>
              <h2 id="source-heading">全部信息源</h2>
              <span>{sources.length} 个来源</span>
            </div>
            <button className="icon-button" aria-label="重新载入" onClick={() => void load()}>↻</button>
          </div>

          {loading && <div className="state-message">正在同步信息源…</div>}
          {!loading && loadError && (
            <div className="state-message error-state">
              <strong>暂时无法加载</strong>
              <span>{loadError}</span>
              <button className="text-button" onClick={() => void load()}>重试</button>
            </div>
          )}
          {!loading && !loadError && sources.length === 0 && (
            <div className="state-message">
              <strong>还没有信息源</strong>
              <span>添加一个 RSS Feed，开始构建你的阅读中枢。</span>
            </div>
          )}
          {!loading && !loadError && sources.length > 0 && (
            <div className="source-list">
              {sources.map((source) => (
                <article className="source-row" key={source.id}>
                  <div className="source-avatar" aria-hidden="true">{source.name.slice(0, 1).toUpperCase()}</div>
                  <div className="source-copy">
                    <div className="source-title-line">
                      <h3>{source.name}</h3>
                      <span className="kind-badge">{source.kind.toUpperCase()}</span>
                    </div>
                    <p>{source.locator}</p>
                  </div>
                  <div className="source-health">
                    <span className={source.enabled ? 'health active' : 'health'} />
                    {source.enabled ? '已启用' : '已暂停'}
                  </div>
                  {health[source.id] && (
                    <div className="source-diagnostics" aria-label={`${source.name} 抓取诊断`}>
                      <span>状态 {health[source.id].status}</span>
                      {health[source.id].last_requested_at && <span>最近 {formatTime(health[source.id].last_requested_at!)}</span>}
                      <span>{health[source.id].candidate_count} 候选 · {health[source.id].new_count} 新增 · {health[source.id].updated_count} 更新</span>
                      <span>{health[source.id].duration_milliseconds} ms · 连续失败 {health[source.id].consecutive_failures}</span>
                      {health[source.id].next_scheduled_at && <span>下次 {formatTime(health[source.id].next_scheduled_at!)}</span>}
                      {health[source.id].last_error && <em>{health[source.id].last_error}</em>}
                    </div>
                  )}
                  <div className="source-actions">
                    <button
                      className="toggle-button"
                      aria-label={`${source.enabled ? '暂停' : '恢复'} ${source.name}`}
                      onClick={() => void handleToggle(source)}
                    >
                      {source.enabled ? '暂停' : '恢复'}
                    </button>
                    <button
                      className="refresh-button"
                      aria-label={`刷新 ${source.name}`}
                      disabled={!source.enabled}
                      onClick={() => void handleRun(source)}
                    >
                      ↻
                    </button>
                    <button
                      className="delete-source-button"
                      aria-label={`删除 ${source.name}`}
                      onClick={() => setSourceToDelete(source)}
                    >
                      删除
                    </button>
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

      {isMobile && mobileNavigationOpen && (
        <div
          className="mobile-drawer-backdrop"
          aria-hidden="true"
          onMouseDown={() => closeMobileNavigation()}
        />
      )}
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
    <div className="annotations-page">
      <header className="page-header annotation-header">
        <div>
          <p className="eyebrow">READING NOTES</p>
          <h1>阅读笔记</h1>
          <p className="page-description">集中保存 Apple Books、Kindle 和其他阅读器中的高亮与批注。</p>
        </div>
        <button className="button primary" onClick={() => setShowImport((current) => !current)}>
          {showImport ? '收起导入' : '导入批注'}
        </button>
      </header>

      {showImport && (
        <section className="annotation-import" aria-labelledby="annotation-import-title">
          <div>
            <p className="eyebrow">NEW ANNOTATION</p>
            <h2 id="annotation-import-title">添加一条阅读批注</h2>
            <p>第一版支持结构化手工导入；Apple Books 与 Kindle 批量格式将在取得真实导出样本后接入。</p>
          </div>
          <form onSubmit={(event) => void submit(event)}>
            <div className="annotation-form-grid">
              <label>
                <span>来源平台</span>
                <select value={form.provider} onChange={(event) => setForm({ ...form, provider: event.target.value })}>
                  <option value="apple-books">Apple Books</option>
                  <option value="kindle">Kindle</option>
                  <option value="other">其他</option>
                </select>
              </label>
              <label>
                <span>书名</span>
                <input required value={form.book_title} onChange={(event) => setForm({ ...form, book_title: event.target.value })} />
              </label>
              <label>
                <span>作者</span>
                <input value={form.book_author} onChange={(event) => setForm({ ...form, book_author: event.target.value })} />
              </label>
              <label>
                <span>章节</span>
                <input value={form.chapter} onChange={(event) => setForm({ ...form, chapter: event.target.value })} />
              </label>
              <label>
                <span>位置</span>
                <input value={form.location} onChange={(event) => setForm({ ...form, location: event.target.value })} />
              </label>
              <label>
                <span>高亮颜色</span>
                <select value={form.highlight_color} onChange={(event) => setForm({ ...form, highlight_color: event.target.value })}>
                  <option value="yellow">黄色</option>
                  <option value="green">绿色</option>
                  <option value="blue">蓝色</option>
                  <option value="pink">粉色</option>
                  <option value="">未指定</option>
                </select>
              </label>
            </div>
            <label>
              <span>高亮原文</span>
              <textarea required value={form.highlight} onChange={(event) => setForm({ ...form, highlight: event.target.value })} />
            </label>
            <label>
              <span>原始批注</span>
              <textarea value={form.note} onChange={(event) => setForm({ ...form, note: event.target.value })} />
            </label>
            {importMessage && <p className="annotation-import-message" role="status">{importMessage}</p>}
            <button className="button primary" type="submit" disabled={importing}>
              {importing ? '正在导入…' : '加入导入队列'}
            </button>
          </form>
        </section>
      )}

      {loading && <div className="reader-state">正在加载阅读笔记…</div>}
      {!loading && error && <div className="reader-state">{error}</div>}
      {!loading && !error && books.length === 0 && (
        <div className="annotation-empty">
          <strong>还没有阅读批注</strong>
          <span>导入第一条高亮后，Pulse 会按书籍自动整理。</span>
        </div>
      )}
      {!loading && !error && books.length > 0 && (
        <section className="annotation-books" aria-label="书籍批注">
          {books.map(({ detail, entries: bookEntries }) => (
            <article className="annotation-book" key={annotationBookKey(detail)}>
              <div className="annotation-book-heading">
                <div>
                  <span>{detail.provider === 'apple-books' ? 'APPLE BOOKS' : detail.provider.toUpperCase()}</span>
                  <h2>{detail.book_title}</h2>
                  {detail.book_author && <p>{detail.book_author}</p>}
                </div>
                <strong>{bookEntries.length} 条批注</strong>
              </div>
              <div className="annotation-highlights">
                {(expandedBooks.has(annotationBookKey(detail)) ? bookEntries : bookEntries.slice(0, 3)).map((item) => (
                  <blockquote key={item.id}>
                    <p>{item.summary}</p>
                    {item.annotation?.annotation_note && <footer>{item.annotation.annotation_note}</footer>}
                    <small>{[item.annotation?.chapter, item.annotation?.location].filter(Boolean).join(' · ')}</small>
                  </blockquote>
                ))}
                {bookEntries.length > 3 && (
                  <button
                    className="annotation-expand"
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
                  </button>
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
    <main className="save-page">
      <section className="save-card" aria-labelledby="save-page-title">
        <div className="save-brand"><span className="brand-mark" aria-hidden="true">P</span>Pulse</div>
        <p className="eyebrow">READ LATER</p>
        <h1 id="save-page-title">保存到 Pulse</h1>
        <p className="save-description">确认网页信息后，将它加入你的阅读列表。</p>
        <form onSubmit={(event) => void submit(event)}>
          <label>
            <span>网页地址</span>
            <input
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
            <input required maxLength={500} value={title} onChange={(event) => setTitle(event.target.value)} />
          </label>
          {manualSources.length > 0 && (
            <label>
              <span>保存到</span>
              <select value={sourceID} onChange={(event) => setSourceID(event.target.value)}>
                {manualSources.map((source) => (
                  <option key={source.id} value={source.id}>
                    {source.name}{source.enabled ? '' : '（已暂停，将自动恢复）'}
                  </option>
                ))}
              </select>
            </label>
          )}
          {loading && <p className="save-help">正在加载 Manual Source…</p>}
          {!loading && loadError && <p className="form-error" role="alert">{loadError}</p>}
          {!loading && !loadError && manualSources.length === 0 && (
            <div className="save-empty-source">
              <strong>还没有 Manual Source</strong>
              <span>Pulse 会在你确认后创建“网页收藏”，不会创建隐藏 Source。</span>
            </div>
          )}
          {error && <p className="form-error" role="alert">{error}</p>}
          {success && <p className="save-success" role="status">{success}</p>}
          <div className="dialog-actions">
            <button className="button secondary" type="button" onClick={onClose}>关闭</button>
            {manualSources.length > 0 ? (
              <button className="button primary" type="submit" disabled={saving || loading}>
                {saving ? '正在保存…' : '保存网页'}
              </button>
            ) : (
              <button
                className="button primary"
                type="button"
                disabled={saving || loading || Boolean(loadError)}
                onClick={() => void createSourceAndSave()}
              >
                {saving ? '正在创建…' : '创建“网页收藏”并保存'}
              </button>
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
  const dialogRef = useRef<HTMLElement>(null)
  const saveTarget = `${window.location.origin}${window.location.pathname}#save?`
  const bookmarklet = `javascript:(()=>{const p=new URLSearchParams({url:location.href,title:document.title});window.open(${JSON.stringify(saveTarget)}+p.toString(),'_blank','popup,width=520,height=680,noopener');void 0})()`

  useEffect(() => {
    return () => returnFocusElement?.focus()
  }, [returnFocusElement])

  function handleKeyDown(event: ReactKeyboardEvent<HTMLElement>) {
    if (event.key === 'Escape') {
      event.preventDefault()
      onClose()
      return
    }
    if (event.key !== 'Tab') return
    const focusable = Array.from(dialogRef.current?.querySelectorAll<HTMLElement>(
      'button:not([disabled]), textarea:not([disabled]), [href], [tabindex]:not([tabindex="-1"])',
    ) || [])
    if (focusable.length === 0) return
    const first = focusable[0]
    const last = focusable[focusable.length - 1]
    if (event.shiftKey && document.activeElement === first) {
      event.preventDefault()
      last.focus()
    } else if (!event.shiftKey && document.activeElement === last) {
      event.preventDefault()
      first.focus()
    }
  }

  return (
    <div className="dialog-backdrop" onMouseDown={(event) => event.target === event.currentTarget && onClose()}>
      <section
        ref={dialogRef}
        className="dialog bookmarklet-dialog"
        role="dialog"
        aria-modal="true"
        aria-labelledby="bookmarklet-title"
        onKeyDown={handleKeyDown}
      >
        <button className="dialog-close" aria-label="关闭" onClick={onClose}>×</button>
        <p className="eyebrow">BOOKMARKLET</p>
        <h2 id="bookmarklet-title">安装“保存到 Pulse”</h2>
        <p className="dialog-description">新建一个浏览器书签，把下面整段代码粘贴到书签的地址栏。</p>
        <label className="bookmarklet-code">
          <span>Bookmarklet 代码</span>
          <textarea autoFocus readOnly value={bookmarklet} onFocus={(event) => event.currentTarget.select()} />
        </label>
        <div className="bookmarklet-platforms">
          <section>
            <h3>Mac Chrome</h3>
            <ol className="bookmarklet-steps">
              <li>复制上面的完整代码。</li>
              <li>新建书签，名称填写“保存到 Pulse”。</li>
              <li>将代码粘贴到书签地址，然后在任意文章页点击它。</li>
            </ol>
          </section>
          <section>
            <h3>iPhone Chrome</h3>
            <ol className="bookmarklet-steps">
              <li>先把任意网页添加到 Chrome 书签。</li>
              <li>长按刚创建的书签，选择“编辑”，名称改为“保存到 Pulse”。</li>
              <li>把 URL 替换为上面的完整代码并保存。</li>
              <li>打开要收藏的网页，再从书签中点“保存到 Pulse”。</li>
            </ol>
          </section>
        </div>
        <div className="dialog-actions">
          <button className="button primary" onClick={onClose}>完成</button>
        </div>
      </section>
    </div>
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
    <div className="dialog-backdrop" onMouseDown={(event) => event.target === event.currentTarget && !deleting && onCancel()}>
      <section
        className="dialog delete-dialog"
        role="dialog"
        aria-modal="true"
        aria-labelledby="delete-source-title"
        tabIndex={-1}
        onKeyDown={(event) => {
          if (event.key === 'Escape' && !deleting) {
            onCancel()
          }
        }}
      >
        <p className="eyebrow">ARCHIVE SOURCE</p>
        <h2 id="delete-source-title">删除信息源？</h2>
        <p className="dialog-description">
          “{source.name}”将停止抓取并从订阅列表中移除。
        </p>
        <p className="delete-preservation">已经抓取的文章、收藏和笔记都会保留。</p>
        <div className="dialog-actions">
          <button className="button secondary" disabled={deleting} onClick={onCancel} autoFocus>取消删除</button>
          <button
            className="button danger"
            disabled={deleting}
            aria-label={`确认删除 ${source.name}`}
            onClick={onConfirm}
          >
            {deleting ? '正在删除…' : '确认删除'}
          </button>
        </div>
      </section>
    </div>
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
    <div className="reader-page">
      <header className="reader-header">
        <div className="reader-title" aria-hidden={mobile || undefined}>
          <h1>{title}</h1>
          <span className="reader-count">{loading ? '正在更新…' : `${entries.length} 篇`}</span>
        </div>
        <div className="reader-controls">
          <button
            className="mark-all-read"
            disabled={markingAllRead}
            aria-label={sourceName ? `将 ${sourceName} 全部标记为已读` : '将全部文章标记为已读'}
            onClick={() => void markAllRead()}
          >
            <span aria-hidden="true">✓✓</span>
            <span className="reader-action-label">{markingAllRead ? '正在标记…' : '全部标记为已读'}</span>
          </button>
          <label className="reader-search">
            <span className="sr-only">搜索文章</span>
            <input value={search} onChange={(event) => setSearch(event.target.value)} placeholder="搜索文章" />
          </label>
        </div>
      </header>
      {readerNotice && <div className="reader-notice" role="status">{readerNotice}</div>}
      <section className="entry-stream" aria-label="文章列表">
          {loading && <p className="reader-state">正在加载文章…</p>}
          {error && <p className="reader-state error-state">{error}</p>}
          {!loading && !error && entries.length === 0 && <p className="reader-state">这里还没有文章。</p>}
          {entries.map((item) => (
            <article
              className={`stream-entry ${item.read_at ? 'read' : ''} ${selected?.id === item.id ? 'expanded' : ''}`}
              key={item.id}
            >
              <button
                className="stream-entry-summary"
                aria-expanded={selected?.id === item.id}
                onClick={(event) => toggleEntry(item, event.currentTarget.closest('article')!)}
              >
                <span className="unread-dot" aria-hidden="true" />
                <span className="stream-source">{sourceNames[item.source_id] || item.author || '未知来源'}</span>
                <strong>{item.display_title || item.source_title || '无标题'}</strong>
                <span className="stream-summary">{htmlToText(item.summary || item.content_html || '') || '没有摘要'}</span>
                <time dateTime={item.discovered_at}>{compactTime(item.discovered_at)}</time>
                <span className="expand-chevron" aria-hidden="true">⌄</span>
              </button>
              {selected?.id === item.id && (
                <div
                  className="stream-entry-detail"
                  ref={(element) => {
                    if (!element || readingAreaToScroll.current !== item.id) return
                    readingAreaToScroll.current = ''
                    window.requestAnimationFrame(() => {
                      element.scrollIntoView({ behavior: 'smooth', block: 'start' })
                    })
                  }}
                >
                  <div className="entry-reading-column">
                    <div className="entry-detail-bar">
                      <span>{selected.author || sourceNames[selected.source_id] || '未知来源'}</span>
                      <div className="entry-detail-actions">
                        {selected.canonical_url && (
                          <a href={selected.canonical_url} target="_blank" rel="noreferrer">查看原文 ↗</a>
                        )}
                        <div className="entry-action-menu">
                          <button
                            className="entry-more-button"
                            aria-label="更多操作"
                            aria-expanded={actionMenuOpen}
                            onClick={() => setActionMenuOpen((open) => !open)}
                          >
                            •••
                          </button>
                          {actionMenuOpen && (
                            <div className="entry-action-popover">
                              <button aria-label="标记未读" onClick={() => void patch(selected, { read: false })}>标记未读</button>
                              <button aria-label={selected.starred_at ? '取消收藏' : '收藏文章'} onClick={() => void patch(selected, { starred: !selected.starred_at })}>
                                {selected.starred_at ? '取消收藏' : '收藏文章'}
                              </button>
                              <button aria-label={selected.later_at ? '移出稍后阅读' : '稍后阅读'} onClick={() => void patch(selected, { later: !selected.later_at })}>
                                {selected.later_at ? '移出稍后阅读' : '稍后阅读'}
                              </button>
                              <button onClick={() => {
                                setNotesOpen((open) => !open)
                                setActionMenuOpen(false)
                              }}>
                                编辑标题与笔记
                              </button>
                            </div>
                          )}
                        </div>
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
                    {notesOpen && <div className="entry-notes">
                      <label>
                        <span>显示标题</span>
                        <input
                          value={selected.display_title}
                          onChange={(event) => setSelected({ ...selected, display_title: event.target.value })}
                          placeholder={selected.source_title}
                        />
                      </label>
                      <label>
                        <span>笔记</span>
                        <textarea
                          value={selected.note}
                          onChange={(event) => setSelected({ ...selected, note: event.target.value })}
                          placeholder="记录你的想法…"
                        />
                      </label>
                      <button className="button secondary" onClick={() => void patch(selected, {
                        display_title: selected.display_title,
                        note: selected.note,
                      })}>
                        保存标题与笔记
                      </button>
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

function htmlToText(value: string): string {
  return value
    .replace(/<script[\s\S]*?<\/script>/gi, '')
    .replace(/<style[\s\S]*?<\/style>/gi, '')
    .replace(/<[^>]+>/g, ' ')
    .replace(/\s+/g, ' ')
    .trim()
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
    <div className="dialog-backdrop" onMouseDown={(event) => event.target === event.currentTarget && onClose()}>
      <section className="dialog" role="dialog" aria-modal="true" aria-labelledby="dialog-title">
        <button className="dialog-close" aria-label="关闭" onClick={onClose}>×</button>
        <p className="eyebrow">NEW SOURCE</p>
        <h2 id="dialog-title">添加信息源</h2>
        <div className="wizard-steps" aria-label="配置步骤">
          <span className={!preview ? 'current' : 'complete'}>1 配置</span>
          <span className={preview ? 'current' : ''}>2 预览并保存</span>
        </div>
        <p className="dialog-description">先临时抓取并检查文章身份；确认前不会写入数据库。</p>

        <form onSubmit={(event) => void testSource(event)}>
          <label>
            <span>来源类型</span>
            <select
              value={kind}
              onChange={(event) => {
                setKind(event.target.value as SourceKind)
                setPreview(null)
              }}
            >
              <option value="rss">RSS / Atom / JSON Feed</option>
              <option value="json-api">JSON API</option>
              <option value="html">静态 HTML</option>
            </select>
          </label>
          <label>
            <span>名称</span>
            <input
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
            <input
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
            <div className="mapping-fields">
              <label>
                <span>列表路径</span>
                <input required value={itemsPath} onChange={(event) => setItemsPath(event.target.value)} placeholder="data.items" />
              </label>
              <div className="field-grid">
                <label>
                  <span>ID 字段</span>
                  <input required value={idField} onChange={(event) => setIDField(event.target.value)} placeholder="id" />
                </label>
                <label>
                  <span>标题字段</span>
                  <input value={titleField} onChange={(event) => setTitleField(event.target.value)} placeholder="title" />
                </label>
                <label>
                  <span>URL 字段</span>
                  <input value={urlField} onChange={(event) => setURLField(event.target.value)} placeholder="url" />
                </label>
              </div>
              <label>
                <span>分页方式</span>
                <select value={paginationMode} onChange={(event) => setPaginationMode(event.target.value)}>
                  <option value="none">不分页</option>
                  <option value="page">页码参数</option>
                  <option value="next">下一页 URL</option>
                  <option value="cursor">游标</option>
                </select>
              </label>
              {(paginationMode === 'next' || paginationMode === 'cursor') && (
                <label>
                  <span>{paginationMode === 'next' ? '下一页路径' : '游标路径'}</span>
                  <input
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
                  <input
                    value={paginationParam}
                    onChange={(event) => setPaginationParam(event.target.value)}
                    placeholder={paginationMode === 'page' ? 'page' : 'cursor'}
                  />
                </label>
              )}
            </div>
          )}
          {kind === 'html' && (
            <div className="mapping-fields">
              <label>
                <span>页面模式</span>
                <select value={htmlMode} onChange={(event) => setHTMLMode(event.target.value)}>
                  <option value="collection">列表页面</option>
                  <option value="single">单文档</option>
                </select>
              </label>
              {htmlMode === 'collection' && (
                <label>
                  <span>条目选择器</span>
                  <input required value={itemSelector} onChange={(event) => setItemSelector(event.target.value)} placeholder="article.card" />
                </label>
              )}
              <div className="field-grid">
                <label>
                  <span>标题选择器</span>
                  <input required value={titleSelector} onChange={(event) => setTitleSelector(event.target.value)} placeholder="h2.title" />
                </label>
                <label>
                  <span>链接选择器</span>
                  <input required value={linkSelector} onChange={(event) => setLinkSelector(event.target.value)} placeholder="a.permalink" />
                </label>
                <label>
                  <span>正文选择器</span>
                  <input value={contentSelector} onChange={(event) => setContentSelector(event.target.value)} placeholder=".content" />
                </label>
              </div>
              <div className="selector-legend">
                <span>条目 <code>{htmlMode === 'collection' ? itemSelector : 'document'}</code></span>
                <span>标题 <code>{titleSelector}</code></span>
                <span>链接 <code>{linkSelector}@href</code></span>
              </div>
            </div>
          )}
          {error && <p className="form-error" role="alert">{error}</p>}
          {preview && (
            <div className="preview-panel">
              <div className="preview-summary">
                <strong>连接成功</strong>
                <span>发现 {preview.candidates.length} 条预览内容</span>
              </div>
              {kind === 'html' && (
                <div className="visual-preview-label">
                  选择器提取预览
                </div>
              )}
              <div className="preview-list">
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
          <div className="dialog-actions">
            <button className="button secondary" type="button" onClick={onClose}>取消</button>
            {!preview && (
              <button className="button primary" type="submit" disabled={testing}>
                {testing ? '正在测试…' : '测试并预览'}
              </button>
            )}
            {preview && (
              <>
                <button className="button secondary" type="button" onClick={() => setPreview(null)}>
                  返回修改
                </button>
                <button className="button primary" type="button" disabled={submitting} onClick={() => void save()}>
                  {submitting ? '正在保存…' : '保存并启用'}
                </button>
              </>
            )}
          </div>
        </form>
      </section>
    </div>
  )
}

function NavIcon({ name }: { name: string }) {
  const paths: Record<string, string> = {
    inbox: 'M4 5h16v12H4z M4 13h4l2 3h4l2-3h4',
    source: 'M5 6a13 13 0 0 1 13 13 M5 11a8 8 0 0 1 8 8 M6 18h.01',
    star: 'm12 3 2.7 5.5 6.1.9-4.4 4.3 1 6.1-5.4-2.8-5.4 2.8 1-6.1-4.4-4.3 6.1-.9z',
    clock: 'M12 21a9 9 0 1 0 0-18 9 9 0 0 0 0 18z M12 7v5l3 2',
    bookmark: 'M6 4h12v17l-6-4-6 4z',
    book: 'M4 5.5A3.5 3.5 0 0 1 7.5 2H12v18H7.5A3.5 3.5 0 0 0 4 23z M20 5.5A3.5 3.5 0 0 0 16.5 2H12v18h4.5A3.5 3.5 0 0 1 20 23z',
  }
  return (
    <svg aria-hidden="true" viewBox="0 0 24 24">
      <path d={paths[name]} />
    </svg>
  )
}
