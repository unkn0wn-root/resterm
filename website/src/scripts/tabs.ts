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

export function mountTabs() {
  for (const root of document.querySelectorAll('[data-tabs]')) mount(root);
}
