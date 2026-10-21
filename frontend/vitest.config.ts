import { fileURLToPath } from 'node:url';
import { defineConfig, mergeConfig } from 'vitest/config';
import viteConfig from './vite.config';

export default mergeConfig(
  viteConfig,
  defineConfig({
    resolve: {
      conditions: ['browser'],
      // CodeMirror tidak dimuat di jsdom: MarkdownEditor jatuh ke <textarea>.
      // Test yang butuh editor menimpa modul ini dengan vi.mock.
      alias: [
        {
          find: /^(.*)\/markdownEditor\/codemirror(\.ts)?$/,
          replacement: fileURLToPath(new URL('./src/test-stubs/codemirror-unavailable.ts', import.meta.url)),
        },
      ],
    },
    test: {
      environment: 'jsdom',
      include: ['src/**/*.test.ts'],
      exclude: ['e2e/**', 'test/**', 'node_modules/**'],
      setupFiles: ['src/test-setup.ts'],
      restoreMocks: true,
    },
  }),
);
