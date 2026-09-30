export const site = {
  name: 'Resterm',
  tagline: 'An API-as-code workbench for the terminal.',
  description:
    'Resterm is a terminal API client. Requests live in plain .http files in your repo. Run them in the TUI or in CI.',
  repo: 'https://github.com/unkn0wn-root/resterm',
  sponsor: 'https://github.com/sponsors/unkn0wn-root',
  license: 'Apache-2.0',
};

export const repoFile = (path: string, line?: number) =>
  `${site.repo}/blob/main/${path}${line ? `?plain=1#L${line}` : ''}`;

export const docHref = (slug: string, hash?: string) => `/docs/${slug}/${hash ? `#${hash}` : ''}`;
