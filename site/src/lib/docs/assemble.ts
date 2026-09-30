import type { Heading, RootContent } from 'mdast';
import type { Section, SourceDoc } from './parse';

export interface SourceSpec {
  file: string;
  // Sections that are left off the site on purpose, like a hand-written index.
  exclude?: string[];
}

export interface PageSpec<S extends string = string> {
  slug: string;
  title: string;
  summary: string;
  source: S;
  // Heading ids from the source file. The first one is the page itself, so its
  // heading becomes the page title. A source's h1 id stands for the text
  // between the title and the first section.
  sections: string[];
}

export interface GroupSpec<S extends string = string> {
  title: string;
  pages: PageSpec<S>[];
}

export interface TocItem {
  id: string;
  depth: number;
  text: string;
}

export interface DocPage {
  slug: string;
  title: string;
  summary: string;
  group: string;
  file: string;
  line: number;
  id: string;
  nodes: RootContent[];
  toc: TocItem[];
}

export interface Target {
  slug: string;
  primary: boolean;
}

export class Anchors {
  private targets = new Map<string, Target>();
  private starts = new Map<string, string>();

  set(file: string, id: string, target: Target) {
    this.targets.set(`${file}#${id}`, target);
  }

  start(file: string, slug: string) {
    this.starts.set(file, slug);
  }

  has(file: string): boolean {
    return this.starts.has(file);
  }

  resolve(file: string, id?: string): Target | undefined {
    if (!id) {
      const slug = this.starts.get(file);
      return slug ? { slug, primary: true } : undefined;
    }
    return this.targets.get(`${file}#${id}`);
  }
}

const excluded = Symbol('excluded');
type Owner = PageSpec | typeof excluded;

// assemble cuts the parsed sources into pages. Every section ends up on exactly
// one page, through its own claim or its parent's, or the build stops. A new
// top-level heading in docs/ therefore has to be placed in the nav.
export function assemble(
  docs: Map<string, SourceDoc>,
  sources: Record<string, SourceSpec>,
  groups: GroupSpec[],
): { pages: DocPage[]; anchors: Anchors } {
  const errors: string[] = [];
  const owners = new Map<Section, Owner>();
  const intros = new Map<string, PageSpec>();
  const sourceOf = (key: string) => {
    const spec = sources[key];
    const doc = spec && docs.get(spec.file);
    if (!doc) errors.push(`unknown source "${key}"`);
    return doc;
  };

  for (const [key, spec] of Object.entries(sources)) {
    const doc = sourceOf(key);
    for (const id of spec.exclude ?? []) {
      const section = doc?.byId.get(id);
      if (section) owners.set(section, excluded);
      else errors.push(`docs/${spec.file}: excluded section "${id}" does not exist`);
    }
  }

  for (const page of groups.flatMap((g) => g.pages)) {
    const doc = sourceOf(page.source);
    if (!doc) continue;
    if (!page.sections.length) errors.push(`page "${page.slug}" claims no sections`);
    for (const id of page.sections) {
      if (id === doc.titleId) {
        const other = intros.get(doc.file);
        if (other) errors.push(`docs/${doc.file}: intro is on both "${other.slug}" and "${page.slug}"`);
        intros.set(doc.file, page);
        continue;
      }
      const section = doc.byId.get(id);
      if (!section) {
        errors.push(`page "${page.slug}": docs/${doc.file} has no heading with id "${id}"`);
        continue;
      }
      const other = owners.get(section);
      if (other) {
        const name = other === excluded ? 'the exclude list' : `"${other.slug}"`;
        errors.push(`docs/${doc.file}: "${id}" is on both ${name} and "${page.slug}"`);
      }
      owners.set(section, page);
    }
  }

  const ownerOf = (section: Section): Owner | undefined =>
    owners.get(section) ?? (section.parent ? ownerOf(section.parent) : undefined);

  for (const doc of docs.values()) {
    if (doc.intro.length && !intros.has(doc.file)) {
      errors.push(`docs/${doc.file}: the text before the first section is on no page. Claim "${doc.titleId}".`);
    }
    for (const section of doc.byId.values()) {
      const owner = owners.get(section);
      if (!section.parent && !owner) {
        errors.push(
          `docs/${doc.file}:${section.line}: "${section.title}" is on no page. Add "${section.id}" to src/docs/nav.ts.`,
        );
      }
      if (owner && owner !== excluded && section.parent && ownerOf(section.parent) === owner) {
        errors.push(`page "${owner.slug}" claims "${section.id}" twice, directly and through its parent`);
      }
    }
  }

  if (errors.length) throw new Error(`docs do not map onto the nav:\n  ${errors.join('\n  ')}`);

  const anchors = new Anchors();
  const pages: DocPage[] = [];

  for (const group of groups) {
    for (const spec of group.pages) {
      const doc = sourceOf(spec.source)!;
      const page: DocPage = {
        slug: spec.slug,
        title: spec.title,
        summary: spec.summary,
        group: group.title,
        file: doc.file,
        line: 1,
        id: spec.sections[0],
        nodes: [],
        toc: [],
      };

      const emit = (section: Section, delta: number, primary: boolean) => {
        anchors.set(doc.file, section.id, { slug: page.slug, primary });
        if (!primary) {
          const depth = Math.min(section.depth + delta, 6);
          const heading: Heading = { ...section.heading, depth: depth as Heading['depth'] };
          heading.data = { ...heading.data, hProperties: { id: section.id } };
          page.nodes.push(heading);
          if (depth <= 3) page.toc.push({ id: section.id, depth, text: section.title });
        }
        page.nodes.push(...section.body);
        for (const child of section.children) {
          if (ownerOf(child) === spec) emit(child, delta, false);
        }
      };

      spec.sections.forEach((id, i) => {
        const primary = i === 0;
        if (id === doc.titleId) {
          anchors.set(doc.file, id, { slug: page.slug, primary });
          page.nodes.push(...doc.intro);
          return;
        }
        const section = doc.byId.get(id)!;
        if (primary) page.line = section.line;
        emit(section, primary ? 1 - section.depth : 2 - section.depth, primary);
      });

      page.nodes = structuredClone(page.nodes);
      pages.push(page);
    }
  }

  // A link to a whole file lands on the page holding its intro, or else on the
  // first page cut from it.
  for (const file of docs.keys()) {
    const slug = intros.get(file)?.slug ?? pages.find((p) => p.file === file)?.slug;
    if (slug) anchors.start(file, slug);
  }
  return { pages, anchors };
}
