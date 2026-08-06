import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { QueryClientProvider } from '@tanstack/react-query'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import * as api from '../api'
import type { SelectionTool } from '../api'
import { createQueryClient } from '../query'

import { SelectionToolsSettings } from './SelectionToolsSettings'

const tools: SelectionTool[] = [
  { id: 't1', name: 'AI 解读', prompt_template: '请解释：{{selection}}', enabled: true, position: 0, created_at: '', updated_at: '' },
  { id: 't2', name: 'AI 翻译', prompt_template: '翻译成英文：{{selection}}', enabled: false, position: 1, created_at: '', updated_at: '' },
]

vi.mock('../api', async () => {
  const actual = await vi.importActual<typeof api>('../api')
  return {
    ...actual,
    listSelectionTools: vi.fn(),
    createSelectionTool: vi.fn(),
    updateSelectionTool: vi.fn(),
    deleteSelectionTool: vi.fn(),
    reorderSelectionTools: vi.fn(),
  }
})

function renderSettings() {
  const client = createQueryClient()
  return render(
    <QueryClientProvider client={client}>
      <SelectionToolsSettings />
    </QueryClientProvider>,
  )
}

beforeEach(() => {
  vi.clearAllMocks()
  vi.mocked(api.listSelectionTools).mockResolvedValue(tools)
})

describe('SelectionToolsSettings', () => {
  it('lists tools in position order with enabled state', async () => {
    renderSettings()
    expect(await screen.findByText('AI 解读')).toBeInTheDocument()
    expect(screen.getByText('AI 翻译')).toBeInTheDocument()
    expect(screen.getByText('已启用')).toBeInTheDocument()
    expect(screen.getByText('已停用')).toBeInTheDocument()
  })

  it('creates a tool with a valid template', async () => {
    vi.mocked(api.createSelectionTool).mockResolvedValue(tools[0])
    renderSettings()
    fireEvent.click(await screen.findByRole('button', { name: /新建工具/ }))
    fireEvent.change(screen.getByPlaceholderText('例如：AI 解读'), { target: { value: '总结要点' } })
    fireEvent.change(screen.getByPlaceholderText(/请用中文解释/), { target: { value: '总结：{{selection}}' } })
    fireEvent.click(screen.getByRole('button', { name: '创建工具' }))
    await waitFor(() => expect(vi.mocked(api.createSelectionTool)).toHaveBeenCalledWith({
      name: '总结要点',
      prompt_template: '总结：{{selection}}',
      enabled: true,
    }))
  })

  it('rejects a template missing the selection placeholder', async () => {
    renderSettings()
    fireEvent.click(await screen.findByRole('button', { name: /新建工具/ }))
    fireEvent.change(screen.getByPlaceholderText('例如：AI 解读'), { target: { value: '无效工具' } })
    fireEvent.change(screen.getByPlaceholderText(/请用中文解释/), { target: { value: '没有占位符' } })
    fireEvent.click(screen.getByRole('button', { name: '创建工具' }))
    expect(await screen.findByRole('alert')).toHaveTextContent('{{selection}}')
    expect(vi.mocked(api.createSelectionTool)).not.toHaveBeenCalled()
  })

  it('edits an existing tool', async () => {
    vi.mocked(api.updateSelectionTool).mockResolvedValue(tools[0])
    renderSettings()
    fireEvent.click(await screen.findByRole('button', { name: '编辑 AI 解读' }))
    const nameInput = screen.getByPlaceholderText('例如：AI 解读')
    fireEvent.change(nameInput, { target: { value: 'AI 深度解读' } })
    fireEvent.click(screen.getByRole('button', { name: '保存修改' }))
    await waitFor(() => expect(vi.mocked(api.updateSelectionTool)).toHaveBeenCalledWith('t1', {
      name: 'AI 深度解读',
      prompt_template: '请解释：{{selection}}',
      enabled: true,
    }))
  })

  it('toggles a tool enabled state', async () => {
    vi.mocked(api.updateSelectionTool).mockResolvedValue(tools[0])
    renderSettings()
    fireEvent.click(await screen.findByRole('button', { name: '停用 AI 解读' }))
    await waitFor(() => expect(vi.mocked(api.updateSelectionTool)).toHaveBeenCalledWith('t1', {
      name: 'AI 解读',
      prompt_template: '请解释：{{selection}}',
      enabled: false,
    }))
  })

  it('deletes a tool after confirmation', async () => {
    vi.mocked(api.deleteSelectionTool).mockResolvedValue(undefined)
    renderSettings()
    fireEvent.click(await screen.findByRole('button', { name: '删除 AI 翻译' }))
    fireEvent.click(await screen.findByRole('button', { name: '确认删除 AI 翻译' }))
    await waitFor(() => expect(vi.mocked(api.deleteSelectionTool)).toHaveBeenCalledWith('t2'))
  })

  it('saves a new order after drag and drop', async () => {
    vi.mocked(api.reorderSelectionTools).mockResolvedValue([tools[1], tools[0]])
    renderSettings()
    const first = await screen.findByText('AI 解读')
    const second = screen.getByText('AI 翻译')
    const firstRow = first.closest('li')!
    const secondRow = second.closest('li')!
    fireEvent.dragStart(secondRow)
    fireEvent.dragOver(firstRow)
    fireEvent.drop(firstRow)
    await waitFor(() => expect(vi.mocked(api.reorderSelectionTools)).toHaveBeenCalledWith(['t2', 't1']))
  })
})
