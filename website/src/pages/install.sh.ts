import type { APIRoute } from 'astro';
import script from '../../../install.sh?raw';

export const GET: APIRoute = () => new Response(script);
