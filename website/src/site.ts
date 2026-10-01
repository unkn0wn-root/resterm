export const site = {
  name: 'Resterm',
  tagline: 'An API-as-code workbench for the terminal.',
  description:
    'Resterm is a terminal API client. Requests live in plain .http files in your repo. Run them in the TUI or in CI.',
  repo: 'https://github.com/unkn0wn-root/resterm',
  sponsor: 'https://github.com/sponsors/unkn0wn-root',
  license: 'Apache-2.0',
};

const ref = process.env.SITE_REF || 'main';

export const repoFile = (path: string) => `${site.repo}/blob/${ref}/${path}`;

export const docHref = (slug: string, hash?: string) => {
  const path = slug ? `/docs/${slug}/` : '/docs/';
  return hash ? `${path}#${hash}` : path;
};
