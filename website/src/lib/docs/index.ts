import type { Root } from 'mdast';
import { visit } from 'unist-util-visit';
import { docHref, repoFile } from '../../site';
import { resolveLink, slugOf } from './links';
import { parsePage, parseToc, type SourcePage, type TocEntry, type TocItem } from './parse';
import { renderMarkdown } from './render';
import { searchRecords, type SearchRecord } from './search';

export interface PageLink {
  title: string;
  href: string;
  summary: string;
}

export interface Page {
  slug: string;
  href: string;
  anchor: string;
  title: string;
  group: string;
  description: string;
  html: string;
  toc: TocItem[];
  source: { path: string; url: string };
  prev?: PageLink;
  next?: PageLink;
}

export interface NavGroup {
  title: string;
  pages: PageLink[];
}

export interface Docs {
  groups: NavGroup[];
  pages: Page[];
  search: SearchRecord[];
}

interface ResolvedEntry extends TocEntry {
  group: string;
  slug: string;
  href: string;
  page: SourcePage;
}

// Old docs files stay in the repo for existing links; they are not site pages.
const stubs = new Set(['resterm.md', 'cli.md', 'restermscript.md']);

// Vite tracks these files, so editing docs/ reloads the dev server.
const files = import.meta.glob<string>('../../../../docs/**/*.md', { query: '?raw', import: 'default', eager: true });

const link = (entry: ResolvedEntry): PageLink => ({ title: entry.label, href: entry.href, summary: entry.summary });

export async function build(texts: ReadonlyMap<string, string>): Promise<Docs> {
  const index = texts.get('README.md');
  if (index === undefined) throw new Error('docs/README.md: the table of contents is missing');
  const toc = parseToc(index);
  const entries: ResolvedEntry[] = [];
  const groups: NavGroup[] = [];
  const errors: string[] = [];
  const sources = new Map<string, SourcePage>();
  const routes = new Map<string, string>();

  for (const group of toc) {
    const links: PageLink[] = [];
    for (const entry of group.pages) {
      const text = texts.get(entry.file);
      if (sources.has(entry.file)) {
        errors.push(`docs/README.md:${entry.line}: docs/${entry.file} is listed twice`);
        continue;
      }
      if (text === undefined || entry.file === 'README.md' || stubs.has(entry.file)) {
        errors.push(`docs/README.md:${entry.line}: docs/${entry.file} is not a page file`);
        continue;
      }
      const slug = slugOf(entry.file);
      const href = docHref(slug);
      const other = routes.get(slug);
      if (other !== undefined) {
        errors.push(`docs/README.md:${entry.line}: docs/${other} and docs/${entry.file} both map to "${href}"`);
      } else routes.set(slug, entry.file);
      const page = parsePage(entry.file, text);
      sources.set(entry.file, page);
      const resolved: ResolvedEntry = { ...entry, group: group.title, slug, href, page };
      entries.push(resolved);
      links.push(link(resolved));
    }
    groups.push({ title: group.title, pages: links });
  }
  for (const file of texts.keys()) {
    if (file !== 'README.md' && !stubs.has(file) && !sources.has(file)) {
      errors.push(`docs/${file} is on no page. Add it to docs/README.md.`);
    }
  }
  if (errors.length) throw new Error(`docs do not map onto docs/README.md:\n  ${errors.join('\n  ')}`);

  const pages = await Promise.all(
    entries.map(async (entry, i): Promise<Page> => {
      const { page } = entry;
      const tree: Root = { type: 'root', children: page.nodes };
      visit(tree, (node) => {
        if (node.type === 'link' || node.type === 'definition') node.url = resolveLink(node.url, entry.file, sources);
      });
      const path = `docs/${entry.file}`;
      const prev = entries[i - 1];
      const next = entries[i + 1];
      return {
        slug: entry.slug,
        href: entry.href,
        anchor: page.anchor,
        title: page.title,
        group: entry.group,
        description: entry.summary,
        html: await renderMarkdown(tree),
        toc: page.toc,
        source: { path, url: repoFile(path) },
        prev: prev && link(prev),
        next: next && link(next),
      };
    }),
  );

  return {
    groups,
    pages,
    search: entries.flatMap(({ page, slug, group }) =>
      searchRecords({ title: page.title, nodes: page.nodes, slug, group }),
    ),
  };
}

let cached: Promise<Docs> | undefined;

export function getDocs(): Promise<Docs> {
  cached ??= build(
    new Map(Object.entries(files).map(([path, text]) => [path.replace(/^(\.\.\/)+docs\//, ''), text] as const)),
  );
  return cached;
}
