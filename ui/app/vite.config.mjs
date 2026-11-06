import { defineConfig } from "vite";
import elm from "vite-plugin-elm";
import { compression, defineAlgorithm } from "vite-plugin-compression2";

export default defineConfig({
  base: "./",  // ensure that `--web.route.prefix` works correctly.
  build: {
    outDir: "dist/elm",
  },
  plugins: [
    elm(),
    compression({
      include: [/\.(eot|ttf|ico|js|mjs|json|css|svg)$/],
      threshold: 0,
      deleteOriginalAssets: true,
      algorithms: [
        defineAlgorithm("gzip", { level: 9 }),
        defineAlgorithm("brotliCompress", {
          params: { 1: 11 }, // zlib.constants.BROTLI_PARAM_QUALITY = 1
        }),
      ],
    }),
    // HTML is only gzip-compressed, since proxies that rewrite HTML, such as
    // the Kubernetes API server proxy, do not support Brotli.
    compression({
      include: [/\.html$/],
      threshold: 0,
      deleteOriginalAssets: true,
      algorithms: [defineAlgorithm("gzip", { level: 9 })],
    }),
  ],
});
