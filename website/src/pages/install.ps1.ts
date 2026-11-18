import type { APIRoute } from 'astro';
import script from '../../../install.ps1?raw';

export const GET: APIRoute = () => new Response(script);
