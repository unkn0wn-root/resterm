import { say } from './status';

export function handleCopy() {
  document.addEventListener('click', async (event) => {
    const button = (event.target as Element).closest<HTMLElement>('[data-copy]');
    if (!button) return;
    const text = button.dataset.copy || (button.closest('figure')?.querySelector('pre')?.textContent ?? '');
    try {
      await navigator.clipboard.writeText(text);
    } catch {
      say('Copy failed. The browser blocked clipboard access.');
      return;
    }
    const lines = text.split('\n').length;
    say(lines > 1 ? `Copied ${lines} lines` : 'Copied to clipboard');
    const label = (button.dataset.label ??= button.textContent ?? '');
    button.textContent = 'copied';
    setTimeout(() => (button.textContent = label), 1500);
  });
}
