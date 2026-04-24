# Resterm CLI

Resterm has two commandline entry points:

- `resterm` opens the interactive TUI and also exposes import, update, history, and collection tooling.
- `resterm run` executes the same `.http` / `.rest` files headlessly without opening the TUI.

Use this guide for commandline behavior. For request syntax, directives, workflows, auth, and UI behavior, see [`resterm.md`](./resterm.md). For RestermScript, see [`restermscript.md`](./restermscript.md).

## Command Overview

| Command | What it does |
| --- | --- |
| `resterm [file]` | Open the TUI in the current workspace or a specific request file. |
| `resterm run [flags] <file\|->` | Execute request files, workflows, compare runs, or profile runs without the TUI. |
| `resterm init [dir]` | Bootstrap a new Resterm workspace. |
| `resterm collection ...` | Export, import, pack, and unpack portable request bundles. |
| `resterm history ...` | Export, import, inspect, compact, and verify persisted history. |
| `resterm --from-curl ...` | Convert curl commands into `.http` files. |
| `resterm --from-openapi ...` | Generate `.http` collections from OpenAPI documents. |
| `resterm --check-update`, `resterm --update`, `resterm --version` | Inspect or update the installed binary. |

## Shared Execution Flags

These flags are shared by `resterm` and `resterm run` when request execution is involved.

| Flag | Description |
| --- | --- |
| `--workspace <dir>` | Workspace root used for file discovery and relative resolution. |
| `--recursive` | Recursively scan the workspace for request files. |
| `--env <name>` | Select an environment explicitly. |
| `--env-file <path>` | Use an explicit environment JSON file. |
| `--timeout <duration>` | Default HTTP timeout. |
| `--insecure` | Skip TLS certificate verification. |
| `--follow` | Follow redirects. Pass `--follow=false` to disable it. |
| `--proxy <url>` | HTTP proxy URL. |
| `--compare <envs>` | Default comma/space-delimited compare targets. |
| `--compare-base <env>` | Baseline environment for compare runs. |
| `--trace-otel-endpoint <url>` | OTLP collector endpoint used by `@trace`. |
| `--trace-otel-insecure` | Disable TLS for OTLP trace export. |
| `--trace-otel-service <name>` | Override the exported `service.name`. |

## `resterm run`

`resterm run` is the headless execution path. It parses a request file, selects one or more targets, runs them with the same engine used by the TUI, and writes the result to stdout.

```bash
resterm run [flags] <file|->
```

- Pass a file path to execute a request document from disk.
- Pass `-` to read the request file from stdin.

### Selecting What To Run

If you do not pass a selector:

- a file with one request runs that request automatically
- a file with multiple requests prompts for a choice when stdin/stdout are TTYs
- a file with multiple requests prints a numbered list and exits with code `2` in non-interactive use

Selection flags:

| Flag | Description |
| --- | --- |
| `--request <name>` / `-r <name>` | Run one named request. |
| `--workflow <name>` | Run one named workflow. |
| `--tag <tag>` | Run every request tagged with the given tag. |
| `--line <n>` | Run the request or workflow whose source range contains line `n`. |
| `--all` | Run every request in the file. |
| `--profile` | Force profile mode for the selected request. |

Selector rules:

- `--request` cannot be combined with `--tag` or `--line`
- `--all` cannot be combined with `--request`, `--tag`, or `--line`
- `--line` cannot be combined with other selectors
- `--workflow` cannot be combined with request selectors
- `--workflow` cannot be combined with `--compare` or `--profile`

### Output Formats

`resterm run` supports the following output modes:

| Format | Behavior |
| --- | --- |
| `auto` | For exactly one request result, render a human request view similar to the TUI. Otherwise, fall back to the text report. |
| `text` | Stable human-readable summary for requests, workflows, compare runs, and profiles. |
| `json` | Machine-readable JSON report. |
| `junit` | JUnit XML report for CI systems. |
| `pretty` | Force the single-request Pretty view. Requires exactly one request result. |
| `raw` | Force the single-request Raw view. Requires exactly one request result. |

