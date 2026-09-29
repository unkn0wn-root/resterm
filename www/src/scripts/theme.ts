import { say } from './status';

type Theme = 'dark' | 'light';

const names: Record<Theme, string> = { dark: 'Default', light: 'Daybreak' };

export function toggleTheme() {
  const next: Theme = document.documentElement.dataset.theme === 'light' ? 'dark' : 'light';
  document.documentElement.dataset.theme = next;
  try {
    localStorage.setItem('theme', next);
  } catch {
    // Storage can be blocked. The theme still applies to this page.
  }
  say(`Theme set to ${names[next]}`);
}
