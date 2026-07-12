import { defineConfig } from "vite";
import vue from "@vitejs/plugin-vue";
import wails from "@wailsio/runtime/plugins/vite";
import path from "path";

// https://vitejs.dev/config/
export default defineConfig({
  plugins: [vue(), wails("./bindings")],
  resolve: {
    alias: {
      "@": path.resolve(__dirname, "src"),
      "@bindings": path.resolve(__dirname, "bindings"),
    },
  },
  build: {
    rollupOptions: {
      output: {
        manualChunks: {
          // 将大体积第三方库拆分为独立 chunk，减小主包体积、改善缓存
          xterm: ['xterm', 'xterm-addon-fit', 'xterm-addon-search', 'xterm-addon-web-links'],
          highlight: ['highlight.js'],
          markdown: ['marked'],
        }
      }
    }
  }
});