Related flags:

| Flag | Description |
| --- | --- |
| `--format <mode>` | One of `auto`, `text`, `json`, `junit`, `pretty`, or `raw`. |
| `--body` | Print only the response body for exactly one request result. |
| `--headers` | Include request and response headers when a single-request view is rendered. |
| `--color <mode>` | Pretty-output color mode: `auto`, `always`, `never`. |

Output rules worth knowing:

- `--body` only works with `--format auto`, `--format pretty`, or `--format raw`
- `--body` still preserves exit status; a failing run can print only the body and still exit `1`
- `--format pretty`, `--format raw`, and `--body` all require exactly one request result
- `--color auto` enables ANSI output only when stdout is a TTY and the terminal supports color
- `--color always` forces pretty color even when output is piped

### Execution Controls

| Flag | Description |
| --- | --- |
| `--fail-fast` | Stop after the first failed top-level result and mark the remaining selected requests as skipped. |
| `--exit-code-mode <mode>` | `detailed` returns classified CI exit codes; `summary` preserves the legacy `0`/`1`/`2` contract. |

JSON output includes a top-level `schemaVersion`, `summary.exitCode`, `summary.failureCodes`, and per-result `failure` metadata when a result fails. Workflow, compare, and profile failures include the same structured failure object at the step or profile-iteration level.

### Artifacts And Persisted State

`resterm run` can write execution artifacts and optionally persist runtime state between invocations.

| Flag | Description |
| --- | --- |
| `--artifact-dir <dir>` | Write artifacts produced by the run. |
| `--state-dir <dir>` | Root directory for persisted runner state. |
| `--persist-globals` | Persist captured globals between runs. |
| `--persist-auth` | Persist cached auth state between runs. |
| `--history` | Persist run history to the state directory. |

Behavior:

- stream transcripts are written under `<artifact-dir>/streams/`
- trace summaries are written under `<artifact-dir>/traces/`
- when persistence is enabled and `--state-dir` is omitted, Resterm uses `<config-dir>/runner`
- `--persist-globals` writes `runtime.json`
- `--persist-auth` writes `auth.json`
- `--history` writes `history.db`
- JSON output includes artifact paths such as `transcriptPath` and `artifactPath` when those files are written

### Exit Codes

By default, `resterm run` uses detailed exit codes so CI/CD systems can distinguish operational failures from assertion failures. Pass `--exit-code-mode summary` when existing automation expects only pass/fail/usage exit codes.

| Exit code | Meaning |
| --- | --- |
| `0` | All selected results passed. |
| `1` | Execution completed, but at least one result failed. This includes request failures, test failures, and trace budget breaches. |
| `2` | Usage or selection error. This includes invalid flag combinations, parse errors, unsupported formats, ambiguous selection, and missing request files. |
| `3` | Internal or unknown runtime failure. |
| `20` | Timeout or deadline failure. |
| `21` | Network failure such as DNS, dial, connection reset, or proxy failure. |
| `22` | TLS or certificate failure. |
| `23` | Authentication or authorization failure. |
| `24` | Script execution failure. |
| `25` | Filesystem, state, artifact, or history persistence failure. |
| `26` | Protocol failure such as malformed HTTP/gRPC/streaming behavior. |
| `27` | Route/tunnel failure such as SSH or Kubernetes port-forward setup. |
| `130` | Canceled execution. |

In `--exit-code-mode summary`, completed failed runs and runtime failures exit `1`, usage errors exit `2`, and successful runs exit `0`.

### Examples

Run a single request file headlessly:

```bash
resterm run ./requests.http
```

Run a named request and print the TUI-style pretty view:

```bash
resterm run --request login --format pretty ./requests.http
```

Run a workflow:

```bash
resterm run --workflow smoke ./requests.http
```

Run every request tagged `smoke` and write JSON:

```bash
resterm run --tag smoke --format json ./requests.http > run.json
```

