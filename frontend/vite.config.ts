/// <reference types="vitest/config" />
import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'

// O build vai para dentro do backend, que embute os arquivos no binário.
export default defineConfig({
  plugins: [vue()],
  build: {
    outDir: '../backend/internal/web/dist/app',
    emptyOutDir: true,
  },
  server: {
    port: 5180,
    proxy: { '/api': 'http://127.0.0.1:8095' },
  },
  test: {
    environment: 'jsdom',
  },
})
