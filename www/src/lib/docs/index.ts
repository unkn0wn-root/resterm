import type { Root } from 'mdast';
import { visit } from 'unist-util-visit';
import { groups, sources } from '../../docs/nav';
import { docHref, repoFile } from '../../site';
import { assemble, type DocPage, type GroupSpec, type PageSpec, type SourceSpec, type TocItem } from './assemble';
import { resolveLink } from './links';
import { parseSource, type SourceDoc } from './parse';
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
  // The id of the section the page is built around. The h1 carries it, so
  // GitHub-style links to that heading still land on the page.
  anchor: string;
  title: string;
  group: string;
  description: string;
  html: string;
  toc: TocItem[];
  source: { path: string; line: number; url: string };
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

// Vite tracks these files, so editing docs/ reloads the dev server.
const files = import.meta.glob<string>('../../../../docs/*.md', { query: '?raw', import: 'default', eager: true });

function loadSources(): Map<string, SourceDoc> {
  const docs = new Map<string, SourceDoc>();
  for (const [path, text] of Object.entries(files)) {
    const file = path.slice(path.lastIndexOf('/') + 1);
    docs.set(file, parseSource(file, text));
  }
  return docs;
}

const link = (p: Pick<PageSpec, 'slug' | 'title' | 'summary'>): PageLink => ({
  title: p.title,
  href: docHref(p.slug),
  summary: p.summary,
});

export async function build(
  docs: Map<string, SourceDoc>,
  specs: Record<string, SourceSpec>,
  nav: GroupSpec[],
): Promise<Docs> {
  const { pages, anchors } = assemble(docs, specs, nav);

  const render = async (page: DocPage, i: number): Promise<Page> => {
    const tree: Root = { type: 'root', children: page.nodes };
    visit(tree, (node) => {
      if (node.type === 'link' || node.type === 'definition') {
        node.url = resolveLink(node.url, page.file, page.slug, anchors);
      }
    });
    const path = `docs/${page.file}`;
    return {
      slug: page.slug,
      href: docHref(page.slug),
      anchor: page.id,
      title: page.title,
      group: page.group,
      description: page.summary,
      html: await renderMarkdown(tree),
      toc: page.toc,
      source: { path, line: page.line, url: repoFile(path, page.line) },
      prev: pages[i - 1] && link(pages[i - 1]),
      next: pages[i + 1] && link(pages[i + 1]),
    };
  };

  return {
    groups: nav.map((g) => ({ title: g.title, pages: g.pages.map(link) })),
    pages: await Promise.all(pages.map(render)),
    search: pages.flatMap(searchRecords),
  };
}

let cached: Promise<Docs> | undefined;

export function getDocs(): Promise<Docs> {
  cached ??= build(loadSources(), sources, groups);
  return cached;
}
