import GithubSlugger from 'github-slugger';
import type { Heading, RootContent } from 'mdast';
import { toString } from 'mdast-util-to-string';
import remarkGfm from 'remark-gfm';
import remarkParse from 'remark-parse';
import { unified } from 'unified';

export interface Section {
  id: string;
  title: string;
  depth: number;
  line: number;
  heading: Heading;
  body: RootContent[];
  parent?: Section;
  children: Section[];
}

export interface SourceDoc {
  file: string;
  title: string;
  titleId: string;
  intro: RootContent[];
  sections: Section[];
  byId: Map<string, Section>;
}

const parser = unified().use(remarkParse).use(remarkGfm);

// Heading ids use GitHub's slug rules over the whole file, in order, so every
// anchor matches the one GitHub shows for docs/<file>.md.
export function parseSource(file: string, text: string): SourceDoc {
  const root = parser.parse(text);
  const slugger = new GithubSlugger();
  const doc: SourceDoc = { file, title: '', titleId: '', intro: [], sections: [], byId: new Map() };

  let current: Section | undefined;
  const open: Section[] = [];

  for (const node of root.children) {
    if (node.type !== 'heading') {
      if (current) current.body.push(node);
      else if (doc.title) doc.intro.push(node);
      continue;
    }

    const title = toString(node);
    const id = slugger.slug(title);
    if (node.depth === 1 && !doc.title) {
      doc.title = title;
      doc.titleId = id;
      continue;
    }

    while (open.length && open.at(-1)!.depth >= node.depth) open.pop();
    const parent = open.at(-1);
    const section: Section = {
      id,
      title,
      depth: node.depth,
      line: node.position?.start.line ?? 0,
      heading: node,
      body: [],
      parent,
      children: [],
    };
    if (parent) parent.children.push(section);
    else doc.sections.push(section);
    doc.byId.set(id, section);
    open.push(section);
    current = section;
  }

  for (const section of doc.byId.values()) trimRule(section.body);
  trimRule(doc.intro);
  return doc;
}

// The manual separates top-level sections with "---". On a page of its own a
// trailing rule is noise.
function trimRule(nodes: RootContent[]) {
  while (nodes.at(-1)?.type === 'thematicBreak') nodes.pop();
}
