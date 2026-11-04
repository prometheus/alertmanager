import path from 'node:path';

import react from '@vitejs/plugin-react';
import license from 'rollup-plugin-license';
import { defineConfig } from 'vite';
import { compression, defineAlgorithm } from 'vite-plugin-compression2';

const licenseFile = path.resolve(
  import.meta.dirname,
  'node_modules/.cache/alertmanager-third-party-licenses.txt'
);

export default defineConfig({
  base: './',
  // Kept in sync with `compilerOptions.paths` in tsconfig.json, which only
  // teaches tsc about these.
  resolve: {
    alias: {
      '@': path.resolve(import.meta.dirname, 'src'),
      '@test-utils': path.resolve(import.meta.dirname, 'test-utils'),
    },
  },
  build: {
    outDir: '../app/dist/mantine',
    emptyOutDir: true,
  },
  plugins: [
    react(),
    license({
      thirdParty: {
        includePrivate: false,
        output: {
          file: licenseFile,
        },
      },
    }),
    compression({
      include: [/\.(css|js|txt)$/],
      artifacts: () => [
        {
          src: licenseFile,
          replace: (destination) => path.join(destination, 'assets/third-party-licenses.txt'),
        },
      ],
      threshold: 0,
      deleteOriginalAssets: true,
      skipIfLargerOrEqual: false,
      algorithms: [
        defineAlgorithm('gzip', { level: 9 }),
        defineAlgorithm('brotliCompress', {
          params: { 1: 11 },
        }),
      ],
    }),
    // HTML is only gzip-compressed, since proxies that rewrite HTML, such as
    // the Kubernetes API server proxy, do not support Brotli.
    compression({
      include: [/\.html$/],
      threshold: 0,
      deleteOriginalAssets: true,
      skipIfLargerOrEqual: false,
      algorithms: [defineAlgorithm('gzip', { level: 9 })],
    }),
  ],
  test: {
    globals: true,
    environment: 'jsdom',
    setupFiles: './vitest.setup.mjs',
  },
  server: {
    proxy: {
      '/api': {
        target: 'http://127.0.0.1:9093',
      },
    },
  },
});
