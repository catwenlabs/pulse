import { useCallback, useEffect, useRef, useState, type CSSProperties, type ReactNode } from 'react'

// Reader typography preferences, Apple-Books style: the reader exposes font
// family, font size, line height, and letter spacing for chapter content.
// Values live per browser in localStorage; every field is validated on load
// so corrupted storage falls back to defaults instead of breaking the reader.

const STORAGE_KEY = 'pulse.reader-typography'

export interface FontOption { id: string; label: string; stack: string }
export interface SpacingOption { id: string; label: string; value: number }

export const FONT_FAMILIES: readonly FontOption[] = [
  { id: 'serif', label: '宋体', stack: 'var(--font-serif)' },
  { id: 'sans', label: '黑体', stack: 'var(--font-sans)' },
  { id: 'kai', label: '楷体', stack: '"Kaiti SC", "STKaiti", "KaiTi", "Noto Serif CJK SC", serif' },
]

export const LINE_HEIGHTS: readonly SpacingOption[] = [
  { id: 'compact', label: '紧凑', value: 1.5 },
  { id: 'normal', label: '标准', value: 1.75 },
  { id: 'loose', label: '宽松', value: 2.05 },
]

export const LETTER_SPACINGS: readonly SpacingOption[] = [
  { id: 'tight', label: '紧凑', value: -0.01 },
  { id: 'normal', label: '标准', value: 0 },
  { id: 'wide', label: '宽松', value: 0.04 },
]

export const FONT_SIZE_MIN = 14
export const FONT_SIZE_MAX = 24

export interface ReaderTypography {
  fontFamily: string
  fontSize: number
  lineHeight: number
  letterSpacing: number
}

export const defaultReaderTypography: ReaderTypography = {
  fontFamily: FONT_FAMILIES[0].id,
  fontSize: 17,
  lineHeight: LINE_HEIGHTS[1].value,
  letterSpacing: LETTER_SPACINGS[1].value,
}

function findSpacing(options: readonly SpacingOption[], value: unknown): number | null {
  for (const option of options) {
    if (option.value === value) return option.value
  }
  return null
}

// loadReaderTypography merges stored preferences over defaults. Unknown or
// out-of-range fields drop back per-field, so a partially corrupted entry
// keeps its valid parts.
export function loadReaderTypography(storage: Storage | null = safeStorage()): ReaderTypography {
  const stored = defaultReaderTypography
  if (!storage) return stored
  let parsed: unknown
  try {
    parsed = JSON.parse(storage.getItem(STORAGE_KEY) ?? '')
  } catch {
    return stored
  }
  if (typeof parsed !== 'object' || parsed === null) return stored
  const candidate = parsed as Record<string, unknown>

  const fontFamily = FONT_FAMILIES.some((option) => option.id === candidate.fontFamily)
    ? String(candidate.fontFamily)
    : stored.fontFamily
  const fontSize = Number.isInteger(candidate.fontSize)
    && Number(candidate.fontSize) >= FONT_SIZE_MIN
    && Number(candidate.fontSize) <= FONT_SIZE_MAX
    ? Number(candidate.fontSize)
    : stored.fontSize
  const lineHeight = findSpacing(LINE_HEIGHTS, candidate.lineHeight) ?? stored.lineHeight
  const letterSpacing = findSpacing(LETTER_SPACINGS, candidate.letterSpacing) ?? stored.letterSpacing

  return { fontFamily, fontSize, lineHeight, letterSpacing }
}

function safeStorage(): Storage | null {
  try {
    return window.localStorage
  } catch {
    return null
  }
}

export interface ReaderTypographyControls {
  typography: ReaderTypography
  update: (patch: Partial<ReaderTypography>) => void
  reset: () => void
}

export function useReaderTypography(): ReaderTypographyControls {
  const [typography, setTypography] = useState<ReaderTypography>(() => loadReaderTypography())

  const update = useCallback((patch: Partial<ReaderTypography>) => {
    setTypography((current) => {
      const next = { ...current, ...patch }
      try {
        window.localStorage.setItem(STORAGE_KEY, JSON.stringify(next))
      } catch {
        // Persistence is best-effort: a blocked or full store must not
        // break in-session reading.
      }
      return next
    })
  }, [])

  const reset = useCallback(() => update(defaultReaderTypography), [update])

  return { typography, update, reset }
}

// readerTypographyStyle turns preferences into the inline style applied to
// each chapter's content container; inline values own the reading surface,
// overriding the .document-content stylesheet defaults.
export function readerTypographyStyle(typography: ReaderTypography): CSSProperties {
  const family = FONT_FAMILIES.find((option) => option.id === typography.fontFamily) ?? FONT_FAMILIES[0]
  return {
    fontFamily: family.stack,
    fontSize: `${typography.fontSize}px`,
    lineHeight: typography.lineHeight,
    letterSpacing: `${typography.letterSpacing}em`,
  }
}

const optionButtonClass = (selected: boolean): string =>
  `cursor-pointer rounded-md border px-2.5 py-1 text-xs transition-colors ${
    selected
      ? 'border-primary/60 bg-primary/5 font-medium text-primary-hover'
      : 'border-border bg-transparent text-muted-foreground hover:bg-accent'
  }`

