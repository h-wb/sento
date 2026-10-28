import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

// https://vite.dev/config/
export default defineConfig({
  plugins: [react()],
  server: {
    proxy: {
      // dev server proxies API calls to the Go backend (SENTO_API_URL when
      // the backend isn't on its default port, e.g. a Paseo worktree service)
      '/api': process.env.SENTO_API_URL ?? 'http://localhost:8484',
    },
  },
})
