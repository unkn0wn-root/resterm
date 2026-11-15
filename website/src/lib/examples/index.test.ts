import { hash } from 'node:crypto';
import { assert, describe, expect, it } from 'vitest';
import type { Example, ExampleGroup } from '../docs';
import { build, type Outline, type OutlineEntry } from '.';

const basicExample: Example = {
  slug: 'basic',
  href: '/examples/basic/',
  title: 'basic.http',
  summary: 'Requests.',
  needs: 'runs as-is',
  run: 'resterm run --all _examples/basic.http',
  docs: { title: 'Guide', href: '/docs/guide/' },
  files: ['_examples/basic.http', '_examples/resterm.env.json'],
  line: 9,
};

const groups: ExampleGroup[] = [
  {
    title: 'Start here',
    examples: [basicExample],
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
        docs: { title: 'Values', href: '/docs/rts/values/' },
        files: ['_examples/rts/all.http', '_examples/rts/helpers.rts'],
        line: 15,
      },
      {
        slug: 'themes/dark',
        href: '/examples/themes/dark/',
        title: 'themes/dark.toml',
        summary: 'A theme.',
        needs: 'runs as-is',
        run: 'RESTERM_THEMES_DIR=_examples/themes resterm',
        docs: { title: 'Theming', href: '/docs/theming/' },
        files: ['_examples/themes/dark.toml'],
        line: 16,
      },
    ],
  },
];

const files = {
  '_examples/basic.http': 'GET https://example.com\n',
  '_examples/resterm.env.json': '{}\n',
  '_examples/rts/all.http': '# @use ./helpers.rts\nGET https://example.com\n',
  '_examples/rts/helpers.rts': 'export fn mode() {\n  return "dev"\n}\n',
  '_examples/themes/dark.toml': '[metadata]\n',
} satisfies Record<string, string>;

const request = { line: 1, method: 'GET', name: 'https://example.com' };

const stale = 'does not match website/src/content/requests.json. Run make website-requests.';

const strayEntry =
  'is in website/src/content/requests.json but is not a request file in _examples/. Run make website-requests.';

const text = (html: string) => html.replace(/<[^>]+>/g, '');

const texts = (extra: Record<string, string> = {}) => new Map(Object.entries({ ...files, ...extra }));

// entry is the requests.json value for a file with this text.
const entry = (code: string, ...requests: OutlineEntry['requests']) => ({
  sha256: hash('sha256', code),
  requests,
});

const outline = (extra: Outline = {}): Outline => ({
  '_examples/basic.http': entry(files['_examples/basic.http'], request),
  '_examples/rts/all.http': entry(files['_examples/rts/all.http'], { ...request, line: 2 }),
  ...extra,
});

const page = (...paths: string[]): ExampleGroup[] => [
  { title: 'Start here', examples: [{ ...basicExample, files: paths }] },
];

