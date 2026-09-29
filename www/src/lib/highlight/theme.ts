import type { ThemeRegistration } from 'shiki';

// One theme for both color schemes. Every color is a CSS variable, so the
// light and dark palettes live in src/styles/tokens.css and code blocks follow
// the page theme without rendering twice.

const token = (scope: string | string[], name: string, fontStyle?: string) => ({
  scope,
  settings: { foreground: `var(--syn-${name})`, ...(fontStyle && { fontStyle }) },
});

export const restermTheme: ThemeRegistration = {
  name: 'resterm',
  type: 'dark',
  colors: {
    'editor.foreground': 'var(--syn-fg)',
    'editor.background': 'var(--code-bg)',
  },
  tokenColors: [
    token(['comment', 'punctuation.definition.comment'], 'comment'),
    token('comment.marker.resterm', 'comment'),
    token('meta.separator.resterm', 'separator'),
    token('keyword.directive.resterm', 'directive', 'bold'),
    token('keyword.request-line.resterm', 'request', 'bold'),
    token('string.directive-args.resterm', 'value'),
    token('variable.parameter.option.resterm', 'key'),
    token('string.option-value.resterm', 'option'),

    token('keyword.declaration.rts', 'directive', 'bold'),
    token('keyword.control.rts', 'control', 'bold'),
    token('constant.language.rts', 'literal', 'bold'),
    token('keyword.operator.logical.rts', 'logical', 'bold'),
    token('entity.name.function.rts', 'function', 'bold'),
    token('entity.name.method.rts', 'option'),
    token('string.quoted.rts', 'value'),
    token('constant.numeric.rts', 'option'),

    // Shell, JSON, TOML and the rest use the monokai-like colors the
    // response pane uses for bodies.
    token(['string', 'string.quoted'], 'string'),
    token(['constant.numeric', 'constant.language', 'constant.character'], 'number'),
    token(['support.type.property-name', 'entity.name.tag', 'variable.other.key'], 'prop'),
    token(['keyword', 'storage', 'keyword.operator'], 'keyword'),
    token(['entity.name.function', 'support.function', 'meta.function-call'], 'function'),
    token(['variable', 'variable.other', 'variable.parameter'], 'variable'),
    token(['punctuation.separator.key-value', 'punctuation.definition.variable'], 'punct'),
  ],
};
