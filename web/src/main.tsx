import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import { Notifications } from '@mantine/notifications'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { createBrowserRouter, RouterProvider } from 'react-router-dom'
import { UnsavedChangesProvider } from './UnsavedChangesProvider'
import '@mantine/core/styles.css'
import '@mantine/notifications/styles.css'
import '@mantine/spotlight/styles.css'
import '@mantine/charts/styles.css'
import '@mantine/nprogress/styles.css'
import './index.css'
import App from './App.tsx'
import { ThemeProvider } from './ThemeProvider'

const queryClient = new QueryClient({
  defaultOptions: {
    queries: { retry: 1, refetchOnWindowFocus: false },
  },
})

const router = createBrowserRouter([{ path: '*', element: <UnsavedChangesProvider><App /></UnsavedChangesProvider> }])

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <ThemeProvider>
      <Notifications position="bottom-right" limit={4} />
      <QueryClientProvider client={queryClient}>
        <RouterProvider router={router} />
      </QueryClientProvider>
    </ThemeProvider>
  </StrictMode>,
)
