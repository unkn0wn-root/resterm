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
});
