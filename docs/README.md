# Resterm documentation

Start with [Install](install.md) and [Quick start](quick-start.md), then use the topics below as you need them.

These Markdown files also supply the pages at https://resterm.app/docs/. To edit or add a page, see [website/README.md](../website/README.md#docs-come-from-docs).

## Getting started

- [Install](install.md): Install Resterm with Homebrew, the install scripts, a release binary or Go.
- [Quick start](quick-start.md): Write your first request file, send a request, or create a starter project with resterm init.
- [UI tour](ui-tour.md): Find your way around the terminal interface (TUI), from the editor to help and response tabs.
- [Key bindings](key-bindings.md): Every default shortcut, and how to change them in a bindings file.
- [Workspaces and files](workspaces.md): How Resterm finds request files, and how to send a request without one.

## Request files

- [Request file anatomy](request-files.md): Separators, comments, directives and bodies in .http and .rest files.
- [Variables and environments](variables.md): Choose an environment, reuse values, generate test data and keep secrets out of request files.
- [Captures](captures.md): Store values from a response and reuse them in later requests.
- [Authentication](authentication.md): Static tokens, captured tokens, OAuth 2.0 and tokens from CLIs you already use.
- [JavaScript hooks](scripting.md): JavaScript pre-request and test scripts with @script.
- [HTTP transport and settings](http-settings.md): Set base URLs, timeouts, proxies, TLS and other connection options.

## Protocols

- [GraphQL](graphql.md): Send GraphQL queries with operations and variables from request files.
- [gRPC](grpc.md): Call gRPC methods with reflection or descriptors, including streaming calls.
- [WebSocket and SSE](streaming.md): Server-Sent Events and WebSocket sessions, transcripts and the live console.

## Testing and automation

- [Workflows](workflows.md): Chain named requests into steps with @workflow and @step.
- [Polling and retries](polling-and-retries.md): Repeat a request until a condition holds, or retry it on failure.
- [Compare runs](compare-runs.md): Send the same request to several environments and diff the results.
- [Profiling](profiling.md): Repeat a request to measure response times, percentiles and failures.
- [Mock servers](mock-servers.md): Serve mock responses defined next to your requests, with matching and sequences.
- [Recording traffic](recording.md): Put the Resterm proxy in front of an API and save the traffic as requests or mocks.
- [History and diffing](history.md): Browse, replay and diff past responses.
- [Headless Go API](headless.md): Run request files from your own Go code with the headless package.

## Connectivity

- [SSH tunnels](ssh-tunnels.md): Send requests through an SSH bastion that Resterm opens and closes for you.
- [Kubernetes port-forwards](kubernetes.md): Send requests through a Kubernetes port-forward managed by Resterm.

## RestermScript

- [Overview](rts/README.md): What RestermScript is, when to use it and where it runs.
- [Language](rts/language.md): Comments, literals, operators, types and error handling.
- [Statements](rts/statements.md): Bindings, functions, conditionals, switch and loops.
- [Modules and exports](rts/modules.md): Share logic between request files with .rts modules and @use.
- [Standard library](rts/stdlib.md): Built-in helpers for text, JSON, lists, time, crypto and more.
- [Host objects](rts/host-objects.md): The env, vars, request, response, trace, stream and mock objects.
- [Directives](rts/directives.md): Directives that evaluate RestermScript: @apply, @assert, @if, @for-each and others.
- [Patterns and limits](rts/patterns.md): Common patterns, hard limits and the reasons behind the design.

## CLI

- [Overview](cli/README.md): Choose a command and look up its argument order and shared flags.
- [resterm run](cli/run.md): Run request files without the TUI, for scripts and CI.
- [resterm mock](cli/mock.md): Serve, reset, clear and verify mock servers from the command line.
- [resterm record](cli/record.md): Record application traffic from the command line.
- [resterm init](cli/init.md): Create a new workspace with starter files.
- [resterm collection](cli/collection.md): Export, import, pack and unpack request bundles.
- [resterm history](cli/history.md): Export, import, inspect and compact stored history.
- [Import curl and OpenAPI](cli/import.md): Turn curl commands and OpenAPI documents into request files.

## Reference

- [Configuration](configuration.md): Where Resterm keeps settings, history and themes, plus editor diagnostics.
- [Theming](theming.md): Write and test your own color theme.
- [Collection sharing](collections.md): Package a workspace as a bundle you can commit and import elsewhere.
- [Security](security.md): What request files can run, the safety limits, and telemetry.
- [Compatibility](compatibility.md): What stays stable through v1 and what may change.
- [Examples](examples.md): Ready-to-run request files in the _examples directory.
- [Troubleshooting](troubleshooting.md): Fixes for common problems and small tips.
