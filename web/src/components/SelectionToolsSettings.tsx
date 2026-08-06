import { FormEvent, useMemo, useRef, useState, type DragEvent } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { GripVertical, Plus, X } from 'lucide-react'
import { toast } from 'sonner'

import * as api from '../api'
import type { SelectionTool, SelectionToolInput } from '../api'
import { Button } from './ui/button'
import { Dialog, DialogClose, DialogContent, DialogDescription, DialogTitle } from './ui/dialog'
import { Input } from './ui/input'
import { Textarea } from './ui/textarea'
import { cn } from '../lib/utils'
import { queryKeys } from '../query'

const maxToolNameLength = 40
const maxPromptTemplateLength = 4000

/**
 * SelectionToolsSettings manages the user-configurable selection tools
 * (划词工具) shown in the floating toolbar. It supports create, edit,
 * enable/disable, drag-to-reorder (saved atomically), and delete. The list
 * order here is exactly the order the toolbar surfaces tools.
 */
export function SelectionToolsSettings() {
  const queryClient = useQueryClient()
  const toolsQuery = useQuery({ queryKey: queryKeys.chatTools, queryFn: api.listSelectionTools })
  const tools = useMemo(
    () => [...(toolsQuery.data ?? [])].sort((left, right) => left.position - right.position),
    [toolsQuery.data],
  )
  const [editorOpen, setEditorOpen] = useState(false)
  const [editing, setEditing] = useState<SelectionTool | null>(null)
  const [deletingTool, setDeletingTool] = useState<SelectionTool | null>(null)
  const [orderedIds, setOrderedIds] = useState<string[] | null>(null)
  const dragId = useRef<string | null>(null)
  const [dropTarget, setDropTarget] = useState<string | null>(null)

  const displayTools = useMemo(() => {
    if (!orderedIds) return tools
    const byId = new Map(tools.map((tool) => [tool.id, tool]))
    const reordered = orderedIds.flatMap((id) => {
      const tool = byId.get(id)
      return tool ? [tool] : []
    })
    return reordered.length === tools.length ? reordered : tools
  }, [orderedIds, tools])

  function invalidate() {
    void queryClient.invalidateQueries({ queryKey: queryKeys.chatTools })
  }

  const toggleMutation = useMutation({
    mutationFn: (tool: SelectionTool) =>
      api.updateSelectionTool(tool.id, {
        name: tool.name,
        prompt_template: tool.prompt_template,
        enabled: !tool.enabled,
      }),
    onSuccess: () => {
      invalidate()
    },
    onError: (cause) => {
      toast.error(cause instanceof Error ? cause.message : '无法更新划词工具', { duration: 6000 })
    },
  })

  const reorderMutation = useMutation({
    mutationFn: (ids: string[]) => api.reorderSelectionTools(ids),
    onSuccess: () => {
      setOrderedIds(null)
      invalidate()
    },
    onError: (cause) => {
      setOrderedIds(null)
      toast.error(cause instanceof Error ? cause.message : '无法保存工具顺序', { duration: 6000 })
    },
  })

  function beginDrag(id: string, event: DragEvent<HTMLElement>) {
    dragId.current = id
    if (event.dataTransfer) {
      event.dataTransfer.effectAllowed = 'move'
      event.dataTransfer.setData('text/plain', id)
    }
  }

  function allowDrop(id: string, event: DragEvent<HTMLElement>) {
    const dragged = dragId.current
    if (!dragged || dragged === id) return
    event.preventDefault()
    setDropTarget(id)
    if (event.dataTransfer) event.dataTransfer.dropEffect = 'move'
  }

  function handleDrop(id: string, event: DragEvent<HTMLElement>) {
    event.preventDefault()
    const dragged = dragId.current
    dragId.current = null
    setDropTarget(null)
    if (!dragged || dragged === id) return
    const ids = displayTools.map((tool) => tool.id)
    const remaining = ids.filter((toolId) => toolId !== dragged)
    const targetIndex = remaining.indexOf(id)
    if (targetIndex < 0) return
    remaining.splice(targetIndex, 0, dragged)
    setOrderedIds(remaining)
    reorderMutation.mutate(remaining)
  }

  function endDrag() {
    dragId.current = null
    setDropTarget(null)
    if (!reorderMutation.isPending) setOrderedIds(null)
  }

  return (
    <section className="grid gap-5" aria-labelledby="selection-tools-heading">
      <header className="flex flex-wrap items-end justify-between gap-4">
        <div>
          <p className="text-xs font-semibold uppercase tracking-[0.14em] text-muted-foreground">AI · 划词工具</p>
          <h1 id="selection-tools-heading" className="m-0 mt-1 font-serif text-3xl font-semibold tracking-tight text-foreground">
            划词工具
          </h1>
          <p className="mb-0 mt-2 max-w-prose text-sm leading-6 text-muted-foreground">
            选中正文后，这些工具会出现在浮动工具条中。模板必须包含 {'{{selection}}'} 占位符。
          </p>
        </div>
        <Button onClick={() => { setEditing(null); setEditorOpen(true) }}>
          <Plus className="size-4" aria-hidden="true" /> 新建工具
        </Button>
      </header>

      {toolsQuery.isPending && (
        <div className="grid min-h-[160px] place-items-center rounded-xl border bg-card text-sm text-muted-foreground">
          正在加载划词工具…
        </div>
      )}
      {toolsQuery.error && (
        <div className="grid min-h-[160px] place-items-center gap-2 rounded-xl border bg-card text-sm text-destructive">
          <span>{toolsQuery.error instanceof Error ? toolsQuery.error.message : '无法加载划词工具'}</span>
          <Button unstyled className="text-primary-hover underline" onClick={() => void toolsQuery.refetch()}>重试</Button>
        </div>
      )}
      {!toolsQuery.isPending && !toolsQuery.error && displayTools.length === 0 && (
        <div className="grid min-h-[160px] place-items-center gap-1 rounded-xl border border-dashed bg-card/50 px-6 py-12 text-center text-sm text-muted-foreground">
          <strong className="text-foreground">还没有划词工具</strong>
          <span>新建一个工具，选中文字后即可一键调用 AI。</span>
        </div>
      )}
      {!toolsQuery.isPending && !toolsQuery.error && displayTools.length > 0 && (
        <ul className="grid gap-2" aria-label="划词工具列表">
          {displayTools.map((tool) => (
            <li
              key={tool.id}
              draggable
              onDragStart={(event) => beginDrag(tool.id, event)}
              onDragOver={(event) => allowDrop(tool.id, event)}
              onDrop={(event) => handleDrop(tool.id, event)}
              onDragEnd={endDrag}
              className={cn(
                'flex cursor-grab items-center gap-3 rounded-xl border bg-card px-4 py-3 shadow-sm active:cursor-grabbing',
                dropTarget === tool.id && 'border-primary/60 bg-primary/5',
                !tool.enabled && 'opacity-70',
              )}
            >
              <GripVertical className="size-4 shrink-0 text-muted-foreground/60" aria-hidden="true" />
              <div className="min-w-0 flex-1">
                <div className="flex items-center gap-2">
                  <span className="truncate text-sm font-semibold text-foreground">{tool.name}</span>
                  <span
                    className={cn(
                      'shrink-0 rounded-md px-2 py-0.5 text-[10px] font-semibold uppercase tracking-wide',
                      tool.enabled ? 'bg-emerald-500/10 text-emerald-600' : 'bg-muted text-muted-foreground',
                    )}
                  >
                    {tool.enabled ? '已启用' : '已停用'}
                  </span>
                </div>
                <p className="mb-0 mt-0.5 truncate text-xs text-muted-foreground" title={tool.prompt_template}>
                  {tool.prompt_template}
                </p>
              </div>
              <div className="flex shrink-0 items-center gap-1">
                <Button variant="ghost" size="sm" aria-label={`${tool.enabled ? '停用' : '启用'} ${tool.name}`} onClick={() => toggleMutation.mutate(tool)}>
                  {tool.enabled ? '停用' : '启用'}
                </Button>
                <Button variant="ghost" size="sm" aria-label={`编辑 ${tool.name}`} onClick={() => { setEditing(tool); setEditorOpen(true) }}>
                  编辑
                </Button>
                <Button
                  variant="ghost"
                  size="sm"
                  className="text-destructive hover:bg-destructive/10 hover:text-destructive"
                  aria-label={`删除 ${tool.name}`}
                  onClick={() => setDeletingTool(tool)}
                >
                  删除
                </Button>
              </div>
            </li>
          ))}
        </ul>
      )}

      {editorOpen && (
        <ToolEditorDialog
          tool={editing}
          onClose={() => { setEditorOpen(false); setEditing(null) }}
          onSaved={() => {
            setEditorOpen(false)
            setEditing(null)
            invalidate()
          }}
        />
      )}
      {deletingTool && (
        <DeleteToolDialog
          tool={deletingTool}
          onCancel={() => setDeletingTool(null)}
          onDeleted={() => {
            setDeletingTool(null)
            invalidate()
          }}
        />
      )}
    </section>
  )
}

