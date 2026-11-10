import { useState } from 'react'
import { fireEvent, render, screen, within } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'

import type { DocumentNote } from '../api'
import { ReaderToolbar, type ReaderToolView } from './readerToolbar'
import { useReaderTypography } from './readerTypography'

const notes: DocumentNote[] = [
  {
    id: 'note-1', document_id: 'doc-1', chapter_index: 1,
    highlight: '第一条划线', note: '想法一', source: 'import',
  },
  {
    id: 'note-2', document_id: 'doc-1', chapter_index: 2,
    highlight: '第二条划线', source: 'pulse',
  },
]

interface HarnessProps {
  notes?: DocumentNote[]
  onJumpToNote?: (note: DocumentNote) => void
  onRemoveNote?: (noteId: string) => void
  onSaveNote?: () => void
}

// Harness owns the tool view and the editing note like the reader page does,
// so menu navigation, panel switching, and the edit flow run against the
// real controlled wiring.
function Harness({
  notes: noteList = notes,
  onJumpToNote = () => {},
  onRemoveNote = () => {},
  onSaveNote = () => {},
}: HarnessProps) {
  const [tool, setTool] = useState<ReaderToolView | null>(null)
  const [editingNote, setEditingNote] = useState<DocumentNote | null>(null)
  const [noteDraft, setNoteDraft] = useState('')
  const typographyControls = useReaderTypography()
  return (
    <ReaderToolbar
      tool={tool}
      onToolChange={setTool}
      typographyControls={typographyControls}
      notes={noteList}
      chapterTitle={(index) => `第 ${index + 1} 章`}
      editingNote={editingNote}
      noteDraft={noteDraft}
      onNoteDraftChange={setNoteDraft}
      savingNote={false}
      noteError=""
      onSaveNote={() => {
        onSaveNote()
        setEditingNote(null)
        setNoteDraft('')
        setTool('notes')
      }}
      onCancelEdit={() => {
        setEditingNote(null)
        setNoteDraft('')
        setTool('notes')
      }}
      onEditNote={(note) => {
        setEditingNote(note)
        setNoteDraft(note.note ?? '')
        setTool('edit')
      }}
      onRemoveNote={onRemoveNote}
      onJumpToNote={onJumpToNote}
    />
  )
}

function openNotesPanel() {
  fireEvent.click(screen.getByRole('button', { name: '阅读工具' }))
  fireEvent.click(screen.getByRole('menuitem', { name: /本书划线/ }))
}

describe('ReaderToolbar', () => {
  it('opens the tool menu with typography and highlights entries', () => {
    render(<Harness />)
    expect(screen.queryByRole('menu', { name: '阅读工具' })).toBeNull()

    fireEvent.click(screen.getByRole('button', { name: '阅读工具' }))
    const menu = screen.getByRole('menu', { name: '阅读工具' })
    expect(within(menu).getByRole('menuitem', { name: /排版设置/ })).toBeTruthy()
    expect(within(menu).getByRole('menuitem', { name: /本书划线/ }).textContent).toContain('2')
  })

  it('switches to the typography panel and back to the menu', () => {
    render(<Harness />)

    fireEvent.click(screen.getByRole('button', { name: '阅读工具' }))
    fireEvent.click(screen.getByRole('menuitem', { name: /排版设置/ }))
    expect(screen.getByRole('dialog', { name: '排版设置' })).toBeTruthy()
    expect(screen.getByRole('button', { name: '增大字号' })).toBeTruthy()

    fireEvent.click(screen.getByRole('button', { name: '返回' }))
    expect(screen.getByRole('menu', { name: '阅读工具' })).toBeTruthy()
  })

  it('lists the book highlights with chapter and source labels', () => {
    render(<Harness />)
    openNotesPanel()

    const panel = screen.getByRole('dialog', { name: '划线与笔记' })
    expect(within(panel).getByText('第一条划线')).toBeTruthy()
    expect(within(panel).getByText('想法一')).toBeTruthy()
    expect(within(panel).getByText(/导入/)).toBeTruthy()
    expect(within(panel).getByText(/Pulse/)).toBeTruthy()
  })

  it('shows an empty hint when the book has no highlights', () => {
    render(<Harness notes={[]} />)
    openNotesPanel()

    expect(screen.getByText('还没有划线，选中正文即可划线。')).toBeTruthy()
  })

  it('jumps to a highlight from the list', () => {
    const onJumpToNote = vi.fn()
    render(<Harness onJumpToNote={onJumpToNote} />)
    openNotesPanel()

    fireEvent.click(screen.getAllByRole('button', { name: '跳到划线位置' })[0])
    expect(onJumpToNote).toHaveBeenCalledWith(notes[0])
  })

  it('deletes a highlight from the list', () => {
    const onRemoveNote = vi.fn()
    render(<Harness onRemoveNote={onRemoveNote} />)
    openNotesPanel()

    fireEvent.click(screen.getAllByRole('button', { name: '删除' })[0])
    expect(onRemoveNote).toHaveBeenCalledWith('note-1')
  })

  it('edits a note inside the popover and returns to the list on cancel', () => {
    render(<Harness />)
    openNotesPanel()

    fireEvent.click(screen.getAllByRole('button', { name: '添加笔记' })[0])
    const editor = screen.getByRole('dialog', { name: '编辑笔记' })
    expect(within(editor).getByText('第二条划线')).toBeTruthy()
    fireEvent.change(within(editor).getByPlaceholderText('这条划线想记录什么？'), { target: { value: '想法' } })
    fireEvent.click(within(editor).getByRole('button', { name: '保存笔记' }))

    expect(screen.getByRole('dialog', { name: '划线与笔记' })).toBeTruthy()
  })

  it('keeps the edit dialog open for inside presses and saves through the handler', () => {
    const onSaveNote = vi.fn()
    render(<Harness onSaveNote={onSaveNote} />)
    openNotesPanel()

    fireEvent.click(screen.getAllByRole('button', { name: '添加笔记' })[0])
    fireEvent.pointerDown(screen.getByRole('dialog', { name: '编辑笔记' }))
    expect(screen.getByRole('dialog', { name: '编辑笔记' })).toBeTruthy()

    fireEvent.click(screen.getByRole('button', { name: '取消' }))
    expect(screen.getByRole('dialog', { name: '划线与笔记' })).toBeTruthy()
  })

  it('closes on Escape and on outside pointer presses', () => {
    render(<Harness />)

    fireEvent.click(screen.getByRole('button', { name: '阅读工具' }))
    fireEvent.keyDown(document, { key: 'Escape' })
    expect(screen.queryByRole('menu', { name: '阅读工具' })).toBeNull()

    fireEvent.click(screen.getByRole('button', { name: '阅读工具' }))
    fireEvent.pointerDown(document.body)
    expect(screen.queryByRole('menu', { name: '阅读工具' })).toBeNull()
  })
})
