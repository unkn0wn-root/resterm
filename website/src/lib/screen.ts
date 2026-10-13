export interface Style {
  fg?: string;
  bg?: string;
  a?: string;
}

export type Run = [style: number, text: string, width?: number];

export interface Frames {
  cols: number;
  rows: number;
  styles: Style[];
  frames: { id: string; lines: Run[][] }[];
}

// Let the font connect box borders. Give other symbols a fixed cell width.
const plain = /^[\0-\x7f\u2500-\u257f]$/;

// Draw blocks with CSS to avoid gaps, and draw ⬢ because the font lacks it.
const drawn: Record<string, string> = {
  '█': 'g d full',
  '▀': 'g d top',
  '▄': 'g d e e4',
  '▁': 'g d e e1',
  '▂': 'g d e e2',
  '▃': 'g d e e3',
  '▅': 'g d e e5',
  '▆': 'g d e e6',
  '▇': 'g d e e7',
  '▌': 'g d left',
  '▐': 'g d right',
  '░': 'g d shade',
  '⬢': 'g d hex',
};

const escape = (s: string) => s.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;');

const color = (c: string | undefined, def: string) => (c?.startsWith('@') ? `var(--ansi-${c.slice(1)})` : c || def);

function paint({ fg, bg, a = '' }: Style) {
  let f = color(fg, 'var(--screen-fg)');
  let b = color(bg, '');
  if (a.includes('r')) [f, b] = [b || 'var(--screen-bg)', f];
  if (a.includes('f')) f = `color-mix(in srgb, ${f} 55%, transparent)`;
  const cls = [a.includes('b') && 'B', a.includes('i') && 'I', a.includes('u') && 'U'].filter(Boolean).join(' ');
  return { cls, fg: fg || a ? f : '', bg: b };
}

// Variation selectors must stay with their base character in one cell.
const graphemes = new Intl.Segmenter(undefined, { granularity: 'grapheme' });

function text(s: string): string {
  let out = '';
  for (const { segment: ch } of graphemes.segment(s)) {
    out += plain.test(ch) ? escape(ch) : `<span class="${drawn[ch] ?? 'g'}">${ch}</span>`;
  }
  return out;
}

// Share the background span to avoid seams when cell widths fall between pixels.
export function renderScreen(styles: Style[], lines: Run[][]): string {
  const paints = styles.map(paint);
  const run = ([id, t, w]: Run, bg: string) => {
    const { cls, fg } = paints[id]!;
    const style = [fg && `color:${fg}`, bg && `background:${bg}`].filter(Boolean).join(';');
    const cell = w ? `<span class="g g2">${escape(t)}</span>` : text(t);
    return `<span${cls && ` class="${cls}"`}${style && ` style="${style}"`}>${cell}</span>`;
  };
  return lines
    .map((line) => {
      const groups: { bg: string; runs: Run[] }[] = [];
      for (const r of line) {
        const { bg } = paints[r[0]]!;
        const last = groups.at(-1);
        if (bg && last?.bg === bg) last.runs.push(r);
        else groups.push({ bg, runs: [r] });
      }
      const html = groups.map(({ bg, runs }) =>
        runs.length === 1
          ? run(runs[0]!, bg)
          : `<span style="background:${bg}">${runs.map((r) => run(r, '')).join('')}</span>`,
      );
      return `<div class="r">${html.join('')}</div>`;
    })
    .join('');
}
