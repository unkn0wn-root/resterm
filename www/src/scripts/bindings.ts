// Shared by the key handler and the help dialog, so the two cannot drift.

export type Action = 'search' | 'help' | 'theme' | 'home' | 'docs' | 'top' | 'bottom' | 'down' | 'up' | 'prev' | 'next';

export interface Binding {
  label: string;
  help: string;
  // Key sequences, with a space between keys: "g d".
  keys: Record<string, Action>;
}

export const bindings: Binding[] = [
  { label: '/', help: 'Search the docs (also Ctrl K)', keys: { '/': 'search' } },
  { label: 'j k', help: 'Scroll down and up', keys: { j: 'down', k: 'up' } },
  { label: 'g g  G', help: 'Jump to the top or the bottom', keys: { 'g g': 'top', G: 'bottom' } },
  { label: '[  ]', help: 'Previous and next docs page', keys: { '[': 'prev', ']': 'next' } },
  { label: 'g d', help: 'Docs index', keys: { 'g d': 'docs' } },
  { label: 'g h', help: 'Home page', keys: { 'g h': 'home' } },
  { label: 't', help: 'Switch between dark and light', keys: { t: 'theme' } },
  { label: '?', help: 'Show this help', keys: { '?': 'help' } },
  { label: 'Esc', help: 'Close a dialog', keys: {} },
];
