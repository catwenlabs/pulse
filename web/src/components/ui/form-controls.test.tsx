import { render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'

import { Select } from './select'
import { Textarea } from './textarea'

describe('form and surface primitives', () => {
  it('exposes consistent Tailwind focus and surface contracts', () => {
    render(
      <div data-testid="surface" className="rounded-xl border border-border bg-card">
        <Select aria-label="来源"><option>RSS</option></Select>
        <Textarea aria-label="笔记" />
      </div>,
    )

    expect(screen.getByTestId('surface')).toHaveClass('border-border', 'bg-card')
    expect(screen.getByRole('combobox', { name: '来源' })).toHaveClass('focus-visible:ring-3')
    expect(screen.getByRole('textbox', { name: '笔记' })).toHaveClass('resize-y')
  })
})
