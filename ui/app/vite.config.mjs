import { defineConfig } from "vite";
import elm from "vite-plugin-elm";

export default defineConfig({
  base: "./",  // ensure that `--web.route.prefix` works correctly.
  plugins: [
    elm(),
  ],
});
