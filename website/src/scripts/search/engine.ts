import type { SearchRecord } from '../../lib/docs/search';

export interface Entry extends SearchRecord {
  head: string;
  page: string;
  text: string;
}

export interface Hit {
  entry: Entry;
  score: number;
}

export function prepare(records: SearchRecord[]): Entry[] {
  return records.map((r) => ({ ...r, head: r.h.toLowerCase(), page: r.p.toLowerCase(), text: r.t.toLowerCase() }));
}

export function tokenize(query: string): string[] {
  return query.toLowerCase().split(/\s+/).filter(Boolean);
}

const isWord = (c: string | undefined) => !!c && /[\p{L}\p{N}_]/u.test(c);

// wordAt reports how a token sits in a string: 3 for a whole word, 2 for the
// start of a word, 1 anywhere else and 0 when missing.
function wordAt(haystack: string, token: string): number {
  let best = 0;
  for (let i = haystack.indexOf(token); i >= 0; i = haystack.indexOf(token, i + 1)) {
    const starts = !isWord(haystack[i - 1]) || !isWord(token[0]);
    const ends = !isWord(haystack[i + token.length]) || !isWord(token.at(-1));
    best = Math.max(best, starts ? (ends ? 3 : 2) : 1);
    if (best === 3) break;
  }
  return best;
}

function count(haystack: string, token: string, max: number): number {
  let n = 0;
  for (let i = haystack.indexOf(token); i >= 0 && n < max; i = haystack.indexOf(token, i + token.length)) n++;
  return n;
}

// Every token has to appear in the heading, the page title or the text.
// Headings weigh most, so "oauth" finds the OAuth section before the pages that
// only mention it.
function score(entry: Entry, tokens: string[], phrase: string): number {
  let total = 0;
  for (const token of tokens) {
    const head = wordAt(entry.head, token);
    const page = wordAt(entry.page, token);
    const text = count(entry.text, token, 4);
    if (!head && !page && !text) return 0;
    total += head * 6 + page * 2 + text;
  }
  if (tokens.length > 1) {
    if (entry.head.includes(phrase)) total += 10;
    else if (entry.text.includes(phrase)) total += 4;
  }
  if (entry.head === phrase) total += 12;
  return total;
}

export function search(entries: Entry[], query: string, limit = 30): Hit[] {
  const tokens = tokenize(query);
  if (!tokens.length) return [];
  const phrase = tokens.join(' ');
  const hits: Hit[] = [];
  for (const entry of entries) {
    const s = score(entry, tokens, phrase);
    if (s > 0) hits.push({ entry, score: s });
  }
  // Array.prototype.sort is stable, so ties keep the docs order.
  hits.sort((a, b) => b.score - a.score || a.entry.head.length - b.entry.head.length);
  return hits.slice(0, limit);
}

export interface Span {
  text: string;
  mark: boolean;
}

export function snippet(text: string, tokens: string[], width = 220): Span[] {
  const lower = text.toLowerCase();
  const first = Math.min(...tokens.map((t) => lower.indexOf(t)).filter((i) => i >= 0), Infinity);
  let start = first === Infinity ? 0 : Math.max(0, first - width / 3);
  if (start > 0) start = text.indexOf(' ', start) + 1 || start;
  const end = Math.min(text.length, start + width);
  const part = text.slice(start, end);

  const marks: [number, number][] = [];
  const low = part.toLowerCase();
  for (const token of tokens) {
    for (let i = low.indexOf(token); i >= 0; i = low.indexOf(token, i + token.length))
      marks.push([i, i + token.length]);
  }
  marks.sort((a, b) => a[0] - b[0]);

  const spans: Span[] = [];
  let at = 0;
  for (const [from, to] of marks) {
    if (from < at) continue;
    if (from > at) spans.push({ text: part.slice(at, from), mark: false });
    spans.push({ text: part.slice(from, to), mark: true });
    at = to;
  }
  if (at < part.length) spans.push({ text: part.slice(at), mark: false });
  if (start > 0) spans.unshift({ text: '...', mark: false });
  if (end < text.length) spans.push({ text: '...', mark: false });
  return spans;
}