describe('build', () => {
  it('turns each example into a page with its files', async () => {
    const out = await build(groups, texts(), outline());
    const [basic, rts, theme] = out.pages;
    assert.exists(basic);
    assert.exists(rts);
    assert.exists(theme);

    expect(out.pages.map((p) => p.slug)).toEqual(['basic', 'rts/all', 'themes/dark']);
    expect(out.groups).toEqual([
      {
        title: 'Start here',
        pages: [{ title: 'basic.http', href: '/examples/basic/', summary: 'Requests.' }],
      },
      {
        title: 'Scripts',
        pages: [
          { title: 'rts/all.http', href: '/examples/rts/all/', summary: 'Scripts.' },
          { title: 'themes/dark.toml', href: '/examples/themes/dark/', summary: 'A theme.' },
        ],
      },
    ]);

    expect(rts.group).toBe('Scripts');
    expect(rts.needs).toBe('start the local mock first');
    expect(rts.docs).toEqual({ title: 'Values', href: '/docs/rts/values/' });
    expect(rts.files).toEqual([
      {
        path: '_examples/rts/all.http',
        name: 'all.http',
        lang: 'http',
        code: '# @use ./helpers.rts\nGET https://example.com\n',
        url: 'https://github.com/unkn0wn-root/resterm/blob/main/_examples/rts/all.http',
        requests: [{ id: 'file-0-L2', line: 2, method: 'GET', name: 'https://example.com' }],
      },
      {
        path: '_examples/rts/helpers.rts',
        name: 'helpers.rts',
        lang: 'rts',
        code: 'export fn mode() {\n  return "dev"\n}\n',
        url: 'https://github.com/unkn0wn-root/resterm/blob/main/_examples/rts/helpers.rts',
        requests: [],
      },
    ]);
    expect(basic.files.map((f) => f.path)).toEqual(['_examples/basic.http', '_examples/resterm.env.json']);
    expect(theme.files.map((f) => f.path)).toEqual(['_examples/themes/dark.toml']);

    expect(rts.runHtml).toContain('data-copy');
    expect(text(rts.runHtml)).toContain('resterm _examples/rts/all.http');
    expect(out.clone).toContain('data-copy');
    expect(text(out.clone)).toContain(
      'git clone --depth 1 --branch main https://github.com/unkn0wn-root/resterm.git\ncd resterm',
    );

    expect(basic.prev).toBeUndefined();
    expect(basic.next?.href).toBe('/examples/rts/all/');
    expect(rts.prev?.title).toBe('basic.http');
    expect(rts.next?.href).toBe('/examples/themes/dark/');
    expect(theme.next).toBeUndefined();

    expect(out.search).toContainEqual({
      h: 'rts/all.http',
      p: 'rts/all.http',
      g: 'Examples',
      u: '/examples/rts/all/',
      t: 'Scripts. _examples/rts/all.http _examples/rts/helpers.rts',
    });
  });

  it('fails on a listed file that does not exist', async () => {
    const input = texts();
    input.delete('_examples/rts/helpers.rts');
    await expect(build(groups, input, outline())).rejects.toThrow(
      'docs/examples.md:15: _examples/rts/helpers.rts does not exist',
    );
  });

  it.each(['_examples/notes.txt', '_examples/rts/resterm.env.json', '_examples/themes/resterm.env.json'])(
    'fails on %s, which is on no example',
    async (path) => {
      await expect(build(groups, texts({ [path]: '{}\n' }), outline())).rejects.toThrow(
        `${path} is on no example. Add it to docs/examples.md.`,
      );
    },
  );

  it('gives each request an id from its tab and line', async () => {
    const a = '###\nGET https://a.example\n';
    const b = '###\nGET https://b.example\n';
    const out = await build(
      page('_examples/a.http', '_examples/env.json', '_examples/b.http'),
      new Map([
        ['_examples/a.http', a],
        ['_examples/env.json', '{}\n'],
        ['_examples/b.http', b],
      ]),
      {
        '_examples/a.http': entry(a, { line: 2, method: 'GET', name: 'https://a.example' }),
        '_examples/b.http': entry(b, { line: 2, method: 'GET', name: 'https://b.example' }),
      },
    );

    expect(out.pages[0]?.files.map((f) => f.requests)).toEqual([
      [{ id: 'file-0-L2', line: 2, method: 'GET', name: 'https://a.example' }],
      [],
      [{ id: 'file-2-L2', line: 2, method: 'GET', name: 'https://b.example' }],
    ]);
  });

  it('leaves other files without requests', async () => {
    const paths = ['_examples/env.json', '_examples/helpers.rts', '_examples/dark.toml', '_examples/notes.http.bak'];
    const out = await build(page(...paths), new Map(paths.map((p) => [p, 'GET https://example.com\n'])), {});
    expect(out.pages[0]?.files.map((f) => [f.path, f.requests])).toEqual(paths.map((p) => [p, []]));
  });

  it('fails on a request file that changed after requests.json was made', async () => {
    await expect(
      build(groups, texts({ '_examples/basic.http': 'GET https://example.com/v2\n' }), outline()),
    ).rejects.toThrow(`_examples/basic.http ${stale}`);
  });

  it.each(['_examples/a.http', '_examples/a.rest', '_examples/A.HTTP'])(
    'fails on %s, a request file that requests.json does not list',
    async (path) => {
      await expect(build(page(path), new Map([[path, 'GET https://example.com\n']]), {})).rejects.toThrow(
        `${path} ${stale}`,
      );
    },
  );

  it('fails on a requests.json entry for a file that is gone', async () => {
    const gone = { '_examples/gone.http': entry('GET https://example.com\n', request) };
    await expect(build(groups, texts(), outline(gone))).rejects.toThrow(`_examples/gone.http ${strayEntry}`);
  });

  it('fails on a requests.json entry for a file that is not a request file', async () => {
    const script = { '_examples/rts/helpers.rts': entry(files['_examples/rts/helpers.rts'], request) };
    await expect(build(groups, texts(), outline(script))).rejects.toThrow(`_examples/rts/helpers.rts ${strayEntry}`);
  });
});
