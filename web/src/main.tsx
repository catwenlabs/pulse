import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'

import { RouterProvider } from '@tanstack/react-router'

import { router } from './router'

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <RouterProvider router={router} />
  </StrictMode>,
)

if ('serviceWorker' in navigator && import.meta.env.PROD) {
  void navigator.serviceWorker.register('/service-worker.js')
}
