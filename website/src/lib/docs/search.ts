import type { RootContent } from 'mdast';
import { toString } from 'mdast-util-to-string';
import { docHref } from '../../site';

// One record per heading, so a hit can link straight to the section.
export interface SearchRecord {
  // heading
  h: string;
  // page title
  p: string;
  // group
  g: string;
  // url
  u: string;
  // section text, whitespace collapsed
  t: string;
}

const maxText = 1600;

export function searchRecords(page: {
  slug: string;
  title: string;
  group: string;
  nodes: readonly RootContent[];
}): SearchRecord[] {
  const records: SearchRecord[] = [];
  const record = (h: string, u: string): SearchRecord => ({ h, p: page.title, g: page.group, u, t: '' });
  let current = record(page.title, docHref(page.slug));
  const parts: string[] = [];

  const flush = () => {
    current.t = parts.join(' ').replace(/\s+/g, ' ').trim().slice(0, maxText);
    records.push(current);
    parts.length = 0;
  };

  for (const node of page.nodes) {
    if (node.type === 'heading' && node.depth <= 3) {
      flush();
      const id = node.data?.hProperties?.id;
      current = record(toString(node), docHref(page.slug, typeof id === 'string' ? id : undefined));
      continue;
    }
    if (node.type !== 'html') parts.push(toString(node));
  }
  flush();
  return records;
}
