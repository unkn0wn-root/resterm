import { assert, describe, expect, it } from 'vitest';
import { build } from '.';

const toc = `# Docs

Intro for GitHub.

## Group

- [Guide](guide.md): How to use it.
- [Language](rts/README.md): The language.

## Other

- [Values](rts/values.md): Values and \`types\`.
`;

const guide = `# Guide

Intro text.

## Setup

See [usage](#usage), [values](rts/values.md#literals) and [the index](README.md).

### Details

Setup details.

## Usage

Code lives in [examples](../_examples/basic.http).
`;

const lang = `# RestermScript

Read [values](values.md) and [setup](../guide.md#setup).
`;

const values = `# Values

## Literals

Back to the [language](README.md) and [the guide](../guide.md).
`;

const docs = (extra: Record<string, string> = {}) =>
  new Map(
    Object.entries({
      'README.md': toc,
      'guide.md': guide,
      'rts/README.md': lang,
      'rts/values.md': values,
      'resterm.md': '# Old\n\n## Moved\n',
      ...extra,
    }),
  );

describe('build', () => {
  it('turns files into pages and rewrites links', async () => {
    const out = await build(docs());
    const [g, l, v] = out.pages;
    assert.exists(g);
    assert.exists(l);
    assert.exists(v);

    expect(out.groups).toEqual([
      {
        title: 'Group',
        pages: [
          { title: 'Guide', href: '/docs/guide/', summary: 'How to use it.' },
          { title: 'Language', href: '/docs/rts/', summary: 'The language.' },
        ],
      },
      {
        title: 'Other',
        pages: [{ title: 'Values', href: '/docs/rts/values/', summary: 'Values and types.' }],
      },
    ]);

    expect(g.title).toBe('Guide');
    expect(g.anchor).toBe('guide');
    expect(g.source.path).toBe('docs/guide.md');
    expect(g.html).toMatch(/Intro text[\s\S]*<h2 id="setup">[\s\S]*<h3 id="details">/);
    expect(g.html).toContain('href="#usage"');
    expect(g.html).toContain('href="/docs/rts/values/#literals"');
    expect(g.html).toContain('href="/docs/"');
    expect(g.html).toContain('href="https://github.com/unkn0wn-root/resterm/blob/main/_examples/basic.http"');
    expect(g.toc.map((t) => t.id)).toEqual(['setup', 'details', 'usage']);

    expect(l.slug).toBe('rts');
    expect(l.title).toBe('RestermScript');
    expect(l.html).toContain('href="/docs/rts/values/"');
    expect(l.html).toContain('href="/docs/guide/#setup"');

    expect(v.group).toBe('Other');
    expect(v.html).toContain('href="/docs/rts/"');
    expect(v.html).toContain('href="/docs/guide/"');

    expect(g.next?.href).toBe('/docs/rts/');
    expect(l.prev?.href).toBe('/docs/guide/');
    expect(l.next?.href).toBe('/docs/rts/values/');
    expect(v.prev?.title).toBe('Language');
    expect(g.prev).toBeUndefined();
    expect(v.next).toBeUndefined();
    expect(out.search.map((r) => r.u)).toContain('/docs/guide/#details');
  });

  it('fails when the table of contents is missing', async () => {
    const input = docs();
    input.delete('README.md');
    await expect(build(input)).rejects.toThrow('docs/README.md: the table of contents is missing');
  });

  it.each(['', '# Docs\n', '# Docs\n\n## Empty\n'])('fails on an empty table of contents: %j', async (text) => {
    await expect(build(new Map([['README.md', text]]))).rejects.toThrow('must contain at least one page');
  });

  it.each(['', ' '])('fails on an empty page label: %j', async (label) => {
    const input = docs({ 'README.md': toc.replace('[Guide]', `[${label}]`) });
    await expect(build(input)).rejects.toThrow('docs/README.md:7: the page label must not be empty');
  });

  it('fails when a page is listed in multiple groups', async () => {
    const input = docs({ 'README.md': toc.replace('## Other', '## Other\n\n- [Guide again](guide.md): Duplicate.') });
    await expect(build(input)).rejects.toThrow('docs/guide.md is listed twice');
  });

  it.each([
    ['guide.md', 'guide/README.md'],
    ['guide/README.md', 'guide.md'],
  ])('fails on a route collision with %s listed first', async (first, second) => {
    const input = docs({
      'README.md': toc.replace(
        '- [Guide](guide.md): How to use it.',
        `- [First](${first}): First page.\n- [Second](${second}): Second page.`,
      ),
      'guide/README.md': '# Other guide\n',
    });
    await expect(build(input)).rejects.toThrow(`docs/${first} and docs/${second} both map to "/docs/guide/"`);
  });

  it('keeps duplicate heading anchors consistent across links, HTML, and search', async () => {
    const out = await build(
      docs({
        'guide.md': '# Guide\n\n## Guide\n\nSee [next](#guide-2).\n\n## Guide\n',
        'rts/README.md': lang.replace('#setup', '#guide-1'),
      }),
    );
    const page = out.pages[0];
    assert.exists(page);
    expect(page.anchor).toBe('guide');
    expect(page.toc.map((item) => item.id)).toEqual(['guide-1', 'guide-2']);
    expect(page.html).toContain('<h2 id="guide-1">');
    expect(page.html).toContain('<h2 id="guide-2">');
    expect(page.html).toContain('href="#guide-2"');
    expect(out.search.map((record) => record.u)).toContain('/docs/guide/#guide-2');
  });

  it('omits both navigation neighbors for a single page', async () => {
    const out = await build(
      new Map([
        ['README.md', '# Docs\n\n## Group\n\n- [Guide](guide.md): A guide.\n'],
        ['guide.md', '# Guide\n'],
      ]),
    );
    expect(out.pages).toHaveLength(1);
    expect(out.pages[0]?.prev).toBeUndefined();
    expect(out.pages[0]?.next).toBeUndefined();
  });

  it('fails when a file is not in the table of contents', async () => {
    await expect(build(docs({ 'lost.md': '# Lost\n' }))).rejects.toThrow(/docs\/lost.md is on no page/);
  });

  it('fails when the table of contents names a missing file', async () => {
    const input = docs();
    input.delete('rts/values.md');
    await expect(build(input)).rejects.toThrow(/README.md:12: docs\/rts\/values.md is not a page file/);
  });

  it.each(['Text only.\n', '#\n\nText.\n'])('fails on a page without a title: %j', async (text) => {
    await expect(build(docs({ 'guide.md': text }))).rejects.toThrow('docs/guide.md: the page has no "# Title" heading');
  });

  it('fails on an entry without a summary', async () => {
    const input = docs({ 'README.md': toc.replace('): How to use it.', ')') });
    await expect(build(input)).rejects.toThrow(/README.md:7: write the item as/);
  });

  it('fails on a link to a heading that does not exist', async () => {
    const input = docs({ 'rts/values.md': '# Values\n\n## Literals\n\nSee [x](../guide.md#nope).\n' });
    await expect(build(input)).rejects.toThrow(/link "..\/guide.md#nope" points to a heading that does not exist/);
  });

  it('fails on a link to a doc that is not a page', async () => {
    const input = docs({ 'rts/values.md': '# Values\n\n## Literals\n\nSee [x](../resterm.md).\n' });
    await expect(build(input)).rejects.toThrow(/points to docs\/resterm.md, which is not a page/);
  });
});
