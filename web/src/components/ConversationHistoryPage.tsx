import { useState } from 'react'
import { useInfiniteQuery, useQueryClient } from '@tanstack/react-query'
import { MessageSquareText, Trash2 } from 'lucide-react'
import { toast } from 'sonner'

import * as api from '../api'
import type { ChatConversation } from '../api'
import { ChatDialog } from './ChatDialog'
import { Button } from './ui/button'
import { Dialog, DialogContent, DialogDescription, DialogTitle } from './ui/dialog'
import { queryKeys } from '../query'

const pageSize = 50

/**
 * ConversationHistoryPage lists every persisted AI conversation, newest first
 * (cursor-paginated). Each row's label is the tool name plus an excerpt of the
 * selected text. Reopening a conversation loads it read/write in ChatDialog;
 * deleting asks for confirmation and cascades to its messages.
 */
export function ConversationHistoryPage() {
  const queryClient = useQueryClient()
  const [reopenId, setReopenId] = useState<string | null>(null)
  const [deleting, setDeleting] = useState<ChatConversation | null>(null)

  const conversationsQuery = useInfiniteQuery({
    queryKey: queryKeys.chatConversations,
    initialPageParam: '',
    queryFn: ({ pageParam }) => api.listConversations(pageSize, pageParam),
    getNextPageParam: (lastPage) => (lastPage.has_more && lastPage.next_cursor ? lastPage.next_cursor : undefined),
  })

  const conversations = conversationsQuery.data?.pages.flatMap((page) => page.items) ?? []

  function invalidate() {
    void queryClient.invalidateQueries({ queryKey: queryKeys.chatConversations })
  }

  return (
    <section className="grid gap-5" aria-labelledby="conversation-history-heading">
      <header>
        <p className="text-xs font-semibold uppercase tracking-[0.14em] text-muted-foreground">AI · 对话历史</p>
        <h1 id="conversation-history-heading" className="m-0 mt-1 font-serif text-3xl font-semibold tracking-tight text-foreground">
          AI 对话
        </h1>
        <p className="mb-0 mt-2 max-w-prose text-sm leading-6 text-muted-foreground">
          这里保存了你通过划词工具发起的所有 AI 对话。
        </p>
      </header>

      {conversationsQuery.isPending && (
        <div className="grid min-h-[160px] place-items-center rounded-xl border bg-card text-sm text-muted-foreground">
          正在加载对话历史…
        </div>
      )}
      {conversationsQuery.error && (
        <div className="grid min-h-[160px] place-items-center gap-2 rounded-xl border bg-card text-sm text-destructive">
          <span>{conversationsQuery.error instanceof Error ? conversationsQuery.error.message : '无法加载对话历史'}</span>
          <Button unstyled className="text-primary-hover underline" onClick={() => void conversationsQuery.refetch()}>重试</Button>
        </div>
      )}
      {!conversationsQuery.isPending && !conversationsQuery.error && conversations.length === 0 && (
        <div className="grid min-h-[160px] place-items-center gap-1 rounded-xl border border-dashed bg-card/50 px-6 py-12 text-center text-sm text-muted-foreground">
          <strong className="text-foreground">还没有 AI 对话</strong>
          <span>选中正文并使用划词工具后，对话会保存在这里。</span>
        </div>
      )}
      {conversations.length > 0 && (
        <>
          <ul className="grid gap-2" aria-label="AI 对话列表">
            {conversations.map((conversation) => (
              <li
                key={conversation.id}
                className="flex items-center gap-3 rounded-xl border bg-card px-4 py-3 shadow-sm transition-colors hover:bg-muted/30"
              >
                <span className="grid size-9 shrink-0 place-items-center rounded-lg bg-primary/10 text-primary" aria-hidden="true">
                  <MessageSquareText className="size-4" />
                </span>
                <button
                  type="button"
                  className="min-w-0 flex-1 cursor-pointer border-0 bg-transparent p-0 text-left"
                  onClick={() => setReopenId(conversation.id)}
                  aria-label={`打开对话：${conversationLabel(conversation)}`}
                >
                  <span className="block truncate text-sm font-semibold text-foreground">
                    {conversation.tool_name}
                  </span>
                  <span className="mt-0.5 block truncate text-xs text-muted-foreground">
                    {conversationExcerpt(conversation)}
                  </span>
                </button>
                <Button
                  variant="ghost"
                  size="icon"
                  aria-label={`删除对话 ${conversation.tool_name}`}
                  className="text-destructive hover:bg-destructive/10 hover:text-destructive"
                  onClick={() => setDeleting(conversation)}
                >
                  <Trash2 className="size-4" aria-hidden="true" />
                </Button>
              </li>
            ))}
          </ul>
          {conversationsQuery.hasNextPage && (
            <div className="flex justify-center">
              <Button
                variant="secondary"
                disabled={conversationsQuery.isFetchingNextPage}
                onClick={() => void conversationsQuery.fetchNextPage()}
              >
                {conversationsQuery.isFetchingNextPage ? '正在加载…' : '加载更多'}
              </Button>
            </div>
          )}
        </>
      )}

      <ChatDialog
        open={reopenId !== null}
        onOpenChange={(open) => { if (!open) setReopenId(null) }}
        start={null}
        conversationId={reopenId}
      />
      {deleting && (
        <DeleteConversationDialog
          conversation={deleting}
          onCancel={() => setDeleting(null)}
          onDeleted={() => {
            setDeleting(null)
            invalidate()
          }}
        />
      )}
    </section>
  )
}

function conversationLabel(conversation: ChatConversation): string {
  return `${conversation.tool_name}：${conversationExcerpt(conversation)}`
}

function conversationExcerpt(conversation: ChatConversation): string {
  const text = conversation.selected_text.replace(/\s+/g, ' ').trim()
  return text.length > 60 ? `${text.slice(0, 60)}…` : text
}

function DeleteConversationDialog({
  conversation,
  onCancel,
  onDeleted,
}: {
  conversation: ChatConversation
  onCancel: () => void
  onDeleted: () => void
}) {
  const [deleting, setDeleting] = useState(false)

  async function confirm() {
    setDeleting(true)
    try {
      await api.deleteConversation(conversation.id)
      onDeleted()
      toast.success('已删除对话', { duration: 3500 })
    } catch (cause) {
      toast.error(cause instanceof Error ? cause.message : '无法删除对话', { duration: 6000 })
      setDeleting(false)
    }
  }

  return (
    <Dialog open onOpenChange={(open) => !open && !deleting && onCancel()}>
      <DialogContent className="w-[min(440px,100%)]">
        <p className="text-xs font-semibold uppercase tracking-widest text-muted-foreground">DELETE CONVERSATION</p>
        <DialogTitle>删除对话？</DialogTitle>
        <DialogDescription className="mb-6 mt-2 text-sm leading-6 text-muted-foreground">
          这条「{conversation.tool_name}」对话及其所有消息将被永久删除。
        </DialogDescription>
        <div className="flex justify-end gap-2">
          <Button variant="secondary" disabled={deleting} onClick={onCancel} autoFocus>取消</Button>
          <Button variant="destructive" disabled={deleting} aria-label="确认删除对话" onClick={() => void confirm()}>
            {deleting ? '正在删除…' : '确认删除'}
          </Button>
        </div>
      </DialogContent>
    </Dialog>
  )
}
