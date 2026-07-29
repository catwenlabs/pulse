import { Toaster as SonnerToaster, type ToasterProps } from 'sonner'

export function Toaster(props: ToasterProps) {
  return (
    <SonnerToaster
      closeButton
      position="top-right"
      richColors
      toastOptions={{
        classNames: {
          toast: 'font-sans',
          title: 'text-sm font-semibold',
          description: 'text-sm text-muted-foreground',
          closeButton: 'border-border bg-background text-muted-foreground hover:text-foreground',
        },
      }}
      {...props}
    />
  )
}
