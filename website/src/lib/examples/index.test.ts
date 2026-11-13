import { assert, describe, expect, it } from 'vitest';
import type { ExampleGroup } from '../docs';
import { build } from '.';

const groups: ExampleGroup[] = [
  {
    title: 'Start here',
    examples: [
      {
        slug: 'basic',
        href: '/examples/basic/',
        title: 'basic.http',
        summary: 'Requests.',
        needs: 'runs as-is',
        run: 'resterm run --all _examples/basic.http',
        docs: { title: 'Guide', href: '/docs/guide/' },
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

const text = (html: string) => html.replace(/<[^>]+>/g, '');

const texts = (extra: Record<string, string> = {}) =>
  new Map(
    Object.entries({
      '_examples/basic.http': 'GET https://example.com\n',
      '_examples/resterm.env.json': '{}\n',
      '_examples/rts/all.http': '# @use ./helpers.rts\nGET https://example.com\n',
      '_examples/rts/helpers.rts': 'export fn mode() {\n  return "dev"\n}\n',
      '_examples/themes/dark.toml': '[metadata]\n',
      ...extra,
    }),
  );

describe('build', () => {
  it('turns each example into a page with its files', async () => {
    const out = await build(groups, texts());
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
      },
      {
        path: '_examples/rts/helpers.rts',
        name: 'helpers.rts',
        lang: 'rts',
        code: 'export fn mode() {\n  return "dev"\n}\n',
        url: 'https://github.com/unkn0wn-root/resterm/blob/main/_examples/rts/helpers.rts',
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
    await expect(build(groups, input)).rejects.toThrow('docs/examples.md:15: _examples/rts/helpers.rts does not exist');
  });

  it.each(['_examples/notes.txt', '_examples/rts/resterm.env.json', '_examples/themes/resterm.env.json'])(
    'fails on %s, which is on no example',
    async (path) => {
      await expect(build(groups, texts({ [path]: '{}\n' }))).rejects.toThrow(
        `${path} is on no example. Add it to docs/examples.md.`,
      );
    },
  );
});
