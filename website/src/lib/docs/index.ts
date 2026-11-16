import type { Root } from 'mdast';
import { visit } from 'unist-util-visit';
import { docHref, exampleHref, repoFile } from '../../site';
import { examplePath, examplesDir, examplesFile, parseLink, resolveLink, slugOf } from './links';
import {
  parseExamples,
  parsePage,
  parseToc,
  type ExampleRow,
  type ExampleSection,
  type SourcePage,
  type TocEntry,
  type TocItem,
} from './parse';
import { renderMarkdown } from './render';
import { throwIfAny } from './report';
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

export interface Example extends Omit<ExampleRow, 'links' | 'docs'> {
  slug: string;
  href: string;
  title: string;
  docs: { title: string; href: string };
  // Repo paths, main file first.
  files: string[];
}

export interface ExampleGroup {
  title: string;
  examples: Example[];
}

export interface Docs {
  groups: NavGroup[];
  pages: Page[];
  examples: ExampleGroup[];
  search: SearchRecord[];
}

interface ResolvedEntry extends TocEntry {
  group: string;
  slug: string;
  href: string;
  page: SourcePage;
}

type Draft = Omit<Example, 'docs'> & Pick<ExampleRow, 'docs'>;

// Old docs files stay in the repo for existing links; they are not site pages.
const stubs = new Set(['resterm.md', 'cli.md', 'restermscript.md']);

// Vite tracks these files, so editing docs/ reloads the dev server.
const files = import.meta.glob<string>('../../../../docs/**/*.md', { query: '?raw', import: 'default', eager: true });

const link = (entry: ResolvedEntry): PageLink => ({ title: entry.label, href: entry.href, summary: entry.summary });

export async function build(texts: ReadonlyMap<string, string>): Promise<Docs> {
  const index = texts.get('README.md');
  if (index === undefined) {
    throw new Error('docs/README.md: the table of contents is missing');
  }

  const toc = parseToc(index);
  const entries: ResolvedEntry[] = [];
  const groups: NavGroup[] = [];
  const errors: string[] = [];
  const listed = new Set<string>();
  const sources = new Map<string, SourcePage>();
  const routes = new Map<string, string>();
  let sections: ExampleSection[] = [];

  for (const group of toc) {
    const links: PageLink[] = [];
    for (const entry of group.pages) {
      const text = texts.get(entry.file);
      if (listed.has(entry.file)) {
        errors.push(`docs/README.md:${entry.line}: docs/${entry.file} is listed twice`);
        continue;
      }
      if (text === undefined || entry.file === 'README.md' || stubs.has(entry.file)) {
        errors.push(`docs/README.md:${entry.line}: docs/${entry.file} is not a page file`);
        continue;
      }
      listed.add(entry.file);

      if (entry.file === examplesFile) {
        sections = parseExamples(text);
        links.push({ title: entry.label, href: exampleHref(''), summary: entry.summary });
        continue;
      }

      const slug = slugOf(entry.file);
      const href = docHref(slug);
      const other = routes.get(slug);
      if (other !== undefined) {
        errors.push(`docs/README.md:${entry.line}: docs/${other} and docs/${entry.file} both map to "${href}"`);
      } else {
        routes.set(slug, entry.file);
      }

      const page = parsePage(entry.file, text);
      sources.set(entry.file, page);
      const resolved: ResolvedEntry = { ...entry, group: group.title, slug, href, page };
      entries.push(resolved);
      links.push(link(resolved));
    }
    groups.push({ title: group.title, pages: links });
  }

  for (const file of texts.keys()) {
    if (file !== 'README.md' && !stubs.has(file) && !listed.has(file)) {
      errors.push(`docs/${file} is on no page. Add it to docs/README.md.`);
    }
  }

  throwIfAny(errors, 'docs do not map onto docs/README.md');

  // Resolve the Docs cells only after every row has its page. A Docs link to an
  // example further down then fails the same way as one to an example above it.
  const drafts: { title: string; rows: Draft[] }[] = [];
  const hrefs = new Map<string, string>();
  const slugs = new Map<string, string>();
  const seen = new Set<string>();

  for (const section of sections) {
    const rows: Draft[] = [];
    for (const { links, ...row } of section.rows) {
      const at = `docs/examples.md:${row.line}`;
      const paths: string[] = [];
      for (const url of links) {
        const to = parseLink(url, examplesFile);
        const path = to && examplePath(to);
        if (!to || !path) {
          errors.push(`${at}: link "${url}" does not point into ${examplesDir}/`);
          continue;
        }
        if (to.hash) {
          errors.push(`${at}: link "${url}" points to a heading the example page does not have`);
        }
        if (seen.has(path)) {
          errors.push(`${at}: ${path} is listed twice`);
        }
        seen.add(path);
        paths.push(path);
      }

      const [main] = paths;
      if (!main) {
        continue;
      }

      const title = main.slice(examplesDir.length + 1);
      const slug = title.replace(/\.[^./]+$/, '');
      const href = exampleHref(slug);
      const other = slugs.get(slug);
      if (other !== undefined) {
        errors.push(`${at}: ${other} and ${main} both map to "${href}"`);
      } else {
        slugs.set(slug, main);
      }

      hrefs.set(main, href);
      rows.push({ ...row, slug, href, title, files: paths });
    }
    drafts.push({ title: section.title, rows });
  }

  const examples = drafts.map(({ title, rows }): ExampleGroup => ({
    title,
    examples: rows.map(({ docs, ...example }) => {
      let href = '';
      try {
        href = resolveLink(docs.url, examplesFile, sources, hrefs, example.line);
      } catch (err) {
        if (!(err instanceof Error)) {
          throw err;
        }
        errors.push(err.message);
      }

      if (href && !href.startsWith(docHref(''))) {
        errors.push(`docs/examples.md:${example.line}: the Docs cell must link to a docs page`);
      }

      return { ...example, docs: { title: docs.label, href } };
    }),
  }));

  throwIfAny(errors, `docs/examples.md does not map onto ${examplesDir}/`);

  const pages = await Promise.all(
    entries.map(async (entry, i): Promise<Page> => {
      const { page } = entry;
      const tree: Root = { type: 'root', children: page.nodes };
      visit(tree, (node) => {
        if (node.type === 'link' || node.type === 'definition') {
          node.url = resolveLink(node.url, entry.file, sources, hrefs);
        }
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
    examples,
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
