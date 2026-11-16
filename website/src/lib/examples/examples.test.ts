import { describe, expect, it } from 'vitest';
import { getExamples } from '.';

describe('real examples', () => {
  it('map onto docs/examples.md and render', async () => {
    const { pages } = await getExamples();
    expect(pages.length).toBeGreaterThan(30);
    for (const page of pages) {
      expect(page.runHtml, page.slug).toContain('data-copy');
      expect(page.files.length, page.slug).toBeGreaterThan(0);
    }
  });

  it('anchors every request on a line of its file', async () => {
    const { pages } = await getExamples();
    let total = 0;

    for (const page of pages) {
      const ids = page.files.flatMap((f) => f.requests.map((r) => r.id));
      expect(new Set(ids).size, page.slug).toBe(ids.length);
      total += ids.length;

      for (const f of page.files) {
        const lines = f.code.split('\n');
        for (const r of f.requests) {
          const at = `${f.path}:${r.line}`;
          expect(r.line, at).toBeGreaterThanOrEqual(1);
          expect(r.line, at).toBeLessThanOrEqual(lines.length);
          expect(lines[r.line - 1]?.trim(), at).not.toBe('');
          expect(lines[r.line - 1], at).not.toMatch(/^\s*###/);
        }
      }
    }

    expect(total).toBeGreaterThan(50);
  });
});
