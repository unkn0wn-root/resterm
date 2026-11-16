const root = document.documentElement;
const toggle = document.querySelector<HTMLButtonElement>('[data-nav-toggle]');

const setOpen = (open: boolean) => {
  root.toggleAttribute('data-nav-open', open);
  toggle?.setAttribute('aria-expanded', String(open));
};

toggle?.addEventListener('click', () => setOpen(!root.hasAttribute('data-nav-open')));
document.addEventListener('keydown', (e) => {
  if (e.key === 'Escape') setOpen(false);
});
document.addEventListener('click', (e) => {
  const target = e.target as Element;
  if (root.hasAttribute('data-nav-open') && !target.closest('#docs-nav, [data-nav-toggle]')) setOpen(false);
});

// The outline marks the hash target while it is on screen. Otherwise it marks
// the last target above the line a third of the way down the window. The hash
// target wins because a short request would pass the mark straight on to the
// next one.
const links = new Map<string, HTMLAnchorElement>();
for (const a of document.querySelectorAll<HTMLAnchorElement>('[data-outline]')) links.set(a.dataset.outline!, a);

// Targets are in document order. The Requests pane lists workflows last, but a
// workflow can sit above its requests in the file.
const targets = [...document.querySelectorAll<HTMLElement>('[id]')].filter((t) => links.has(t.id));

if (targets.length) {
  let active: HTMLAnchorElement | undefined;
  let queued = false;

  const update = () => {
    queued = false;
    const target = document.querySelector<HTMLElement>(':target');
    let current: HTMLElement | undefined;

    if (target && links.has(target.id) && !target.closest('[hidden]')) {
      const { top } = target.getBoundingClientRect();
      if (top >= 0 && top < innerHeight) {
        current = target;
      }
    }

    if (!current) {
      const line = innerHeight / 3;
      for (const t of targets) {
        // A target in a hidden tab has no position.
        if (t.closest('[hidden]')) {
          continue;
        }
        if (t.getBoundingClientRect().top > line) {
          break;
        }
        current = t;
      }
    }

    const next = current && links.get(current.id);
    if (next === active) {
      return;
    }

    active?.removeAttribute('aria-current');
    next?.setAttribute('aria-current', 'true');
    active = next;
  };

  const queue = () => {
    if (queued) {
      return;
    }

    queued = true;
    requestAnimationFrame(update);
  };

  addEventListener('scroll', queue, { passive: true });
  // A click or an arrow key can switch tabs and show other targets. Back and
  // forward can change the hash target without scrolling.
  addEventListener('click', queue);
  addEventListener('keydown', queue);
  addEventListener('hashchange', queue);
  update();
}
