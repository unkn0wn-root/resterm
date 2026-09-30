import { docHref, repoFile } from '../../site';
import type { SourcePage } from './parse';

const scheme = /^[a-z][a-z\d+.-]*:|^\/\//i;
const root = 'https://docs.invalid/docs/';

export const slugOf = (file: string) => file.replace(/(^|\/)README\.md$/, '').replace(/\.md$/, '');

export function resolveLink(url: string, file: string, sources: ReadonlyMap<string, SourcePage>): string {
  if (scheme.test(url)) return url;

  const to = new URL(url, root + file);
  if (!to.href.startsWith(root) || !to.pathname.endsWith('.md')) {
    return new URL(url, repoFile(`docs/${file}`)).href;
  }

  const target = decodeURIComponent(to.pathname.slice('/docs/'.length));
  const hash = decodeURIComponent(to.hash.slice(1));
  if (target === 'README.md') return docHref('');

  const page = sources.get(target);
  if (!page) throw new Error(`docs/${file}: link "${url}" points to docs/${target}, which is not a page`);
  if (hash && !page.ids.has(hash)) {
    throw new Error(`docs/${file}: link "${url}" points to a heading that does not exist`);
  }
  if (target === file && hash) return `#${hash}`;
  return docHref(slugOf(target), hash);
}
