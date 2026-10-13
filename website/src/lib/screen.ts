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

// The font joins box lines across rows. Other symbols need explicit cell widths.
const plain = /^[\0-\x7f\u2500-\u257f]$/;

// CSS fills block cells without gaps and supplies the missing ⬢ glyph.
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

function attrs({ fg, bg, a = '' }: Style): string {
  let f = color(fg, 'var(--screen-fg)');
  let b = color(bg, '');
  if (a.includes('r')) [f, b] = [b || 'var(--screen-bg)', f];
  if (a.includes('f')) f = `color-mix(in srgb, ${f} 55%, transparent)`;
  const style = [(fg || a) && `color:${f}`, b && `background:${b}`].filter(Boolean).join(';');
  const cls = [a.includes('b') && 'B', a.includes('i') && 'I', a.includes('u') && 'U'].filter(Boolean).join(' ');
  return (cls && ` class="${cls}"`) + (style && ` style="${style}"`);
}

// Keep variation selectors in the same cell as their base character.
const graphemes = new Intl.Segmenter(undefined, { granularity: 'grapheme' });

function text(s: string): string {
  let out = '';
  for (const { segment: ch } of graphemes.segment(s)) {
    out += plain.test(ch) ? escape(ch) : `<span class="${drawn[ch] ?? 'g'}">${ch}</span>`;
  }
  return out;
}

export function renderScreen(styles: Style[], lines: Run[][]): string {
  const spans = styles.map(attrs);
  return lines
    .map((line) => {
      const runs = line.map(
        ([id, t, w]) => `<span${spans[id]}>${w ? `<span class="g g2">${escape(t)}</span>` : text(t)}</span>`,
      );
      return `<div class="r">${runs.join('')}</div>`;
    })
    .join('');
}
