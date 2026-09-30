import { toString } from 'mdast-util-to-string';
import { docHref } from '../../site';
import type { DocPage } from './assemble';

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

export function searchRecords(page: DocPage): SearchRecord[] {
  const records: SearchRecord[] = [];
  let current: SearchRecord = { h: page.title, p: page.title, g: page.group, u: docHref(page.slug), t: '' };
  const parts: string[] = [];

  const flush = () => {
    current.t = parts.join(' ').replace(/\s+/g, ' ').trim().slice(0, maxText);
    records.push(current);
    parts.length = 0;
  };

  for (const node of page.nodes) {
    if (node.type === 'heading' && node.depth <= 3) {
      flush();
      const id = (node.data?.hProperties as { id?: string } | undefined)?.id;
      current = { h: toString(node), p: page.title, g: page.group, u: docHref(page.slug, id), t: '' };
      continue;
    }
    if (node.type !== 'html') parts.push(toString(node));
  }
  flush();
  return records;
}
