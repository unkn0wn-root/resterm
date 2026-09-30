import { fileURLToPath } from 'node:url';
import sitemap from '@astrojs/sitemap';
import { defineConfig } from 'astro/config';

export default defineConfig({
  site: process.env.SITE_URL || 'https://resterm.app',
  trailingSlash: 'always',
  build: {
    format: 'directory',
  },
  integrations: [sitemap()],
  vite: {
    server: {
      fs: {
        // docs/, _media/ and CHANGELOG.md live at the repo root.
        allow: [fileURLToPath(new URL('..', import.meta.url))],
      },
    },
  },
});
