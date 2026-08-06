import { useCallback, useEffect, useRef, useState, type ReactNode } from 'react'

import type { SelectionTool } from '../api'

const desktopToolLimit = 5
const mobileToolLimit = 3
const selectionExcerptLimit = 80
const mobileMediaQuery = '(max-width: 767px) and (pointer: coarse)'

export interface SelectionTargetProps {
  tools: SelectionTool[]
  onSelect: (tool: SelectionTool, selection: string) => void
  children: ReactNode
  /** aria-label for the landmark wrapping selectable content. */
  label?: string
}

interface ToolbarPosition {
  top: number
  left: number
}

/**
 * SelectionTarget is the explicit opt-in surface for selection tools. It only
 * reacts to selections that begin inside its container, ignores empty
 * selections and editable controls, and clears or repositions the toolbar when
 * the selection collapses, scrolls out of scope, or the target unmounts.
 *
 * Desktop shows a floating toolbar anchored near the selection (kept inside the
 * viewport); touch layouts use a bottom action bar so the toolbar does not
 * collide with the browser's native selection menu.
 */
export function SelectionTarget({ tools, onSelect, children, label = '可选中的内容区域' }: SelectionTargetProps) {
  const containerRef = useRef<HTMLDivElement>(null)
  const [selection, setSelection] = useState('')
  const [position, setPosition] = useState<ToolbarPosition | null>(null)
  const [isTouch, setIsTouch] = useState(false)

  useEffect(() => {
    if (typeof window.matchMedia !== 'function') return
    const media = window.matchMedia(mobileMediaQuery)
    const update = () => setIsTouch(media.matches)
    update()
    media.addEventListener('change', update)
    return () => media.removeEventListener('change', update)
  }, [])

  const clearToolbar = useCallback(() => {
    setSelection('')
    setPosition(null)
  }, [])

  useEffect(() => {
    const handleSelection = () => {
      const active = window.getSelection()
      if (!active || active.isCollapsed) {
        clearToolbar()
        return
      }
      const text = active.toString().trim()
      if (!text) {
        clearToolbar()
        return
      }
      const anchor = active.anchorNode
      const container = containerRef.current
      if (!container || !anchor || !container.contains(anchor)) {
        return
      }
      if (isEditableTarget(anchor)) {
        clearToolbar()
        return
      }
      const rect = active.getRangeAt(0).getBoundingClientRect()
      const containerRect = container.getBoundingClientRect()
      if (rect.width === 0 && rect.height === 0) {
        return
      }
      const top = rect.top - containerRect.top
      const left = clamp(rect.left - containerRect.left + rect.width / 2, 0, containerRect.width)
      setSelection(text)
      setPosition({ top, left })
    }
    document.addEventListener('selectionchange', handleSelection)
    return () => document.removeEventListener('selectionchange', handleSelection)
  }, [clearToolbar])

  useEffect(() => {
    const handleScroll = () => {
      if (!position) return
      const active = window.getSelection()
      if (!active || active.isCollapsed) {
        clearToolbar()
        return
      }
      const anchor = active.anchorNode
      const container = containerRef.current
      if (!container || !anchor || !container.contains(anchor)) {
        clearToolbar()
      }
    }
    window.addEventListener('scroll', handleScroll, true)
    return () => window.removeEventListener('scroll', handleScroll, true)
  }, [position, clearToolbar])

  const enabledTools = tools.filter((tool) => tool.enabled)
  const visibleCount = isTouch ? mobileToolLimit : desktopToolLimit
  const direct = enabledTools.slice(0, visibleCount)
  const overflow = enabledTools.slice(visibleCount)
  const [overflowOpen, setOverflowOpen] = useState(false)

  useEffect(() => {
    if (!selection) setOverflowOpen(false)
  }, [selection])

  const choose = (tool: SelectionTool) => {
    if (!selection) return
    onSelect(tool, selection)
    clearToolbar()
    window.getSelection()?.removeAllRanges()
  }

  const showToolbar = Boolean(position) && selection.length > 0 && enabledTools.length > 0
  const excerpt = selection.length > selectionExcerptLimit
    ? `${selection.slice(0, selectionExcerptLimit)}…`
    : selection

  return (
    <div ref={containerRef} className="relative" aria-label={label}>
      {children}
      {showToolbar && !isTouch && (
        <div
          role="toolbar"
          aria-label="划词工具"
          className="absolute z-40 flex -translate-x-1/2 -translate-y-full items-center gap-1 rounded-lg border border-border bg-popover p-1 shadow-lg"
          style={{ top: position!.top - 8, left: position!.left }}
        >
          {direct.map((tool) => (
            <ToolbarButton key={tool.id} onClick={() => choose(tool)}>{tool.name}</ToolbarButton>
          ))}
          {overflow.length > 0 && (
            <OverflowMenu
              open={overflowOpen}
              onToggle={() => setOverflowOpen((open) => !open)}
              tools={overflow}
              onSelectTool={choose}
            />
          )}
        </div>
      )}
      {showToolbar && isTouch && (
        <div
          role="toolbar"
          aria-label="划词工具"
          className="fixed inset-x-0 bottom-0 z-40 flex items-center gap-1 border-t border-border bg-popover px-2 pb-[max(0.5rem,env(safe-area-inset-bottom))] pt-2 shadow-lg"
        >
          <span className="min-w-0 flex-1 truncate text-xs text-muted-foreground" title={selection}>{excerpt}</span>
          {direct.map((tool) => (
            <ToolbarButton key={tool.id} onClick={() => choose(tool)}>{tool.name}</ToolbarButton>
          ))}
          {overflow.length > 0 && (
            <OverflowMenu
              open={overflowOpen}
              onToggle={() => setOverflowOpen((open) => !open)}
              tools={overflow}
              onSelectTool={choose}
            />
          )}
        </div>
      )}
    </div>
  )
}

function ToolbarButton({ onClick, children }: { onClick: () => void; children: ReactNode }) {
  return (
    <button
      type="button"
      onClick={onClick}
      className="inline-flex min-h-9 items-center rounded-md px-2.5 text-sm font-medium text-popover-foreground hover:bg-accent focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
    >
      {children}
    </button>
  )
}

function OverflowMenu({
  open,
  onToggle,
  tools,
  onSelectTool,
}: {
  open: boolean
  onToggle: () => void
  tools: SelectionTool[]
  onSelectTool: (tool: SelectionTool) => void
}) {
  return (
    <div className="relative">
      <ToolbarButton onClick={onToggle}>更多</ToolbarButton>
      {open && (
        <div role="menu" className="absolute right-0 top-full mt-1 min-w-32 rounded-md border border-border bg-popover p-1 shadow-lg">
          {tools.map((tool) => (
            <button
              key={tool.id}
              type="button"
              role="menuitem"
              onClick={() => onSelectTool(tool)}
              className="block w-full rounded-md px-2.5 py-1.5 text-left text-sm text-popover-foreground hover:bg-accent"
            >
              {tool.name}
            </button>
          ))}
        </div>
      )}
    </div>
  )
}

function isEditableTarget(node: Node): boolean {
  let current: Node | null = node
  while (current && current.nodeType !== Node.ELEMENT_NODE) {
    current = current.parentNode
  }
  if (!current || !(current instanceof HTMLElement)) return false
  const element = current.closest('input, textarea, [contenteditable="true"], [contenteditable=""]')
  return Boolean(element)
}

function clamp(value: number, min: number, max: number): number {
  return Math.min(Math.max(value, min), max)
}
