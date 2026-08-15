import { useRef, useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'

import {
  importDocumentNotes,
  linkDocumentNote,
  listAllNotes,
  listDocuments,
  type DocumentNote,
  type DocumentSummary,
} from '../api'

// NotesHubPage is the reading hub's notes view: every note across books in
// stable book groups, the shared search box, the unmatched area with manual
// linking, and batch import through the neutral JSON contract.
export function NotesHubPage({ onOpenDocument }: { onOpenDocument: (id: string) => void }) {
  const queryClient = useQueryClient()
  const [search, setSearch] = useState('')
  const notesQuery = useQuery({
    queryKey: ['notes-hub', search],
    queryFn: () => listAllNotes(search),
  })
  const [linkTarget, setLinkTarget] = useState<DocumentNote | null>(null)
  const [importMessage, setImportMessage] = useState('')
  const [importing, setImporting] = useState(false)
  const fileInputRef = useRef<HTMLInputElement>(null)

  const notes = notesQuery.data ?? []
  const groups = groupByBook(notes)

  const importFile = async (file: File | undefined) => {
    if (!file) return
    setImporting(true)
    setImportMessage('')
    try {
      const text = await file.text()
      const parsed = JSON.parse(text) as { notes?: unknown }
      const summary = await importDocumentNotes((parsed.notes ?? []) as never)
      setImportMessage(`已导入 ${summary.imported} 条，未匹配 ${summary.unmatched} 条`)
      await queryClient.invalidateQueries({ queryKey: ['notes-hub'] })
    } catch (cause) {
      setImportMessage(cause instanceof Error ? cause.message : '导入失败')
    } finally {
      setImporting(false)
    }
  }

  return (
    <div className="mx-auto h-full w-full max-w-[900px] overflow-y-auto px-4 py-6">
      <header className="mb-4 flex flex-wrap items-center gap-3">
        <h1 className="text-xl font-semibold">笔记</h1>
        <div className="ml-auto flex items-center gap-2">
          <input
            type="search"
            placeholder="搜索高亮、笔记或书名"
            value={search}
            onChange={(event) => setSearch(event.target.value)}
            className="min-w-56 rounded-md border border-input bg-background px-3 py-2 text-sm"
          />
          <button
            type="button"
            className="rounded-md bg-primary px-3 py-2 text-sm font-medium text-primary-foreground disabled:opacity-50"
            disabled={importing}
            onClick={() => fileInputRef.current?.click()}
          >
            {importing ? '导入中…' : '导入笔记'}
          </button>
          <input
            ref={fileInputRef}
            type="file"
            accept=".json,application/json"
            className="hidden"
            onChange={(event) => {
              void importFile(event.target.files?.[0])
              event.target.value = ''
            }}
          />
        </div>
      </header>
      {importMessage !== '' && (
        <p className="mb-3 rounded-md border border-border bg-accent/40 px-3 py-2 text-sm">{importMessage}</p>
      )}
      {notesQuery.isPending ? (
        <p className="text-sm text-muted-foreground">加载中…</p>
      ) : groups.length === 0 ? (
        <p className="text-sm text-muted-foreground">还没有任何笔记</p>
      ) : (
        <div className="space-y-6">
          {groups.map((group) => (
            <section key={group.key} aria-label={group.title}>
              <h2 className="mb-2 flex flex-wrap items-baseline gap-2 text-base font-semibold">
                {group.title}
                {group.author && <span className="text-sm font-normal text-muted-foreground">{group.author}</span>}
                {group.unmatched && (
                  <span className="rounded-full bg-amber-500/15 px-2 py-0.5 text-xs font-medium text-amber-700 dark:text-amber-400">
                    未匹配
                  </span>
                )}
              </h2>
              <ul className="divide-y divide-border rounded-lg border border-border">
                {group.notes.map((note) => (
                  <NoteRow
                    key={note.id}
                    note={note}
                    onOpenDocument={onOpenDocument}
                    onLink={() => setLinkTarget(note)}
                  />
                ))}
              </ul>
            </section>
          ))}
        </div>
      )}
      {linkTarget && (
        <LinkDialog
          note={linkTarget}
          onClose={() => setLinkTarget(null)}
          onLinked={async () => {
            setLinkTarget(null)
            await queryClient.invalidateQueries({ queryKey: ['notes-hub'] })
          }}
        />
      )}
    </div>
  )
}

interface NoteGroup {
  key: string
  title: string
  author: string
  unmatched: boolean
  notes: DocumentNote[]
}

function groupByBook(notes: DocumentNote[]): NoteGroup[] {
  const groups = new Map<string, NoteGroup>()
  for (const note of notes) {
    const key = note.document_id || `${note.book_identifier}|${note.book_title}|${note.book_author}`
    const existing = groups.get(key)
    if (existing) {
      existing.notes.push(note)
    } else {
      groups.set(key, {
        key,
        title: note.book_title || '未命名文档',
        author: note.book_author ?? '',
        unmatched: !note.document_id,
        notes: [note],
      })
    }
  }
  return [...groups.values()]
}

function NoteRow({
  note,
  onOpenDocument,
  onLink,
}: {
  note: DocumentNote
  onOpenDocument: (id: string) => void
  onLink: () => void
}) {
  return (
    <li className="px-4 py-3 text-sm">
      <p className="border-l-2 border-primary/60 pl-2 text-muted-foreground">{note.highlight}</p>
      {note.note && <p className="mt-1 pl-2">{note.note}</p>}
      <p className="mt-1 flex flex-wrap items-center gap-2 pl-2 text-xs text-muted-foreground">
        <span>{note.source === 'import' ? '导入' : 'Pulse'}</span>
        {note.document_id ? (
          <button
            type="button"
            className="text-primary hover:underline"
            onClick={() => onOpenDocument(note.document_id!)}
          >
            打开文档
          </button>
        ) : (
          <button type="button" className="text-primary hover:underline" onClick={onLink}>
            关联
          </button>
        )}
      </p>
    </li>
  )
}

function LinkDialog({ note, onClose, onLinked }: { note: DocumentNote; onClose: () => void; onLinked: () => void }) {
  const documentsQuery = useQuery({
    queryKey: ['documents', ''],
    queryFn: () => listDocuments(),
  })
  const [error, setError] = useState('')
  const [linking, setLinking] = useState(false)

  const link = async (document: DocumentSummary) => {
    setLinking(true)
    setError('')
    try {
      await linkDocumentNote(note.id, document.id)
      onLinked()
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : '关联失败')
    } finally {
      setLinking(false)
    }
  }

  return (
    <div role="dialog" aria-label="关联到文档" className="fixed inset-0 z-50 grid place-items-center bg-black/40 p-4">
      <div className="w-full max-w-md rounded-lg border border-border bg-background p-4 shadow-lg">
        <p className="mb-1 text-sm font-semibold">把这条笔记关联到哪本文档？</p>
        <p className="mb-3 line-clamp-2 border-l-2 border-primary/60 pl-2 text-sm text-muted-foreground">{note.highlight}</p>
        {error !== '' && <p className="mb-2 text-sm text-destructive">{error}</p>}
        {documentsQuery.isPending ? (
          <p className="text-sm text-muted-foreground">加载中…</p>
        ) : (
          <ul className="max-h-64 space-y-1 overflow-y-auto">
            {(documentsQuery.data ?? []).map((document) => (
              <li key={document.id}>
                <button
                  type="button"
                  className="w-full truncate rounded-md px-3 py-2 text-left text-sm hover:bg-accent disabled:opacity-50"
                  disabled={linking}
                  onClick={() => void link(document)}
                >
                  {document.title}
                  {document.author && <span className="ml-2 text-muted-foreground">{document.author}</span>}
                </button>
              </li>
            ))}
          </ul>
        )}
        <button type="button" className="mt-3 rounded-md border border-border px-3 py-1.5 text-sm" onClick={onClose}>
          取消
        </button>
      </div>
    </div>
  )
}
