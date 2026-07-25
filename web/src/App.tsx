import { FormEvent, useEffect, useState } from 'react'

import * as api from './api'
import type { CreateSourceInput, Entry, EntryPatch, Folder, PreviewResult, Source, SourceHealth, SourceKind } from './api'
import './styles.css'

type Notice = { tone: 'success' | 'error'; message: string }
type View = 'sources' | 'inbox' | 'starred' | 'later'

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
  }

  return (
    <div className="app-shell">
      <aside className="sidebar">
        <div className="sidebar-brand-row">
          <a className="brand" href="/" aria-label="Pulse 首页" onClick={() => showStream('inbox')}>
            <span className="brand-mark" aria-hidden="true">P</span>
            <span>Pulse</span>
          </a>
          <button className="sidebar-add" aria-label="添加信息源" onClick={() => setShowCreate(true)}>
            <span aria-hidden="true">＋</span><span className="sr-only">添加信息源</span>
          </button>
        </div>

        <nav className="main-nav" aria-label="主导航">
          <a className={activeView === 'inbox' && !selectedSourceID ? 'active' : ''} href="#inbox" onClick={() => showStream('inbox')}><NavIcon name="inbox" />全部文章</a>
          <a className={activeView === 'starred' ? 'active' : ''} href="#starred" onClick={() => showStream('starred')}><NavIcon name="star" />收藏</a>
          <a className={activeView === 'later' ? 'active' : ''} href="#later" onClick={() => showStream('later')}><NavIcon name="clock" />稍后阅读</a>
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
          <button className={activeView === 'sources' ? 'active' : ''} onClick={() => setActiveView('sources')}>
            <NavIcon name="source" />管理信息源
          </button>
          <span><span className="status-dot" />本地服务已连接</span>
        </div>
      </aside>

      <main className={`main-content ${activeView === 'sources' ? 'source-main' : 'reader-main'}`}>
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
        ) : (
          <Reader
            view={activeView}
            sourceID={selectedSourceID}
            sourceName={sources.find((source) => source.id === selectedSourceID)?.name}
            sources={sources}
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
    </div>
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
}: {
  view: Exclude<View, 'sources'>
  sourceID: string
  sourceName?: string
  sources: Source[]
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
      setSelected(null)
      setActionMenuOpen(false)
      setNotesOpen(false)
    }
    document.addEventListener('keydown', handleKeyDown)
    return () => document.removeEventListener('keydown', handleKeyDown)
  }, [selected])

  async function patch(item: Entry, change: EntryPatch) {
    const updated = await api.updateEntry(item.id, change)
    setEntries((current) => current.map((candidate) => candidate.id === updated.id ? updated : candidate))
    setSelected(updated)
  }

  function toggleEntry(item: Entry, element: HTMLElement) {
    if (selected?.id === item.id) {
      setSelected(null)
      setActionMenuOpen(false)
      setNotesOpen(false)
      return
    }

    setSelected(item)
    setActionMenuOpen(false)
    setNotesOpen(false)
    window.requestAnimationFrame(() => {
      element.scrollIntoView({ behavior: 'smooth', block: 'start' })
    })
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
        <div className="reader-title">
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
            {markingAllRead ? '正在标记…' : '全部标记为已读'}
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
                <div className="stream-entry-detail">
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
  }
  return (
    <svg aria-hidden="true" viewBox="0 0 24 24">
      <path d={paths[name]} />
    </svg>
  )
}
