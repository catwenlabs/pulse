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
      <DialogPrimitive.Overlay className="fixed inset-0 z-50 grid place-items-center bg-black/50 p-4 backdrop-blur-sm data-[state=closed]:animate-out data-[state=open]:animate-in">
        <DialogPrimitive.Content
          className={cn(
            'relative max-h-[calc(100dvh-2rem)] w-full max-w-lg overflow-y-auto rounded-xl border bg-background p-7 text-base leading-6 text-foreground shadow-lg outline-none max-md:p-5',
            '[&_form]:grid [&_form]:gap-5 [&_label]:grid [&_label]:gap-2 [&_label]:text-sm [&_label]:font-semibold',
            '[&_[role=alert]]:text-destructive [&_h2]:text-2xl [&_h2]:font-semibold [&_h2]:leading-tight',
            className,
          )}
          {...props}
        >
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
      <DialogPrimitive.Overlay className="fixed inset-0 z-20 bg-black/50 backdrop-blur-sm" />
      <DialogPrimitive.Content asChild {...props}>
        {children}
      </DialogPrimitive.Content>
    </DialogPrimitive.Portal>
  )
}
