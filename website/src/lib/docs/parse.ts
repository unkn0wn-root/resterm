import GithubSlugger from 'github-slugger';
import type { Heading, Nodes, RootContent } from 'mdast';
import { toString } from 'mdast-util-to-string';
import remarkGfm from 'remark-gfm';
import remarkParse from 'remark-parse';
import { unified } from 'unified';
import { throwIfAny } from './report';

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

export interface ExampleRow {
  // Links as written, relative to docs/. The first one is the main file.
  links: string[];
  summary: string;
  needs: Need;
  run: string;
  docs: { label: string; url: string };
  line: number;
}

export interface ExampleSection {
  title: string;
  rows: ExampleRow[];
}

export type Need = (typeof needs)[number];

const parser = unified().use(remarkParse).use(remarkGfm);

const needs = [
  'runs as-is',
  'start the local mock first',
  'needs a GitHub token',
  'needs your own server',
  'needs your own SSH host',
  'needs your own cluster',
] as const;

const columns = ['Example', 'What it shows', 'Needs', 'Run', 'Docs'];
const header = columns.join(' | ');

const lineOf = (node: Nodes) => node.position?.start.line ?? 0;

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
      const line = lineOf(node);
      errors.push(`docs/README.md:${line}: expected only a list of pages under "${group.title}"`);
      continue;
    }
    for (const item of node.children) {
      const line = lineOf(item);
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
  throwIfAny(errors, 'docs/README.md is not a table of contents');
  return groups;
}

export function parseExamples(text: string): ExampleSection[] {
  const groups: { heading: Heading; body: RootContent[] }[] = [];
  const sections: ExampleSection[] = [];
  const errors: string[] = [];
  const at = (node: Nodes) => `docs/examples.md:${lineOf(node)}`;
  let titled = false;

  for (const node of parser.parse(text).children) {
    const group = groups.at(-1);
    if (node.type === 'heading' && node.depth === 2) {
      groups.push({ heading: node, body: [] });
    } else if (group) {
      group.body.push(node);
    } else if (node.type === 'heading' && node.depth === 1) {
      if (titled) {
        errors.push(`${at(node)}: write only one "# Title" heading`);
      }
      titled = true;
    } else if (node.type !== 'paragraph') {
      errors.push(`${at(node)}: write only paragraphs before the first group`);
    }
  }

  for (const { heading, body } of groups) {
    const title = toString(heading);
    const table = body.find((node) => node.type === 'table');
    const rows: ExampleRow[] = [];
    sections.push({ title, rows });

    for (const node of body) {
      if (node !== table) {
        errors.push(`${at(node)}: expected only one table under "${title}"`);
      }
    }

    if (!table) {
      errors.push(`${at(heading)}: "${title}" has no table`);
      continue;
    }

    const [head] = table.children;
    if (head?.children.map((cell) => toString(cell)).join(' | ') !== header) {
      errors.push(`${at(table)}: the columns must be "${header}"`);
      continue;
    }

    if (table.children.length === 1) {
      errors.push(`${at(table)}: "${title}" has no rows`);
      continue;
    }

    for (const row of table.children.slice(1)) {
      const [example, shows, need, run, docs, ...extra] = row.children;
      if (!example || !shows || !need || !run || !docs || extra.length) {
        errors.push(`${at(row)}: the row must have ${columns.length} cells`);
        continue;
      }

      const links = example.children.filter((node) => node.type === 'link');
      const summary = toString(shows).trim();
      const value = toString(need).trim();
      const kind = needs.find((n) => n === value);
      const [code] = run.children.length === 1 ? run.children : [];
      const [link] = docs.children.length === 1 ? docs.children : [];

      if (!links.length) {
        errors.push(`${at(row)}: the Example cell needs a link to a file`);
      } else if (links.some((l) => !toString(l).trim())) {
        errors.push(`${at(row)}: an Example link has no label`);
      } else if (!summary) {
        errors.push(`${at(row)}: the What it shows cell must not be empty`);
      } else if (!kind) {
        errors.push(`${at(row)}: "${value}" is not a Needs value. Use one of: ${needs.join(', ')}`);
      } else if (code?.type !== 'inlineCode') {
        errors.push(`${at(row)}: the Run cell must be one code span`);
      } else if (!code.value.trim()) {
        errors.push(`${at(row)}: the Run cell must not be empty`);
      } else if (link?.type !== 'link') {
        errors.push(`${at(row)}: the Docs cell must be one link`);
      } else if (!toString(link).trim()) {
        errors.push(`${at(row)}: the Docs link has no label`);
      } else {
        rows.push({
          links: links.map((l) => l.url),
          summary,
          needs: kind,
          run: code.value.trim(),
          docs: { label: toString(link).trim(), url: link.url },
          line: lineOf(row),
        });
      }
    }
  }

  throwIfAny(errors, 'docs/examples.md is not a list of examples');
  return sections;
}
