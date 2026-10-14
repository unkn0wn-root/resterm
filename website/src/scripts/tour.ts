import darkUrl from '../content/tour/frames-dark-editor-hidden.json?url';
import lightUrl from '../content/tour/frames-light-editor-hidden.json?url';
import darkPanesHiddenUrl from '../content/tour/panes-dark-editor-hidden.json?url';
import darkPanesUrl from '../content/tour/panes-dark.json?url';
import lightPanesHiddenUrl from '../content/tour/panes-light-editor-hidden.json?url';
import lightPanesUrl from '../content/tour/panes-light.json?url';
import { renderScreen, type Frames, type PaneData } from '../lib/screen';
import { say } from './status';

const hideEditor = 'g 2';

const scrollKeys: Record<string, number> = { j: 1, ArrowDown: 1, PageDown: 1, k: -1, ArrowUp: -1, PageUp: -1 };

// Leave touch swipes to page scrolling.
const fine = matchMedia('(hover: hover) and (pointer: fine)').matches;

const paneUrls = {
  base: { dark: darkPanesUrl, light: lightPanesUrl },
  alt: { dark: darkPanesHiddenUrl, light: lightPanesHiddenUrl },
};

const scrolls = new WeakMap<HTMLElement, (dir: number, page: boolean) => boolean>();

async function load<T>(urls: { dark: string; light: string }) {
  return Promise.all(
    Object.entries(urls).map(async ([theme, url]) => {
      const res = await fetch(url);
      if (!res.ok) throw new Error(`${url}: ${res.status}`);
      return [theme, (await res.json()) as T] as const;
    }),
  );
}

async function addHiddenEditor(steps: HTMLElement[]) {
  for (const [theme, set] of await load<Frames>({ dark: darkUrl, light: lightUrl })) {
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

async function addPanes(steps: HTMLElement[], kind: keyof typeof paneUrls) {
  for (const [theme, data] of await load<PaneData>(paneUrls[kind])) {
    steps.forEach((step, at) => {
      const screen = step.querySelector(`.screen.${kind}.${theme}`) as HTMLElement;
      const grid = screen.firstElementChild!;
      const home = step.dataset.focus!;
      const base = grid.innerHTML;
      const all = data.frames[at]!.panes;
      let focus = home;

      const boxes = all.flatMap(({ id, body }) => {
        if (!body) return [];
        const box = grid.cloneNode(false) as HTMLElement;
        box.classList.add('scroller');
        box.style.cssText = `--x:${body.x};--y:${body.y};--w:${body.w};--h:${body.h}`;
        box.innerHTML = renderScreen(data.styles, data.strips[body.strip]!);
        // Keep keyboard events on the step when the browser focuses a scroll box.
        box.tabIndex = -1;
        box.addEventListener('focus', () => step.focus({ preventScroll: true }));
        screen.append(box);
        return [{ id, body, box }];
      });

      const show = (id: string) => {
        if (id !== focus) {
          const next = all.find((p) => p.id === id)?.focus;
          grid.innerHTML = next ? renderScreen(data.styles, next) : base;
          focus = id;
        }
        for (const b of boxes) {
          const idle = b.id !== id && !b.body.keep;
          b.box.classList.toggle('idle', idle);
          if (idle) b.box.scrollTop = 0;
        }
      };

      for (const b of boxes) {
        b.box.addEventListener(
          'scroll',
          () => {
            if (focus !== b.id && b.box.scrollTop) show(b.id);
          },
          { passive: true },
        );
      }

      const cols = Number(screen.style.getPropertyValue('--cols'));
      screen.addEventListener('click', (e) => {
        const r = grid.getBoundingClientRect();
        const x = ((e.clientX - r.left) / r.width) * cols;
        const y = ((e.clientY - r.top) / r.height) * grid.childElementCount;
        const p = all.find(({ box: [bx, by, bw, bh] }) => x >= bx && x < bx + bw && y >= by && y < by + bh);
        if (p && p.id !== focus) show(p.id);
      });

      // Hidden elements ignore scrollTop changes. Reset when the step becomes visible.
      new MutationObserver(() => {
        if (step.hidden) return;
        for (const b of boxes) b.box.scrollTop = 0;
        show(home);
      }).observe(step, { attributeFilter: ['hidden'] });

      show(home);
      scrolls.set(screen, (dir, page) => {
        const b = boxes.find((x) => x.id === focus);
        if (!b) return false;
        const t = b.box.scrollTop;
        const row = b.box.getBoundingClientRect().height / b.body.h;
        b.box.scrollTop = (Math.round(t / row) + dir * (page ? b.body.h - 1 : 1)) * row;
        return b.box.scrollTop !== t;
      });
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
      // Saving the preference is optional; the toggle should still work.
    }
    if (!on) return apply();
    loaded ??= addHiddenEditor(steps).then(() => {
      if (fine) void addPanes(steps, 'alt');
    });
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

  if (fine) {
    new IntersectionObserver(
      (entries, observer) => {
        if (!entries.some((e) => e.isIntersecting)) return;
        observer.disconnect();
        void addPanes(steps, 'base');
      },
      { rootMargin: '50% 0px' },
    ).observe(root);
  }

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

      const dir = scrollKeys[e.key];
      if (dir) {
        const screen = [...step.querySelectorAll<HTMLElement>('.screen')].find((s) => s.checkVisibility());
        if (screen && scrolls.get(screen)?.(dir, e.key.startsWith('Page'))) {
          e.preventDefault();
          e.stopPropagation();
          pending = '';
          return;
        }
      }

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
    // Keep the editor visible if the saved preference can't be read.
  }
}

for (const root of document.querySelectorAll<HTMLElement>('[data-tour]')) mount(root);
