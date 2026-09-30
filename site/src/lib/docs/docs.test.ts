import { describe, expect, it } from 'vitest';
import { getDocs } from '.';

describe('real docs', () => {
  it('map onto the nav and render', async () => {
    const docs = await getDocs();
    expect(docs.pages.length).toBeGreaterThan(30);
    for (const page of docs.pages) expect(page.html.length, page.slug).toBeGreaterThan(0);
  });
});
