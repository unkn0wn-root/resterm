import { bindings, type Action } from './bindings';

const sequences = new Map<string, Action>();
const prefixes = new Set<string>();
for (const b of bindings) {
  for (const [seq, action] of Object.entries(b.keys)) {
    sequences.set(seq, action);
    const keys = seq.split(' ');
    for (let i = 1; i < keys.length; i++) prefixes.add(keys.slice(0, i).join(' '));
  }
}

function typing(target: EventTarget | null): boolean {
  if (!(target instanceof HTMLElement)) return false;
  return target.isContentEditable || /^(INPUT|TEXTAREA|SELECT)$/.test(target.tagName);
}

// listenKeys runs vim-style key sequences. A pending prefix such as "g" waits
// one second for the next key.
export function listenKeys(run: (action: Action) => void) {
  let pending = '';
  let timer: number | undefined;

  document.addEventListener('keydown', (e) => {
    if ((e.ctrlKey || e.metaKey) && e.key.toLowerCase() === 'k') {
      e.preventDefault();
      run('search');
      return;
    }
    if (e.ctrlKey || e.metaKey || e.altKey || e.isComposing || typing(e.target)) return;
    if (document.querySelector('dialog[open]')) return;

    const seq = pending ? `${pending} ${e.key}` : e.key;
    clearTimeout(timer);
    pending = '';

    const action = sequences.get(seq);
    if (action) {
      e.preventDefault();
      run(action);
    } else if (prefixes.has(seq)) {
      e.preventDefault();
      pending = seq;
      timer = window.setTimeout(() => (pending = ''), 1000);
    }
  });
}
