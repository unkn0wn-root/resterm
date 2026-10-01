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
  // Docs render through src/lib/docs. In dev Astro still runs its own markdown
  // pass over the imported .md files, and its Shiki does not know rts.
  markdown: { syntaxHighlight: false },
  vite: {
    server: {
      fs: {
        // docs/, _media/ and CHANGELOG.md live at the repo root.
        allow: [fileURLToPath(new URL('..', import.meta.url))],
      },
    },
  },
});
