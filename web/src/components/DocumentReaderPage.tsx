import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { useQuery } from '@tanstack/react-query'

import { createDocumentNote, deleteDocumentNote, getDocument, listDocumentNotes, saveDocumentProgress, updateDocumentNote, type Document, type DocumentNote } from '../api'
import { highlightContentHtml, normalizePassage, type HighlightPassage } from '../lib/documentHighlights'
import { resolveDocumentAssets } from './documentAssets'
import { SelectionChatSurface } from './SelectionChatSurface'
import { ReaderTypographyButton, readerTypographyStyle, useReaderTypography } from './readerTypography'

const PROGRESS_SAVE_DELAY_MS = 1200
// Progress is recorded from an anchor line this many pixels below the
// scrollport top — roughly the first line of readable text.
const READING_ANCHOR_OFFSET_PX = 80

// DocumentReaderPage renders one book as continuous scrolling chapters with a
// chapter sidebar, restoring and persisting the reading position (chapter
// index plus the scrolled fraction within it).
export function DocumentReaderPage({ documentID }: { documentID: string }) {
  const documentQuery = useQuery({
    queryKey: ['documents', documentID],
    queryFn: () => getDocument(documentID),
  })
  const doc = documentQuery.data

  if (documentQuery.isPending) {
    return <p className="px-4 py-6 text-sm text-muted-foreground">加载中…</p>
  }
  if (!doc) {
    return <p className="px-4 py-6 text-sm text-destructive">文档不存在</p>
  }
  return <ReaderBody document={doc} />
}

