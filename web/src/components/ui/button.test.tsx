import { render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'

import { Button, buttonVariants } from './button'

describe('Button', () => {
  it('merges variants with responsive layout classes', () => {
    render(<Button className="max-md:flex-1">保存</Button>)

    expect(screen.getByRole('button', { name: '保存' })).toHaveClass(
      'bg-primary',
      'max-md:flex-1',
    )
    expect(buttonVariants({ variant: 'secondary', className: 'max-md:flex-1' })).toContain(
      'max-md:flex-1',
    )
  })
})
