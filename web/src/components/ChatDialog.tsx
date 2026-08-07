import { useCallback, useEffect, useRef, useState, type FormEvent } from 'react'
import {
  ChevronDown,
  Loader2,
  Quote,
  RotateCcw,
  SendHorizontal,
  ShieldAlert,
  Sparkles,
  Square,
} from 'lucide-react'

import * as api from '../api'
import type { ChatMessage, ChatStreamEvent, SelectionTool } from '../api'
import { Button } from './ui/button'
import { Dialog, DialogContent, DialogDescription, DialogTitle } from './ui/dialog'
import { ChatMarkdown } from './ChatMarkdown'
import { cn } from '../lib/utils'

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
  const scrollRef = useRef<HTMLDivElement | null>(null)

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

  const streaming = Boolean(assistantDraft?.streaming)
  const lastMessage = messages[messages.length - 1]
  const canRetry = !streaming && lastMessage?.role === 'assistant'
    && (lastMessage.status === 'failed' || lastMessage.status === 'cancelled')
  const canFollowUp = !streaming && lastMessage?.role === 'assistant' && lastMessage.status === 'completed'
  const visible: ChatMessage[] = assistantDraft ? [...messages, assistantDraft] : messages

  // Keep the newest reply in view while streaming or when messages arrive.
  useEffect(() => {
    const el = scrollRef.current
    if (el) el.scrollTop = el.scrollHeight
  }, [visible.length, assistantDraft?.content])

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

  const toolName = conversation?.tool_name ?? start?.tool.name ?? 'AI 对话'
  const contextBlocks: Array<{ key: string; label: string; content: string }> = []
  if (conversation?.selected_text) {
    contextBlocks.push({ key: 'selection', label: '选中文本', content: conversation.selected_text })
  }
  if (start?.tool.prompt_template) {
    contextBlocks.push({ key: 'prompt', label: '指令模板', content: start.tool.prompt_template })
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="flex max-h-[85dvh] w-full flex-col gap-0 overflow-hidden rounded-2xl p-0 shadow-[0_24px_80px_rgba(32,29,23,.28)] sm:max-w-2xl">
        <header className="border-b border-border/70 bg-card px-5 py-4">
          <div className="flex items-center gap-3">
            <span className="grid size-9 shrink-0 place-items-center rounded-xl bg-primary/10 text-primary" aria-hidden="true">
              <Sparkles className="size-4" />
            </span>
            <div className="min-w-0">
              <DialogTitle className="truncate text-[15px]! font-semibold leading-5">{toolName}</DialogTitle>
              <DialogDescription className="sr-only">与 AI 助手的对话</DialogDescription>
              <p className="m-0 mt-0.5 text-xs! leading-4 text-muted-foreground">
                {streaming ? '正在生成回复…' : '基于选中内容的 AI 对话'}
              </p>
            </div>
          </div>
          {contextBlocks.length > 0 && (
            <div className="mt-3 flex flex-wrap gap-1.5">
              {contextBlocks.map((block) => (
                <ContextChip key={block.key} label={block.label} content={block.content} />
              ))}
            </div>
          )}
        </header>

        {!firstUseAcknowledged && pendingStart ? (
          <FirstUseNotice toolName={pendingStart.tool.name} onAcknowledge={acknowledgeFirstUse} />
        ) : (
          <>
            <div ref={scrollRef} className="min-h-0 flex-1 overflow-y-auto overscroll-contain bg-background/40 px-5 py-4">
              <MessageList messages={visible} />
              {error && (
                <p role="alert" className="mt-3 rounded-lg border border-destructive/20 bg-destructive/5 px-3 py-2 text-xs leading-5 text-destructive">
                  {error}
                </p>
              )}
            </div>

            <footer className="border-t border-border/70 bg-card px-4 py-3">
              <form onSubmit={(e) => void sendFollowUp(e)} className="flex! items-end gap-2!">
                <div className="min-w-0 flex-1">
                  <label htmlFor="chat-follow-up" className="sr-only">追问内容</label>
                  <textarea
                    id="chat-follow-up"
                    value={followUp}
                    onChange={(e) => setFollowUp(e.target.value)}
                    onKeyDown={(e) => {
                      if (e.key === 'Enter' && !e.shiftKey && !e.nativeEvent.isComposing) {
                        e.preventDefault()
                        if (canFollowUp && followUp.trim()) void sendFollowUp(e)
                      }
                    }}
                    placeholder={canFollowUp ? '继续追问…（Enter 发送，Shift+Enter 换行）' : streaming ? '正在生成…' : '等待回复…'}
                    disabled={!canFollowUp}
                    rows={1}
                    className="max-h-32 min-h-10 w-full resize-none rounded-xl border border-input bg-background px-3.5 py-2.5 text-sm leading-5 shadow-[inset_0_1px_2px_rgba(51,46,36,.04)] outline-none transition-[border-color,box-shadow] placeholder:text-muted-foreground/70 focus:border-primary focus:ring-3 focus:ring-primary/10 disabled:cursor-not-allowed disabled:opacity-50"
                  />
                </div>
                {streaming ? (
                  <Button
                    type="button"
                    variant="secondary"
                    size="icon"
                    onClick={() => void stop()}
                    aria-label="停止生成"
                    className="size-10 shrink-0 rounded-xl"
                  >
                    <Square className="size-3.5 fill-current" aria-hidden="true" />
                  </Button>
                ) : (
                  <Button
                    type="submit"
                    size="icon"
                    disabled={!canFollowUp || !followUp.trim()}
                    aria-label="发送追问"
                    className="size-10 shrink-0 rounded-xl"
                  >
                    <SendHorizontal className="size-4" aria-hidden="true" />
                  </Button>
                )}
              </form>
              {canRetry && (
                <div className="mt-2 flex justify-center">
                  <Button variant="ghost" size="sm" onClick={() => void retry()} className="text-xs">
                    <RotateCcw className="size-3.5" aria-hidden="true" />
                    重试
                  </Button>
                </div>
              )}
            </footer>
          </>
        )}
      </DialogContent>
    </Dialog>
  )
}

