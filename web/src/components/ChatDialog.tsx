import { useCallback, useEffect, useRef, useState, type FormEvent } from 'react'
import { Loader2, Send, StopCircle, RotateCcw, ChevronDown, ChevronRight, ShieldAlert } from 'lucide-react'

import * as api from '../api'
import type { ChatMessage, ChatStreamEvent, SelectionTool } from '../api'
import { Button } from './ui/button'
import { Dialog, DialogContent, DialogDescription, DialogTitle } from './ui/dialog'
import { ChatMarkdown } from './ChatMarkdown'

const firstUseStorageKey = 'pulse:ai-chat:first-use-ack'

export interface ChatDialogStart {
  tool: SelectionTool
  selection: string
}

export interface ChatDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  start: ChatDialogStart | null
  /** Reopen an existing conversation by id; takes precedence after load. */
  conversationId?: string | null
  tools?: SelectionTool[]
}

interface LocalAssistant extends ChatMessage {
  streaming?: boolean
}

/**
 * ChatDialog owns one Conversation's lifecycle: it creates the conversation
 * from a selection (or loads an existing one), streams the Assistant reply,
 * supports explicit stop, retry of failed/cancelled replies, and follow-up
 * questions. Closing the dialog hides it without aborting the stream; only
 * unmount (navigation away) aborts, which the server records as a disconnect.
 */
