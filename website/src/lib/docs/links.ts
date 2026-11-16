import { docHref, exampleHref, repoFile } from '../../site';
import type { SourcePage } from './parse';

const scheme = /^[a-z][a-z\d+.-]*:|^\/\//i;
const root = 'https://docs.invalid/docs/';

export const examplesDir = '_examples';
export const examplesFile = 'examples.md';

export const slugOf = (file: string) => file.replace(/(^|\/)README\.md$/, '').replace(/\.md$/, '');

// parseLink resolves a link in docs/<file> against a placeholder origin.
// Absolute URLs give undefined.
export function parseLink(url: string, file: string): URL | undefined {
  return scheme.test(url) ? undefined : new URL(url, root + file);
}

// examplePath returns the repo path for a link into _examples/. Only these
// paths are decoded, because decodeURIComponent throws on a stray % and other
// links should still go to GitHub.
export function examplePath(to: URL): string | undefined {
  const path = to.pathname.slice(1);
  return path.split('/')[0] === examplesDir ? decodeURIComponent(path) : undefined;
}

// resolveLink turns a relative link in docs/<file> into a site or GitHub URL.
// examples maps the main file of each example to its page. line, when given,
// is added to error messages.
export function resolveLink(
  url: string,
  file: string,
  sources: ReadonlyMap<string, SourcePage>,
  examples: ReadonlyMap<string, string>,
  line?: number,
): string {
  const at = line ? `docs/${file}:${line}` : `docs/${file}`;
  const to = parseLink(url, file);
  if (!to) {
    return url;
  }

  const example = examplePath(to);
  if (example) {
    const href = examples.get(example);
    if (!href) {
      throw new Error(`${at}: link "${url}" points to ${example}, which is not an example`);
    }
    if (to.hash) {
      throw new Error(`${at}: link "${url}" points to a heading the example page does not have`);
    }
    return href;
  }

  if (!to.href.startsWith(root) || !to.pathname.endsWith('.md')) {
    return new URL(url, repoFile(`docs/${file}`)).href;
  }

  const target = decodeURIComponent(to.pathname.slice('/docs/'.length));
  const hash = decodeURIComponent(to.hash.slice(1));
  if (target === 'README.md') {
    return docHref('');
  }
  if (target === examplesFile) {
    if (hash) {
      throw new Error(`${at}: link "${url}" points to a heading the examples page does not have`);
    }
    return exampleHref('');
  }

  const page = sources.get(target);
  if (!page) {
    throw new Error(`${at}: link "${url}" points to docs/${target}, which is not a page`);
  }
  if (hash && !page.ids.has(hash)) {
    throw new Error(`${at}: link "${url}" points to a heading that does not exist`);
  }
  if (target === file && hash) {
    return `#${hash}`;
  }

  return docHref(slugOf(target), hash);
}
