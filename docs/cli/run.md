# resterm run

`resterm run` is the headless execution path. It parses a request file, selects one or more targets, runs them with the same engine used by the TUI, and writes the result to stdout.

```bash
resterm run [flags] <file|->
```

- Pass a file path to execute a request document from disk.
- Pass `-` to read the request file from stdin.
- Flags may appear before or after the file argument. See [Argument Order](README.md#argument-order).

## Selecting What To Run

If you do not pass a selector:

- a file with one request runs that request automatically
- a file with multiple requests prompts for a choice when stdin/stdout are TTYs
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
- In the TUI these flags outlive a workspace change. Opening another workspace looks for the same environment there rather than taking its default, and leaves nothing selected when it does not exist. `--env-file` is kept across workspaces, and Resterm warns when the new one has an environment file of its own. See [Environment files](../variables.md#environment-files).
- Groups you do not pass keep their declared defaults. Profiles may contain spaces: `--env-group 'app=dev app 1'`.
- Grouped compare requires `--compare-group`. Separate compare targets with commas when profile names contain spaces, for example `--compare 'dev app 1,dev app 2' --compare-group app`.
- An unknown group, profile, or baseline is rejected before any request is sent.

## Output Formats

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

| Flag | Short | Description |
| --- | --- | --- |
| `--format <mode>` | `-f <mode>` | One of `auto`, `text`, `json`, `junit`, `pretty`, or `raw`. |
| `--body` | `-b` | Print only the response body for exactly one request result. |
| `--headers` | `-H` | Include request and response headers when a single-request view is rendered. |
| `--color <mode>` | `-c <mode>` | Pretty-output color mode: `auto`, `always`, `never`. |

Output rules worth knowing:

- `--body` only works with `--format auto`, `--format pretty`, or `--format raw`
- `--body` still preserves exit status; a failing run can print only the body and still exit `1`
- `--format pretty`, `--format raw`, and `--body` all require exactly one request result
- `--color auto` enables ANSI output only when stdout is a TTY and the terminal supports color
- `--color always` forces pretty color even when output is piped

## Execution Controls

| Flag | Short | Description |
| --- | --- | --- |
| `--fail-fast` | `-ff` | Stop after the first failed top-level result and mark the remaining selected requests as skipped. |
| `--exit-code-mode <mode>` | `-m <mode>` | `detailed` returns classified CI exit codes; `summary` preserves the legacy `0`/`1`/`2` contract. |

JSON output includes a top-level `schemaVersion`, `summary.exitCode`, `summary.failureCodes`, and per-result `failure` metadata when a result fails. Workflow, compare, and profile failures include the same structured failure object at the step or profile-iteration level. gRPC results include `grpc.statusDetails` with each status detail message encoded as JSON when the server returns any.

## Artifacts And Persisted State

`resterm run` can write execution artifacts and optionally persist runtime state between invocations.

| Flag | Short | Description |
| --- | --- | --- |
| `--artifact-dir <dir>` | `-A <dir>` | Write artifacts produced by the run. |
| `--state-dir <dir>` | `-s <dir>` | Root directory for persisted runner state. |
| `--persist-globals` | `-G` | Persist captured globals between runs. |
| `--persist-auth` | `-P` | Persist cached auth state between runs. |
| `--history` | `-y` | Persist run history to the state directory. |

Behavior:

- stream transcripts are written under `<artifact-dir>/streams/`
- trace summaries are written under `<artifact-dir>/traces/`
- when persistence is enabled and `--state-dir` is omitted, Resterm uses `<config-dir>/runner/<workspace>-<digest>`, one directory per workspace. State written before this became per-workspace stays at `<config-dir>/runner` and is not migrated, so the first run after upgrading re-authenticates
- `--state-dir` is used exactly as given, so pass the same value only for workspaces that are meant to share state
- persisted globals and auth are keyed by environment scope, and a scope names the environment file it came from, so two projects that both define a `dev` environment never read each other's globals or OAuth tokens even under one `--state-dir`
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
