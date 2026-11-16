// Tabs follow the ARIA tabs pattern: arrow keys along the tablist orientation
// move between tabs and only the selected tab is in the tab order.
function mount(root: Element) {
  const tabs = [...root.querySelectorAll<HTMLButtonElement>('[role="tab"]')];
  const vertical = root.querySelector('[role="tablist"]')?.getAttribute('aria-orientation') === 'vertical';
  const [next, prev] = vertical ? ['ArrowDown', 'ArrowUp'] : ['ArrowRight', 'ArrowLeft'];
  const select = (index: number, focus: boolean) => {
    tabs.forEach((tab, i) => {
      const on = i === index;
      tab.setAttribute('aria-selected', String(on));
      tab.tabIndex = on ? 0 : -1;
      const panel = document.getElementById(tab.getAttribute('aria-controls') ?? '');
      if (panel) panel.hidden = !on;
      if (on && focus) tab.focus();
    });
  };
  const keys: Record<string, (i: number) => number> = {
    [next]: (i) => (i + 1) % tabs.length,
    [prev]: (i) => (i - 1 + tabs.length) % tabs.length,
    Home: () => 0,
    End: () => tabs.length - 1,
  };
  tabs.forEach((tab, i) => {
    tab.addEventListener('click', () => select(i, false));
    tab.addEventListener('keydown', (e) => {
      const move = keys[e.key];
      if (!move) return;
      e.preventDefault();
      select(move(i), true);
    });
  });
}

// reveal selects the tab whose hidden panel contains the element with this id,
// so it can be scrolled to. It returns that element, or undefined when there
// was nothing to reveal.
function reveal(id: string): HTMLElement | undefined {
  const target = document.getElementById(id);
  const panel = target?.closest('[role="tabpanel"][hidden]');
  const tab = document.getElementById(panel?.getAttribute('aria-labelledby') ?? '');
  if (!target || !tab) {
    return undefined;
  }

  tab.click();
  return target;
}

export function mountTabs() {
  for (const root of document.querySelectorAll('[data-tabs]')) {
    mount(root);
  }

  // This runs before the browser follows the link, so it scrolls to a visible
  // target. A click with a modifier key opens a new browser tab and leaves this
  // page alone.
  document.addEventListener('click', (e) => {
    const link = (e.target as Element).closest('a[href^="#"]');
    const modified = e.metaKey || e.ctrlKey || e.shiftKey || e.altKey;
    if (link && !modified) {
      reveal(link.getAttribute('href')!.slice(1));
    }
  });

  // A hash in a hidden panel, on load or from back and forward, opens its tab
  // and scrolls there.
  const follow = () => reveal(location.hash.slice(1))?.scrollIntoView();
  follow();
  addEventListener('hashchange', follow);
}
