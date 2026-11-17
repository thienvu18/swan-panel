import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import tailwindcss from '@tailwindcss/vite'

// Expose Vite on the LAN for development with real mobile/desktop browsers.
// Keep API requests same-origin so Authorization headers, SSE and cookies flow
// through the Vite proxy to the Air-managed Go backend on port 8080.
export default defineConfig({
  plugins: [react(), tailwindcss()],
  server: {
    host: '0.0.0.0',
    port: 3000,
    strictPort: true,
    watch: {
      usePolling: process.env.VITE_USE_POLLING === 'true',
    },
    proxy: {
      '/api': {
        target: 'http://127.0.0.1:8080',
        changeOrigin: false,
      },
    },
  },
  build: {
    outDir: 'dist',
    emptyOutDir: true,
    // Vite 8 uses Rolldown. Keep frequently reused dependencies in stable,
    // cacheable chunks instead of shipping them with the application entry.
    rolldownOptions: {
      output: {
        codeSplitting: {
          groups: [
            { name: 'react-vendor', test: /node_modules[\\/](react|react-dom|scheduler)[\\/]/, priority: 30 },
            { name: 'query-vendor', test: /node_modules[\\/]@tanstack[\\/]/, priority: 20 },
            { name: 'i18n-vendor', test: /node_modules[\\/](i18next|react-i18next|i18next-browser-languagedetector)[\\/]/, priority: 20 },
            { name: 'icons-vendor', test: /node_modules[\\/]lucide-react[\\/]/, priority: 20 },
          ],
        },
      },
    },
  },
})
