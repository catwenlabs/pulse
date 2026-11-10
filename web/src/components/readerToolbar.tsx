import { useEffect, useRef, type ReactNode } from 'react'
import { ChevronLeft, Highlighter, Settings2, Type } from 'lucide-react'

import type { DocumentNote } from '../api'
import { ReaderTypographyPanel, type ReaderTypographyControls } from './readerTypography'

// ReaderToolbar is the reader's floating tool entry: one bottom-right button
// that opens a menu above it. Each menu item swaps the popover to its panel —
// today typography settings and the book's highlight list — so new reader
// tools join here instead of growing more floating buttons. Stays z-30 so
// the z-40 selection toolbar covers it while text is selected. Closes on
// Escape and on outside pointer presses; scrolling keeps it open.

export type ReaderToolView = 'menu' | 'typography' | 'notes' | 'edit'

interface ReaderToolbarProps {
  tool: ReaderToolView | null
  onToolChange: (tool: ReaderToolView | null) => void
  typographyControls: ReaderTypographyControls
  notes: DocumentNote[]
  chapterTitle: (chapterIndex: number) => string
  editingNote: DocumentNote | null
  noteDraft: string
  onNoteDraftChange: (draft: string) => void
  savingNote: boolean
  noteError: string
  onSaveNote: () => void
  // Leaving the editor always lands back on the notes list, whether the
  // edit was started from the list or from the highlight notice.
  onCancelEdit: () => void
  onEditNote: (note: DocumentNote) => void
  onRemoveNote: (noteId: string) => void
  onJumpToNote: (note: DocumentNote) => void
}

export function ReaderToolbar({
  tool,
  onToolChange,
  typographyControls,
  notes,
  chapterTitle,
  editingNote,
  noteDraft,
  onNoteDraftChange,
  savingNote,
  noteError,
  onSaveNote,
  onCancelEdit,
  onEditNote,
  onRemoveNote,
  onJumpToNote,
}: ReaderToolbarProps) {
  const rootRef = useRef<HTMLDivElement>(null)
  const open = tool !== null

  useEffect(() => {
    if (!open) return
    const onPointerDown = (event: PointerEvent) => {
      if (rootRef.current && event.target instanceof Node && !rootRef.current.contains(event.target)) {
        onToolChange(null)
      }
    }
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key === 'Escape') onToolChange(null)
    }
    document.addEventListener('pointerdown', onPointerDown)
    document.addEventListener('keydown', onKeyDown)
    return () => {
      document.removeEventListener('pointerdown', onPointerDown)
      document.removeEventListener('keydown', onKeyDown)
    }
  }, [open, onToolChange])

  return (
    <div ref={rootRef} className="fixed bottom-6 right-6 z-30 max-md:bottom-[max(1rem,env(safe-area-inset-bottom))] max-md:right-4">
      {tool === 'menu' && (
        <div
          role="menu"
          aria-label="阅读工具"
          className="absolute bottom-14 right-0 w-40 rounded-xl border border-border bg-card p-2 shadow-[0_18px_50px_rgba(32,29,23,.22)]"
        >
          <ReaderToolMenuItem label="排版设置" onClick={() => onToolChange('typography')}>
            <Type className="size-4" />
          </ReaderToolMenuItem>
          <ReaderToolMenuItem label="本书划线" onClick={() => onToolChange('notes')}>
            <Highlighter className="size-4" />
            <span className="ml-auto tabular-nums text-xs text-muted-foreground">{notes.length}</span>
          </ReaderToolMenuItem>
        </div>
      )}
      {tool === 'typography' && (
        <div
          role="dialog"
          aria-label="排版设置"
          className="absolute bottom-14 right-0 w-[min(320px,calc(100vw-2rem))] rounded-xl border border-border bg-card p-4 shadow-[0_18px_50px_rgba(32,29,23,.22)]"
        >
          <PanelHeader title="排版设置" onBack={() => onToolChange('menu')} />
          <ReaderTypographyPanel controls={typographyControls} />
        </div>
      )}
      {tool === 'notes' && (
        <div
          role="dialog"
          aria-label="划线与笔记"
          className="absolute bottom-14 right-0 w-[min(320px,calc(100vw-2rem))] rounded-xl border border-border bg-card p-4 shadow-[0_18px_50px_rgba(32,29,23,.22)]"
        >
          <ReaderNotesPanel
            notes={notes}
            chapterTitle={chapterTitle}
            onBack={() => onToolChange('menu')}
            onEditNote={onEditNote}
            onRemoveNote={onRemoveNote}
            onJumpToNote={onJumpToNote}
          />
        </div>
      )}
      {tool === 'edit' && editingNote && (
        <div
          role="dialog"
          aria-label="编辑笔记"
          className="absolute bottom-14 right-0 w-[min(320px,calc(100vw-2rem))] rounded-xl border border-border bg-card p-4 shadow-[0_18px_50px_rgba(32,29,23,.22)]"
        >
          <ReaderNoteEditor
            highlight={editingNote.highlight}
            noteDraft={noteDraft}
            onNoteDraftChange={onNoteDraftChange}
            savingNote={savingNote}
            noteError={noteError}
            onSaveNote={onSaveNote}
            onCancel={onCancelEdit}
            onBack={onCancelEdit}
          />
        </div>
      )}
      <button
        type="button"
        aria-label="阅读工具"
        aria-expanded={open}
        className="grid size-11 cursor-pointer place-items-center rounded-full border border-border bg-card text-muted-foreground shadow-[0_8px_24px_rgba(32,29,23,.18)] transition-colors hover:bg-accent hover:text-primary-hover"
        onClick={() => onToolChange(open ? null : 'menu')}
      >
        <Settings2 className="size-5" />
      </button>
    </div>
  )
}

