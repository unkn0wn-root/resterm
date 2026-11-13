import { ref, repoFile, site } from '../../site';
import { getDocs, type Example, type ExampleGroup, type NavGroup, type PageLink } from '../docs';
import { renderMarkdown } from '../docs/render';
import { throwIfAny } from '../docs/report';
import type { SearchRecord } from '../docs/search';

export interface ExampleFile {
  path: string;
  name: string;
  lang: string;
  code: string;
  url: string;
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

export async function build(groups: ExampleGroup[], texts: ReadonlyMap<string, string>): Promise<Examples> {
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
        files.push({
          path,
          name,
          lang: name.slice(name.lastIndexOf('.') + 1),
          code,
          url: repoFile(path),
        });
      }
      entries.push({ ...example, group: group.title, files });
    }
  }

  for (const path of texts.keys()) {
    if (!listed.has(path)) {
      errors.push(`${path} is on no example. Add it to docs/examples.md.`);
    }
  }

  throwIfAny(errors, '_examples/ does not map onto docs/examples.md');

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
    ),
  );
  return cached;
}
