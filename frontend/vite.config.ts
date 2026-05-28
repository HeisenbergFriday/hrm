import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import { visualizer } from 'rollup-plugin-visualizer'

function packageChunkName(id: string, prefix: string) {
  const [, pkgPath] = id.split('/node_modules/')
  if (!pkgPath) return prefix

  const parts = pkgPath.split('/')
  const packageName = parts[0].startsWith('@')
    ? `${parts[0].slice(1)}-${parts[1] || 'pkg'}`
    : parts[0]

  return `${prefix}-${packageName.replace(/[^a-zA-Z0-9_-]/g, '-')}`
}

function antdComponentChunkName(id: string) {
  const match = id.match(/\/node_modules\/antd\/(?:es|lib)\/([^/]+)/)
  const component = match?.[1]
  if (!component) return 'vendor-antd-core'

  if (component === '_util' || component === 'style' || component === 'theme' || component === 'locale') {
    return 'vendor-antd-core'
  }

  return `vendor-antd-${component.replace(/[^a-zA-Z0-9_-]/g, '-')}`
}

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
            return antdComponentChunkName(normalizedId)
          }
          // antd 图标库
          if (normalizedId.includes('/node_modules/@ant-design/')) {
            if (
              normalizedId.includes('/node_modules/@ant-design/icons/') ||
              normalizedId.includes('/node_modules/@ant-design/icons-svg/')
            ) {
              return 'vendor-antd-icons'
            }
            return packageChunkName(normalizedId, 'vendor-ant-design')
          }
          if (
            normalizedId.includes('/node_modules/rc-') ||
            normalizedId.includes('/node_modules/@rc-component/')
          ) {
            return packageChunkName(normalizedId, 'vendor-rc')
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
          if (
            normalizedId.includes('/node_modules/@tanstack/react-query/') ||
            normalizedId.includes('/node_modules/axios/') ||
            normalizedId.includes('/node_modules/dayjs/') ||
            normalizedId.includes('/node_modules/zustand/')
          ) {
            return 'vendor-app'
          }
          // 其他第三方库
          return 'vendor-other'
        },
      },
    },
  },
})
