import { fireEvent, render, screen } from '@testing-library/react'
import { useState } from 'react'
import { describe, expect, it } from 'vitest'

import { Button } from './button'
import { Dialog, DialogClose, DialogContent, DialogTitle } from './dialog'

function Example() {
  const [open, setOpen] = useState(true)
  return (
    <Dialog open={open} onOpenChange={setOpen}>
      <DialogContent>
        <DialogTitle>确认操作</DialogTitle>
        <DialogClose asChild><Button>关闭</Button></DialogClose>
      </DialogContent>
    </Dialog>
  )
}

describe('Dialog', () => {
  it('delegates Escape and focus management to Radix', () => {
    render(<Example />)
    const dialog = screen.getByRole('dialog', { name: '确认操作' })

    expect(dialog).toBeInTheDocument()
    fireEvent.keyDown(document, { key: 'Escape' })
    expect(screen.queryByRole('dialog', { name: '确认操作' })).not.toBeInTheDocument()
  })
})
