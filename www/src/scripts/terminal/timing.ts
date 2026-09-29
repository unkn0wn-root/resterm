export type Tone = 'cmd' | 'dim' | 'sum';

export const pause = {
  beforeCommand: 600,
  afterEnter: 420,
  line: 150,
  summary: 280,
};

export function delayBefore(tone?: Tone, previous?: Tone): number {
  if (tone === 'cmd') return pause.beforeCommand;
  if (previous === 'cmd') return pause.afterEnter;
  if (tone === 'dim') return 0;
  if (tone === 'sum') return pause.summary;
  return pause.line;
}

// Keys land at an uneven pace, like a person typing.
export function keyDelay(random = Math.random): number {
  return 28 + random() * 42;
}
