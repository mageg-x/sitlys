import { defineConfig, loadEnv } from "vite";
import vue from "@vitejs/plugin-vue";

export default defineConfig(({ mode }) => {
  const env = loadEnv(mode, process.cwd(), "");
  const backendTarget = env.SITLYS_BACKEND_URL || "http://127.0.0.1:8080";

  return {
    plugins: [vue()],
    server: {
      proxy: {
        "/api": {
          target: backendTarget,
          changeOrigin: true,
        },
        "/tracker.js": {
          target: backendTarget,
          changeOrigin: true,
        },
        "/collect": {
          target: backendTarget,
          changeOrigin: true,
        },
        "/healthz": {
          target: backendTarget,
          changeOrigin: true,
        },
      },
    },
    build: {
      outDir: "../server/embed",
      emptyOutDir: true,
      cssCodeSplit: false,
      rollupOptions: {
        output: {
          entryFileNames: "assets/app.js",
          chunkFileNames: "assets/[name].js",
          assetFileNames: assetInfo => {
            if (assetInfo.name && assetInfo.name.endsWith(".css")) {
              return "assets/style.css";
            }
            return "assets/[name][extname]";
          },
        },
      },
    },
  };
});
