import type { GroupSpec, SourceSpec } from '../lib/docs/assemble';

// The site reads docs/*.md from the repo root and cuts it into the pages below.
// Section ids are the GitHub anchors of the headings. The build fails when a
// top-level heading in docs/ is not listed here, so nothing drops off the site.

export const sources = {
  resterm: { file: 'resterm.md', exclude: ['index', 'cli-reference'] },
  cli: { file: 'cli.md', exclude: ['related-docs'] },
  rts: { file: 'restermscript.md' },
} satisfies Record<string, SourceSpec>;

type Source = keyof typeof sources;

export const groups: GroupSpec<Source>[] = [
  {
    title: 'Getting started',
    pages: [
      {
        slug: 'install',
        title: 'Install',
        summary: 'Install Resterm with Homebrew, the install scripts, a release binary or Go.',
        source: 'resterm',
        sections: ['installation'],
      },
      {
        slug: 'quick-start',
        title: 'Quick start',
        summary: 'Write a first request file, send it, and bootstrap a project with resterm init.',
        source: 'resterm',
        sections: ['quick-start', 'initializing-a-project'],
      },
      {
        slug: 'ui-tour',
        title: 'UI tour',
        summary: 'The panes, key bindings, completions, help and response views of the TUI.',
        source: 'resterm',
        sections: ['ui-tour'],
      },
      {
        slug: 'workspaces',
        title: 'Workspaces and files',
        summary: 'How Resterm finds request files, and how to send a request without one.',
        source: 'resterm',
        sections: ['workspaces--files'],
      },
    ],
  },
  {
    title: 'Request files',
    pages: [
      {
        slug: 'request-files',
        title: 'Request file anatomy',
        summary: 'Separators, comments, directives and bodies in .http and .rest files.',
        source: 'resterm',
        sections: ['request-file-anatomy', 'body-content'],
      },
      {
        slug: 'variables',
        title: 'Variables and environments',
        summary: 'Environment files, variable scopes, resolution order, dynamic helpers and secrets.',
        source: 'resterm',
        sections: ['variables-and-environments', 'variable-declarations', 'secret-redaction'],
      },
      {
        slug: 'captures',
        title: 'Captures',
        summary: 'Store values from a response and reuse them in later requests.',
        source: 'resterm',
        sections: ['captures'],
      },
      {
        slug: 'authentication',
        title: 'Authentication',
        summary: 'Static tokens, captured tokens, OAuth 2.0 and tokens from CLIs you already use.',
        source: 'resterm',
        sections: ['authentication', 'authentication-directives'],
      },
      {
        slug: 'scripting',
        title: 'JavaScript hooks',
        summary: 'JavaScript pre-request and test scripts with @script.',
        source: 'resterm',
        sections: ['scripting-api', 'scripting-script'],
      },
      {
        slug: 'http-settings',
        title: 'HTTP transport and settings',
        summary: 'Base URLs, timeouts, proxies, TLS and other transport settings.',
        source: 'resterm',
        sections: ['http-transport--settings'],
      },
    ],
  },
  {
    title: 'Protocols',
    pages: [
      {
        slug: 'graphql',
        title: 'GraphQL',
        summary: 'Send GraphQL queries with operations and variables from request files.',
        source: 'resterm',
        sections: ['graphql'],
      },
      {
        slug: 'grpc',
        title: 'gRPC',
        summary: 'Call gRPC methods with reflection or descriptors, including streaming calls.',
        source: 'resterm',
        sections: ['grpc'],
      },
      {
        slug: 'streaming',
        title: 'WebSocket and SSE',
        summary: 'Server-Sent Events and WebSocket sessions, transcripts and the live console.',
        source: 'resterm',
        sections: ['streaming-sse--websocket'],
      },
    ],
  },
  {
    title: 'Testing and automation',
    pages: [
      {
        slug: 'workflows',
        title: 'Workflows',
        summary: 'Chain named requests into steps with @workflow and @step.',
        source: 'resterm',
        sections: ['workflows'],
      },
      {
        slug: 'polling-and-retries',
        title: 'Polling and retries',
        summary: 'Repeat a request until a condition holds, or retry it on failure.',
        source: 'resterm',
        sections: ['polling-and-retries'],
      },
      {
        slug: 'compare-runs',
        title: 'Compare runs',
        summary: 'Send the same request to several environments and diff the results.',
        source: 'resterm',
        sections: ['compare-runs'],
      },
      {
        slug: 'profiling',
        title: 'Profiling',
        summary: 'Run a request many times and read its latency percentiles.',
        source: 'resterm',
        sections: ['profiling-requests'],
      },
      {
        slug: 'mock-servers',
        title: 'Mock servers',
        summary: 'Serve mock responses defined next to your requests, with matching and sequences.',
        source: 'resterm',
        sections: ['mock-servers'],
      },
      {
        slug: 'recording',
        title: 'Recording traffic',
        summary: 'Put the Resterm proxy in front of an API and save the traffic as requests or mocks.',
        source: 'resterm',
        sections: ['recording-traffic'],
      },
      {
        slug: 'history',
        title: 'History and diffing',
        summary: 'Browse, replay and diff past responses.',
        source: 'resterm',
        sections: ['response-history--diffing'],
      },
    ],
  },
  {
    title: 'Connectivity',
    pages: [
      {
        slug: 'ssh-tunnels',
        title: 'SSH tunnels',
        summary: 'Send requests through an SSH bastion that Resterm opens and closes for you.',
        source: 'resterm',
        sections: ['ssh-tunnels'],
      },
      {
        slug: 'kubernetes',
        title: 'Kubernetes port-forwards',
        summary: 'Send requests through a Kubernetes port-forward managed by Resterm.',
        source: 'resterm',
        sections: ['kubernetes-port-forwards'],
      },
    ],
  },
  {
    title: 'RestermScript',
    pages: [
      {
        slug: 'rts',
        title: 'Overview',
        summary: 'What RestermScript is, when to use it and where it runs.',
        source: 'rts',
        sections: ['restermscript-technical-reference', 'why-this-even-exists', 'when-to-use-it', 'where-it-runs'],
      },
      {
        slug: 'rts/language',
        title: 'Language',
        summary: 'Comments, literals, operators, types and error handling.',
        source: 'rts',
        sections: ['language-overview'],
      },
      {
        slug: 'rts/statements',
        title: 'Statements',
        summary: 'Bindings, functions, conditionals, switch and loops.',
        source: 'rts',
        sections: ['statements'],
      },
      {
        slug: 'rts/modules',
        title: 'Modules and exports',
        summary: 'Share logic between request files with .rts modules and @use.',
        source: 'rts',
        sections: ['modules-and-exports'],
      },
      {
        slug: 'rts/stdlib',
        title: 'Standard library',
        summary: 'Built-in helpers for text, JSON, lists, time, crypto and more.',
        source: 'rts',
        sections: ['standard-library'],
      },
      {
        slug: 'rts/host-objects',
        title: 'Host objects',
        summary: 'The env, vars, request, response, trace, stream and mock objects.',
        source: 'rts',
        sections: ['host-objects-for-request-evaluation'],
      },
      {
        slug: 'rts/directives',
        title: 'Directives',
        summary: 'Directives that evaluate RestermScript: @apply, @assert, @if, @for-each and others.',
        source: 'rts',
        sections: ['directives-and-workflows'],
      },
      {
        slug: 'rts/patterns',
        title: 'Patterns and limits',
        summary: 'Common patterns, hard limits and the reasons behind the design.',
        source: 'rts',
        sections: ['common-patterns', 'limits-and-safety', 'design-constraints-and-why-they-exist'],
      },
    ],
  },
  {
    title: 'CLI',
    pages: [
      {
        slug: 'cli',
        title: 'Overview',
        summary: 'The command-line entry points, their argument order and shared flags.',
        source: 'cli',
        sections: [
          'resterm-cli',
          'command-overview',
          'argument-order',
          'shared-execution-flags',
          'resterm-utility-flags',
          'resterm',
        ],
      },
      {
        slug: 'cli/run',
        title: 'resterm run',
        summary: 'Run request files without the TUI, for scripts and CI.',
        source: 'cli',
        sections: ['resterm-run'],
      },
      {
        slug: 'cli/mock',
        title: 'resterm mock',
        summary: 'Serve, reset, clear and verify mock servers from the command line.',
        source: 'cli',
        sections: ['resterm-mock'],
      },
      {
        slug: 'cli/record',
        title: 'resterm record',
        summary: 'Record application traffic from the command line.',
        source: 'cli',
        sections: ['resterm-record'],
      },
      {
        slug: 'cli/init',
        title: 'resterm init',
        summary: 'Create a new workspace with starter files.',
        source: 'cli',
        sections: ['resterm-init'],
      },
      {
        slug: 'cli/collection',
        title: 'resterm collection',
        summary: 'Export, import, pack and unpack request bundles.',
        source: 'cli',
        sections: ['resterm-collection'],
      },
      {
        slug: 'cli/history',
        title: 'resterm history',
        summary: 'Export, import, inspect and compact stored history.',
        source: 'cli',
        sections: ['resterm-history'],
      },
      {
        slug: 'cli/import',
        title: 'Import curl and OpenAPI',
        summary: 'Turn curl commands and OpenAPI documents into request files.',
        source: 'cli',
        sections: ['import-examples'],
      },
    ],
  },
  {
    title: 'Reference',
    pages: [
      {
        slug: 'configuration',
        title: 'Configuration',
        summary: 'Where Resterm keeps settings, history and themes, and editor diagnostics.',
        source: 'resterm',
        sections: ['configuration'],
      },
      {
        slug: 'theming',
        title: 'Theming',
        summary: 'Write and test your own color theme.',
        source: 'resterm',
        sections: ['theming'],
      },
      {
        slug: 'collections',
        title: 'Collection sharing',
        summary: 'Package a workspace as a bundle you can commit and import elsewhere.',
        source: 'resterm',
        sections: ['collection-sharing'],
      },
      {
        slug: 'security',
        title: 'Security',
        summary: 'What request files can run, the safety limits, and telemetry.',
        source: 'resterm',
        sections: ['security'],
      },
      {
        slug: 'compatibility',
        title: 'Compatibility',
        summary: 'What stays stable through v1 and what may change.',
        source: 'resterm',
        sections: ['compatibility'],
      },
      {
        slug: 'examples',
        title: 'Examples',
        summary: 'Ready-to-run request files in the _examples directory.',
        source: 'resterm',
        sections: ['examples-1'],
      },
      {
        slug: 'troubleshooting',
        title: 'Troubleshooting',
        summary: 'Fixes for common problems and small tips.',
        source: 'resterm',
        sections: ['troubleshooting--tips'],
      },
    ],
  },
];