export function ChatDialog({ open, onOpenChange, start, conversationId, tools = [] }: ChatDialogProps) {
  const [conversation, setConversation] = useState<api.ChatConversation | null>(null)
  const [messages, setMessages] = useState<ChatMessage[]>([])
  const [assistantDraft, setAssistantDraft] = useState<LocalAssistant | null>(null)
  const [followUp, setFollowUp] = useState('')
  const [error, setError] = useState('')
  const [firstUseAcknowledged, setFirstUseAcknowledged] = useState(
    () => typeof localStorage !== 'undefined' && localStorage.getItem(firstUseStorageKey) === '1',
  )
  const [pendingStart, setPendingStart] = useState<ChatDialogStart | null>(null)
  const abortRef = useRef<AbortController | null>(null)
  const lastStreamKey = useRef('')

  const acknowledgeFirstUse = () => {
    try { localStorage.setItem(firstUseStorageKey, '1') } catch { /* storage may be unavailable */ }
    setFirstUseAcknowledged(true)
  }

  const streamGenerate = useCallback(async (conversationID: string, mode: 'generate' | 'retry') => {
    setError('')
    setAssistantDraft({ id: 'draft', conversation_id: conversationID, role: 'assistant', content: '', status: 'streaming', streaming: true, created_at: '', updated_at: '' })
    const key = `${mode}-${conversationID}-${Date.now()}`
    lastStreamKey.current = key
    const controller = new AbortController()
    abortRef.current = controller
    try {
      const terminal = await api.streamAssistant(
        conversationID,
        mode,
        key,
        (event: ChatStreamEvent) => {
          if (event.kind === 'delta' && event.delta) {
            setAssistantDraft((draft) => draft ? { ...draft, content: draft.content + event.delta! } : draft)
          }
        },
        controller.signal,
      )
      setAssistantDraft(null)
      setMessages((prev) => [...prev, terminalMessage(terminal)])
    } catch (err) {
      setAssistantDraft(null)
      // The server records the terminal state; a follow-up GET reconciles.
      setError(errorMessage(err))
    }
  }, [])

  // Start a brand-new conversation from a selection when the dialog opens.
  useEffect(() => {
    if (!open || !start || conversation) return
    if (!firstUseAcknowledged) {
      setPendingStart(start)
      return
    }
    void (async () => {
      try {
        const created = await api.createConversation(
          { tool_id: start.tool.id, selection: start.selection },
          `start-${Date.now()}`,
        )
        setConversation(created.conversation)
        setMessages([created.user_message])
        await streamGenerate(created.conversation.id, 'generate')
      } catch (err) {
        setError(errorMessage(err))
      }
    })()
  }, [open, start, conversation, firstUseAcknowledged, streamGenerate])

  // Load an existing conversation when reopened from history.
  useEffect(() => {
    if (!open || !conversationId || conversation) return
    void (async () => {
      try {
        const detail = await api.getConversation(conversationId)
        setConversation(detail.conversation)
        setMessages(detail.messages)
      } catch (err) {
        setError(errorMessage(err))
      }
    })()
  }, [open, conversationId, conversation])

  // Reset when the dialog closes.
  useEffect(() => {
    if (open) return
    setConversation(null)
    setMessages([])
    setAssistantDraft(null)
    setFollowUp('')
    setError('')
    setPendingStart(null)
  }, [open])

  // Abort on unmount (genuine disconnect). Hiding the dialog does not abort.
  useEffect(() => () => abortRef.current?.abort(), [])

  const stop = async () => {
    if (!conversation) return
    try {
      await api.stopGeneration(conversation.id)
    } catch (err) {
      setError(errorMessage(err))
    }
  }

  const retry = async () => {
    if (!conversation) return
    await streamGenerate(conversation.id, 'retry')
  }

  const sendFollowUp = async (event: FormEvent) => {
    event.preventDefault()
    if (!conversation || !followUp.trim()) return
    const content = followUp.trim()
    setFollowUp('')
    try {
      const message = await api.sendFollowUp(conversation.id, content, `fu-${Date.now()}`)
      setMessages((prev) => [...prev, message])
      await streamGenerate(conversation.id, 'generate')
    } catch (err) {
      setError(errorMessage(err))
    }
  }

  const streaming = Boolean(assistantDraft?.streaming)
  const lastMessage = messages[messages.length - 1]
  const canRetry = !streaming && lastMessage?.role === 'assistant'
    && (lastMessage.status === 'failed' || lastMessage.status === 'cancelled')
  const canFollowUp = !streaming && lastMessage?.role === 'assistant' && lastMessage.status === 'completed'
  const visible: ChatMessage[] = assistantDraft ? [...messages, assistantDraft] : messages

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="flex max-h-[85vh] flex-col gap-0 p-0 sm:max-w-2xl">
        <div className="border-b border-border px-4 py-3">
          <DialogTitle className="text-base font-semibold">{conversation?.tool_name ?? start?.tool.name ?? 'AI 对话'}</DialogTitle>
          <DialogDescription className="sr-only">与 AI 助手的对话</DialogDescription>
          {conversation && (
            <CollapsibleSelection label="选中文本" content={conversation.selected_text} />
          )}
          {start?.tool && (
            <CollapsibleSelection label="指令模板" content={start.tool.prompt_template} />
          )}
        </div>

        {!firstUseAcknowledged && pendingStart ? (
          <FirstUseNotice toolName={pendingStart.tool.name} onAcknowledge={acknowledgeFirstUse} />
        ) : (
          <>
            <div className="min-h-0 flex-1 overflow-y-auto px-4 py-3">
              <MessageList messages={visible} />
              {error && <p role="alert" className="mt-2 text-sm text-destructive">{error}</p>}
            </div>
            <div className="flex items-center gap-2 border-t border-border px-4 py-2">
              {streaming && (
                <Button variant="ghost" size="sm" onClick={() => void stop()} aria-label="停止生成">
                  <StopCircle className="size-4" aria-hidden="true" />停止
                </Button>
              )}
              {canRetry && (
                <Button variant="ghost" size="sm" onClick={() => void retry()} aria-label="重试">
                  <RotateCcw className="size-4" aria-hidden="true" />重试
                </Button>
              )}
              <form onSubmit={(e) => void sendFollowUp(e)} className="ml-auto flex w-full max-w-md items-center gap-2">
                <input
                  type="text"
                  value={followUp}
                  onChange={(e) => setFollowUp(e.target.value)}
                  placeholder={canFollowUp ? '继续追问…' : streaming ? '正在生成…' : '等待回复…'}
                  disabled={!canFollowUp}
                  aria-label="追问内容"
                  className="min-h-9 flex-1 rounded-md border border-border bg-background px-3 text-sm disabled:opacity-50"
                />
                <Button type="submit" size="sm" disabled={!canFollowUp || !followUp.trim()} aria-label="发送追问">
                  <Send className="size-4" aria-hidden="true" />
                </Button>
              </form>
            </div>
          </>
        )}
      </DialogContent>
    </Dialog>
  )
}

