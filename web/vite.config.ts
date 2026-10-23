import { defineConfig } from "vite";
import vue from "@vitejs/plugin-vue";
import { fileURLToPath, URL } from "node:url";
import { purgeVendoredCss } from "./scripts/purge-vendored-css";

// https://vite.dev/config/
export default defineConfig({
  plugins: [vue(), purgeVendoredCss()],
  resolve: {
    alias: {
      "@": fileURLToPath(new URL("./src", import.meta.url)),
    },
  },
  server: {
    proxy: {
      "/api": {
        target: "http://localhost:8080",
      },
    },
  },
  build: {
    commonjsOptions: {
      include: [/node_modules/],
    },
  },
});
