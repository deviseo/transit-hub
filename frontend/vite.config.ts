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
  build: {
    // ECharts 与 zrender 存在循环模块图，合并后的延迟 vendor chunk 当前为约 572 kB。
    chunkSizeWarningLimit: 600,
    rollupOptions: {
      output: {
        manualChunks(id) {
          if (id.includes('node_modules/echarts/') || id.includes('node_modules/zrender/')) return 'echarts'
        },
      },
    },
  },
})