function ReaderToolMenuItem({ label, onClick, children }: { label: string; onClick: () => void; children: ReactNode }) {
  return (
    <button
      type="button"
      role="menuitem"
      className="flex w-full cursor-pointer items-center gap-2 rounded-lg px-2.5 py-2 text-sm text-card-foreground transition-colors hover:bg-accent hover:text-primary-hover"
      onClick={onClick}
    >
      {children}
      <span>{label}</span>
    </button>
  )
}

function PanelHeader({ title, onBack }: { title: string; onBack: () => void }) {
  return (
    <div className="mb-3 flex items-center gap-1.5 border-b border-border pb-2">
      <button
        type="button"
        aria-label="返回"
        className="cursor-pointer rounded-md border-0 bg-transparent p-1 text-muted-foreground transition-colors hover:bg-accent hover:text-primary-hover"
        onClick={onBack}
      >
        <ChevronLeft className="size-4" />
      </button>
      <span className="text-sm font-medium">{title}</span>
    </div>
  )
}

interface ReaderNotesPanelProps {
  notes: DocumentNote[]
  chapterTitle: (chapterIndex: number) => string
  onBack: () => void
  onEditNote: (note: DocumentNote) => void
  onRemoveNote: (noteId: string) => void
  onJumpToNote: (note: DocumentNote) => void
}

function ReaderNotesPanel({ notes, chapterTitle, onBack, onEditNote, onRemoveNote, onJumpToNote }: ReaderNotesPanelProps) {
  return (
    <>
      <PanelHeader title={`划线与笔记 (${notes.length})`} onBack={onBack} />
      {notes.length === 0 ? (
        <p className="text-sm text-muted-foreground">还没有划线，选中正文即可划线。</p>
      ) : (
        <ul className="max-h-[min(24rem,55vh)] space-y-3 overflow-y-auto">
          {notes.map((note) => (
            <li key={note.id} className="text-sm">
              <button
                type="button"
                aria-label="跳到划线位置"
                title={note.highlight}
                className="block w-full cursor-pointer rounded-md border-0 bg-transparent p-0 text-left"
                onClick={() => onJumpToNote(note)}
              >
                <p className="line-clamp-3 border-l-2 border-primary/60 pl-2 text-muted-foreground">{note.highlight}</p>
              </button>
              {note.note && <p className="mt-1 line-clamp-2 pl-2">{note.note}</p>}
              <p className="mt-1 flex items-center gap-2 pl-2 text-xs text-muted-foreground">
                <span className="min-w-0 truncate">
                  {chapterTitle(note.chapter_index)}
                  {' · '}
                  {note.source === 'import' ? '导入' : 'Pulse'}
                </span>
                <button
                  type="button"
                  className="cursor-pointer rounded border-0 bg-transparent px-0 text-xs text-primary-hover hover:underline"
                  onClick={() => onEditNote(note)}
                >
                  {note.note ? '编辑' : '添加笔记'}
                </button>
                <button
                  type="button"
                  className="cursor-pointer rounded border-0 bg-transparent px-0 text-xs text-destructive hover:underline"
                  onClick={() => onRemoveNote(note.id)}
                >
                  删除
                </button>
              </p>
            </li>
          ))}
        </ul>
      )}
    </>
  )
}

interface ReaderNoteEditorProps {
  highlight: string
  noteDraft: string
  onNoteDraftChange: (draft: string) => void
  savingNote: boolean
  noteError: string
  onSaveNote: () => void
  onCancel: () => void
  onBack: () => void
}

function ReaderNoteEditor({ highlight, noteDraft, onNoteDraftChange, savingNote, noteError, onSaveNote, onCancel, onBack }: ReaderNoteEditorProps) {
  return (
    <>
      <PanelHeader title="编辑笔记" onBack={onBack} />
      <p className="mb-2 line-clamp-3 border-l-2 border-primary/60 pl-2 text-sm text-muted-foreground" title={highlight}>
        {highlight}
      </p>
      <textarea
        rows={4}
        className="mb-2 w-full resize-y rounded-md border border-input bg-background px-3 py-2 text-sm"
        placeholder="这条划线想记录什么？"
        value={noteDraft}
        onChange={(event) => onNoteDraftChange(event.target.value)}
      />
      {noteError !== '' && <p className="mb-2 text-sm text-destructive">{noteError}</p>}
      <div className="flex justify-end gap-2">
        <button
          type="button"
          className="cursor-pointer rounded-md border border-border bg-transparent px-3 py-1.5 text-sm"
          onClick={onCancel}
        >
          取消
        </button>
        <button
          type="button"
          className="cursor-pointer rounded-md bg-primary px-3 py-1.5 text-sm font-medium text-primary-foreground disabled:opacity-50"
          disabled={savingNote}
          onClick={onSaveNote}
        >
          保存笔记
        </button>
      </div>
    </>
  )
}
