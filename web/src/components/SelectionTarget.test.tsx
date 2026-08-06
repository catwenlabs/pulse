import { act, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import type { SelectionTool } from '../api'

import { SelectionTarget } from './SelectionTarget'

const tools: SelectionTool[] = [
  { id: 't1', name: 'AI 解读', prompt_template: '{{selection}}', enabled: true, position: 0, created_at: '', updated_at: '' },
  { id: 't2', name: 'AI 翻译', prompt_template: '{{selection}}', enabled: true, position: 1, created_at: '', updated_at: '' },
]

afterEach(() => {
  vi.unstubAllGlobals()
  vi.restoreAllMocks()
})

interface StubSelection {
  text: string
  anchorNode: Node | null
}

function stubSelection(container: HTMLElement, text: string, editable = false): void {
  const anchorNode = editable ? container.querySelector('[contenteditable]') : container.firstChild
  const range = { getBoundingClientRect: () => ({ top: 100, left: 100, width: 40, height: 20, right: 140, bottom: 120, x: 100, y: 100, toJSON: () => ({}) }) }
  const selection = {
    isCollapsed: text === '',
    toString: () => text,
    anchorNode,
    getRangeAt: () => range,
    removeAllRanges: () => {},
  }
  vi.stubGlobal('getSelection', () => selection)
  // Force desktop layout for determinism.
  vi.stubGlobal('matchMedia', (query: string) => ({
    matches: false,
    media: query,
    addEventListener: () => {},
    removeEventListener: () => {},
  }))
}

function fireSelectionChange() {
  act(() => {
    document.dispatchEvent(new Event('selectionchange'))
  })
}

describe('SelectionTarget', () => {
  it('shows tools for a selection inside the target and reports the selection', () => {
    const onSelect = vi.fn()
    const { container } = render(
      <SelectionTarget tools={tools} onSelect={onSelect}>
        <p>some selectable text</p>
      </SelectionTarget>,
    )
    stubSelection(container, 'selectable')
    fireSelectionChange()

    const button = screen.getByRole('button', { name: 'AI 解读' })
    fireEvent.click(button)
    expect(onSelect).toHaveBeenCalledWith(expect.objectContaining({ id: 't1' }), 'selectable')
  })

  it('ignores empty and whitespace-only selections', () => {
    const onSelect = vi.fn()
    const { container } = render(
      <SelectionTarget tools={tools} onSelect={onSelect}>
        <p>text</p>
      </SelectionTarget>,
    )
    stubSelection(container, '   ')
    fireSelectionChange()
    expect(screen.queryByRole('toolbar')).not.toBeInTheDocument()
  })

  it('ignores selections that begin in editable controls', () => {
    const onSelect = vi.fn()
    const { container } = render(
      <SelectionTarget tools={tools} onSelect={onSelect}>
        <div contentEditable suppressContentEditableWarning>editable text</div>
      </SelectionTarget>,
    )
    stubSelection(container, 'editable text', true)
    fireSelectionChange()
    expect(screen.queryByRole('toolbar')).not.toBeInTheDocument()
  })

  it('places overflow tools under 更多', () => {
    const many: SelectionTool[] = Array.from({ length: 7 }, (_, i) => ({
      id: `t${i}`, name: `工具${i}`, prompt_template: '{{selection}}', enabled: true, position: i, created_at: '', updated_at: '',
    }))
    const { container } = render(
      <SelectionTarget tools={many} onSelect={() => {}}>
        <p>text</p>
      </SelectionTarget>,
    )
    stubSelection(container, 'text')
    fireSelectionChange()

    // Desktop shows the first five directly and hides the rest under 更多.
    expect(screen.getByRole('button', { name: '工具0' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: '工具4' })).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: '工具5' })).not.toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: '更多' }))
    fireEvent.click(screen.getByRole('menuitem', { name: '工具5' }))
  })
})
