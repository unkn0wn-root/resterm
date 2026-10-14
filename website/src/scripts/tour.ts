import darkUrl from '../content/tour/frames-dark-editor-hidden.json?url';
import lightUrl from '../content/tour/frames-light-editor-hidden.json?url';
import { renderScreen, type Frames } from '../lib/screen';
import { say } from './status';

const hideEditor = 'g 2';

async function addHiddenEditor(steps: HTMLElement[]) {
  const sets = await Promise.all(
    Object.entries({ dark: darkUrl, light: lightUrl }).map(async ([theme, url]) => {
      const res = await fetch(url);
      if (!res.ok) throw new Error(`${url}: ${res.status}`);
      return [theme, (await res.json()) as Frames] as const;
    }),
  );
  for (const [theme, set] of sets) {
    steps.forEach((step, i) => {
      const base = step.querySelector(`.screen.base.${theme}`) as HTMLElement;
      // Cloning preserves Astro's scoped style attributes.
      const screen = base.cloneNode(false) as HTMLElement;
      screen.classList.replace('base', 'alt');
      const grid = base.firstElementChild!.cloneNode(false) as HTMLElement;
      grid.innerHTML = renderScreen(set.styles, set.frames[i]!.lines);
      screen.append(grid);
      base.after(screen);
    });
  }
}

function mount(root: HTMLElement) {
  const tabs = [...root.querySelectorAll<HTMLButtonElement>('[role="tab"]')];
  const steps = [...root.querySelectorAll<HTMLElement>('[role="tabpanel"]')];
  const toggle = root.querySelector('[data-tour-editor]') as HTMLButtonElement;
  const label = toggle.lastElementChild!;

  let hidden = false;
  let loaded: Promise<void> | undefined;

  const apply = () => {
    root.classList.toggle('editor-hidden', hidden);
    label.textContent = hidden ? 'Show editor' : 'Hide editor';
  };

  const setHidden = (on: boolean) => {
    hidden = on;
    try {
      localStorage.setItem('tour-editor', on ? 'hidden' : 'shown');
    } catch {
      // Keep the toggle working when storage is unavailable.
    }
    if (!on) return apply();
    loaded ??= addHiddenEditor(steps);
    loaded.then(apply, () => {
      loaded = undefined;
      hidden = false;
      apply();
      say('Could not load the screens without the editor');
    });
  };

  const go = (to: number) => {
    const i = (to + steps.length) % steps.length;
    tabs[i]!.click();
    steps[i]!.focus();
  };

  toggle.addEventListener('click', () => setHidden(!hidden));

  steps.forEach((step, at) => {
    const next = step.querySelector('[data-tour-next]') as HTMLButtonElement;
    const want = step.dataset.press!.replace(/Shift\+/g, '');
    let pending = '';
    let nudge: number | undefined;

    next.addEventListener('click', () => go(at + 1));

    step.addEventListener('keydown', (e) => {
      if (e.target !== step) return;
      if (e.key === 'Escape') return step.blur();
      // Resterm also accepts Ctrl/Cmd/Alt+Enter.
      if ((e.ctrlKey || e.metaKey || e.altKey) && e.key !== 'Enter') return;
      if (e.key.length > 1 && !['Enter', 'ArrowLeft', 'ArrowRight'].includes(e.key)) return;
      e.preventDefault();
      e.stopPropagation();

      if (e.key === 'ArrowRight') return go(at + 1);
      if (e.key === 'ArrowLeft') return go(at - 1);

      const tries = pending ? [`${pending} ${e.key}`, e.key] : [e.key];
      pending = '';
      for (const seq of tries) {
        if (seq === want) return go(at + 1);
        if (seq === hideEditor) return setHidden(!hidden);
        if ([want, hideEditor].some((s) => s.startsWith(`${seq} `))) {
          pending = seq;
          return;
        }
      }

      next.classList.add('nudge');
      clearTimeout(nudge);
      nudge = window.setTimeout(() => next.classList.remove('nudge'), 600);
    });
  });

  try {
    if (localStorage.getItem('tour-editor') === 'hidden') setHidden(true);
  } catch {
    // Fall back to the default layout.
  }
}

for (const root of document.querySelectorAll<HTMLElement>('[data-tour]')) mount(root);
