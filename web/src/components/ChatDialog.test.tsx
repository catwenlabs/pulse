import { render, screen, waitFor } from '@testing-library/react'
import { fireEvent } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import * as api from '../api'
import type { ChatStreamEvent, ConversationCreated, SelectionTool } from '../api'

import { ChatDialog } from './ChatDialog'

const tool: SelectionTool = {
  id: 't1', name: 'AI 解读', prompt_template: '请解释：{{selection}}',
  enabled: true, position: 0, created_at: '', updated_at: '',
}

vi.mock('../api', async () => {
  const actual = await vi.importActual<typeof api>('../api')
  return {
    ...actual,
    createConversation: vi.fn(),
    streamAssistant: vi.fn(),
    stopGeneration: vi.fn(),
    sendFollowUp: vi.fn(),
    getConversation: vi.fn(),
  }
})

beforeEach(() => {
  localStorage.setItem('pulse:ai-chat:first-use-ack', '1')
  vi.mocked(api.createConversation).mockResolvedValue({
    conversation: { id: 'c1', selected_text: 'E=mc^2', tool_name: 'AI 解读', prompt_template: '{{selection}}', created_at: '', updated_at: '' },
    user_message: { id: 'm1', conversation_id: 'c1', role: 'user', content: '请解释：E=mc^2', created_at: '', updated_at: '' },
  } satisfies ConversationCreated)
  vi.mocked(api.stopGeneration).mockResolvedValue(undefined)
})

afterEach(() => {
  vi.clearAllMocks()
  localStorage.clear()
})

function streamOnce(events: ChatStreamEvent[], terminal: ChatStreamEvent) {
  vi.mocked(api.streamAssistant).mockImplementation(async (_id, _mode, _key, onEvent) => {
    for (const event of events) onEvent(event)
    return terminal
  })
}

describe('ChatDialog', () => {
  it('creates a conversation and streams the assistant reply', async () => {
    streamOnce(
      [
        { kind: 'delta', delta: 'Hello ' },
        { kind: 'delta', delta: 'world' },
      ],
      { kind: 'completed', content: 'Hello world', status: 'completed' },
    )

    render(
      <ChatDialog
        open
        onOpenChange={() => {}}
        start={{ tool, selection: 'E=mc^2' }}
      />,
    )

    expect(await screen.findByText('请解释：E=mc^2')).toBeInTheDocument()
    await screen.findByText('Hello world')
    expect(vi.mocked(api.createConversation)).toHaveBeenCalledWith(
      expect.objectContaining({ tool_id: 't1', selection: 'E=mc^2' }),
      expect.any(String),
    )
  })

  it('calls stopGeneration when the stop button is pressed', async () => {
    // Keep the stream pending so the stop button stays visible.
    vi.mocked(api.streamAssistant).mockImplementation(() => new Promise(() => {}))
    render(
      <ChatDialog open onOpenChange={() => {}} start={{ tool, selection: 'E=mc^2' }} />,
    )
    const stop = await screen.findByRole('button', { name: '停止生成' })
    fireEvent.click(stop)
    await waitFor(() => expect(vi.mocked(api.stopGeneration)).toHaveBeenCalledWith('c1'))
  })

  it('shows the first-use disclosure before sending anything', () => {
    localStorage.removeItem('pulse:ai-chat:first-use-ack')
    render(
      <ChatDialog open onOpenChange={() => {}} start={{ tool, selection: 'E=mc^2' }} />,
    )
    expect(screen.getByText('选中文本将发送给 AI')).toBeInTheDocument()
    expect(vi.mocked(api.createConversation)).not.toHaveBeenCalled()
    fireEvent.click(screen.getByRole('button', { name: '我知道了' }))
    // After acknowledging, the conversation creation kicks off.
    expect(vi.mocked(api.createConversation)).toHaveBeenCalled()
  })

  it('offers retry when the last assistant reply failed', async () => {
    streamOnce([], { kind: 'failed', content: '', status: 'failed', error: 'boom' })
    render(
      <ChatDialog open onOpenChange={() => {}} start={{ tool, selection: 'E=mc^2' }} />,
    )
    const retry = await screen.findByRole('button', { name: '重试' })
    expect(retry).toBeInTheDocument()
    // Retry re-opens the stream in retry mode.
    streamOnce(
      [{ kind: 'delta', delta: 'fixed' }],
      { kind: 'completed', content: 'fixed', status: 'completed' },
    )
    fireEvent.click(retry)
    await screen.findByText('fixed')
    expect(vi.mocked(api.streamAssistant)).toHaveBeenLastCalledWith(
      'c1', 'retry', expect.any(String), expect.any(Function), expect.any(AbortSignal),
    )
  })
})