function validateToolInput(input: SelectionToolInput): string {
  const name = input.name.trim()
  if (!name) return '请输入工具名称'
  if ([...name].length > maxToolNameLength) return `工具名称不能超过 ${maxToolNameLength} 个字符`
  const template = input.prompt_template
  if (!template.trim()) return '请输入指令模板'
  if (!template.includes('{{selection}}')) return '指令模板必须包含 {{selection}} 占位符'
  if ([...template].length > maxPromptTemplateLength) return `指令模板不能超过 ${maxPromptTemplateLength} 个字符`
  return ''
}

function ToolEditorDialog({
  tool,
  onClose,
  onSaved,
}: {
  tool: SelectionTool | null
  onClose: () => void
  onSaved: () => void
}) {
  const [name, setName] = useState(tool?.name ?? '')
  const [template, setTemplate] = useState(tool?.prompt_template ?? '')
  const [enabled, setEnabled] = useState(tool?.enabled ?? true)
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState('')

  async function submit(event: FormEvent) {
    event.preventDefault()
    const input: SelectionToolInput = { name: name.trim(), prompt_template: template, enabled }
    const validationError = validateToolInput(input)
    if (validationError) {
      setError(validationError)
      return
    }
    setSaving(true)
    setError('')
    try {
      if (tool) await api.updateSelectionTool(tool.id, input)
      else await api.createSelectionTool(input)
      onSaved()
      toast.success(tool ? `已更新 ${input.name}` : `已创建 ${input.name}`, { duration: 3500 })
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : '保存划词工具失败')
      setSaving(false)
    }
  }

  return (
    <Dialog open onOpenChange={(open) => !open && !saving && onClose()}>
      <DialogContent className="w-[min(560px,100%)]">
        <DialogClose className="absolute right-4 top-4 grid size-8 cursor-pointer place-items-center rounded-md text-muted-foreground hover:bg-accent hover:text-foreground" aria-label="关闭">
          <X className="size-4" aria-hidden="true" />
        </DialogClose>
        <p className="text-xs font-semibold uppercase tracking-widest text-muted-foreground">
          {tool ? 'EDIT TOOL' : 'NEW TOOL'}
        </p>
        <DialogTitle>{tool ? '编辑划词工具' : '新建划词工具'}</DialogTitle>
        <DialogDescription className="mb-4 mt-2 text-sm leading-6 text-muted-foreground">
          模板中的 {'{{selection}}'} 会被替换为选中的文字。
        </DialogDescription>
        <form className="grid gap-4" onSubmit={(event) => void submit(event)}>
          <label className="grid gap-1.5 text-sm font-medium">
            <span>工具名称</span>
            <Input autoFocus required maxLength={maxToolNameLength} value={name} onChange={(event) => setName(event.target.value)} placeholder="例如：AI 解读" />
          </label>
          <label className="grid gap-1.5 text-sm font-medium">
            <span>指令模板</span>
            <Textarea
              required
              rows={5}
              maxLength={maxPromptTemplateLength}
              value={template}
              onChange={(event) => setTemplate(event.target.value)}
              placeholder={'请用中文解释下面这段内容：\n\n{{selection}}'}
            />
          </label>
          <label className="flex items-center gap-2 text-sm font-medium">
            <input
              type="checkbox"
              className="size-4 accent-primary"
              checked={enabled}
              onChange={(event) => setEnabled(event.target.checked)}
            />
            <span>启用此工具</span>
          </label>
          {error && <p className="m-0 text-sm text-destructive" role="alert">{error}</p>}
          <div className="mt-1 flex justify-end gap-2">
            <Button type="button" variant="ghost" disabled={saving} onClick={onClose}>取消</Button>
            <Button type="submit" disabled={saving}>{saving ? '保存中…' : tool ? '保存修改' : '创建工具'}</Button>
          </div>
        </form>
      </DialogContent>
    </Dialog>
  )
}

