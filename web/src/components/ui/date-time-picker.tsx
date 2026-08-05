import { useEffect, useMemo, useState } from 'react'
import { CalendarDays, Check, Clock3, RotateCcw } from 'lucide-react'

import { Button } from './button'
import { Calendar } from './calendar'
import { Popover, PopoverContent, PopoverTrigger } from './popover'
import { Select } from './select'
import { cn } from '../../lib/utils'

export interface DateTimePickerProps {
  value: string
  onChange: (value: string) => void
  id?: string
  disabled?: boolean
  placeholder?: string
  defaultTime?: string
  className?: string
  'aria-describedby'?: string
}

const timeOptions = Array.from({ length: 96 }, (_, index) => {
  const hours = Math.floor(index / 4)
  const minutes = (index % 4) * 15
  return `${String(hours).padStart(2, '0')}:${String(minutes).padStart(2, '0')}`
})

function parseLocalDateTime(value: string): Date | undefined {
  const match = /^(\d{4})-(\d{2})-(\d{2})T(\d{2}):(\d{2})$/.exec(value)
  if (!match) return undefined
  const date = new Date(Number(match[1]), Number(match[2]) - 1, Number(match[3]), Number(match[4]), Number(match[5]))
  if (
    date.getFullYear() !== Number(match[1])
    || date.getMonth() !== Number(match[2]) - 1
    || date.getDate() !== Number(match[3])
    || date.getHours() !== Number(match[4])
    || date.getMinutes() !== Number(match[5])
  ) return undefined
  return date
}

function formatLocalDateTime(date: Date, time: string): string {
  const [hours, minutes] = time.split(':').map(Number)
  const next = new Date(date)
  next.setHours(hours || 0, minutes || 0, 0, 0)
  const pad = (value: number) => String(value).padStart(2, '0')
  return `${next.getFullYear()}-${pad(next.getMonth() + 1)}-${pad(next.getDate())}T${pad(next.getHours())}:${pad(next.getMinutes())}`
}

function formatTime(date: Date): string {
  return `${String(date.getHours()).padStart(2, '0')}:${String(date.getMinutes()).padStart(2, '0')}`
}

function formatDisplayValue(date: Date): string {
  return `${date.getFullYear()}年${date.getMonth() + 1}月${date.getDate()}日 ${formatTime(date)}`
}

export function DateTimePicker({
  value,
  onChange,
  id,
  disabled = false,
  placeholder = '选择日期和时间',
  defaultTime = '00:00',
  className,
  'aria-describedby': ariaDescribedBy,
}: DateTimePickerProps) {
  const parsedValue = parseLocalDateTime(value)
  const [open, setOpen] = useState(false)
  const [draftDate, setDraftDate] = useState<Date | undefined>(parsedValue)
  const [draftTime, setDraftTime] = useState(parsedValue ? formatTime(parsedValue) : defaultTime)
  const [month, setMonth] = useState<Date>(parsedValue ?? new Date())
  const timeID = id ? `${id}-time` : undefined
  const availableTimes = useMemo(() => (
    Array.from(new Set([...timeOptions, draftTime].filter(Boolean))).sort()
  ), [draftTime])

  useEffect(() => {
    const nextDate = parseLocalDateTime(value)
    setDraftDate(nextDate)
    setDraftTime(nextDate ? formatTime(nextDate) : defaultTime)
    if (nextDate) setMonth(nextDate)
  }, [defaultTime, value])

  function handleOpenChange(nextOpen: boolean) {
    if (nextOpen) {
      const nextDate = parseLocalDateTime(value)
      setDraftDate(nextDate)
      setDraftTime(nextDate ? formatTime(nextDate) : defaultTime)
      setMonth(nextDate ?? new Date())
    }
    setOpen(nextOpen)
  }

  function handleDateSelect(nextDate: Date | undefined) {
    if (!nextDate) return
    setDraftDate(nextDate)
    onChange(formatLocalDateTime(nextDate, draftTime || defaultTime))
  }

  function handleTimeChange(nextTime: string) {
    setDraftTime(nextTime)
    if (draftDate && nextTime) onChange(formatLocalDateTime(draftDate, nextTime))
  }

  function clearValue() {
    setDraftDate(undefined)
    setDraftTime(defaultTime)
    onChange('')
    setOpen(false)
  }

  return (
    <Popover open={open} onOpenChange={handleOpenChange}>
      <PopoverTrigger asChild>
        <Button
          id={id}
          type="button"
          variant="secondary"
          disabled={disabled}
          aria-haspopup="dialog"
          aria-expanded={open}
          aria-describedby={ariaDescribedBy}
          className={cn('w-full min-w-0 justify-start gap-2 overflow-hidden rounded-xl bg-card/80 text-left font-medium', !parsedValue && 'text-muted-foreground', className)}
        >
          <CalendarDays className="size-4 shrink-0 text-primary" aria-hidden="true" />
          <span className="min-w-0 truncate">{parsedValue ? formatDisplayValue(parsedValue) : placeholder}</span>
        </Button>
      </PopoverTrigger>
      <PopoverContent align="start" collisionPadding={12} className="date-time-picker-popover w-[calc(100vw-2rem)] max-w-[22rem] p-3">
        <Calendar
          mode="single"
          selected={draftDate}
          onSelect={handleDateSelect}
          month={month}
          onMonthChange={setMonth}
          autoFocus
          aria-label="选择日期"
        />
        <div className="date-time-picker-time-row">
          <label htmlFor={timeID} className="date-time-picker-time-label">
            <Clock3 className="size-4" aria-hidden="true" />
            <span>时间</span>
          </label>
          <Select
            id={timeID}
            value={draftDate ? draftTime : ''}
            onChange={(event) => handleTimeChange(event.target.value)}
            disabled={!draftDate}
            aria-label="选择时间"
            className="h-10 flex-1 rounded-lg bg-background text-sm"
          >
            <option value="">先选择日期</option>
            {availableTimes.map((time) => <option key={time} value={time}>{time}</option>)}
          </Select>
        </div>
        <div className="date-time-picker-actions">
          <Button type="button" variant="ghost" size="sm" onClick={clearValue} disabled={!parsedValue}>
            <RotateCcw className="size-3.5" aria-hidden="true" />
            清除
          </Button>
          <Button type="button" size="sm" onClick={() => setOpen(false)}>
            <Check className="size-3.5" aria-hidden="true" />
            完成
          </Button>
        </div>
      </PopoverContent>
    </Popover>
  )
}
