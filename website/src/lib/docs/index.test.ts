import { assert, describe, expect, it } from 'vitest';
import { build } from '.';

const toc = `# Docs

Intro for GitHub.

## Group

- [Guide](guide.md): How to use it.
- [Language](rts/README.md): The language.

## Other

- [Values](rts/values.md): Values and \`types\`.
- [Examples](examples.md): Files to run.
`;

const guide = `# Guide

Intro text.

## Setup

See [usage](#usage), [values](rts/values.md#literals) and [the index](README.md).

### Details

Setup details.

## Usage

Code lives in [examples](../_examples/basic.http), listed on [one page](examples.md), and in [the CLI](../cmd/resterm/main.go).
`;

const lang = `# RestermScript

Read [values](values.md) and [setup](../guide.md#setup).
`;

const values = `# Values

## Literals

Back to the [language](README.md) and [the guide](../guide.md).
`;

const columns = `| Example | What it shows | Needs | Run | Docs |
| --- | --- | --- | --- | --- |`;

const examples = `# Examples

Intro for GitHub.

## Start here

${columns}
| [basic.http](../_examples/basic.http) with [resterm.env.json](../_examples/resterm.env.json) | Requests and \`helpers\`. | runs as-is | \`resterm run --all _examples/basic.http\` | [Values](rts/values.md#literals) |

## Scripts

${columns}
| [rts/all.http](../_examples/rts/all.http) with [rts/helpers.rts](../_examples/rts/helpers.rts) | Scripts. | start the local mock first | \`resterm _examples/rts/all.http\` | [Guide](guide.md) |
`;

const exampleCell = '| [basic.http](../_examples/basic.http) with [resterm.env.json](../_examples/resterm.env.json) |';

