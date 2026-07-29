import { fireEvent, render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'

import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuTrigger } from './dropdown-menu'

describe('DropdownMenu', () => {
  it('provides menu semantics and Escape handling', () => {
    render(
      <DropdownMenu>
        <DropdownMenuTrigger>更多操作</DropdownMenuTrigger>
        <DropdownMenuContent>
          <DropdownMenuItem>收藏文章</DropdownMenuItem>
        </DropdownMenuContent>
      </DropdownMenu>,
    )

    fireEvent.pointerDown(screen.getByRole('button', { name: '更多操作' }), { button: 0 })
    expect(screen.getByRole('menuitem', { name: '收藏文章' })).toBeInTheDocument()
    fireEvent.keyDown(document, { key: 'Escape' })
    expect(screen.queryByRole('menuitem', { name: '收藏文章' })).not.toBeInTheDocument()
  })
})