function MessageList({ messages }: { messages: ChatMessage[] }) {
  if (messages.length === 0) {
    return (
      <div className="grid min-h-40 place-items-center">
        <p className="flex items-center gap-2 text-sm text-muted-foreground">
          <Loader2 className="size-4 animate-spin" aria-hidden="true" />
          正在开始对话…
        </p>
      </div>
    )
  }
  return (
    <div className="flex flex-col gap-4">
      {messages.map((message) => (
        <MessageBubble key={message.id} message={message} />
      ))}
    </div>
  )
}

function MessageBubble({ message }: { message: ChatMessage }) {
  const isUser = message.role === 'user'
  return (
    <div className={cn('flex', isUser ? 'justify-end' : 'justify-start')}>
      <div
        className={cn(
          'min-w-0 px-3.5 py-2.5 text-sm leading-6',
          isUser
            ? 'max-w-[85%] rounded-2xl rounded-br-md bg-primary text-primary-foreground shadow-sm whitespace-pre-wrap'
            : 'max-w-[92%] rounded-2xl rounded-bl-md border border-border/70 bg-card text-card-foreground shadow-[0_1px_2px_rgba(51,46,36,.05)]',
        )}
      >
        {isUser ? message.content : (
          <>
            <ChatMarkdown content={message.content || ''} />
            {message.status === 'failed' && message.error && (
              <p className="m-0 mt-2 border-t border-destructive/15 pt-2 text-xs text-destructive">{message.error}</p>
            )}
            {message.status === 'streaming' && !message.content && (
              <span className="inline-flex items-center gap-1.5 text-xs text-muted-foreground">
                <Loader2 className="size-3.5 animate-spin" aria-hidden="true" />
                正在生成…
              </span>
            )}
          </>
        )}
      </div>
    </div>
  )
}

function ContextChip({ label, content }: { label: string; content: string }) {
  const [open, setOpen] = useState(false)
  if (!content) return null
  return (
    <div className="min-w-0">
      <button
        type="button"
        onClick={() => setOpen((v) => !v)}
        aria-expanded={open}
        className={cn(
          'inline-flex max-w-full cursor-pointer items-center gap-1.5 rounded-full border px-2.5 py-1 text-[11px] font-medium transition-colors',
          open
            ? 'border-primary/30 bg-primary/10 text-primary-hover'
            : 'border-border bg-background text-muted-foreground hover:border-primary/25 hover:text-foreground',
        )}
      >
        <Quote className="size-3 shrink-0" aria-hidden="true" />
        <span className="truncate">{label}</span>
        <ChevronDown className={cn('size-3 shrink-0 transition-transform', open && 'rotate-180')} aria-hidden="true" />
      </button>
      {open && (
        <pre className="mt-1.5 max-h-32 overflow-auto whitespace-pre-wrap rounded-lg border border-border/70 bg-muted/60 px-3 py-2 font-sans text-xs leading-5 text-foreground">
          {content}
        </pre>
      )}
    </div>
  )
}

function FirstUseNotice({ toolName, onAcknowledge }: { toolName: string; onAcknowledge: () => void }) {
  return (
    <div className="grid gap-4 px-6 py-8" role="dialog" aria-label="首次使用提示">
      <span className="grid size-11 place-items-center rounded-2xl bg-primary/10 text-primary" aria-hidden="true">
        <ShieldAlert className="size-5" />
      </span>
      <div>
        <p className="m-0 text-base font-semibold text-foreground">选中文本将发送给 AI</p>
        <p className="m-0 mt-2 text-sm leading-6 text-muted-foreground">
          使用「{toolName}」时，你选中的文本会发送给已配置的 AI 服务（Provider）以生成回复。
          如果该服务是远程的，选中文本会离开 Pulse。请在确认理解后继续。
        </p>
      </div>
      <div>
        <Button onClick={onAcknowledge}>我知道了</Button>
      </div>
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
