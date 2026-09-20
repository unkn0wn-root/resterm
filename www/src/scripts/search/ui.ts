import type { SearchRecord } from '../../lib/docs/search';
import { prepare, search, snippet, tokenize, type Entry, type Hit } from './engine';

const suggestions = ['@capture', 'oauth', 'mock', 'workflow', 'ssh', '--env-file'];

let entries: Promise<Entry[]> | undefined;

function load(): Promise<Entry[]> {
  entries ??= fetch('/search.json')
    .then((r) => {
      if (!r.ok) throw new Error(`search index: HTTP ${r.status}`);
      return r.json() as Promise<SearchRecord[]>;
    })
    .then(prepare)
    .catch((err) => {
      entries = undefined;
      throw err;
    });
  return entries;
}

function el<K extends keyof HTMLElementTagNameMap>(tag: K, className?: string, text?: string) {
  const node = document.createElement(tag);
  if (className) node.className = className;
  if (text) node.textContent = text;
  return node;
}

export function mountSearch() {
  const dialog = document.getElementById('search') as HTMLDialogElement | null;
  const input = document.getElementById('search-input') as HTMLInputElement | null;
  const list = document.getElementById('search-results');
  const detail = document.getElementById('search-detail');
  if (!dialog || !input || !list || !detail) return () => {};

  let hits: Hit[] = [];
  let selected = 0;
  let tokens: string[] = [];

  const showDetail = () => {
    detail.replaceChildren();
    const hit = hits[selected];
    if (!hit) return;
    const { entry } = hit;
    detail.append(el('p', 'd-path', `${entry.g} › ${entry.p}`), el('p', 'd-head', entry.h));
    const text = el('p', 'd-text');
    for (const span of snippet(entry.t, tokens)) {
      text.append(span.mark ? el('mark', undefined, span.text) : document.createTextNode(span.text));
    }
    detail.append(text, el('p', 'd-open', `Enter  ${entry.u}`));
  };

  const select = (i: number) => {
    if (!hits.length) return;
    selected = (i + hits.length) % hits.length;
    for (const [n, item] of [...list.children].entries()) {
      item.setAttribute('aria-selected', String(n === selected));
    }
    list.children[selected]?.scrollIntoView({ block: 'nearest' });
    showDetail();
  };

  const empty = (message: string, withSuggestions = false) => {
    list.replaceChildren(el('li', 'r-empty', message));
    detail.replaceChildren();
    if (withSuggestions) {
      detail.append(el('p', 'd-path', 'Try'));
      for (const s of suggestions) {
        const button = el('button', 'chip', s);
        button.type = 'button';
        button.addEventListener('click', () => {
          input.value = s;
          void run();
          input.focus();
        });
        detail.append(button);
      }
    }
  };

  const run = async () => {
    const query = input.value;
    tokens = tokenize(query);
    if (!tokens.length) {
      hits = [];
      empty('Type to search every docs section.', true);
      return;
    }
    let all: Entry[];
    try {
      all = await load();
    } catch {
      empty('The search index could not be loaded.');
      return;
    }
    if (input.value !== query) return;
    hits = search(all, query);
    if (!hits.length) {
      empty(`No section matches "${query}".`);
      return;
    }
    list.replaceChildren(
      ...hits.map((hit, i) => {
        const item = el('li');
        item.role = 'option';
        item.id = `search-hit-${i}`;
        item.append(el('span', 'r-head', hit.entry.h), el('span', 'r-page', `${hit.entry.g} › ${hit.entry.p}`));
        item.addEventListener('mousemove', () => selected !== i && select(i));
        item.addEventListener('click', () => go(hit));
        return item;
      }),
    );
    select(0);
  };

  const go = (hit: Hit | undefined) => {
    if (!hit) return;
    dialog.close();
    location.href = hit.entry.u;
  };

  input.addEventListener('input', () => void run());
  input.addEventListener('keydown', (e) => {
    const down = e.key === 'ArrowDown' || (e.ctrlKey && (e.key === 'n' || e.key === 'j'));
    const up = e.key === 'ArrowUp' || (e.ctrlKey && (e.key === 'p' || e.key === 'k'));
    if (down || up) {
      e.preventDefault();
      select(selected + (down ? 1 : -1));
    } else if (e.key === 'Enter') {
      e.preventDefault();
      go(hits[selected]);
    }
  });
  dialog.addEventListener('click', (e) => {
    if (e.target === dialog) dialog.close();
  });

  return () => {
    if (dialog.open) return;
    dialog.showModal();
    input.select();
    void run();
    void load().catch(() => {});
  };
}
