# Resterm CLI

Resterm has four command-line entry points:

- `resterm` opens the interactive TUI. It also handles import, update, history, and collection tasks.
- `resterm run` runs the same `.http` / `.rest` files without opening the TUI.
- `resterm mock` serves mock responses declared in those files.
- `resterm record` forwards HTTP traffic and saves it as requests or mocks.

Use this guide for command-line behavior. For request syntax, directives, workflows, auth, and UI behavior, see the [documentation index](../README.md). For RestermScript, see [RestermScript](../rts/README.md).

## Command Overview

| Command | What it does |
| --- | --- |
| `resterm [file]` | Open the TUI in the current workspace or a specific request file. |
| `resterm run [flags] <file\|->` | Execute request files, workflows, compare runs, or profile runs without the TUI. |
| `resterm mock [flags] [file\|dir]` | Serve and optionally hot-reload `# @mock` response blocks. |
| `resterm record --upstream <origin> --out <file> [flags]` | Record application traffic as requests, mocks, or both. |
| `resterm mock reset [flags] [sequence]` | Reset all sequence cursors or every cursor with one name. |
| `resterm mock clear [flags]` | Clear the standalone mock journal and access logs. |
| `resterm mock verify [flags] [file\|dir]` | Verify exact `# @expect` call counts against a running mock server. |
| `resterm init [dir]` | Create a new Resterm workspace. |
| `resterm collection ...` | Export, import, pack, and unpack portable request bundles. |
| `resterm history ...` | Export, import, inspect, compact, and verify persisted history. |
| `resterm --from-curl ...` | Convert curl commands into `.http` files. |
| `resterm --from-openapi ...` | Generate `.http` collections from OpenAPI documents. |
| `resterm --check-update`, `resterm --update`, `resterm --version` | Inspect or update the installed binary. |

## Argument Order

Flags and positional arguments can be mixed in any order, so `resterm run requests.http --request createPost` and `resterm run --request createPost requests.http` are the same command. Everything after a `--` terminator is treated as a positional argument, which is how you pass a file whose name starts with a dash.

## Shared Execution Flags

These flags are shared by `resterm` and `resterm run` when request execution is involved.

| Flag | Short | Description |
| --- | --- | --- |
| `--workspace <dir>` | `-w <dir>` | Workspace root used for file discovery and relative resolution. |
| `--recursive` | `-R` | Recursively scan the workspace for request files. |
| `--env <name>` | `-e <name>` | Select an environment explicitly. |
| `--env-group <group=profile>` |  | Select one group profile. Repeat for multiple groups. |
| `--env-file <path>` | `-E <path>` | Use an explicit environment JSON file. |
| `--timeout <duration>` | `-t <duration>` | Default HTTP timeout. |
| `--insecure` | `-k` | Skip TLS certificate verification. |
| `--follow` | `-L` | Follow redirects. Pass `--follow=false` or `-L=false` to disable it. |
| `--max-redirects <n>` | | Maximum redirects to follow. The default is 10. Use `0` to disable redirects. |
| `--proxy <url>` | `-x <url>` | HTTP proxy URL. |
| `--max-response-size <size>` | | Largest response body to read, such as `100mb`. The default is 32 MiB. Use `none` to remove the limit. |
| `--compare <envs>` | `-C <envs>` | Default comma/space-delimited compare targets. |
| `--compare-base <env>` | `-B <env>` | Baseline environment for compare runs. |
| `--compare-group <group>` |  | In grouped mode, vary this group while holding all other groups fixed. |
| `--trace-otel-endpoint <url>` | `-toe <url>` | OTLP collector endpoint used by `@trace`. |
| `--trace-otel-insecure` | `-toi` | Disable TLS for OTLP trace export. |
| `--trace-otel-service <name>` | `-tos <name>` | Override the exported `service.name`. |

## `resterm`

Use `resterm` without subcommands when you want the TUI:

```bash
resterm
resterm --workspace ./api-tests
resterm --file ./requests.http
```

These flags belong to the top-level `resterm` command:

| Flag | Short | Description |
| --- | --- | --- |
| `--file <path>` | `-f <path>` | Path to a `.http` / `.rest` file to open. |
| `--version` | `-v` | Print the version, commit, build date, and checksum. |
| `--check-update` | `-c` | Check for newer releases and exit. |
| `--update` | `-u` | Download and install the latest release, if available. |
| `--from-curl <cmd-or-path>` | `-fc <cmd-or-path>` | Curl command or file path to convert. |
| `--from-openapi <path-or-url>` | `-fo <path-or-url>` | OpenAPI specification (local file or `http(s)` URL) to convert. |
| `--http-out <path>` | `-o <path>` | Destination path for the generated `.http` file. |
| `--openapi-base-var <name>` | `-ob <name>` | Variable name for the generated base URL. |
| `--openapi-resolve-refs` | `-or` | Resolve external `$ref` references during OpenAPI import. |
| `--openapi-include-deprecated` | `-od` | Include deprecated operations when generating requests. |
| `--openapi-server-index <n>` | `-os <n>` | Preferred server index from the spec to use as the base URL. |
| `--openapi-mode <mode>` |  | Generate `requests` (default), `mocks`, or `both`. |
