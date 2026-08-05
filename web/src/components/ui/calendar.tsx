import { ChevronLeft, ChevronRight } from 'lucide-react'
import { DayPicker, getDefaultClassNames, type ChevronProps, type DayPickerProps } from 'react-day-picker'
import { zhCN } from 'date-fns/locale'

import { cn } from '../../lib/utils'

function CalendarChevron({ orientation, className, size = 16 }: ChevronProps) {
  if (orientation === 'left') return <ChevronLeft className={className} size={size} aria-hidden="true" />
  if (orientation === 'right') return <ChevronRight className={className} size={size} aria-hidden="true" />
  return <ChevronRight className={className} size={size} aria-hidden="true" />
}

export function Calendar({ className, classNames, components, showOutsideDays = true, ...props }: DayPickerProps) {
  const defaultClassNames = getDefaultClassNames()

  return (
    <DayPicker
      {...props}
      locale={zhCN}
      weekStartsOn={1}
      showOutsideDays={showOutsideDays}
      className={cn('p-1', className)}
      classNames={{
        ...classNames,
        root: cn('text-sm', defaultClassNames.root, classNames?.root),
        months: cn('flex flex-col gap-4', defaultClassNames.months, classNames?.months),
        month: cn('flex flex-col gap-4', defaultClassNames.month, classNames?.month),
        month_caption: cn('relative flex h-8 items-center justify-center', defaultClassNames.month_caption, classNames?.month_caption),
        caption_label: cn('text-sm font-semibold', defaultClassNames.caption_label, classNames?.caption_label),
        nav: cn('absolute inset-x-0 top-0 flex items-center justify-between', defaultClassNames.nav, classNames?.nav),
        button_previous: cn('grid size-8 place-items-center rounded-lg border border-transparent text-muted-foreground hover:bg-accent hover:text-foreground disabled:pointer-events-none disabled:opacity-40', defaultClassNames.button_previous, classNames?.button_previous),
        button_next: cn('grid size-8 place-items-center rounded-lg border border-transparent text-muted-foreground hover:bg-accent hover:text-foreground disabled:pointer-events-none disabled:opacity-40', defaultClassNames.button_next, classNames?.button_next),
        month_grid: cn('w-full border-collapse', defaultClassNames.month_grid, classNames?.month_grid),
        weekdays: cn('flex', defaultClassNames.weekdays, classNames?.weekdays),
        weekday: cn('w-9 rounded-md text-[0.72rem] font-medium text-muted-foreground', defaultClassNames.weekday, classNames?.weekday),
        week: cn('mt-1 flex w-full', defaultClassNames.week, classNames?.week),
        day: cn('relative size-9 p-0 text-center text-sm focus-within:z-10', defaultClassNames.day, classNames?.day),
        day_button: cn('grid size-9 place-items-center rounded-lg border border-transparent p-0 font-normal text-foreground hover:bg-accent hover:text-accent-foreground focus-visible:z-10 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring', defaultClassNames.day_button, classNames?.day_button),
        selected: cn('[&>.rdp-day_button]:bg-primary [&>.rdp-day_button]:text-primary-foreground [&>.rdp-day_button]:hover:bg-primary [&>.rdp-day_button]:hover:text-primary-foreground', defaultClassNames.selected, classNames?.selected),
        today: cn('[&>.rdp-day_button]:bg-accent [&>.rdp-day_button]:font-semibold [&>.rdp-day_button]:text-accent-foreground', defaultClassNames.today, classNames?.today),
        outside: cn('text-muted-foreground opacity-50', defaultClassNames.outside, classNames?.outside),
        disabled: cn('text-muted-foreground opacity-50', defaultClassNames.disabled, classNames?.disabled),
        hidden: cn('invisible', defaultClassNames.hidden, classNames?.hidden),
      }}
      components={{ Chevron: CalendarChevron, ...components }}
    />
  )
}