function MessageList({ messages }: { messages: ChatMessage[] }) {
  if (messages.length === 0) {
    return <p className="text-sm text-muted-foreground">正在开始对话…</p>
  }
  return (
    <div className="flex flex-col gap-3">
      {messages.map((message) => (
        <MessageBubble key={message.id} message={message} />
      ))}
    </div>
  )
}

function MessageBubble({ message }: { message: ChatMessage }) {
  const isUser = message.role === 'user'
  return (
    <div className={isUser ? 'flex justify-end' : 'flex justify-start'}>
      <div
        className={
          isUser
            ? 'max-w-[85%] rounded-2xl rounded-br-sm bg-primary px-3 py-2 text-sm text-primary-foreground whitespace-pre-wrap'
            : 'max-w-[90%] rounded-2xl rounded-bl-sm bg-card px-3 py-2 text-sm text-card-foreground'
        }
      >
        {isUser ? message.content : (
          <>
            <ChatMarkdown content={message.content || ''} />
            {message.status === 'failed' && message.error && (
              <p className="mt-1 text-xs text-destructive">{message.error}</p>
            )}
            {message.status === 'streaming' && !message.content && (
              <span className="inline-flex items-center gap-1 text-xs text-muted-foreground"><Loader2 className="size-3 animate-spin" aria-hidden="true" />正在生成…</span>
            )}
          </>
        )}
      </div>
    </div>
  )
}

function CollapsibleSelection({ label, content }: { label: string; content: string }) {
  const [open, setOpen] = useState(false)
  if (!content) return null
  return (
    <div className="mt-1">
      <button type="button" onClick={() => setOpen((v) => !v)} className="inline-flex items-center gap-1 text-xs text-muted-foreground hover:text-foreground" aria-expanded={open}>
        {open ? <ChevronDown className="size-3" aria-hidden="true" /> : <ChevronRight className="size-3" aria-hidden="true" />}
        {label}
      </button>
      {open && <pre className="mt-1 max-h-32 overflow-auto whitespace-pre-wrap rounded-md bg-muted px-2 py-1 text-xs text-foreground">{content}</pre>}
    </div>
  )
}

function FirstUseNotice({ toolName, onAcknowledge }: { toolName: string; onAcknowledge: () => void }) {
  return (
    <div className="px-4 py-6 text-sm" role="dialog" aria-label="首次使用提示">
      <div className="mb-2 flex items-center gap-2 font-medium text-foreground">
        <ShieldAlert className="size-4 text-primary" aria-hidden="true" />
        选中文本将发送给 AI
      </div>
      <p className="text-muted-foreground">
        使用「{toolName}」时，你选中的文本会发送给已配置的 AI 服务（Provider）以生成回复。
        如果该服务是远程的，选中文本会离开 Pulse。请在确认理解后继续。
      </p>
      <Button className="mt-3" onClick={onAcknowledge}>我知道了</Button>
    </div>
  )
}

function terminalMessage(event: ChatStreamEvent): ChatMessage {
  return {
    id: event.message_id ?? `term-${Date.now()}`,
    conversation_id: event.conversation_id ?? '',
    role: 'assistant',
    content: event.content ?? '',
    status: event.status,
    provider: event.provider,
    model: event.model,
    prompt_tokens: event.prompt_tokens,
    completion_tokens: event.completion_tokens,
    finish_reason: event.finish_reason,
    error: event.error,
    created_at: '',
    updated_at: '',
  }
}

function errorMessage(err: unknown): string {
  if (err instanceof Error) return err.message
  return 'AI 对话出错，请稍后重试。'
}
