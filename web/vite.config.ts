import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

// https://vite.dev/config/
export default defineConfig({
  plugins: [react()],
  server: {
    proxy: {
      // dev server proxies API calls to the Go backend
      '/api': 'http://localhost:8484',
    },
  },
})
