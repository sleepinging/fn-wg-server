import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import { viteSingleFile } from 'vite-plugin-singlefile'

// 使用 vite-plugin-singlefile 将所有 JS/CSS 内联到 index.html
// 这样只需要一次 HTTP 请求就能加载整个前端（解决 CGI 模式下资源文件路由问题）
export default defineConfig({
  plugins: [react(), viteSingleFile()],
  build: {
    outDir: '../ui',
    emptyOutDir: true,
    sourcemap: false,
    minify: false,          // 不压缩混淆，便于阅读错误栈
    cssCodeSplit: false,
    assetsInlineLimit: 100000000,
  },
  server: {
    proxy: {
      '/api': {
        target: 'http://localhost:8080',
        changeOrigin: true,
      },
    },
  },
})
