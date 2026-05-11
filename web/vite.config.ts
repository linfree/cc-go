import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import tailwindcss from '@tailwindcss/vite'

const BACKEND_PORT = process.env.BACKEND_PORT || '18080'

export default defineConfig({
  plugins: [tailwindcss(), react()],
  server: {
    proxy: {
      '/api': `http://localhost:${BACKEND_PORT}`,
      '/ws': {
        target: `ws://localhost:${BACKEND_PORT}`,
        ws: true,
      },
    },
  },
  build: {
    outDir: '../cmd/cc-go/web-dist',
    emptyOutDir: true,
  },
})