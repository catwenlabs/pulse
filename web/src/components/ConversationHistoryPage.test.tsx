import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { QueryClientProvider } from '@tanstack/react-query'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import * as api from '../api'
import type { ChatConversation } from '../api'
import { createQueryClient } from '../query'

import { ConversationHistoryPage } from './ConversationHistoryPage'

function conversation(id: string, tool: string, text: string): ChatConversation {
  return {
    id,
    tool_name: tool,
    selected_text: text,
    prompt_template: '{{selection}}',
    created_at: '2026-08-01T00:00:00Z',
    updated_at: '2026-08-01T00:00:00Z',
  }
}

vi.mock('../api', async () => {
  const actual = await vi.importActual<typeof api>('../api')
  return {
    ...actual,
    listConversations: vi.fn(),
    getConversation: vi.fn(),
    deleteConversation: vi.fn(),
  }
})

// ChatDialog streams; stub the streaming surface so reopening is deterministic.
vi.mock('./ChatDialog', () => ({
  ChatDialog: ({ open, conversationId }: { open: boolean; conversationId?: string | null }) =>
    open ? <div data-testid="chat-dialog" data-conversation={conversationId ?? ''} /> : null,
}))

function renderPage() {
  return render(
    <QueryClientProvider client={createQueryClient()}>
      <ConversationHistoryPage />
    </QueryClientProvider>,
  )
}

beforeEach(() => {
  vi.clearAllMocks()
  vi.mocked(api.listConversations).mockResolvedValue({
    items: [conversation('c1', 'AI 解读', 'E=mc^2 是质能方程'), conversation('c2', 'AI 翻译', 'hello world')],
    has_more: false,
  })
})

describe('ConversationHistoryPage', () => {
  it('lists conversations with tool name and selection excerpt', async () => {
    renderPage()
    expect(await screen.findByText('AI 解读')).toBeInTheDocument()
    expect(screen.getByText('E=mc^2 是质能方程')).toBeInTheDocument()
    expect(screen.getByText('AI 翻译')).toBeInTheDocument()
  })

  it('shows an empty state when there are no conversations', async () => {
    vi.mocked(api.listConversations).mockResolvedValue({ items: [], has_more: false })
    renderPage()
    expect(await screen.findByText('还没有 AI 对话')).toBeInTheDocument()
  })

  it('reopens a conversation in the chat dialog', async () => {
    renderPage()
    fireEvent.click(await screen.findByRole('button', { name: /打开对话：AI 解读/ }))
    const dialog = await screen.findByTestId('chat-dialog')
    expect(dialog).toHaveAttribute('data-conversation', 'c1')
  })

  it('deletes a conversation after confirmation', async () => {
    vi.mocked(api.deleteConversation).mockResolvedValue(undefined)
    renderPage()
    fireEvent.click(await screen.findByRole('button', { name: '删除对话 AI 翻译' }))
    fireEvent.click(await screen.findByRole('button', { name: '确认删除对话' }))
    await waitFor(() => expect(vi.mocked(api.deleteConversation)).toHaveBeenCalledWith('c2'))
  })

  it('loads more pages when available', async () => {
    vi.mocked(api.listConversations)
      .mockResolvedValueOnce({ items: [conversation('c1', 'AI 解读', 'first')], next_cursor: 'next', has_more: true })
      .mockResolvedValueOnce({ items: [conversation('c2', 'AI 翻译', 'second')], has_more: false })
    renderPage()
    fireEvent.click(await screen.findByRole('button', { name: '加载更多' }))
    expect(await screen.findByText('second')).toBeInTheDocument()
    expect(vi.mocked(api.listConversations)).toHaveBeenLastCalledWith(50, 'next')
  })
})
