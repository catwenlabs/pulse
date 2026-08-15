import { useState, type ReactNode } from 'react'
import { useQuery } from '@tanstack/react-query'

import type { ConversationContext, SelectionTool } from '../api'
import { listSelectionTools } from '../api'
import { queryKeys } from '../query'
import { ChatDialog, type ChatDialogStart } from './ChatDialog'
import { SelectionTarget } from './SelectionTarget'

/**
 * SelectionChatSurface is the wiring between the reading surface and the AI
 * selection tools. It wraps selectable prose in a SelectionTarget, lazily loads
 * the user's enabled tools, and opens the ChatDialog when a tool is chosen.
 * Wrap any block of readable content to give it selection AI.
 */
export function SelectionChatSurface({
  children,
  label,
  context,
  onHighlight,
}: {
  children: ReactNode
  label?: string
  /** Document coordinates forwarded to conversation creation when this
      surface covers one chapter of an imported document. */
  context?: ConversationContext
  /** Adds a 划线 toolbar action writing the selection as a document note. */
  onHighlight?: (selection: string) => void
}) {
  const toolsQuery = useQuery({
    queryKey: queryKeys.chatTools,
    queryFn: listSelectionTools,
    staleTime: 60_000,
  })
  const [start, setStart] = useState<ChatDialogStart | null>(null)

  const tools = toolsQuery.data ?? []

  function handleSelect(tool: SelectionTool, selection: string) {
    setStart({ tool, selection, context })
  }

  return (
    <>
      <SelectionTarget
        tools={tools}
        onSelect={handleSelect}
        label={label}
        actions={onHighlight ? [{ label: '划线', onSelect: onHighlight }] : []}
      >
        {children}
      </SelectionTarget>
      <ChatDialog
        open={start !== null}
        onOpenChange={(open) => { if (!open) setStart(null) }}
        start={start}
        tools={tools}
      />
    </>
  )
}
