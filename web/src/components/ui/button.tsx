import { cva, type VariantProps } from 'class-variance-authority'
import { forwardRef, type ButtonHTMLAttributes } from 'react'

import { cn } from '../../lib/utils'

const buttonVariants = cva(
  'inline-flex min-h-10 items-center justify-center gap-2 whitespace-nowrap rounded-lg border border-transparent px-4 text-sm font-semibold leading-5 transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 disabled:pointer-events-none disabled:opacity-50',
  {
    variants: {
      variant: {
        default: 'bg-primary text-primary-foreground shadow-sm hover:bg-primary/90',
        secondary: 'border-border bg-background text-foreground shadow-sm hover:bg-accent',
        destructive: 'bg-destructive text-white shadow-sm hover:bg-destructive/90',
        ghost: 'bg-transparent text-muted-foreground hover:bg-accent hover:text-foreground',
      },
      size: {
        default: 'h-10 px-4',
        icon: 'size-10 p-0',
        sm: 'h-9 min-h-9 rounded-md px-3 text-sm',
      },
    },
    defaultVariants: {
      variant: 'default',
      size: 'default',
    },
  },
)

export interface ButtonProps
  extends ButtonHTMLAttributes<HTMLButtonElement>,
    VariantProps<typeof buttonVariants> {
  unstyled?: boolean
}

export const Button = forwardRef<HTMLButtonElement, ButtonProps>(
  ({ className, variant, size, unstyled, ...props }, ref) => (
    <button
      ref={ref}
      className={unstyled ? className : cn(buttonVariants({ variant, size }), className)}
      {...props}
    />
  ),
)
Button.displayName = 'Button'

export { buttonVariants }
