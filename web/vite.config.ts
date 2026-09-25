import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';
import tailwindcss from '@tailwindcss/vite';

export default defineConfig({
  plugins: [react(), tailwindcss()],
  base: './',
  build: {
    outDir: 'dist',
    emptyOutDir: true,
  },
  server: {
    port: 3000,
    proxy: {
      '/api': {
        target: 'http://localhost:9090',
        changeOrigin: true,
      },
      '/_healthz': {
        target: 'http://localhost:9090',
        changeOrigin: true,
      },
      '/_readyz': {
        target: 'http://localhost:9090',
        changeOrigin: true,
      },
    },
  },
});
