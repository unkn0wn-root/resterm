import { describe, expect, it } from 'vitest';
import darkHidden from '../content/tour/frames-dark-editor-hidden.json';
import dark from '../content/tour/frames-dark.json';
import lightHidden from '../content/tour/frames-light-editor-hidden.json';
import light from '../content/tour/frames-light.json';
import panesDarkHidden from '../content/tour/panes-dark-editor-hidden.json';
import panesDark from '../content/tour/panes-dark.json';
import panesLightHidden from '../content/tour/panes-light-editor-hidden.json';
import panesLight from '../content/tour/panes-light.json';
import tour from '../content/tour/tour.json';
import { renderScreen, type Frames, type PaneData, type Run, type Style } from './screen';

const one = (style: Style, ...runs: Run[]) => renderScreen([style], [runs]);

describe('renderScreen', () => {
  it('escapes text and keeps ASCII and box lines as plain characters', () => {
    expect(one({}, [0, 'a<b>&╭─│'])).toBe('<div class="r"><span>a&lt;b&gt;&amp;╭─│</span></div>');
  });

  it('gives other symbols a cell, and draws block characters with CSS', () => {
    expect(one({}, [0, '⇄█'], [0, '字', 2])).toBe(
      '<div class="r"><span><span class="g">⇄</span><span class="g d full">█</span></span>' +
        '<span><span class="g g2">字</span></span></div>',
    );
  });

  it('keeps a variation selector in the cell of the character before it', () => {
    expect(one({}, [0, '✔\ufe0e ok'])).toBe('<div class="r"><span><span class="g">✔\ufe0e</span> ok</span></div>');
  });

  it('maps colors and attributes', () => {
    expect(one({ fg: '@7', a: 'b' }, [0, 'x'])).toContain('class="B" style="color:var(--ansi-7)"');
    expect(one({ fg: '#ffffff', bg: '#000000', a: 'r' }, [0, 'x'])).toContain(
      'style="color:#000000;background:#ffffff"',
    );
    expect(one({ fg: '#ffffff', a: 'f' }, [0, 'x'])).toContain('color:color-mix(in srgb, #ffffff 55%, transparent)');
  });

  it('paints one background under runs that share it', () => {
    const styles = [
      { fg: '#ffffff', bg: '#8fbc8f' },
      { fg: '#000000', bg: '#8fbc8f', a: 'b' },
      { fg: '#8fbc8f', a: 'r' },
      { bg: '#000000' },
    ];
    expect(
      renderScreen(styles, [
        [
          [0, ' '],
          [1, '>_'],
          [2, ' '],
          [0, ' '],
          [3, ' '],
        ],
      ]),
    ).toBe(
      '<div class="r"><span style="background:#8fbc8f"><span style="color:#ffffff"> </span>' +
        '<span class="B" style="color:#000000">&gt;_</span><span style="color:var(--screen-bg)"> </span>' +
        '<span style="color:#ffffff"> </span></span><span style="background:#000000"> </span></div>',
    );
  });
});

describe('tour frames', () => {
  const sets = Object.entries({ dark, light, darkHidden, lightHidden }) as [string, Frames][];

  it('match the steps in both themes and both editor layouts', () => {
    const ids = tour.steps.map((s) => s.id);
    for (const [, f] of sets) {
      expect(f.frames.map((fr) => fr.id)).toEqual(ids);
      expect([f.cols, f.rows]).toEqual([tour.cols, tour.rows]);
      for (const fr of f.frames) expect(fr.lines).toHaveLength(tour.rows);
    }
  });

  const cells = (row: string) =>
    row
      .replace(/<span class="g( g2)?[^"]*">[^<]*<\/span>/g, (_, wide) => (wide ? '__' : '_'))
      .replace(/<[^>]+>/g, '')
      .replace(/&\w+;/g, '_').length;

  it('render every row at full width, so the pane borders line up', () => {
    for (const [name, f] of sets) {
      for (const fr of f.frames) {
        const rows = renderScreen(f.styles, fr.lines).split('</div>').slice(0, -1);
        rows.forEach((row, y) => expect(cells(row), `${name} ${fr.id} row ${y}`).toBe(f.cols));
      }
    }
  });
});

describe('tour panes', () => {
  const sets = [
    ['dark', dark, panesDark],
    ['light', light, panesLight],
    ['darkHidden', darkHidden, panesDarkHidden],
    ['lightHidden', lightHidden, panesLightHidden],
  ] as [string, Frames, PaneData][];

  const graphemes = new Intl.Segmenter(undefined, { granularity: 'grapheme' });
  // Style IDs differ between captures, so compare the style values for each cell.
  const expand = (styles: Style[], row: Run[]) =>
    row.flatMap(([id, t, w]) => {
      const st = JSON.stringify(styles[id]);
      return w ? [st + t, ...Array<string>(w - 1).fill(st)] : [...graphemes.segment(t)].map((g) => st + g.segment);
    });

  const all = sets.flatMap(([name, f, pd]) =>
    pd.frames.flatMap(({ id, panes }, i) => panes.map((p) => ({ at: `${name} ${id} ${p.id}`, f, pd, i, p, panes }))),
  );

  it('match the steps and keep every row at full width', () => {
    const ids = tour.steps.map((s) => s.id);
    for (const [, , pd] of sets) expect(pd.frames.map((fr) => fr.id)).toEqual(ids);
    for (const { at, f, pd, i, p } of all) {
      expect(p.focus === undefined, at).toBe(p.id === tour.steps[i]!.focus);
      if (p.focus) expect(p.focus, at).toHaveLength(f.rows);
      for (const row of p.focus ?? []) expect(expand(pd.styles, row), at).toHaveLength(f.cols);
      const b = p.body;
      if (!b) continue;
      const [x, y, w, h] = p.box;
      expect([b.x, b.w, b.y + b.h], at).toEqual([x + 1, w - 2, y + h - 1]);
      const strip = pd.strips[b.strip]!;
      expect(strip.length, at).toBeGreaterThan(b.h);
      for (const row of strip) expect(expand(pd.styles, row), at).toHaveLength(b.w);
    }
  });

  it('start each body with the rows the screen shows before any scroll', () => {
    for (const { at, f, pd, i, p, panes } of all) {
      const b = p.body;
      if (!b) continue;
      const rows = (styles: Style[], lines: Run[][]) =>
        lines.slice(b.y, b.y + b.h).map((row) => expand(styles, row).slice(b.x, b.x + b.w));
      const top = pd.strips[b.strip]!.slice(0, b.h).map((row) => expand(pd.styles, row));
      const step = rows(f.styles, f.frames[i]!.lines);
      expect(top, at).toEqual(p.focus ? rows(pd.styles, p.focus) : step);
      if (!b.keep) continue;
      for (const q of panes) {
        if (q !== p) expect(top, `${at} under ${q.id}`).toEqual(q.focus ? rows(pd.styles, q.focus) : step);
      }
    }
  });
});
