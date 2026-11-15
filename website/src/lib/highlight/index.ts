import { createHighlighter, type Highlighter } from 'shiki';
import type { Element } from 'hast';
import { restermHttp, restermScript } from './grammars';
import { restermTheme } from './theme';

const bundled = ['bash', 'json', 'toml', 'powershell', 'go', 'yaml'] as const;

const aliases: Record<string, string> = {
  sh: 'bash',
  shell: 'bash',
  zsh: 'bash',
  console: 'bash',
  ps1: 'powershell',
  yml: 'yaml',
};

const labels: Record<string, string> = {
  'resterm-http': 'http',
  shellscript: 'shell',
};

let highlighter: Promise<Highlighter> | undefined;

function load(): Promise<Highlighter> {
  highlighter ??= createHighlighter({
    themes: [restermTheme],
    langs: [...bundled, restermHttp, restermScript],
  });
  return highlighter;
}

// resolve maps a fence language or file extension to a loaded grammar,
// or to plain text.
function resolve(h: Highlighter, lang: string): string {
  const requested = aliases[lang] ?? lang;
  return h.getLoadedLanguages().includes(requested) ? h.getLanguage(requested).name : 'text';
}

export interface Highlighted {
  label: string;
  pre: Element;
}

export interface LineAnchor {
  line: number;
  id: string;
}

export interface HighlightOptions {
  cursor?: number;
  anchors?: readonly LineAnchor[];
}

export async function highlight(code: string, lang = ''): Promise<Highlighted> {
  const h = await load();
  const resolved = resolve(h, lang);
  const root = h.codeToHast(code, { lang: resolved, theme: restermTheme.name! });
  const pre = root.children[0] as Element;
  return { label: labels[resolved] ?? resolved, pre };
}

// highlightHtml renders a standalone block. cursor marks one line (1-based) the
// way the TUI marks the line under the cursor. Each anchor sets an id on its
// line, so links can jump to it.
export async function highlightHtml(
  code: string,
  lang: string,
  { cursor, anchors = [] }: HighlightOptions = {},
): Promise<string> {
  const h = await load();
  const ids = new Map(anchors.map((a) => [a.line, a.id]));

  return h.codeToHtml(code, {
    lang: resolve(h, lang),
    theme: restermTheme.name!,
    transformers: [
      {
        line(node, line) {
          if (line === cursor) {
            this.addClassToHast(node, 'cursor');
          }

          const id = ids.get(line);
          if (id) {
            node.properties.id = id;
          }
        },
      },
    ],
  });
}
