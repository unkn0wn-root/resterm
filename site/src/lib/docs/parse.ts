import GithubSlugger from 'github-slugger';
import type { RootContent } from 'mdast';
import { toString } from 'mdast-util-to-string';
import remarkGfm from 'remark-gfm';
import remarkParse from 'remark-parse';
import { unified } from 'unified';

export interface TocItem {
  id: string;
  depth: number;
  text: string;
}

export interface SourcePage {
  title: string;
  anchor: string;
  nodes: RootContent[];
  ids: Set<string>;
  toc: TocItem[];
}

export interface TocEntry {
  file: string;
  label: string;
  summary: string;
  line: number;
}

export interface TocGroup {
  title: string;
  pages: TocEntry[];
}

const parser = unified().use(remarkParse).use(remarkGfm);

export function parsePage(file: string, text: string): SourcePage {
  // Use one slugger per file so repeated headings get the same IDs as on GitHub.
  const slugger = new GithubSlugger();
  const nodes: RootContent[] = [];
  const ids = new Set<string>();
  const toc: TocItem[] = [];
  let head: { title: string; anchor: string } | undefined;

  for (const node of parser.parse(text).children) {
    if (node.type !== 'heading') {
      nodes.push(node);
      continue;
    }
    const title = toString(node);
    const id = slugger.slug(title);
    ids.add(id);
    if (node.depth === 1 && !head && title) {
      head = { title, anchor: id };
      continue;
    }
    node.data = { ...node.data, hProperties: { id } };
    if (node.depth <= 3) toc.push({ id, depth: node.depth, text: title });
    nodes.push(node);
  }

  if (!head) throw new Error(`docs/${file}: the page has no "# Title" heading`);
  return { ...head, nodes, ids, toc };
}

export function parseToc(text: string): TocGroup[] {
  const groups: TocGroup[] = [];
  const errors: string[] = [];

  for (const node of parser.parse(text).children) {
    if (node.type === 'heading' && node.depth === 2) {
      groups.push({ title: toString(node), pages: [] });
      continue;
    }
    const group = groups.at(-1);
    if (!group) continue;
    if (node.type !== 'list') {
      const line = node.position?.start.line ?? 0;
      errors.push(`docs/README.md:${line}: expected only a list of pages under "${group.title}"`);
      continue;
    }
    for (const item of node.children) {
      const line = item.position?.start.line ?? 0;
      const [para] = item.children;
      const [link, ...rest] = para?.type === 'paragraph' ? para.children : [];
      const after = toString(rest);
      const summary = after.startsWith(':') ? after.slice(1).trim() : '';
      if (item.children.length !== 1 || link?.type !== 'link' || !summary) {
        errors.push(`docs/README.md:${line}: write the item as "[Label](file.md): summary"`);
        continue;
      }
      const label = toString(link).trim();
      if (!label) {
        errors.push(`docs/README.md:${line}: the page label must not be empty`);
        continue;
      }
      group.pages.push({ file: link.url.replace(/^\.\//, ''), label, summary, line });
    }
  }

  if (!errors.length && !groups.some((group) => group.pages.length)) {
    errors.push('docs/README.md: the table of contents must contain at least one page');
  }
  if (errors.length) throw new Error(`docs/README.md is not a table of contents:\n  ${errors.join('\n  ')}`);
  return groups;
}
