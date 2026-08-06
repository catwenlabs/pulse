import { render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'

import { ChatMarkdown, sanitizeUrl } from './ChatMarkdown'

describe('ChatMarkdown', () => {
  it('renders fenced code and inline code', () => {
    render(<ChatMarkdown content={'```\nconst x = 1\n```\nUse `y`.'} />)
    expect(screen.getByText('const x = 1')).toBeInTheDocument()
    expect(screen.getByText('y')).toBeInTheDocument()
  })

  it('renders inline and block LaTeX without stripping formulas', () => {
    const { container } = render(<ChatMarkdown content={'Inline $E=mc^2$ and block:\n\n$$a^2 + b^2 = c^2$$'} />)
    // KaTeX renders math into elements with the katex class.
    expect(container.querySelectorAll('.katex').length).toBeGreaterThan(0)
  })

  it('does not render raw model-supplied HTML', () => {
    const { container } = render(<ChatMarkdown content={'<img src=x onerror="alert(1)">text'} />)
    const img = container.querySelector('img')
    expect(img).toBeNull()
    expect(container.textContent).toContain('text')
  })

  it('drops javascript: links and keeps safe links', () => {
    const { container } = render(<ChatMarkdown content={'[safe](https://example.com) and [bad](javascript:alert(1))'} />)
    const links = container.querySelectorAll('a')
    expect(links).toHaveLength(1)
    expect(links[0].getAttribute('href')).toBe('https://example.com')
  })

  it('sanitizeUrl blocks dangerous schemes', () => {
    expect(sanitizeUrl('javascript:alert(1)')).toBeUndefined()
    expect(sanitizeUrl('data:text/html,<script>')).toBeUndefined()
    expect(sanitizeUrl('https://example.com')).toBe('https://example.com')
    expect(sanitizeUrl('/relative/path')).toBe('/relative/path')
  })
})
