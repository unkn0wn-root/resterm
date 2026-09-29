import { describe, expect, it } from 'vitest';
import { build } from '.';
import { parseSource } from './parse';

const guide = `# Guide

Intro text.

## Setup

See [usage](#usage) and [the flags](other.md#flags).

### Details

Setup details.

## Usage

Use it. Code lives in [examples](../_examples/basic.http).

### Moved

This part belongs with the other file's page.
`;

const other = `# Other

## Flags

Back to [setup](guide.md#setup) and to [moved](guide.md#moved).
`;

const docs = () =>
  new Map([
    ['guide.md', parseSource('guide.md', guide)],
    ['other.md', parseSource('other.md', other)],
  ]);

const sources = { guide: { file: 'guide.md' }, other: { file: 'other.md' } };

describe('build', () => {
  it('cuts sources into pages and rewrites links', async () => {
    const out = await build(docs(), sources, [
      {
        title: 'Group',
        pages: [
          { slug: 'setup', title: 'Setup', summary: '', source: 'guide', sections: ['guide', 'setup'] },
          { slug: 'usage', title: 'Usage', summary: '', source: 'guide', sections: ['usage'] },
          { slug: 'flags', title: 'Flags', summary: '', source: 'other', sections: ['flags'] },
          { slug: 'moved', title: 'Moved', summary: '', source: 'guide', sections: ['moved'] },
        ],
      },
    ]);
    const [setup, usage, flags, moved] = out.pages;

    expect(setup.html).toMatch(/Intro text[\s\S]*<h2 id="setup">[\s\S]*<h3 id="details">/);
    expect(setup.html).toContain('href="/docs/usage/"');
    expect(setup.html).toContain('href="/docs/flags/"');

    expect(usage.html).not.toContain('id="usage"');
    expect(usage.html).not.toContain('Moved');
    expect(usage.html).toContain('href="https://github.com/unkn0wn-root/resterm/blob/main/_examples/basic.http"');
    expect(usage.anchor).toBe('usage');

    expect(flags.html).toContain('href="/docs/setup/#setup"');
    expect(flags.html).toContain('href="/docs/moved/"');
    expect(moved.html).toContain('This part belongs');

    expect(setup.next?.href).toBe('/docs/usage/');
    expect(out.search.map((r) => r.u)).toContain('/docs/setup/#details');
  });

  it('fails when a top-level section is on no page', async () => {
    await expect(
      build(docs(), sources, [
        {
          title: 'Group',
          pages: [
            { slug: 'setup', title: 'Setup', summary: '', source: 'guide', sections: ['guide', 'setup'] },
            { slug: 'flags', title: 'Flags', summary: '', source: 'other', sections: ['flags'] },
          ],
        },
      ]),
    ).rejects.toThrow(/guide.md:13: "Usage" is on no page/);
  });

  it('fails on a link to a heading that does not exist', async () => {
    const broken = docs();
    broken.set('other.md', parseSource('other.md', '# Other\n\n## Flags\n\nSee [x](guide.md#nope).\n'));
    await expect(
      build(broken, sources, [
        {
          title: 'Group',
          pages: [
            { slug: 'all', title: 'All', summary: '', source: 'guide', sections: ['guide', 'setup', 'usage'] },
            { slug: 'flags', title: 'Flags', summary: '', source: 'other', sections: ['flags'] },
          ],
        },
      ]),
    ).rejects.toThrow(/link "guide.md#nope" points to a heading that does not exist/);
  });

  it('fails when a section is claimed twice', async () => {
    await expect(
      build(docs(), sources, [
        {
          title: 'Group',
          pages: [
            { slug: 'a', title: 'A', summary: '', source: 'guide', sections: ['guide', 'setup', 'usage'] },
            { slug: 'b', title: 'B', summary: '', source: 'guide', sections: ['setup'] },
            { slug: 'flags', title: 'Flags', summary: '', source: 'other', sections: ['flags'] },
          ],
        },
      ]),
    ).rejects.toThrow(/"setup" is on both "a" and "b"/);
  });
});

describe('parseSource', () => {
  it('gives duplicate headings the GitHub suffix', () => {
    const doc = parseSource('x.md', '# X\n\n## Examples\n\n### Examples\n\n## Other\n');
    expect([...doc.byId.keys()]).toEqual(['examples', 'examples-1', 'other']);
  });
});
