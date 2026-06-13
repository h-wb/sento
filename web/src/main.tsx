import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import { Notifications } from '@mantine/notifications'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { BrowserRouter } from 'react-router-dom'
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

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <ThemeProvider>
      <Notifications position="bottom-right" limit={4} />
      <QueryClientProvider client={queryClient}>
        <BrowserRouter>
          <App />
        </BrowserRouter>
      </QueryClientProvider>
    </ThemeProvider>
  </StrictMode>,
)
