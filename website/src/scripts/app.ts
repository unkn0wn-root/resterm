import type { Action } from './bindings';
import { handleCopy } from './copy';
import { listenKeys } from './keys';
import { mountSearch } from './search/ui';
import { trackScroll } from './status';
import { mountTabs } from './tabs';
import { toggleTheme } from './theme';

const openSearch = mountSearch();
const keys = document.getElementById('keys') as HTMLDialogElement | null;

const follow = (rel: 'prev' | 'next') => {
  const link = document.querySelector<HTMLLinkElement>(`link[rel="${rel}"]`);
  if (link) location.href = link.href;
};

const actions: Record<Action, () => void> = {
  search: openSearch,
  help: () => keys?.showModal(),
  theme: toggleTheme,
  home: () => (location.href = '/'),
  docs: () => (location.href = '/docs/'),
  top: () => scrollTo({ top: 0 }),
  bottom: () => scrollTo({ top: document.documentElement.scrollHeight }),
  down: () => scrollBy({ top: 64 }),
  up: () => scrollBy({ top: -64 }),
  prev: () => follow('prev'),
  next: () => follow('next'),
};

listenKeys((action) => actions[action]());
handleCopy();
trackScroll();
mountTabs();

document.addEventListener('click', (e) => {
  const target = e.target as Element;
  if (target.closest('[data-search-open]')) actions.search();
  else if (target.closest('[data-keys-open]')) actions.help();
  else if (target.closest('[data-theme-toggle]')) actions.theme();
});
