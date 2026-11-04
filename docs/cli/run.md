# resterm run

Use `resterm run` to run request files from a terminal, script, or CI job. It uses the same engine as the terminal interface (TUI), selects the requests or workflow you asked for, and writes results to standard output (stdout).

```bash
resterm run [flags] <file|->
```

- Pass a file path to run a request file from disk.
- Pass `-` to read the request file from stdin.
- Flags may appear before or after the file argument. See [Argument Order](README.md#argument-order).

## Selecting What To Run

If you do not pass a selector:

- a file with one request runs that request automatically
- a file with multiple requests opens a picker when both stdin and stdout are connected to terminals (TTYs)
- a file with multiple requests prints a numbered list and exits with code `2` in non-interactive use

The interactive picker follows the active theme and can be customized with `styles.cli_run_picker*` without changing TUI list or response-pane styling.

Selection flags:

| Flag | Short | Description |
| --- | --- | --- |
| `--request <name>` | `-r <name>` | Run one named request. |
| `--workflow <name>` | `-W <name>` | Run one named workflow. |
| `--tag <tag>` | `-g <tag>` | Run every request tagged with the given tag. |
| `--line <n>` | `-l <n>` | Run the request or workflow whose source range contains line `n`. |
| `--all` | `-a` | Run every request in the file. |
| `--profile` | `-p` | Force profile mode for the selected request. |

Selector rules:

- `--request` cannot be combined with `--tag` or `--line`
- `--all` cannot be combined with `--request`, `--tag`, or `--line`
- `--line` cannot be combined with other selectors
- `--workflow` cannot be combined with request selectors
- `--workflow` cannot be combined with `--compare` or `--profile`

Environment selection rules:

- Named-environment files use `--env dev`. Grouped files use the repeatable `--env-group group=profile`. The two flags cannot be combined.
- In the TUI, these flags stay in effect when you change workspace. Opening another workspace looks for the same environment there instead of using its default, and leaves nothing selected when it does not exist. `--env-file` is kept across workspaces, and Resterm warns when the new one has an environment file of its own. See [Environment files](../variables.md#environment-files).
- Groups you do not pass keep their declared defaults. Profiles may contain spaces: `--env-group 'app=dev app 1'`.
- Grouped compare requires `--compare-group`. Separate compare targets with commas when profile names contain spaces, for example `--compare 'dev app 1,dev app 2' --compare-group app`.
- An unknown group, profile, or baseline is rejected before any request is sent.

## Output Formats

`resterm run` supports the following output modes:

| Format | Behavior |
| --- | --- |
| `auto` | For exactly one request result, show a request view similar to the TUI. Otherwise, use the text report. |
| `text` | Stable human-readable summary for requests, workflows, compare runs, and profiles, with the name and message of each failed test. |
| `json` | Machine-readable JSON report. |
| `junit` | JUnit XML report for CI systems. |
| `pretty` | Force the single-request Pretty view. Requires exactly one request result. |
| `raw` | Force the single-request Raw view. Requires exactly one request result. |

Related flags:

| Flag | Short | Description |
| --- | --- | --- |
| `--format <mode>` | `-f <mode>` | One of `auto`, `text`, `json`, `junit`, `pretty`, or `raw`. |
| `--body` | `-b` | Print only the response body for exactly one request result. |
| `--headers` | `-H` | Include request and response headers when a single-request view is rendered. |
| `--response` | | Add each response's headers, body, and captured values to `--format json` output. |
| `--color <mode>` | `-c <mode>` | Pretty-output color mode: `auto`, `always`, `never`. |

Output rules:

- `--body` only works with `--format auto`, `--format pretty`, or `--format raw`
- `--body` keeps the exit status, so a failing run can print only the body and still exit `1`
- `--format pretty`, `--format raw`, and `--body` all require exactly one request result
- `--response` only works with `--format json`
- `--color auto` enables ANSI output only when stdout is a TTY and the terminal supports color
- `--color always` forces pretty color even when output is piped

## Execution Controls

| Flag | Short | Description |
| --- | --- | --- |
| `--fail-fast` | `-ff` | Stop after the first failed top-level result and mark the remaining selected requests as skipped. |
| `--exit-code-mode <mode>` | `-m <mode>` | `detailed` returns a code for the kind of failure. `summary` keeps the old `0`/`1`/`2` codes. |

JSON output includes a top-level `schemaVersion`, `summary.exitCode`, and `summary.failureCodes`. Each failed result has a `failure` object. For workflows and compare runs, the same object appears on failed steps; for profiles, it appears on failed iterations. When a gRPC server returns status details, `grpc.statusDetails` contains those messages as JSON.

Text, JSON, and JUnit reports, stream transcripts, trace files, and errors show secret values as `•••`. They mask every secret the run knew by its end: values declared in the request file or loaded through `env:`, secret captures, values a script marks as secret, and auth tokens, including the encoded Basic credential. A secret is found when it is written with JSON, URL, or HTML escapes, and base64 data that holds one is masked in full. A secret that the request file or environment file declares with fewer than 4 characters adds a warning, because masking it can also hide unrelated text that contains it. Parse errors and the list of requests shown when none is selected mask the secret values the file declares. The single-request views and `--body` show the response as it arrived. The single-request views mask the request name, URL, errors, test results, warnings, the request headers, and the WebSocket messages Resterm sent.

With `--response`, each request result, and each workflow, for-each, and compare step, has a `response` object. It holds the response `headers`, the `body`, `bodySize`, and the `captures` the request saved. `bodySize` is the size of the body as it was received. A gzip body is decoded before it is masked, and its headers then have no `Content-Encoding` or `Content-Length`. A body with any other content encoding, an invalid gzip body, or a gzip body that decodes to more than 32 MiB, whatever `--max-response-size` is set to, has no `body`. A body that is not valid UTF-8 is base64-encoded and has `"bodyEncoding": "base64"`. Secret values, `-secret` captures, `Set-Cookie` headers, and sensitive headers such as `Authorization` and `X-API-Key` show as `•••`. Masking uses every secret the run knew, so a value that a later request captures as a secret is masked in earlier responses too. If the masked body is larger than 1 MiB, only its first 1 MiB is kept and `"bodyTruncated": true` is set. The cut never splits a character, so up to 3 bytes fewer can be kept. Profile runs, skipped requests, requests that failed before a response arrived, and requests whose captures failed have no `response` object.

## Artifacts And Persisted State

Save stream transcripts and trace summaries as artifacts, or keep globals, authentication, and history between runs with the flags below.

| Flag | Short | Description |
| --- | --- | --- |
| `--artifact-dir <dir>` | `-A <dir>` | Write artifacts produced by the run. |
| `--state-dir <dir>` | `-s <dir>` | Root directory for persisted runner state. |
| `--persist-globals` | `-G` | Persist captured globals between runs. |
| `--persist-auth` | `-P` | Persist cached auth state between runs. |
| `--history` | `-y` | Persist run history to the state directory. |

Behavior:

- stream transcripts are written under `<artifact-dir>/streams/`
- trace summaries are written under `<artifact-dir>/traces/`, with each phase's start time and duration
- stream transcripts and trace summaries show secret values as `•••`, as the report does
- when persistence is enabled and `--state-dir` is omitted, Resterm uses `<config-dir>/runner/<workspace>-<digest>`, one directory per workspace. State written before this became per-workspace stays at `<config-dir>/runner` and is not migrated, so the first run after upgrading re-authenticates
- `--state-dir` is used exactly as given, so pass the same value only for workspaces that are meant to share state
- persisted globals and auth are keyed by environment scope. A scope includes the environment file it came from, so two projects that both define a `dev` environment never read each other's globals or OAuth tokens, even with the same `--state-dir`
- `--persist-globals` writes `runtime.json`
- `--persist-auth` writes `auth.json`
- `--history` writes `history.db`
- JSON output includes artifact paths such as `transcriptPath` and `artifactPath` when those files are written

## Exit Codes

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

## Examples

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

Select two grouped profiles and compare only the API group:

```bash
resterm run \
  --env-group 'credentials=ci' \
  --env-group 'app=dev app 1' \
  --compare 'dev,uat,prod' \
  --compare-group api \
  --compare-base dev \
  ./requests.http
```

Get the status, headers, body, and captured values of one request as JSON:

```bash
resterm run --request login --format json --response ./requests.http
```

Print only the raw response body from one request:

```bash
resterm run --request create-user --body ./requests.http
```

Read the request document from stdin:

```bash
cat ./requests.http | resterm run - --request health
```

Persist globals, auth, and history between runs:

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
