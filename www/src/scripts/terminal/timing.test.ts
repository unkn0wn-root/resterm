import { describe, expect, it } from 'vitest';
import { delayBefore, keyDelay, pause } from './timing';

describe('delayBefore', () => {
  it('pauses before every command', () => {
    expect(delayBefore('cmd', undefined)).toBe(pause.beforeCommand);
    expect(delayBefore('cmd', 'sum')).toBe(pause.beforeCommand);
  });

  it('waits for the command to run before its first line', () => {
    expect(delayBefore('dim', 'cmd')).toBe(pause.afterEnter);
    expect(delayBefore(undefined, 'cmd')).toBe(pause.afterEnter);
  });

  it('shows detail lines with the line above them', () => {
    expect(delayBefore('dim', undefined)).toBe(0);
  });

  it('spaces results and holds the summary a little longer', () => {
    expect(delayBefore(undefined, undefined)).toBe(pause.line);
    expect(delayBefore('sum', 'dim')).toBe(pause.summary);
  });
});

describe('keyDelay', () => {
  it('stays in its range', () => {
    expect(keyDelay(() => 0)).toBe(28);
    expect(keyDelay(() => 1)).toBe(70);
  });
});
