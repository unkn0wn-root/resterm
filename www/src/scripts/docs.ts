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

// The outline marks the last heading that has scrolled past the top third.
const links = new Map<string, HTMLAnchorElement>();
for (const a of document.querySelectorAll<HTMLAnchorElement>('[data-outline]')) links.set(a.dataset.outline!, a);

const headings = [...links.keys()].map((id) => document.getElementById(id)).filter((h) => h !== null);

if (headings.length) {
  let active: HTMLAnchorElement | undefined;
  let queued = false;
  const update = () => {
    queued = false;
    const line = innerHeight / 3;
    let current: HTMLElement | undefined;
    for (const h of headings) {
      if (h.getBoundingClientRect().top > line) break;
      current = h;
    }
    const next = current && links.get(current.id);
    if (next === active) return;
    active?.removeAttribute('aria-current');
    next?.setAttribute('aria-current', 'true');
    active = next;
  };
  addEventListener(
    'scroll',
    () => {
      if (queued) return;
      queued = true;
      requestAnimationFrame(update);
    },
    { passive: true },
  );
  update();
}
