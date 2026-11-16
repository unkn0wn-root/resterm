import type { APIRoute } from 'astro';
import { getDocs } from '../lib/docs';
import { getExamples } from '../lib/examples';

export const GET: APIRoute = async () => {
  const [docs, examples] = await Promise.all([getDocs(), getExamples()]);
  return Response.json([...docs.search, ...examples.search]);
};