function DeleteToolDialog({
  tool,
  onCancel,
  onDeleted,
}: {
  tool: SelectionTool
  onCancel: () => void
  onDeleted: () => void
}) {
  const [deleting, setDeleting] = useState(false)

  async function confirm() {
    setDeleting(true)
    try {
      await api.deleteSelectionTool(tool.id)
      onDeleted()
      toast.success(`已删除 ${tool.name}`, { duration: 3500 })
    } catch (cause) {
      toast.error(cause instanceof Error ? cause.message : '无法删除划词工具', { duration: 6000 })
      setDeleting(false)
    }
  }

  return (
    <Dialog open onOpenChange={(open) => !open && !deleting && onCancel()}>
      <DialogContent className="w-[min(440px,100%)]">
        <p className="text-xs font-semibold uppercase tracking-widest text-muted-foreground">DELETE TOOL</p>
        <DialogTitle>删除划词工具？</DialogTitle>
        <DialogDescription className="mb-6 mt-2 text-sm leading-6 text-muted-foreground">
          “{tool.name}”将从划词工具条中移除。已有的 AI 对话会保留其内容。
        </DialogDescription>
        <div className="flex justify-end gap-2">
          <Button variant="secondary" disabled={deleting} onClick={onCancel} autoFocus>取消</Button>
          <Button variant="destructive" disabled={deleting} aria-label={`确认删除 ${tool.name}`} onClick={() => void confirm()}>
            {deleting ? '正在删除…' : '确认删除'}
          </Button>
        </div>
      </DialogContent>
    </Dialog>
  )
}
