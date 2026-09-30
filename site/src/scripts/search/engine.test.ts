import { describe, expect, it } from 'vitest';
import { prepare, search, snippet, tokenize } from './engine';

const entries = prepare([
  { h: 'Captures', p: 'Captures', g: 'Request files', u: '/docs/captures/', t: 'Use @capture to store values.' },
  { h: 'Workflows', p: 'Workflows', g: 'Automation', u: '/docs/workflows/', t: 'Steps can @capture tokens too.' },
  { h: 'OAuth 2.0 directive', p: 'Authentication', g: 'Request files', u: '/docs/authentication/#oauth', t: 'x' },
  { h: 'Static tokens', p: 'Authentication', g: 'Request files', u: '/docs/authentication/#static', t: 'oauth' },
]);

describe('search', () => {
  it('ranks heading matches above text matches', () => {
    const hits = search(entries, 'capture');
    expect(hits.map((h) => h.entry.h)).toEqual(['Captures', 'Workflows']);
  });

  it('requires every token', () => {
    expect(search(entries, 'capture tokens').map((h) => h.entry.h)).toEqual(['Workflows']);
  });

  it('keeps punctuation in tokens so directives match', () => {
    expect(search(entries, '@capture')).toHaveLength(2);
  });

  it('prefers a word in the heading over a word in the text', () => {
    expect(search(entries, 'oauth')[0].entry.h).toBe('OAuth 2.0 directive');
  });

  it('returns nothing for an empty query', () => {
    expect(search(entries, '   ')).toEqual([]);
    expect(tokenize(' a  B ')).toEqual(['a', 'b']);
  });
});

describe('snippet', () => {
  it('marks every token and keeps the text intact', () => {
    const spans = snippet('Use @capture to store values.', ['capture', 'store']);
    expect(spans.map((s) => s.text).join('')).toBe('Use @capture to store values.');
    expect(spans.filter((s) => s.mark).map((s) => s.text)).toEqual(['capture', 'store']);
  });

  it('cuts long text around the first hit', () => {
    const text = `${'lorem '.repeat(100)}needle ${'ipsum '.repeat(100)}`;
    const spans = snippet(text, ['needle'], 60);
    expect(spans[0].text).toBe('...');
    expect(spans.at(-1)!.text).toBe('...');
    expect(spans.some((s) => s.mark && s.text === 'needle')).toBe(true);
  });
});
