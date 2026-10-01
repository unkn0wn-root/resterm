import type { APIRoute } from 'astro';
import sharp from 'sharp';
import { wordmark } from '../lib/wordmark';

// The social preview card: the wordmark on the dark theme background. Labels
// are drawn as pixels too, so the image does not depend on fonts being
// installed where the site is built.
const glyphs: Record<string, string[]> = {
  G: ['.##', '#..', '#.#', '#.#', '.##'],
  E: ['###', '#..', '##.', '#..', '###'],
  T: ['###', '.#.', '.#.', '.#.', '.#.'],
  '2': ['##.', '..#', '.#.', '#..', '###'],
  '0': ['.#.', '#.#', '#.#', '#.#', '.#.'],
};

const rect = (x: number, y: number, w: number, h: number) => `<rect x="${x}" y="${y}" width="${w}" height="${h}"/>`;

function label(text: string, x: number, y: number, dot: number): string {
  let out = '';
  [...text].forEach((ch, i) => {
    const glyph = glyphs[ch];
    if (!glyph) throw new Error(`Unsupported label character: ${ch}`);
    glyph.forEach((row, r) => {
      [...row].forEach((c, col) => {
        if (c === '#') out += rect(x + (i * 4 + col) * dot, y + r * dot, dot * 0.9, dot * 0.9);
      });
    });
  });
  return out;
}

export const GET: APIRoute = async () => {
  const { width, height, pixels, shade } = wordmark();
  const unit = 32;
  const left = (1200 - width * unit) / 2;
  const top = (630 - height * unit) / 2;
  const at = (x: number, y: number) => [left + x * unit, top + y * unit] as const;
  const dot = unit * 0.3;

  const [gx, gy] = at(18, -2);
  const [rx, ry] = at(8, height + 2);

  const svg = `<svg xmlns="http://www.w3.org/2000/svg" width="1200" height="630" viewBox="0 0 1200 630">
  <defs><pattern id="s" width="${unit / 2}" height="${unit / 2}" patternUnits="userSpaceOnUse">
    <rect width="${unit / 4}" height="${unit / 4}" fill="#3a2a70"/><rect x="${unit / 4}" y="${unit / 4}" width="${unit / 4}" height="${unit / 4}" fill="#3a2a70"/>
  </pattern></defs>
  <rect width="1200" height="630" fill="#07070b"/>
  <g fill="url(#s)">${shade.map((p) => rect(...at(p.x, p.y), unit, unit * 2)).join('')}</g>
  <g fill="#f5f2ff">${pixels.map((p) => rect(...at(p.x + 0.04, p.y + 0.04), unit * 0.92, unit * 0.92)).join('')}</g>
  <g fill="#6e6a86">
    ${rect(gx, gy - 1.5, unit * 3, 3)}
    <polygon points="${gx + unit * 8.1},${gy - dot * 2.5} ${gx + unit * 8.9},${gy} ${gx + unit * 8.1},${gy + dot * 2.5}"/>
    <polygon points="${rx + unit * 0.9},${ry - dot * 2.5} ${rx + unit * 0.1},${ry} ${rx + unit * 0.9},${ry + dot * 2.5}"/>
    ${rect(rx + unit * 6, ry - 1.5, unit * 3, 3)}
  </g>
  <g fill="#34d399">${label('GET', gx + unit * 4, gy - dot * 2.5, dot)}</g>
  <g fill="#6ef17e">${label('200', rx + unit * 2, ry - dot * 2.5, dot)}</g>
</svg>`;

  const png = await sharp(Buffer.from(svg)).png().toBuffer();
  return new Response(new Uint8Array(png), { headers: { 'Content-Type': 'image/png' } });
};