function OptionRow({ label, children }: { label: string; children: ReactNode }) {
  return (
    <div className="flex items-center justify-between gap-4">
      <span className="w-12 shrink-0 text-xs text-muted-foreground">{label}</span>
      {children}
    </div>
  )
}

// ReaderTypographyPanel renders the settings card body: a font-size stepper
// plus segmented font, line-height, and letter-spacing pickers. It lives
// inside the ReaderTypographyButton popover.
export function ReaderTypographyPanel({ controls }: { controls: ReaderTypographyControls }) {
  const { typography, update, reset } = controls
  const spacingButton = (options: readonly SpacingOption[], current: number, pick: (value: number) => void) => (
    <div role="group" aria-label={options === LINE_HEIGHTS ? '行间距' : '字间距'} className="flex gap-1">
      {options.map((option) => (
        <button
          key={option.id}
          type="button"
          aria-pressed={option.value === current}
          className={optionButtonClass(option.value === current)}
          onClick={() => pick(option.value)}
        >
          {option.label}
        </button>
      ))}
    </div>
  )

  return (
    <div className="space-y-3">
      <OptionRow label="字号">
        <div role="group" aria-label="字号" className="flex items-center gap-2">
          <button
            type="button"
            aria-label="减小字号"
            className="cursor-pointer rounded-md border border-border px-2 py-1 text-xs text-muted-foreground hover:bg-accent disabled:cursor-not-allowed disabled:opacity-40"
            disabled={typography.fontSize <= FONT_SIZE_MIN}
            onClick={() => update({ fontSize: Math.max(FONT_SIZE_MIN, typography.fontSize - 1) })}
          >
            A-
          </button>
          <span className="w-10 text-center text-sm tabular-nums">{typography.fontSize}</span>
          <button
            type="button"
            aria-label="增大字号"
            className="cursor-pointer rounded-md border border-border px-2 py-1 text-xs text-muted-foreground hover:bg-accent disabled:cursor-not-allowed disabled:opacity-40"
            disabled={typography.fontSize >= FONT_SIZE_MAX}
            onClick={() => update({ fontSize: Math.min(FONT_SIZE_MAX, typography.fontSize + 1) })}
          >
            A+
          </button>
        </div>
      </OptionRow>
      <OptionRow label="字体">
        <div role="group" aria-label="字体" className="flex gap-1">
          {FONT_FAMILIES.map((option) => (
            <button
              key={option.id}
              type="button"
              aria-pressed={option.id === typography.fontFamily}
              className={optionButtonClass(option.id === typography.fontFamily)}
              onClick={() => update({ fontFamily: option.id })}
            >
              {option.label}
            </button>
          ))}
        </div>
      </OptionRow>
      <OptionRow label="行间距">
        {spacingButton(LINE_HEIGHTS, typography.lineHeight, (value) => update({ lineHeight: value }))}
      </OptionRow>
      <OptionRow label="字间距">
        {spacingButton(LETTER_SPACINGS, typography.letterSpacing, (value) => update({ letterSpacing: value }))}
      </OptionRow>
      <div className="flex justify-end border-t border-border pt-2">
        <button
          type="button"
          className="cursor-pointer rounded-md border-0 bg-transparent px-2 py-1 text-xs text-primary-hover hover:underline"
          onClick={reset}
        >
          恢复默认
        </button>
      </div>
    </div>
  )
}

// ReaderTypographyButton is the always-reachable reading control: a floating
// "Aa" button pinned to the bottom-right of the reader that opens the
// typography popover above it. Stays z-30 so the z-40 selection toolbar
// covers it while text is selected. Closes on Escape and on outside pointer
// presses; scrolling keeps it open so changes apply live.
export function ReaderTypographyButton({ controls }: { controls: ReaderTypographyControls }) {
  const [open, setOpen] = useState(false)
  const rootRef = useRef<HTMLDivElement>(null)

  useEffect(() => {
    if (!open) return
    const onPointerDown = (event: PointerEvent) => {
      if (rootRef.current && event.target instanceof Node && !rootRef.current.contains(event.target)) {
        setOpen(false)
      }
    }
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key === 'Escape') setOpen(false)
    }
    document.addEventListener('pointerdown', onPointerDown)
    document.addEventListener('keydown', onKeyDown)
    return () => {
      document.removeEventListener('pointerdown', onPointerDown)
      document.removeEventListener('keydown', onKeyDown)
    }
  }, [open])

  return (
    <div ref={rootRef} className="fixed bottom-6 right-6 z-30 max-md:bottom-[max(1rem,env(safe-area-inset-bottom))] max-md:right-4">
      {open && (
        <div
          role="dialog"
          aria-label="排版设置"
          className="absolute bottom-14 right-0 w-[min(320px,calc(100vw-2rem))] rounded-xl border border-border bg-card p-4 shadow-[0_18px_50px_rgba(32,29,23,.22)]"
        >
          <ReaderTypographyPanel controls={controls} />
        </div>
      )}
      <button
        type="button"
        aria-label="排版设置"
        aria-expanded={open}
        className="grid size-11 cursor-pointer place-items-center rounded-full border border-border bg-card text-sm font-semibold text-muted-foreground shadow-[0_8px_24px_rgba(32,29,23,.18)] transition-colors hover:bg-accent hover:text-primary-hover"
        onClick={() => setOpen((value) => !value)}
      >
        Aa
      </button>
    </div>
  )
}
