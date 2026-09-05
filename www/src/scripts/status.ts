const message = document.getElementById('status-msg');
const scroll = document.getElementById('status-scroll');
let timer: number | undefined;

export function say(text: string) {
  if (!message) return;
  message.textContent = text;
  clearTimeout(timer);
  timer = window.setTimeout(() => {
    message.textContent = message.dataset.idle ?? '';
  }, 4000);
}

function position(): string {
  const max = document.documentElement.scrollHeight - window.innerHeight;
  if (max <= 0) return 'All';
  const y = window.scrollY;
  if (y <= 0) return 'Top';
  if (y >= max - 1) return 'Bot';
  return `${Math.round((y / max) * 100)}%`;
}

export function trackScroll() {
  if (!scroll) return;
  let queued = false;
  const update = () => {
    queued = false;
    scroll.textContent = position();
  };
  const queue = () => {
    if (queued) return;
    queued = true;
    requestAnimationFrame(update);
  };
  addEventListener('scroll', queue, { passive: true });
  addEventListener('resize', queue, { passive: true });
  update();
}
