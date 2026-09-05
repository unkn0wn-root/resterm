import type { LanguageRegistration } from 'shiki';

// These grammars follow the resterm editor highlighter
// (internal/ui/editor_metadata_highlighter.go and editor_rts_highlighter.go),
// so code on the site looks the way it does in the TUI.

const marker = String.raw`^\s*(#(?!#)|//|--)\s*`;
const nameEnd = String.raw`(?![\w.-])`;

// Directive argument kinds come from internal/directive/spec.go.
const optionDirectives = [
  'mock',
  'match',
  'expect',
  'ssh',
  'k8s',
  'compare',
  'profile',
  'trace',
  'retry',
  'retry-backoff',
  'settings',
  'sse',
  'websocket',
  'else',
  'default',
];

const tokenDirectives = [
  'auth',
  'graphql',
  'operation',
  'graphql-operation',
  'grpc-plaintext',
  'grpc-reflection',
  'log-sensitive-headers',
  'name',
  'rts',
  'script',
  'timeout',
];

const methods = ['GET', 'POST', 'PUT', 'PATCH', 'DELETE', 'HEAD', 'OPTIONS', 'TRACE', 'CONNECT', 'WS', 'WSS', 'GRPC'];

const alt = (words: string[]) => words.map((w) => w.replace(/[.-]/g, '\\$&')).join('|');

const optionPairs = {
  patterns: [
    {
      match: String.raw`(?<![^\s])([A-Za-z_][\w.-]*)(=)("(?:[^"\\]|\\.)*"|'[^']*'|\S*)`,
      captures: {
        1: { name: 'variable.parameter.option.resterm' },
        3: { name: 'string.option-value.resterm' },
      },
    },
  ],
};

export const restermHttp: LanguageRegistration = {
  name: 'resterm-http',
  scopeName: 'source.resterm-http',
  aliases: ['http', 'rest'],
  patterns: [
    { include: '#separator' },
    { include: '#directive' },
    { include: '#comment' },
    { include: '#request-line' },
  ],
  repository: {
    separator: {
      match: String.raw`^\s*###.*$`,
      name: 'meta.separator.resterm',
    },
    directive: {
      patterns: [
        {
          // "@setting key=value" is the @settings form.
          match: `${marker}(@(?:${alt(optionDirectives)}|setting(?=\\s+\\S*=)))${nameEnd}(.*)$`,
          captures: {
            1: { name: 'comment.marker.resterm' },
            2: { name: 'keyword.directive.resterm' },
            3: { name: 'string.directive-args.resterm', ...optionPairs },
          },
        },
        {
          match: `${marker}(@setting)${nameEnd}\\s+(\\S+?)(:*)(?:(\\s+)(.*))?$`,
          captures: {
            1: { name: 'comment.marker.resterm' },
            2: { name: 'keyword.directive.resterm' },
            3: { name: 'variable.parameter.option.resterm' },
            6: { name: 'string.option-value.resterm' },
          },
        },
        {
          match: `${marker}(@(?:${alt(tokenDirectives)}))${nameEnd}([\\s:]*)(\\S*)`,
          captures: {
            1: { name: 'comment.marker.resterm' },
            2: { name: 'keyword.directive.resterm' },
            4: { name: 'string.directive-args.resterm' },
          },
        },
        {
          match: `${marker}(@[\\w.-]+)([\\s:]*)(.*)$`,
          captures: {
            1: { name: 'comment.marker.resterm' },
            2: { name: 'keyword.directive.resterm' },
            4: { name: 'string.directive-args.resterm' },
          },
        },
      ],
    },
    comment: {
      match: String.raw`^\s*(?:#|//|--).*$`,
      name: 'comment.line.resterm',
    },
    'request-line': {
      match: `^\\s*(?i:${alt(methods)})(?=\\s|$).*$`,
      name: 'keyword.request-line.resterm',
    },
  },
};

const rtsKeywords = {
  decl: ['export', 'module', 'fn', 'let', 'const'],
  control: ['if', 'elif', 'else', 'switch', 'case', 'default', 'return', 'for', 'break', 'continue', 'range', 'try'],
  literal: ['true', 'false', 'null'],
  logical: ['and', 'or', 'not'],
};

const word = (words: string[]) => `\\b(?:${words.join('|')})\\b`;

export const restermScript: LanguageRegistration = {
  name: 'rts',
  scopeName: 'source.rts',
  aliases: ['restermscript'],
  repository: {},
  patterns: [
    { match: '#.*$', name: 'comment.line.rts' },
    { match: String.raw`"(?:[^"\\]|\\.)*"|'(?:[^'\\]|\\.)*'`, name: 'string.quoted.rts' },
    { match: word(rtsKeywords.decl), name: 'keyword.declaration.rts' },
    { match: word(rtsKeywords.control), name: 'keyword.control.rts' },
    { match: word(rtsKeywords.literal), name: 'constant.language.rts' },
    { match: word(rtsKeywords.logical), name: 'keyword.operator.logical.rts' },
    { match: String.raw`\b\d+(?:\.\d+)?\b`, name: 'constant.numeric.rts' },
    {
      match: String.raw`(\.)([A-Za-z_]\w*)(?=\s*\()`,
      captures: { 2: { name: 'entity.name.method.rts' } },
    },
    { match: String.raw`\b[A-Za-z_]\w*(?=\s*\()`, name: 'entity.name.function.rts' },
  ],
};
