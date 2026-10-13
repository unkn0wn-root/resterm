import { describe, expect, it } from 'vitest';
import darkHidden from '../content/tour/frames-dark-editor-hidden.json';
import dark from '../content/tour/frames-dark.json';
import lightHidden from '../content/tour/frames-light-editor-hidden.json';
import light from '../content/tour/frames-light.json';
import tour from '../content/tour/tour.json';
import { renderScreen, type Frames, type Run, type Style } from './screen';

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