Print only the raw response body from one request:

```bash
resterm run --request create-user --body ./requests.http
```

Read the request document from stdin:

```bash
cat ./requests.http | resterm run - --request health
```

Persist globals, auth, and history between invocations:

```bash
resterm run \
  --request login \
  --persist-globals \
  --persist-auth \
  --history \
  --state-dir ./.resterm-run \
  ./requests.http
```

Write stream and trace artifacts:

```bash
resterm run --request events --artifact-dir ./artifacts ./streams.http
```

Force profile mode for a request:

```bash
resterm run --request health --profile ./requests.http
```

Stop after the first failed selected request while still recording skipped results:

```bash
resterm run --tag smoke --fail-fast --format json ./requests.http
```

## `resterm`

Use `resterm` without subcommands when you want the TUI:

```bash
resterm
resterm --workspace ./api-tests
resterm --file ./requests.http
```

Top-level flags also expose a few CLI-only workflows:

| Flag | Description |
| --- | --- |
| `--from-curl <command\|path>` | Convert curl commands into a `.http` file. |
| `--from-openapi <spec>` | Generate a `.http` collection from an OpenAPI document. |
| `--http-out <file>` | Output path for generated `.http` files. |
| `--openapi-base-var <name>` | Override the generated base URL variable name. |
| `--openapi-resolve-refs` | Resolve external `$ref` values during OpenAPI import. |
| `--openapi-include-deprecated` | Keep deprecated operations during OpenAPI generation. |
| `--openapi-server-index <n>` | Pick the preferred server entry from the OpenAPI document. |
| `--check-update` | Check for a newer release and exit. |
| `--update` | Download and install the latest release when supported. |
| `--version` | Print version, commit, build date, and checksum. |

## `resterm init`

`resterm init` bootstraps a workspace with starter files such as `requests.http`, `resterm.env.json`, and the optional standard template helpers.

```bash
resterm init
resterm init ./api-tests
resterm init --template minimal
```

See [Initializing a Project](./resterm.md#initializing-a-project) for templates and flag details.

## `resterm collection`

The collection commands package a workspace so it can be copied or shared safely.

| Command | What it does |
| --- | --- |
| `resterm collection export --workspace <dir> --out <dir>` | Export a Git-friendly bundle directory. |
| `resterm collection import --in <dir> --workspace <dir>` | Import a bundle into another workspace. |
| `resterm collection pack --in <dir> --out <file.zip>` | Pack a bundle directory into a zip archive. |
| `resterm collection unpack --in <file.zip> --out <dir>` | Unpack and validate a bundle archive. |

See [Collection Sharing](./resterm.md#collection-sharing) for safety and manifest behavior.

## `resterm history`

The history commands operate on persisted history storage.

| Command | What it does |
| --- | --- |
| `resterm history export --out <path>` | Export persisted history as JSON. |
| `resterm history import --in <path>` | Import history from JSON. |
| `resterm history backup --out <path>` | Create a SQLite-consistent backup. |
| `resterm history stats` | Print schema version, row counts, and sizes. |
| `resterm history check [--full]` | Run integrity checks. |
| `resterm history compact` | Checkpoint and compact `history.db`. |

## Import Examples

Convert curl into Resterm request files:

```bash
resterm --from-curl "curl https://example.com -H 'X-Test: 1'" --http-out example.http
resterm --from-curl ./requests.curl --http-out requests.http
cat requests.curl | resterm --from-curl - --http-out requests.http
```

Generate a collection from OpenAPI:

```bash
resterm \
  --from-openapi openapi.yml \
  --http-out openapi.http \
  --openapi-resolve-refs \
  --openapi-server-index 1
```

## Related Docs

- Main reference: [`resterm.md`](./resterm.md)
- RestermScript reference: [`restermscript.md`](./restermscript.md)
- Examples: `_examples/basic.http`, `_examples/workflows.http`, `_examples/compare.http`, `_examples/streaming.http`
