import { hash } from 'node:crypto';
import requestsJson from '../../content/requests.json';
import { ref, repoFile, site } from '../../site';
import { getDocs, type Example, type ExampleGroup, type NavGroup, type PageLink } from '../docs';
import { renderMarkdown } from '../docs/render';
import { throwIfAny } from '../docs/report';
import type { SearchRecord } from '../docs/search';

export interface ExampleRequest {
  id: string;
  line: number;
  method: string;
  name: string;
}

export interface ExampleFile {
  path: string;
  name: string;
  lang: string;
  code: string;
  url: string;
  requests: ExampleRequest[];
}

export interface ExamplePage extends Omit<Example, 'files' | 'line'> {
  group: string;
  runHtml: string;
  files: ExampleFile[];
  prev?: PageLink;
  next?: PageLink;
}

export interface Examples {
  groups: NavGroup[];
  pages: ExamplePage[];
  search: SearchRecord[];
  clone: string;
}

export interface OutlineEntry {
  sha256: string;
  requests: Omit<ExampleRequest, 'id'>[];
}

// Outline is the shape of website/src/content/requests.json, which
// website/requests writes.
export type Outline = Readonly<Record<string, OutlineEntry>>;

const outlineFile = 'website/src/content/requests.json';

// Keep in sync with files.IsRequest in Go, which decides the files
// website/requests lists.
const requestFile = /\.(http|rest)$/i;

// Vite tracks these files, so editing _examples/ reloads the dev server.
const sources = import.meta.glob<string>('../../../../_examples/**/*', {
  query: '?raw',
  import: 'default',
  eager: true,
});

const link = ({ title, href, summary }: PageLink): PageLink => ({ title, href, summary });

const commands = (value: string) =>
  renderMarkdown({
    type: 'root',
    children: [{ type: 'code', lang: 'bash', value }],
  });

export async function build(
  groups: ExampleGroup[],
  texts: ReadonlyMap<string, string>,
  outline: Outline,
): Promise<Examples> {
  const entries: Omit<ExamplePage, 'runHtml' | 'prev' | 'next'>[] = [];
  const errors: string[] = [];
  const listed = new Set<string>();

  for (const group of groups) {
    for (const { files: paths, line, ...example } of group.examples) {
      const files: ExampleFile[] = [];
      for (const path of paths) {
        const code = texts.get(path);
        listed.add(path);
        if (code === undefined) {
          errors.push(`docs/examples.md:${line}: ${path} does not exist`);
          continue;
        }

        const name = path.slice(path.lastIndexOf('/') + 1);
        // A file's index here is its tab index on the page, so ids stay unique
        // across tabs.
        const tab = files.length;
        const requests = outline[path]?.requests ?? [];
        files.push({
          path,
          name,
          lang: name.slice(name.lastIndexOf('.') + 1),
          code,
          url: repoFile(path),
          requests: requests.map((r) => ({ id: `file-${tab}-L${r.line}`, ...r })),
        });
      }
      entries.push({ ...example, group: group.title, files });
    }
  }

  for (const [path, code] of texts) {
    if (!listed.has(path)) {
      errors.push(`${path} is on no example. Add it to docs/examples.md.`);
    }
    if (requestFile.test(path) && outline[path]?.sha256 !== hash('sha256', code)) {
      errors.push(`${path} does not match ${outlineFile}. Run make website-requests.`);
    }
  }

  for (const path of Object.keys(outline)) {
    if (!texts.has(path) || !requestFile.test(path)) {
      errors.push(`${path} is in ${outlineFile} but is not a request file in _examples/. Run make website-requests.`);
    }
  }

  throwIfAny(errors, `_examples/ does not match docs/examples.md or ${outlineFile}`);

  const pages = await Promise.all(
    entries.map(async (entry, i): Promise<ExamplePage> => {
      const prev = entries[i - 1];
      const next = entries[i + 1];

      return {
        ...entry,
        runHtml: await commands(entry.run),
        prev: prev && link(prev),
        next: next && link(next),
      };
    }),
  );

  return {
    groups: groups.map((group) => ({ title: group.title, pages: group.examples.map(link) })),
    pages,
    search: entries.map((example) => ({
      h: example.title,
      p: example.title,
      g: 'Examples',
      u: example.href,
      t: [example.summary, ...example.files.map((f) => f.path)].join(' '),
    })),
    clone: await commands(`git clone --depth 1 --branch ${ref} ${site.repo}.git\ncd resterm`),
  };
}

let cached: Promise<Examples> | undefined;

export function getExamples(): Promise<Examples> {
  cached ??= getDocs().then(({ examples }) =>
    build(
      examples,
      new Map(Object.entries(sources).map(([path, text]) => [path.replace(/^(\.\.\/)+/, ''), text] as const)),
      requestsJson,
    ),
  );
  return cached;
}
