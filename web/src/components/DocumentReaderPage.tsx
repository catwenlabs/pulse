import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { useQuery } from '@tanstack/react-query'

import { createDocumentNote, getDocument, listDocumentNotes, saveDocumentProgress, type Document, type DocumentNote } from '../api'
import { SelectionChatSurface } from './SelectionChatSurface'

const PROGRESS_SAVE_DELAY_MS = 1200

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
  const notes = useMemo(() => [...(notesQuery.data ?? []), ...createdNotes], [notesQuery.data, createdNotes])
  const [panelOpen, setPanelOpen] = useState(false)
  const [pendingHighlight, setPendingHighlight] = useState<{ chapter_index: number; selection: string } | null>(null)
  const [noteDraft, setNoteDraft] = useState('')
  const [highlightError, setHighlightError] = useState('')
  const [savingHighlight, setSavingHighlight] = useState(false)

  const saveHighlight = async () => {
    if (!pendingHighlight) return
    setSavingHighlight(true)
    setHighlightError('')
    try {
      const created = await createDocumentNote(doc.id, {
        chapter_index: pendingHighlight.chapter_index,
        highlight: pendingHighlight.selection,
        note: noteDraft,
      })
      setCreatedNotes((current) => [...current, created])
      setPendingHighlight(null)
      setNoteDraft('')
    } catch (cause) {
      setHighlightError(cause instanceof Error ? cause.message : '保存划线失败')
    } finally {
      setSavingHighlight(false)
    }
  }
  const sectionRefs = useRef(new Map<number, HTMLElement>())
  const restoredRef = useRef(false)
  const saveTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null)
  const lastSavedRef = useRef('')
  const [sidebarOpen, setSidebarOpen] = useState(true)

  const scrollToChapter = useCallback((index: number, block: ScrollLogicalPosition = 'start') => {
    sectionRefs.current.get(index)?.scrollIntoView({ block, behavior: 'auto' })
  }, [])

  // Restore the saved position once on mount.
  useEffect(() => {
    if (restoredRef.current) return
    restoredRef.current = true
    const progress = doc.progress
    if (progress && sectionRefs.current.has(progress.chapter_index)) {
      scrollToChapter(progress.chapter_index)
    }
  }, [doc.progress, scrollToChapter])

  const currentProgress = useCallback(() => {
    const sections = [...sectionRefs.current.entries()].sort((left, right) => left[0] - right[0])
    const anchorY = window.scrollY + 80
    let chapter = 0
    let ratio = 0
    for (const [index, section] of sections) {
      const top = section.offsetTop
      const height = Math.max(section.offsetHeight, 1)
      if (top <= anchorY) {
        chapter = index
        ratio = Math.min(Math.max((anchorY - top) / height, 0), 1)
      }
    }
    return { chapter_index: chapter, scroll_ratio: ratio }
  }, [])

  const scheduleSave = useCallback(() => {
    if (saveTimerRef.current) clearTimeout(saveTimerRef.current)
    saveTimerRef.current = setTimeout(() => {
      const progress = currentProgress()
      const key = `${progress.chapter_index}:${progress.scroll_ratio.toFixed(3)}`
      if (key === lastSavedRef.current) return
      lastSavedRef.current = key
      void saveDocumentProgress(doc.id, progress).catch(() => undefined)
    }, PROGRESS_SAVE_DELAY_MS)
  }, [currentProgress, doc.id])

  useEffect(() => {
    const container = containerRef.current
    if (!container) return
    const onScroll = () => scheduleSave()
    document.addEventListener('scroll', onScroll, true)
    return () => {
      document.removeEventListener('scroll', onScroll, true)
      if (saveTimerRef.current) clearTimeout(saveTimerRef.current)
    }
  }, [scheduleSave])

  const chapters = useMemo(() => doc.chapters, [doc.chapters])

  return (
    <div ref={containerRef} className="flex w-full">
      <aside
        className={`sticky top-0 h-screen shrink-0 overflow-y-auto border-r border-border p-3 ${sidebarOpen ? 'w-56' : 'w-0 overflow-hidden p-0'}`}
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
                      <p className="mt-1 pl-2 text-xs text-muted-foreground">
                        {chapters[note.chapter_index]?.title || `第 ${note.chapter_index + 1} 章`}
                        {' · '}
                        {note.source === 'import' ? '导入' : 'Pulse'}
                      </p>
                    </li>
                  ))}
                </ul>
              )}
            </section>
          )}
          {pendingHighlight && (
            <div role="dialog" aria-label="保存划线" className="mb-6 rounded-lg border border-border p-3">
              <p className="mb-2 border-l-2 border-primary/60 pl-2 text-sm text-muted-foreground">{pendingHighlight.selection}</p>
              <textarea
                className="mb-2 w-full rounded-md border border-input bg-background px-3 py-2 text-sm"
                placeholder="这条划线想记录什么？(可选)"
                value={noteDraft}
                onChange={(event) => setNoteDraft(event.target.value)}
              />
              {highlightError !== '' && <p className="mb-2 text-sm text-destructive">{highlightError}</p>}
              <div className="flex gap-2">
                <button
                  type="button"
                  className="rounded-md bg-primary px-3 py-1.5 text-sm font-medium text-primary-foreground disabled:opacity-50"
                  disabled={savingHighlight}
                  onClick={() => void saveHighlight()}
                >
                  保存划线
                </button>
                <button
                  type="button"
                  className="rounded-md border border-border px-3 py-1.5 text-sm"
                  onClick={() => setPendingHighlight(null)}
                >
                  取消
                </button>
              </div>
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
                  setNoteDraft('')
                  setHighlightError('')
                  setPendingHighlight({ chapter_index: chapter.index, selection })
                }}
              >
                <div
                  className="document-content leading-7"
                  dangerouslySetInnerHTML={{ __html: chapter.content_html }}
                />
              </SelectionChatSurface>
            </section>
          ))}
        </div>
      </main>
    </div>
  )
}
