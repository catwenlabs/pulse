import { useRef, useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'

import { listDocuments, importDocument, deleteDocument, type DocumentSummary } from '../api'

// DocumentsPage is the reading hub's library: imported documents, upload,
// and the shared search box (title, author, chapter content). Deleting a
// document cascades to its chapters, reading progress, and notes, so the
// row delete uses a two-step confirm.
export function DocumentsPage({ onOpenDocument }: { onOpenDocument: (id: string) => void }) {
  const queryClient = useQueryClient()
  const [search, setSearch] = useState('')
  const documentsQuery = useQuery({
    queryKey: ['documents', search],
    queryFn: () => listDocuments(search),
  })
  const fileInputRef = useRef<HTMLInputElement>(null)
  const [uploading, setUploading] = useState(false)
  const [uploadError, setUploadError] = useState('')

  const documents = documentsQuery.data ?? []

  const upload = async (file: File | undefined) => {
    if (!file) return
    setUploading(true)
    setUploadError('')
    try {
      await importDocument(file)
      await queryClient.invalidateQueries({ queryKey: ['documents'] })
    } catch (cause) {
      setUploadError(cause instanceof Error ? cause.message : '导入失败')
    } finally {
      setUploading(false)
    }
  }

  const remove = async (doc: DocumentSummary) => {
    try {
      await deleteDocument(doc.id)
      await queryClient.invalidateQueries({ queryKey: ['documents'] })
    } catch (cause) {
      setUploadError(cause instanceof Error ? cause.message : '删除失败')
    }
  }

  return (
    <div className="mx-auto h-full w-full max-w-[900px] overflow-y-auto px-4 py-6">
      <header className="mb-4 flex flex-wrap items-center gap-3">
        <h1 className="text-xl font-semibold">文档库</h1>
        <div className="ml-auto flex items-center gap-2">
          <input
            type="search"
            placeholder="搜索书名、作者或章节内容"
            value={search}
            onChange={(event) => setSearch(event.target.value)}
            className="min-w-56 rounded-md border border-input bg-background px-3 py-2 text-sm"
          />
          <button
            type="button"
            className="rounded-md bg-primary px-3 py-2 text-sm font-medium text-primary-foreground disabled:opacity-50"
            disabled={uploading}
            onClick={() => fileInputRef.current?.click()}
          >
            {uploading ? '导入中…' : '导入文档'}
          </button>
          <input
            ref={fileInputRef}
            type="file"
            accept=".epub,.txt,.md,.markdown"
            className="hidden"
            onChange={(event) => {
              void upload(event.target.files?.[0])
              event.target.value = ''
            }}
          />
        </div>
      </header>
      {uploadError !== '' && (
        <p className="mb-3 rounded-md border border-destructive/40 bg-destructive/10 px-3 py-2 text-sm text-destructive">
          {uploadError}
        </p>
      )}
      {documentsQuery.isPending ? (
        <p className="text-sm text-muted-foreground">加载中…</p>
      ) : documents.length === 0 ? (
        <p className="text-sm text-muted-foreground">还没有导入任何文档</p>
      ) : (
        <ul className="divide-y divide-border rounded-lg border border-border">
          {documents.map((doc) => (
            <DocumentRow key={doc.id} document={doc} onOpen={onOpenDocument} onDelete={() => void remove(doc)} />
          ))}
        </ul>
      )}
    </div>
  )
}

function DocumentRow({
  document: doc,
  onOpen,
  onDelete,
}: {
  document: DocumentSummary
  onOpen: (id: string) => void
  onDelete: () => void
}) {
  const [confirming, setConfirming] = useState(false)
  return (
    <li className="flex items-center">
      <button
        type="button"
        className="flex min-w-0 flex-1 items-center gap-3 px-4 py-3 text-left hover:bg-accent/50"
        onClick={() => onOpen(doc.id)}
      >
        <span className="min-w-0 flex-1">
          <span className="block truncate font-medium">{doc.title}</span>
          {doc.author && <span className="block truncate text-sm text-muted-foreground">{doc.author}</span>}
        </span>
        <span className="shrink-0 text-sm text-muted-foreground">{doc.chapter_count} 章</span>
      </button>
      {confirming ? (
        <button
          type="button"
          className="mr-3 shrink-0 rounded-md border border-destructive/50 px-2 py-1 text-xs font-medium text-destructive hover:bg-destructive/10"
          aria-label={`确认删除 ${doc.title}`}
          onClick={onDelete}
        >
          确认删除
        </button>
      ) : (
        <button
          type="button"
          className="mr-3 shrink-0 rounded-md px-2 py-1 text-xs text-muted-foreground hover:text-destructive"
          aria-label={`删除 ${doc.title}`}
          onClick={() => setConfirming(true)}
        >
          删除
        </button>
      )}
    </li>
  )
}
