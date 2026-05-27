import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import { visualizer } from 'rollup-plugin-visualizer'

// https://vitejs.dev/config/
export default defineConfig({
  plugins: [
    react(),
    // 仅在 ANALYZE=true 时启用 bundle 分析
    process.env.ANALYZE === 'true' && visualizer({
      open: true,
      filename: 'dist/stats.html',
      gzipSize: true,
      brotliSize: true,
    }),
  ].filter(Boolean),
  server: {
    host: '0.0.0.0',
    port: 3000,
    strictPort: true,
    proxy: {
      '/api': {
        target: 'http://localhost:8080',
        changeOrigin: true,
        configure: (proxy) => {
          proxy.on('proxyReq', (proxyReq, req) => {
            if (req.headers.host) {
              proxyReq.setHeader('X-Forwarded-Host', req.headers.host)
            }

            const protoHeader = req.socket.encrypted ? 'https' : 'http'
            proxyReq.setHeader('X-Forwarded-Proto', protoHeader)
          })
        },
      }
    }
  },
  build: {
    rollupOptions: {
      output: {
        manualChunks(id) {
          if (!id.includes('node_modules')) return

          const normalizedId = id.replace(/\\/g, '/')

          // antd 核心组件
          if (normalizedId.includes('/node_modules/antd/')) {
            return 'vendor-antd'
          }
          // antd 图标库
          if (normalizedId.includes('/node_modules/@ant-design/')) {
            return 'vendor-antd-icons'
          }
          // React 核心（精确匹配，避免误伤 react-query 等）
          if (
            normalizedId.includes('/node_modules/react/') ||
            normalizedId.includes('/node_modules/react-dom/') ||
            normalizedId.includes('/node_modules/react-router/') ||
            normalizedId.includes('/node_modules/react-router-dom/')
          ) {
            return 'vendor-react'
          }
          // 其他第三方库
          return 'vendor-other'
        },
      },
    },
  },
})
