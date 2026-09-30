import type { APIRoute } from 'astro';
import { getDocs } from '../lib/docs';

export const GET: APIRoute = async () => {
  const { search } = await getDocs();
  return Response.json(search);
};
