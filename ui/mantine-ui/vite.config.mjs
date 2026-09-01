import path from 'node:path';

import react from '@vitejs/plugin-react';
import license from 'rollup-plugin-license';
import { defineConfig } from 'vite';
import { compression, defineAlgorithm } from 'vite-plugin-compression2';
import tsconfigPaths from 'vite-tsconfig-paths';

const licenseFile = path.resolve(
  import.meta.dirname,
  'node_modules/.cache/alertmanager-third-party-licenses.txt'
);

export default defineConfig({
  base: './',
  plugins: [
    react(),
    tsconfigPaths(),
    license({
      thirdParty: {
        includePrivate: false,
        output: {
          file: licenseFile,
        },
      },
    }),
    compression({
      include: [/\.(css|html|js|txt)$/],
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