function ReaderBody({ document: doc }: { document: Document }) {
  const containerRef = useRef<HTMLDivElement>(null)
  const notesQuery = useQuery({
    queryKey: ['documents', doc.id, 'notes'],
    queryFn: () => listDocumentNotes(doc.id),
  })
  const [createdNotes, setCreatedNotes] = useState<DocumentNote[]>([])
  // Note edits applied on top of query data and created notes, keyed by id.
  const [noteRevisions, setNoteRevisions] = useState<Map<string, DocumentNote>>(() => new Map())
  // Note ids removed while this reader is open; filtered from every list.
  const [deletedNoteIds, setDeletedNoteIds] = useState<Set<string>>(() => new Set())
  const withRevisions = useCallback(
    (note: DocumentNote) => noteRevisions.get(note.id) ?? note,
    [noteRevisions],
  )
  const notes = useMemo(
    () => [...(notesQuery.data ?? []).map(withRevisions), ...createdNotes.map(withRevisions)]
      .filter((note) => !deletedNoteIds.has(note.id)),
    [notesQuery.data, createdNotes, withRevisions, deletedNoteIds],
  )
  const [panelOpen, setPanelOpen] = useState(false)
  // Apple-Books-style highlighting: 划线 saves immediately and colors the
  // text; the note is added or edited afterwards.
  const [highlightNotice, setHighlightNotice] = useState<DocumentNote | null>(null)
  const [editingNote, setEditingNote] = useState<DocumentNote | null>(null)
  const [noteDraft, setNoteDraft] = useState('')
  const [highlightError, setHighlightError] = useState('')
  const [savingNote, setSavingNote] = useState(false)
  const [noteError, setNoteError] = useState('')

  const saveHighlight = async (chapterIndex: number, selection: string) => {
    setHighlightError('')
    // Selecting the same passage twice must not create duplicate notes: the
    // existing note wins and the notice still offers 添加笔记.
    const duplicate = notes.find((note) =>
      note.chapter_index === chapterIndex && normalizePassage(note.highlight) === normalizePassage(selection))
    if (duplicate) {
      setHighlightNotice(duplicate)
      return
    }
    try {
      const created = await createDocumentNote(doc.id, {
        chapter_index: chapterIndex,
        highlight: selection,
      })
      setCreatedNotes((current) => [...current, created])
      setHighlightNotice(created)
    } catch (cause) {
      setHighlightError(cause instanceof Error ? cause.message : '保存划线失败')
    }
  }

  const removeHighlight = async (noteId: string) => {
    setHighlightError('')
    try {
      await deleteDocumentNote(noteId)
      setDeletedNoteIds((current) => new Set(current).add(noteId))
      setHighlightNotice((notice) => (notice?.id === noteId ? null : notice))
      setEditingNote((note) => (note?.id === noteId ? null : note))
    } catch (cause) {
      setHighlightError(cause instanceof Error ? cause.message : '取消划线失败')
    }
  }

  const saveNote = async () => {
    if (!editingNote) return
    setSavingNote(true)
    setNoteError('')
    try {
      const updated = await updateDocumentNote(editingNote.id, { note: noteDraft })
      setNoteRevisions((current) => new Map(current).set(updated.id, updated))
      setEditingNote(null)
      setNoteDraft('')
      setHighlightNotice(null)
    } catch (cause) {
      setNoteError(cause instanceof Error ? cause.message : '保存笔记失败')
    } finally {
      setSavingNote(false)
    }
  }
  const sectionRefs = useRef(new Map<number, HTMLElement>())
  const restoredRef = useRef(false)
  const saveTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null)
  const lastSavedRef = useRef('')
  const [sidebarOpen, setSidebarOpen] = useState(true)
  const typographyControls = useReaderTypography()
  const contentStyle = useMemo(() => readerTypographyStyle(typographyControls.typography), [typographyControls.typography])

  const scrollToChapter = useCallback((index: number, block: ScrollLogicalPosition = 'start') => {
    sectionRefs.current.get(index)?.scrollIntoView({ block, behavior: 'auto' })
  }, [])

  // scrollToProgress places the anchor line at the exact point currentProgress
  // records from: the section top plus the saved fraction of its height.
  const scrollToProgress = useCallback((chapterIndex: number, scrollRatio: number) => {
    const container = containerRef.current
    const section = sectionRefs.current.get(chapterIndex)
    if (!container || !section) return
    const sectionRect = section.getBoundingClientRect()
    const anchorY = container.getBoundingClientRect().top + READING_ANCHOR_OFFSET_PX
    container.scrollTop += sectionRect.top + scrollRatio * sectionRect.height - anchorY
  }, [])

  // Restore the saved position once on mount.
  useEffect(() => {
    if (restoredRef.current) return
    restoredRef.current = true
    if (doc.progress) {
      scrollToProgress(doc.progress.chapter_index, doc.progress.scroll_ratio)
    }
  }, [doc.progress, scrollToProgress])

  const currentProgress = useCallback(() => {
    const container = containerRef.current
    if (!container) return { chapter_index: 0, scroll_ratio: 0 }
    const sections = [...sectionRefs.current.entries()].sort((left, right) => left[0] - right[0])
    const anchorY = container.getBoundingClientRect().top + READING_ANCHOR_OFFSET_PX
    let chapter = 0
    let ratio = 0
    for (const [index, section] of sections) {
      const rect = section.getBoundingClientRect()
      if (rect.top <= anchorY) {
        chapter = index
        ratio = Math.min(Math.max((anchorY - rect.top) / Math.max(rect.height, 1), 0), 1)
      }
    }
    return { chapter_index: chapter, scroll_ratio: ratio }
  }, [])

  const saveProgressNow = useCallback(() => {
    if (!containerRef.current) return
    if (saveTimerRef.current) {
      clearTimeout(saveTimerRef.current)
      saveTimerRef.current = null
    }
    const progress = currentProgress()
    const key = `${progress.chapter_index}:${progress.scroll_ratio.toFixed(3)}`
    if (key === lastSavedRef.current) return
    lastSavedRef.current = key
    void saveDocumentProgress(doc.id, progress).catch(() => undefined)
  }, [currentProgress, doc.id])

  const scheduleSave = useCallback(() => {
    if (saveTimerRef.current) clearTimeout(saveTimerRef.current)
    saveTimerRef.current = setTimeout(() => {
      saveTimerRef.current = null
      saveProgressNow()
    }, PROGRESS_SAVE_DELAY_MS)
  }, [saveProgressNow])

  useEffect(() => {
    const container = containerRef.current
    if (!container) return
    const onScroll = () => scheduleSave()
    // pagehide flushes the latest position when the tab closes or navigates
    // away inside the save debounce window.
    const onPageHide = () => saveProgressNow()
    container.addEventListener('scroll', onScroll)
    window.addEventListener('pagehide', onPageHide)
    return () => {
      container.removeEventListener('scroll', onScroll)
      window.removeEventListener('pagehide', onPageHide)
      if (saveTimerRef.current) {
        clearTimeout(saveTimerRef.current)
        saveTimerRef.current = null
      }
    }
  }, [scheduleSave, saveProgressNow])

  const chapters = useMemo(() => doc.chapters, [doc.chapters])
  const highlightsByChapter = useMemo(() => {
    const map = new Map<number, HighlightPassage[]>()
    for (const note of notes) {
      const passages = map.get(note.chapter_index)
      if (passages) passages.push({ id: note.id, highlight: note.highlight })
      else map.set(note.chapter_index, [{ id: note.id, highlight: note.highlight }])
    }
    return map
  }, [notes])

  // Highlight marks are baked into the chapter HTML before React renders it.
  // Painting them onto the live DOM would be wiped by React 19, which resets
  // innerHTML on every re-render of dangerouslySetInnerHTML content.
  const chapterHtml = useMemo(() => {
    const map = new Map<number, string>()
    for (const chapter of chapters) {
      const resolved = resolveDocumentAssets(chapter.content_html, doc.id)
      const passages = highlightsByChapter.get(chapter.index) ?? []
      map.set(chapter.index, passages.length > 0 ? highlightContentHtml(resolved, passages) : resolved)
    }
    return map
  }, [chapters, doc.id, highlightsByChapter])

  return (
    <div ref={containerRef} data-testid="reader-scroll" className="flex h-full min-h-0 w-full overflow-y-auto">
      <aside
        className={`sticky top-0 h-full shrink-0 overflow-y-auto border-r border-border p-3 ${sidebarOpen ? 'w-56' : 'w-0 overflow-hidden p-0'}`}
      >
        <p className="mb-2 truncate px-2 text-sm font-semibold">{doc.title}</p>
        <ul className="space-y-0.5">
          {chapters.map((chapter) => (
            <li key={chapter.index}>
              <button
                type="button"
                className="w-full truncate rounded-md px-2 py-1.5 text-left text-sm hover:bg-accent"
                onClick={() => scrollToChapter(chapter.index)}
              >
                {chapter.title || `第 ${chapter.index + 1} 章`}
              </button>
            </li>
          ))}
        </ul>
      </aside>
      <main className="min-w-0 flex-1 px-4 py-6 sm:px-8">
        <div className="mx-auto max-w-[720px]">
          <button
            type="button"
            className="mb-3 rounded-md border border-border px-2 py-1 text-xs text-muted-foreground"
            onClick={() => setSidebarOpen((open) => !open)}
          >
            目录
          </button>
          <h1 className="mb-1 text-2xl font-semibold">{doc.title}</h1>
          {doc.author && <p className="mb-6 text-sm text-muted-foreground">{doc.author}</p>}
          <button
            type="button"
            className="mb-4 ml-2 rounded-md border border-border px-2 py-1 text-xs text-muted-foreground"
            onClick={() => setPanelOpen((open) => !open)}
          >
            笔记 ({notes.length})
          </button>
          <a
            className="mb-4 ml-2 inline-block rounded-md border border-border px-2 py-1 text-xs text-muted-foreground"
            href={`/api/v1/documents/${doc.id}/original`}
          >
            下载原件
          </a>
          {panelOpen && (
            <section aria-label="本书笔记" className="mb-6 rounded-lg border border-border p-3">
              {notes.length === 0 ? (
                <p className="text-sm text-muted-foreground">还没有笔记</p>
              ) : (
                <ul className="space-y-3">
                  {notes.map((note) => (
                    <li key={note.id} className="text-sm">
                      <p className="border-l-2 border-primary/60 pl-2 text-muted-foreground">{note.highlight}</p>
                      {note.note && <p className="mt-1 pl-2">{note.note}</p>}
                      <p className="mt-1 flex items-center gap-2 pl-2 text-xs text-muted-foreground">
                        <span>
                          {chapters[note.chapter_index]?.title || `第 ${note.chapter_index + 1} 章`}
                          {' · '}
                          {note.source === 'import' ? '导入' : 'Pulse'}
                        </span>
                        <button
                          type="button"
                          className="cursor-pointer rounded border-0 bg-transparent px-0 text-xs text-primary-hover hover:underline"
                          onClick={() => {
                            setEditingNote(note)
                            setNoteDraft(note.note ?? '')
                            setNoteError('')
                          }}
                        >
                          {note.note ? '编辑' : '添加笔记'}
                        </button>
                        <button
                          type="button"
                          className="cursor-pointer rounded border-0 bg-transparent px-0 text-xs text-destructive hover:underline"
                          onClick={() => {
                            setHighlightError('')
                            void removeHighlight(note.id)
                          }}
                        >
                          删除
                        </button>
                      </p>
                    </li>
                  ))}
                </ul>
              )}
            </section>
          )}
          {highlightNotice && !editingNote && (
            <div role="status" aria-label="已添加划线" className="mb-6 flex items-center gap-3 rounded-lg border border-border p-3">
              <p className="min-w-0 flex-1 truncate border-l-2 border-primary/60 pl-2 text-sm text-muted-foreground">
                {highlightNotice.highlight}
              </p>
              <button
                type="button"
                className="shrink-0 rounded-md bg-primary px-3 py-1.5 text-sm font-medium text-primary-foreground"
                onClick={() => {
                  setEditingNote(highlightNotice)
                  setNoteDraft(highlightNotice.note ?? '')
                  setNoteError('')
                }}
              >
                添加笔记
              </button>
              <button
                type="button"
                aria-label="关闭提示"
                className="shrink-0 cursor-pointer rounded-md border-0 bg-transparent text-xl text-muted-foreground"
                onClick={() => setHighlightNotice(null)}
              >
                ×
              </button>
            </div>
          )}
          {editingNote && (
            <div role="dialog" aria-label="编辑笔记" className="mb-6 rounded-lg border border-border p-3">
              <p className="mb-2 border-l-2 border-primary/60 pl-2 text-sm text-muted-foreground">{editingNote.highlight}</p>
              <textarea
                className="mb-2 w-full rounded-md border border-input bg-background px-3 py-2 text-sm"
                placeholder="这条划线想记录什么？"
                value={noteDraft}
                onChange={(event) => setNoteDraft(event.target.value)}
              />
              {noteError !== '' && <p className="mb-2 text-sm text-destructive">{noteError}</p>}
              <div className="flex gap-2">
                <button
                  type="button"
                  className="rounded-md bg-primary px-3 py-1.5 text-sm font-medium text-primary-foreground disabled:opacity-50"
                  disabled={savingNote}
                  onClick={() => void saveNote()}
                >
                  保存笔记
                </button>
                <button
                  type="button"
                  className="rounded-md border border-border px-3 py-1.5 text-sm"
                  onClick={() => {
                    setEditingNote(null)
                    setNoteDraft('')
                  }}
                >
                  取消
                </button>
              </div>
            </div>
          )}
          {highlightError !== '' && (
            <div role="alert" className="mb-6 rounded-lg border border-destructive/40 bg-destructive/5 p-3 text-sm text-destructive">
              {highlightError}
            </div>
          )}
          {chapters.map((chapter) => (
            <section
              key={chapter.index}
              data-chapter-index={chapter.index}
              ref={(element) => {
                if (element) sectionRefs.current.set(chapter.index, element)
                else sectionRefs.current.delete(chapter.index)
              }}
              className="mb-12"
            >
              <h2 className="mb-4 text-xl font-semibold">{chapter.title}</h2>
              {/* Chapter HTML is sanitized at import; Pulse owns the reading style. */}
              <SelectionChatSurface
                label={`${doc.title} ${chapter.title || `第 ${chapter.index + 1} 章`}`}
                context={{ document_id: doc.id, chapter_index: chapter.index }}
                onHighlight={(selection) => {
                  setHighlightError('')
                  void saveHighlight(chapter.index, selection)
                }}
                onUnhighlight={(noteId) => {
                  setHighlightError('')
                  void removeHighlight(noteId)
                }}
              >
                <div
                  className="document-content"
                  style={contentStyle}
                  dangerouslySetInnerHTML={{ __html: chapterHtml.get(chapter.index) ?? '' }}
                />
              </SelectionChatSurface>
            </section>
          ))}
        </div>
      </main>
      <ReaderTypographyButton controls={typographyControls} />
    </div>
  )
}
