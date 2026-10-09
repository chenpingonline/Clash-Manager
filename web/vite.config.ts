import { fileURLToPath, URL } from 'node:url'
import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'

export default defineConfig({
  base: process.env.VITE_APP_BASE || '/',
  plugins: [vue(), {
    name: 'deployment-title',
    transformIndexHtml(html) {
      return !process.env.VITE_APP_BASE || process.env.VITE_APP_BASE === '/'
        ? html.replace('<title>Clash for fnos</title>', '<title>Clash Manager</title>')
        : html
    },
  }],
  resolve: {
    alias: {
      '@': fileURLToPath(new URL('./src', import.meta.url)),
    },
  },
  build: {
    outDir: 'dist',
    emptyOutDir: true,
  },
  server: {
    host: '127.0.0.1',
    port: 5173,
  },
})
