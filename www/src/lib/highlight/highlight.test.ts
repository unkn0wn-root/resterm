import { describe, expect, it } from 'vitest';
import { highlight } from '.';

async function tokens(code: string, lang: string) {
  const { pre } = await highlight(code, lang);
  const out: [string, string][] = [];
  const walk = (node: any) => {
    if (node.type === 'element' && node.tagName === 'span' && typeof node.properties?.style === 'string') {
      const color = /color:var\(--syn-([\w-]+)\)/.exec(node.properties.style)?.[1];
      const text = node.children.map((c: any) => c.value ?? '').join('');
      if (color && text.trim()) out.push([text.trim(), color]);
    }
    node.children?.forEach(walk);
  };
  walk(pre);
  return out;
}

describe('resterm http grammar', () => {
  it('colors directives like the TUI editor', async () => {
    const t = await tokens('### Pay\n# @mock method=POST path=/pay\n# @name pay\nPOST {{base}}/pay\n// note', 'http');
    expect(t).toEqual([
      ['### Pay', 'separator'],
      ['#', 'comment'],
      ['@mock', 'directive'],
      ['method', 'key'],
      ['=', 'value'],
      ['POST', 'option'],
      ['path', 'key'],
      ['=', 'value'],
      ['/pay', 'option'],
      ['#', 'comment'],
      ['@name', 'directive'],
      ['pay', 'value'],
      ['POST {{base}}/pay', 'request'],
      ['// note', 'comment'],
    ]);
  });

  it('splits @setting into key and value', async () => {
    const t = await tokens('# @setting base-url https://api.example.com/', 'http');
    expect(t).toContainEqual(['base-url', 'key']);
    expect(t).toContainEqual(['https://api.example.com/', 'option']);
  });

  it('treats a doubled marker as a comment', async () => {
    expect(await tokens('## @if not a directive', 'http')).toEqual([['## @if not a directive', 'comment']]);
  });
});

describe('restermscript grammar', () => {
  it('colors keyword classes, functions and methods', async () => {
    const t = await tokens('export fn mode(env) {\n  if env.has("x") { return true }\n}', 'rts');
    expect(t).toContainEqual(['export', 'directive']);
    expect(t).toContainEqual(['mode', 'function']);
    expect(t).toContainEqual(['if', 'control']);
    expect(t).toContainEqual(['has', 'option']);
    expect(t).toContainEqual(['"x"', 'value']);
    expect(t).toContainEqual(['true', 'literal']);
  });
});

describe('languages', () => {
  it('falls back to plain text for unknown languages', async () => {
    expect((await highlight('x', 'nope')).label).toBe('text');
    expect((await highlight('x', 'sh')).label).toBe('shell');
    expect((await highlight('x', 'http')).label).toBe('http');
  });
});
