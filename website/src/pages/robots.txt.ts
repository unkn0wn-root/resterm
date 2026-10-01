import type { APIRoute } from 'astro';

export const GET: APIRoute = ({ site }) => {
  const lines = ['User-agent: *', 'Allow: /'];
  if (site) lines.push(`Sitemap: ${new URL('sitemap-index.xml', site).href}`);
  return new Response(`${lines.join('\n')}\n`);
};
