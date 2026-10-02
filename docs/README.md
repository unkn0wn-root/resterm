# Resterm documentation

Each page below is one file in this directory. The same pages are published at https://resterm.app/docs/. To add or change a page, see [website/README.md](../website/README.md#docs-come-from-docs).

## Getting started

- [Install](install.md): Install Resterm with Homebrew, the install scripts, a release binary or Go.
- [Quick start](quick-start.md): Write a first request file, send it, and bootstrap a project with resterm init.
- [UI tour](ui-tour.md): The panes, completions, help and response views of the TUI.
- [Key bindings](key-bindings.md): Every default shortcut, and how to change them in a bindings file.
- [Workspaces and files](workspaces.md): How Resterm finds request files, and how to send a request without one.

## Request files

- [Request file anatomy](request-files.md): Separators, comments, directives and bodies in .http and .rest files.
- [Variables and environments](variables.md): Environment files, variable scopes, resolution order, dynamic helpers and secrets.
- [Captures](captures.md): Store values from a response and reuse them in later requests.
- [Authentication](authentication.md): Static tokens, captured tokens, OAuth 2.0 and tokens from CLIs you already use.
- [JavaScript hooks](scripting.md): JavaScript pre-request and test scripts with @script.
- [HTTP transport and settings](http-settings.md): Base URLs, timeouts, proxies, TLS and other transport settings.

## Protocols

- [GraphQL](graphql.md): Send GraphQL queries with operations and variables from request files.
- [gRPC](grpc.md): Call gRPC methods with reflection or descriptors, including streaming calls.
- [WebSocket and SSE](streaming.md): Server-Sent Events and WebSocket sessions, transcripts and the live console.

## Testing and automation

- [Workflows](workflows.md): Chain named requests into steps with @workflow and @step.
- [Polling and retries](polling-and-retries.md): Repeat a request until a condition holds, or retry it on failure.
- [Compare runs](compare-runs.md): Send the same request to several environments and diff the results.
- [Profiling](profiling.md): Run a request many times and read its latency percentiles.
- [Mock servers](mock-servers.md): Serve mock responses defined next to your requests, with matching and sequences.
- [Recording traffic](recording.md): Put the Resterm proxy in front of an API and save the traffic as requests or mocks.
- [History and diffing](history.md): Browse, replay and diff past responses.

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

- [Overview](cli/README.md): The command-line entry points, their argument order and shared flags.
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
