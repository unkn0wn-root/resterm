import { readFileSync } from 'node:fs';
import type { APIContext } from 'astro';
import { describe, expect, it } from 'vitest';
import { GET as ps1 } from './install.ps1.ts';
import { GET as sh } from './install.sh.ts';

describe('install scripts', () => {
  it.each([
    ['install.sh', sh],
    ['install.ps1', ps1],
  ])('serves %s byte for byte', async (name, get) => {
    const res = await get({} as APIContext);
    expect(Buffer.from(await res.arrayBuffer())).toEqual(readFileSync(new URL(`../../../${name}`, import.meta.url)));
  });
});
