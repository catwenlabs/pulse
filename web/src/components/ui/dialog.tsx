import * as DialogPrimitive from '@radix-ui/react-dialog'
import type { ComponentProps } from 'react'

import { cn } from '../../lib/utils'

export const Dialog = DialogPrimitive.Root
export const DialogClose = DialogPrimitive.Close
export const DialogTitle = DialogPrimitive.Title
export const DialogDescription = DialogPrimitive.Description

export function DialogContent({
  className,
  children,
  ...props
}: ComponentProps<typeof DialogPrimitive.Content>) {
  return (
    <DialogPrimitive.Portal>
      <DialogPrimitive.Overlay className="dialog-backdrop">
        <DialogPrimitive.Content className={cn('dialog', className)} {...props}>
          {children}
        </DialogPrimitive.Content>
      </DialogPrimitive.Overlay>
    </DialogPrimitive.Portal>
  )
}

export function SheetContent({
  children,
  persistent = false,
  ...props
}: ComponentProps<typeof DialogPrimitive.Content> & { persistent?: boolean }) {
  if (persistent) return children
  return (
    <DialogPrimitive.Portal>
      <DialogPrimitive.Overlay className="mobile-drawer-backdrop" />
      <DialogPrimitive.Content asChild {...props}>
        {children}
      </DialogPrimitive.Content>
    </DialogPrimitive.Portal>
  )
}
