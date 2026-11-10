import { fireEvent, render, screen, within } from '@testing-library/react'
import { beforeEach, describe, expect, it } from 'vitest'

import {
  ReaderTypographyPanel,
  defaultReaderTypography,
  loadReaderTypography,
  readerTypographyStyle,
  useReaderTypography,
} from './readerTypography'

const STORAGE_KEY = 'pulse.reader-typography'

// Harness renders the panel against the real hook and a style probe so
// preference changes are observable as the styles the reader would apply.
function Harness() {
  const controls = useReaderTypography()
  return (
    <div>
      <div data-testid="style-probe" style={readerTypographyStyle(controls.typography)} />
      <ReaderTypographyPanel controls={controls} />
    </div>
  )
}

beforeEach(() => {
  localStorage.clear()
})

describe('loadReaderTypography', () => {
  it('returns defaults when nothing is stored', () => {
    expect(loadReaderTypography()).toEqual(defaultReaderTypography)
  })

  it('returns defaults on corrupted storage', () => {
    localStorage.setItem(STORAGE_KEY, '{not json')
    expect(loadReaderTypography()).toEqual(defaultReaderTypography)
  })

  it('loads a valid stored value', () => {
    localStorage.setItem(STORAGE_KEY, JSON.stringify({
      fontFamily: 'kai', fontSize: 20, lineHeight: 2.05, letterSpacing: 0.04,
    }))
    expect(loadReaderTypography()).toEqual({
      fontFamily: 'kai', fontSize: 20, lineHeight: 2.05, letterSpacing: 0.04,
    })
  })

  it('falls back per field on invalid values', () => {
    localStorage.setItem(STORAGE_KEY, JSON.stringify({
      fontFamily: 'comic-sans', fontSize: 99, lineHeight: 1.234, letterSpacing: 'wide',
    }))
    expect(loadReaderTypography()).toEqual(defaultReaderTypography)
  })
})

describe('ReaderTypographyPanel', () => {
  it('adjusts font size within bounds and persists it', () => {
    render(<Harness />)
    const probe = screen.getByTestId('style-probe') as HTMLElement

    fireEvent.click(screen.getByRole('button', { name: '增大字号' }))
    fireEvent.click(screen.getByRole('button', { name: '增大字号' }))
    expect(probe.style.fontSize).toBe('19px')
    expect(JSON.parse(String(localStorage.getItem(STORAGE_KEY))).fontSize).toBe(19)

    fireEvent.click(screen.getByRole('button', { name: '减小字号' }))
    expect(probe.style.fontSize).toBe('18px')
  })

  it('switches font family and marks the active option', () => {
    render(<Harness />)
    const probe = screen.getByTestId('style-probe') as HTMLElement

    fireEvent.click(screen.getByRole('button', { name: '楷体' }))
    expect(probe.style.fontFamily).toContain('Kaiti SC')
    expect(screen.getByRole('button', { name: '楷体' }).getAttribute('aria-pressed')).toBe('true')
    expect(screen.getByRole('button', { name: '宋体' }).getAttribute('aria-pressed')).toBe('false')
  })

  it('changes line height and letter spacing', () => {
    render(<Harness />)
    const probe = screen.getByTestId('style-probe') as HTMLElement

    fireEvent.click(within(screen.getByRole('group', { name: '行间距' })).getByRole('button', { name: '宽松' }))
    expect(probe.style.lineHeight).toBe('2.05')

    fireEvent.click(within(screen.getByRole('group', { name: '字间距' })).getByRole('button', { name: '紧凑' }))
    expect(probe.style.letterSpacing).toBe('-0.01em')
  })

  it('restores defaults from the reset button', () => {
    render(<Harness />)
    const probe = screen.getByTestId('style-probe') as HTMLElement
    fireEvent.click(screen.getByRole('button', { name: '增大字号' }))
    fireEvent.click(screen.getByRole('button', { name: '黑体' }))

    fireEvent.click(screen.getByRole('button', { name: '恢复默认' }))

    expect(probe.style.fontSize).toBe(`${defaultReaderTypography.fontSize}px`)
    expect(screen.getByRole('button', { name: '宋体' }).getAttribute('aria-pressed')).toBe('true')
    expect(JSON.parse(String(localStorage.getItem(STORAGE_KEY)))).toEqual(defaultReaderTypography)
  })
})