const docs = (extra: Record<string, string> = {}) =>
  new Map(
    Object.entries({
      'README.md': toc,
      'guide.md': guide,
      'rts/README.md': lang,
      'rts/values.md': values,
      'examples.md': examples,
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
        pages: [
          { title: 'Values', href: '/docs/rts/values/', summary: 'Values and types.' },
          { title: 'Examples', href: '/examples/', summary: 'Files to run.' },
        ],
      },
    ]);

    expect(g.title).toBe('Guide');
    expect(g.anchor).toBe('guide');
    expect(g.source.path).toBe('docs/guide.md');
    expect(g.html).toMatch(/Intro text[\s\S]*<h2 id="setup">[\s\S]*<h3 id="details">/);
    expect(g.html).toContain('href="#usage"');
    expect(g.html).toContain('href="/docs/rts/values/#literals"');
    expect(g.html).toContain('href="/docs/"');
    expect(g.html).toContain('href="/examples/basic/"');
    expect(g.html).toContain('href="/examples/"');
    expect(g.html).toContain('href="https://github.com/unkn0wn-root/resterm/blob/main/cmd/resterm/main.go"');
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

  it('maps examples.md onto example groups instead of a page', async () => {
    const out = await build(docs());
    expect(out.pages.map((p) => p.slug)).toEqual(['guide', 'rts', 'rts/values']);
    expect(out.search.map((r) => r.u)).not.toContain('/docs/examples/');

    expect(out.examples).toEqual([
      {
        title: 'Start here',
        examples: [
          {
            slug: 'basic',
            href: '/examples/basic/',
            title: 'basic.http',
            summary: 'Requests and helpers.',
            needs: 'runs as-is',
            run: 'resterm run --all _examples/basic.http',
            docs: { title: 'Values', href: '/docs/rts/values/#literals' },
            files: ['_examples/basic.http', '_examples/resterm.env.json'],
            line: 9,
          },
        ],
      },
      {
        title: 'Scripts',
        examples: [
          {
            slug: 'rts/all',
            href: '/examples/rts/all/',
            title: 'rts/all.http',
            summary: 'Scripts.',
            needs: 'start the local mock first',
            run: 'resterm _examples/rts/all.http',
            docs: { title: 'Guide', href: '/docs/guide/' },
            files: ['_examples/rts/all.http', '_examples/rts/helpers.rts'],
            line: 15,
          },
        ],
      },
    ]);
  });

  it('fails when examples.md is not in the table of contents', async () => {
    const input = docs({ 'README.md': toc.replace('- [Examples](examples.md): Files to run.\n', '') });
    await expect(build(input)).rejects.toThrow('docs/examples.md is on no page. Add it to docs/README.md.');
  });

  it.each([
    ['../_examples/nope.http', '_examples/nope.http'],
    ['../_examples/rts/helpers.rts', '_examples/rts/helpers.rts'],
    ['../_examples/rts/', '_examples/rts/'],
    ['../_examples/nope.http#L3', '_examples/nope.http'],
  ])('fails on a link to %s, which is not an example', async (url, path) => {
    const input = docs({ 'guide.md': guide.replace('../_examples/basic.http', url) });
    await expect(build(input)).rejects.toThrow(
      `docs/guide.md: link "${url}" points to ${path}, which is not an example`,
    );
  });

  it.each([
    ['../LICENSE%', 'https://github.com/unkn0wn-root/resterm/blob/main/LICENSE%25'],
    ['../_examples%2fbasic.http', 'https://github.com/unkn0wn-root/resterm/blob/main/_examples%2fbasic.http'],
    ['../_examples-old/x.http', 'https://github.com/unkn0wn-root/resterm/blob/main/_examples-old/x.http'],
  ])('links %s to GitHub', async (url, href) => {
    const out = await build(docs({ 'guide.md': guide.replace('../cmd/resterm/main.go', url) }));
    expect(out.pages[0]?.html).toContain(`href="${href}"`);
  });

  it('fails on a link to a heading in an example', async () => {
    const input = docs({ 'guide.md': guide.replace('../_examples/basic.http', '../_examples/basic.http#L3') });
    await expect(build(input)).rejects.toThrow(
      'docs/guide.md: link "../_examples/basic.http#L3" points to a heading the example page does not have',
    );
  });

  it('fails when examples.md is listed twice', async () => {
    const input = docs({ 'README.md': `${toc}- [Examples again](examples.md): Again.\n` });
    await expect(build(input)).rejects.toThrow('docs/README.md:14: docs/examples.md is listed twice');
  });

  it('reports a Docs link to a later example only as a link to a non-docs page', async () => {
    const text = examples.replace('[Values](rts/values.md#literals)', '[All](../_examples/rts/all.http)');
    await expect(build(docs({ 'examples.md': text }))).rejects.toThrow(
      new Error(
        'docs/examples.md does not map onto _examples/:\n  docs/examples.md:9: the Docs cell must link to a docs page',
      ),
    );
  });

  it('collects Docs cell errors from every row', async () => {
    const text = examples
      .replace('[Values](rts/values.md#literals)', '[Gone](gone.md)')
      .replace('[Guide](guide.md)', '[Guide](guide.md#nope)');
    await expect(build(docs({ 'examples.md': text }))).rejects.toThrow(
      [
        'docs/examples.md:9: link "gone.md" points to docs/gone.md, which is not a page',
        'docs/examples.md:15: link "guide.md#nope" points to a heading that does not exist',
      ].join('\n  '),
    );
  });

  it('fails on a link to a heading in examples.md', async () => {
    const input = docs({ 'guide.md': guide.replace('(examples.md)', '(examples.md#scripts)') });
    await expect(build(input)).rejects.toThrow(
      'docs/guide.md: link "examples.md#scripts" points to a heading the examples page does not have',
    );
  });

  it.each([
    {
      name: 'a file listed twice',
      text: examples.replace(
        '[rts/helpers.rts](../_examples/rts/helpers.rts)',
        '[resterm.env.json](../_examples/resterm.env.json)',
      ),
      error: 'docs/examples.md:15: _examples/resterm.env.json is listed twice',
    },
    {
      name: 'two examples on one route',
      text: `${examples}| [a.http](../_examples/a.http) | A. | runs as-is | \`a\` | [Guide](guide.md) |
| [a.rts](../_examples/a.rts) | A. | runs as-is | \`a\` | [Guide](guide.md) |
`,
      error: 'docs/examples.md:17: _examples/a.http and _examples/a.rts both map to "/examples/a/"',
    },
    {
      name: 'a file outside _examples/',
      text: examples.replace('[resterm.env.json](../_examples/resterm.env.json)', '[guide](guide.md)'),
      error: 'docs/examples.md:9: link "guide.md" does not point into _examples/',
    },
    {
      name: 'an absolute link to an example',
      text: examples.replace('(../_examples/basic.http)', '(https://host/_examples/basic.http)'),
      error: 'docs/examples.md:9: link "https://host/_examples/basic.http" does not point into _examples/',
    },
    {
      name: 'a link to a heading in an example',
      text: examples.replace('(../_examples/basic.http)', '(../_examples/basic.http#L3)'),
      error: 'docs/examples.md:9: link "../_examples/basic.http#L3" points to a heading the example page does not have',
    },
    {
      name: 'a renamed column',
      text: examples.replace('| Example | What it shows |', '| Example | Shows |'),
      error: 'docs/examples.md:7: the columns must be "Example | What it shows | Needs | Run | Docs"',
    },
    {
      name: 'a missing column',
      text: examples.replace(columns, '| Example | Needs | Run | Docs |\n| --- | --- | --- | --- |'),
      error: 'docs/examples.md:7: the columns must be',
    },
    {
      name: 'a row with a missing cell',
      text: examples.replace(' [Values](rts/values.md#literals) |', ''),
      error: 'docs/examples.md:9: the row must have 5 cells',
    },
    {
      name: 'an unknown Needs value',
      text: examples.replace('| runs as-is |', '| runs |'),
      error:
        'docs/examples.md:9: "runs" is not a Needs value. Use one of: runs as-is, start the local mock first, needs a GitHub token, needs your own server, needs your own SSH host, needs your own cluster',
    },
    {
      name: 'a Run cell with two code spans',
      text: examples.replace('`resterm run --all _examples/basic.http`', '`a` `b`'),
      error: 'docs/examples.md:9: the Run cell must be one code span',
    },
    {
      name: 'a Run cell without a code span',
      text: examples.replace('`resterm run --all _examples/basic.http`', 'resterm'),
      error: 'docs/examples.md:9: the Run cell must be one code span',
    },
    {
      name: 'a Docs cell with two links',
      text: examples.replace('[Values](rts/values.md#literals)', '[Guide](guide.md) [Values](rts/values.md)'),
      error: 'docs/examples.md:9: the Docs cell must be one link',
    },
    {
      name: 'a Docs cell without a link',
      text: examples.replace('[Values](rts/values.md#literals)', 'Values'),
      error: 'docs/examples.md:9: the Docs cell must be one link',
    },
    {
      name: 'an Example cell without a link',
      text: examples.replace(exampleCell, '| basic.http |'),
      error: 'docs/examples.md:9: the Example cell needs a link to a file',
    },
    {
      name: 'a paragraph before a table',
      text: examples.replace('## Scripts\n', '## Scripts\n\nA note.\n'),
      error: 'docs/examples.md:13: expected only one table under "Scripts"',
    },
    {
      name: 'a list before a table',
      text: examples.replace('## Scripts\n', '## Scripts\n\n- a note\n'),
      error: 'docs/examples.md:13: expected only one table under "Scripts"',
    },
    {
      name: 'a paragraph after a table',
      text: `${examples}\nA closing note.\n`,
      error: 'docs/examples.md:17: expected only one table under "Scripts"',
    },
    {
      name: 'a second table',
      text: `${examples}\n${columns}\n`,
      error: 'docs/examples.md:17: expected only one table under "Scripts"',
    },
    {
      name: 'a group without a table',
      text: `${examples}\n## Empty\n`,
      error: 'docs/examples.md:17: "Empty" has no table',
    },
    {
      name: 'a list before the first group',
      text: examples.replace('Intro for GitHub.', '- intro'),
      error: 'docs/examples.md:3: write only paragraphs before the first group',
    },
    {
      name: 'a heading before the first group',
      text: examples.replace('Intro for GitHub.', '### Note'),
      error: 'docs/examples.md:3: write only paragraphs before the first group',
    },
    {
      name: 'a second title',
      text: examples.replace('Intro for GitHub.', '# Again'),
      error: 'docs/examples.md:3: write only one "# Title" heading',
    },
    {
      name: 'a row with an extra cell',
      text: examples.replace('(rts/values.md#literals) |', '(rts/values.md#literals) | extra |'),
      error: 'docs/examples.md:9: the row must have 5 cells',
    },
    {
      name: 'a table without rows',
      text: `${examples}\n## Empty\n\n${columns}\n`,
      error: 'docs/examples.md:19: "Empty" has no rows',
    },
    {
      name: 'an empty What it shows cell',
      text: examples.replace('| Requests and `helpers`. |', '|  |'),
      error: 'docs/examples.md:9: the What it shows cell must not be empty',
    },
    {
      name: 'an empty Run command',
      text: examples.replace('`resterm run --all _examples/basic.http`', '` `'),
      error: 'docs/examples.md:9: the Run cell must not be empty',
    },
    {
      name: 'an Example link without a label',
      text: examples.replace('[basic.http](../_examples/basic.http)', '[](../_examples/basic.http)'),
      error: 'docs/examples.md:9: an Example link has no label',
    },
    {
      name: 'a Docs link to a missing page',
      text: examples.replace('[Values](rts/values.md#literals)', '[Gone](gone.md)'),
      error: 'docs/examples.md:9: link "gone.md" points to docs/gone.md, which is not a page',
    },
    {
      name: 'a Docs link to a missing heading',
      text: examples.replace('[Values](rts/values.md#literals)', '[Values](rts/values.md#nope)'),
      error: 'docs/examples.md:9: link "rts/values.md#nope" points to a heading that does not exist',
    },
    {
      name: 'a Docs link to a website',
      text: examples.replace('[Values](rts/values.md#literals)', '[Google](https://google.com)'),
      error: 'docs/examples.md:9: the Docs cell must link to a docs page',
    },
    {
      name: 'a Docs link to a repo file',
      text: examples.replace('[Values](rts/values.md#literals)', '[Readme](../README.md)'),
      error: 'docs/examples.md:9: the Docs cell must link to a docs page',
    },
    {
      name: 'a Docs link to the examples page',
      text: examples.replace('[Values](rts/values.md#literals)', '[Examples](examples.md)'),
      error: 'docs/examples.md:9: the Docs cell must link to a docs page',
    },
    {
      name: 'a Docs link to its own example',
      text: examples.replace('[Values](rts/values.md#literals)', '[Basic](../_examples/basic.http)'),
      error: 'docs/examples.md:9: the Docs cell must link to a docs page',
    },
    {
      name: 'a Docs link without a label',
      text: examples.replace('[Values](rts/values.md#literals)', '[](guide.md)'),
      error: 'docs/examples.md:9: the Docs link has no label',
    },
  ])('fails on $name', async ({ text, error }) => {
    await expect(build(docs({ 'examples.md': text }))).rejects.toThrow(error);
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
