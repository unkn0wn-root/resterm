import { delayBefore, keyDelay, type Tone } from './timing';

interface Line {
  el: HTMLElement;
  tone: Tone | undefined;
  cmd: HTMLElement | null;
}

const wait = (ms: number) => new Promise<void>((resolve) => setTimeout(resolve, ms));

async function play(term: HTMLElement, lines: Line[], caret: HTMLElement) {
  let previous: Tone | undefined;
  for (const { el, tone, cmd } of lines) {
    const delay = delayBefore(tone, previous);
    previous = tone;

    if (!cmd) {
      await wait(delay);
      caret.remove();
      el.classList.remove('pending');
      continue;
    }

    const text = cmd.textContent ?? '';
    cmd.textContent = '';
    cmd.after(caret);
    el.classList.remove('pending');
    await wait(delay);
    term.classList.add('typing');

    for (const ch of text) {
      cmd.textContent += ch;
      await wait(keyDelay());
    }

    term.classList.remove('typing');
  }
}

function mount(term: HTMLElement) {
  const lines: Line[] = [...term.querySelectorAll<HTMLElement>('.ln')].map((el) => ({
    el,
    tone: el.dataset.tone as Tone | undefined,
    cmd: el.querySelector<HTMLElement>('.cmd'),
  }));

  const caret = document.createElement('span');
  caret.className = 'caret';
  caret.setAttribute('aria-hidden', 'true');
  lines.at(-1)?.cmd?.after(caret);

  if (matchMedia('(prefers-reduced-motion: reduce)').matches) return;

  for (const line of lines) line.el.classList.add('pending');
  new IntersectionObserver(
    (entries, observer) => {
      if (!entries.some((e) => e.isIntersecting)) return;
      observer.disconnect();
      void play(term, lines, caret);
    },
    { rootMargin: '0px 0px -15% 0px' },
  ).observe(term);
}

for (const term of document.querySelectorAll<HTMLElement>('[data-terminal]')) mount(term);
