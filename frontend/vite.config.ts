import path from 'node:path'
import vue from '@vitejs/plugin-vue'
import { defineConfig } from 'vite'

export default defineConfig({
  plugins: [vue()],
  resolve: {
    alias: {
      '@': path.resolve(__dirname, './src'),
    },
  },
  server: {
    port: 5444,
    proxy: {
      '/api': {
        target: 'http://127.0.0.1:5555',
        changeOrigin: true,
      },
    },
  },
})
