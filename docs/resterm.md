# Resterm Documentation

## Index
- [Installation](#installation)
- [Quick Start](#quick-start)
- [UI Tour](#ui-tour)
- [Workspaces & Files](#workspaces--files)
- [Variables and Environments](#variables-and-environments)
- [Request File Anatomy](#request-file-anatomy)
- [Compare Runs](#compare-runs)
- [Workflows](#workflows)
- [Streaming (SSE & WebSocket)](#streaming-sse--websocket)
- [GraphQL](#graphql)
- [gRPC](#grpc)
- [Scripting API](#scripting-api)
- [Authentication](#authentication)
- [SSH Tunnels](#ssh-tunnels)
- [HTTP Transport & Settings](#http-transport--settings)
- [Response History & Diffing](#response-history--diffing)
- [CLI Reference](#cli-reference)
- [Configuration](#configuration)
- [Theming](#theming)
- [Examples](#examples)
- [Troubleshooting & Tips](#troubleshooting--tips)

## Installation

### Prebuilt binaries

1. Download the archive for your platform from the [GitHub Releases](https://github.com/unkn0wn-root/resterm/releases) page (macOS, Linux, or Windows; amd64 and arm64 builds are published).
2. Mark the binary as executable (`chmod +x resterm` on Unix), then copy it into a directory on your `PATH`.
3. Launch with `resterm --help` to confirm the CLI is available.

### Build from source

```bash
go install github.com/unkn0wn-root/resterm/cmd/resterm@latest
```

This requires Go 1.24 or newer. The binary will be installed in `$(go env GOPATH)/bin`.

---

## Quick Start

1. Place one or more `.http` or `.rest` files in a working directory (or use the samples under `_examples/`).
2. Run `resterm --workspace path/to/project`.
3. Use the navigator sidebar to expand a file (`→` or `Space`), highlight a request, and press `Ctrl+Enter` to send it (`Enter` runs, `Space` previews).
4. Inspect responses in the Pretty, Raw, Headers, Diff, Compare, or History tabs on the right; press `g+c` to run the current request across the global `--compare` target list (or its inline `@compare` directive) and review the results without leaving the editor.

A minimal `.http` file looks like this:

```http
### Fetch Status
# @name health
GET https://httpbin.org/status/204
User-Agent: resterm
Accept: application/json

### Create Resource
# @name create
POST https://httpbin.org/anything
Content-Type: application/json

{
  "id": "{{$uuid}}",
  "note": "created from Resterm"
}
```

---

## UI Tour

### Layout

- **Sidebar**: unified navigator tree for files, requests, and workflows with a filter bar and tag/method chips. `→`/`Space` expand files, `g+k`/`g+j` expand or collapse the current branch, and `g+Shift+K`/`g+Shift+J` expand or collapse all. A detail well beneath the list shows the selected request/workflow summary. When focused, `g+h` shrinks and `g+l` expands the sidebar.
- **Editor**: middle pane with modal editing (view mode by default, `i` to insert, `Esc` to return to view). Inline syntax highlighting marks metadata, headers, and bodies.
- **Response panes**: right-hand side displays the most recent response, with optional splits for side-by-side comparisons.
- **Header bar**: shows workspace, active environment, current request, and test summaries.
- **Command bar & status**: contextual hints, progress animations, and notifications.

### Core shortcuts

| Action | Shortcut |
| --- | --- |
| Send active request | `Ctrl+Enter` / `Cmd+Enter` / `Alt+Enter` / `Ctrl+J` / `Ctrl+M` |
| Toggle help overlay | `?` |
| Toggle editor insert mode | `i` / `Esc` |
| Cycle focus (navigator -> editor -> response) | `Tab` / `Shift+Tab` |
| Focus navigator / editor / response panes | `g+r` / `g+i` / `g+p` |
| Open timeline tab | `Ctrl+Alt+L` (or `g+t`) |
| Toggle WebSocket console (Stream tab) | `Ctrl+I` |
| Adjust sidebar or editor width | `g+h` / `g+l` (contextual) |
| Collapse / expand current navigator branch | `g+j` / `g+k` |
| Collapse all / expand all in navigator | `g+Shift+J` / `g+Shift+K` |
| Toggle sidebar / editor / response minimize | `g+1` / `g+2` / `g+3` |
| Zoom focused pane / clear zoom | `g+z` / `g+Z` |
| Stack/inline response pane | `g+s` (stack) / `g+v` (inline) |
| Jump to top/bottom of focused response tab | `g+g` / `G` |
| Cycle Raw tab mode (text / hex / base64, summary for large binary) | `g+b` |
| Load full Raw dump (hex) | `g+Shift+D` |
| Save response body / open externally | `g+Shift+S` / `g+Shift+E` |
| Run compare sweep (`@compare` or `--compare` targets) | `g+c` |
| Navigator filter | `/` to focus; type to search files/requests/tags; `Esc` clears filter and chips |
| Navigator: toggle method filter for selected request | `m` (repeat to switch/clear) |
| Navigator: toggle tag filters from selected item | `t` (repeat to toggle) |
| Navigator: jump to selected request in editor | `l` / `r` (when a request is highlighted) |
| Open environment selector | `Ctrl+E` |
| Save file | `Ctrl+S` |
| Save layout (prompt) | `g+Shift+L` |
| Open file picker | `Ctrl+O` |
| New scratch buffer | `Ctrl+T` |
| Reparse current document | `Ctrl+P` (also `Ctrl+Alt+P`) |
| Refresh workspace files | `Ctrl+Shift+O` |
| Split response vertically / horizontally | `Ctrl+V` / `Ctrl+U` |
| Pin or unpin response pane | `Ctrl+Shift+V` |
| Choose target pane for next response | `Ctrl+F` or `Ctrl+B`, then arrow keys or `h` / `l` |
| Show globals summary / clear globals | `Ctrl+G` / `Ctrl+Shift+G` |
| Quit | `Ctrl+Q` (or `Ctrl+D`) |

The editor supports familiar Vim motions (`h`, `j`, `k`, `l`, `w`, `b`, `gg`, `G`, etc.), visual selections with `v` / `V`, yank and delete operations, undo/redo (`u` / `Ctrl+r`), and a search palette (`Shift+F`, toggle regex with `Ctrl+R` and `n` moves cursor forward and `p` backwards).

### Custom bindings

Resterm looks for `${RESTERM_CONFIG_DIR}/bindings.toml` first and `${RESTERM_CONFIG_DIR}/bindings.json` second (default: `~/.config/resterm`). Missing files fall back to the built-in bindings. Example:

```toml
[bindings]
save_file = ["ctrl+s"]
set_main_split_horizontal = ["g s", "ctrl+alt+s"]
send_request = ["ctrl+enter", "cmd+enter"]
```

- Modifiers use `+` (`ctrl+shift+o`), while chord steps are separated by spaces (`"g s"`).
- Bindings can have at most two steps; `send_request` must remain single-step so it can run inside the editor.
- Unknown action IDs or duplicate bindings cause the file to be rejected (Resterm logs the error and keeps defaults).

#### Binding reference

| Action ID | Description | Default bindings |
| --- | --- | --- |
| `cycle_focus_next` | Cycle focus forward (skips editor insert mode). | `tab` |
| `cycle_focus_prev` | Cycle focus backward. | `shift+tab` |
| `open_env_selector` | Open environment picker. | `ctrl+e` |
| `show_globals` | Show global variable summary. | `ctrl+g` |
| `clear_globals` | Clear global variables. | `ctrl+shift+g` |
| `save_file` | Save the current `.http` / `.rest` file. | `ctrl+s` |
| `save_layout` | Prompt to persist current layout (splits, widths) to settings. | `g shift+l` |
| `toggle_response_split_vertical` | Toggle response inline vs vertical split. | `ctrl+v` |
| `toggle_response_split_horizontal` | Toggle response inline vs horizontal split. | `ctrl+u` |
| `toggle_pane_follow_latest` | Toggle follow-latest for the focused response pane. | `ctrl+shift+v` |
| `toggle_help` | Open/close the help overlay. | `?` (aka `shift+/`) |
| `open_path_modal` | Open the “Open File” modal. | `ctrl+o` |
| `reload_workspace` | Rescan the workspace root(s). | `ctrl+shift+o`, `g shift+o` |
| `open_new_file_modal` | Launch the “New Request” modal. | `ctrl+n` |
| `open_theme_selector` | Open theme selector. | `ctrl+alt+t`, `g m`, `g shift+t` |
| `open_temp_document` | Open a scratch document. | `ctrl+t` |
| `reparse_document` | Reparse the active buffer. | `ctrl+p`, `ctrl+alt+p`, `ctrl+shift+t` |
| `reload_file_from_disk` | Reload the active file from disk (discarding unsaved buffer changes). | `g shift+r` |
| `select_timeline_tab` | Focus the Timeline tab. | `ctrl+alt+l`, `g t` |
| `quit_app` | Quit Resterm. | `ctrl+q`, `ctrl+d` |
| `send_request` | Send the active request (single-step only). | `ctrl+enter`, `cmd+enter`, `alt+enter`, `ctrl+j`, `ctrl+m` |
| `cancel_run` | Cancel the in-flight request, compare, profile, or workflow run. | `ctrl+c` |
| `copy_response_tab` | Copy the focused Pretty/Raw/Headers response tab to the clipboard. | `ctrl+shift+c`, `g y` |
| `toggle_header_preview` | Toggle request vs response headers in the Headers tab. | `g shift+h` |

| Action ID | Description | Default bindings | Repeatable |
| --- | --- | --- | --- |
| `sidebar_width_decrease` / `sidebar_width_increase` | Shrink/grow sidebar width (editor split elsewhere). | `g h`, `g l` | ✓ |
| `sidebar_height_decrease` / `sidebar_height_increase` | Collapse / expand the selected navigator branch. | `g j`, `g k` | ✓ |
| `workflow_height_increase` / `workflow_height_decrease` | Collapse all / expand all navigator branches. | `g shift+j`, `g shift+k` | ✓ |
| `focus_requests` / `focus_response` / `focus_editor_normal` | Jump directly to a pane. | `g r`, `g p`, `g i` | ✗ |
| `set_main_split_horizontal` / `set_main_split_vertical` | Stack vs side-by-side editor/response. | `g s`, `g v` | ✗ |
| `start_compare_run` | Trigger compare sweep for the current request. | `g c` | ✗ |
| `toggle_ws_console` | Toggle the WebSocket console. | `g w` | ✗ |
| `toggle_sidebar_collapse` / `toggle_editor_collapse` / `toggle_response_collapse` | Collapse/expand panes. | `g 1`, `g 2`, `g 3` | ✗ |
| `toggle_zoom` / `clear_zoom` | Zoom current region / clear zoom. | `g z`, `g shift+z` | ✗ |

`send_request` participates in the editor’s “send on Ctrl+Enter” logic, so keep it single-step. All other actions can be remapped to any combination within the constraints above.

### Response panes

- **Pretty**: formatted JSON (or best-effort formatting for other types).
- **Raw**: exact payload text.
- **Stream**: live transcript viewer for WebSocket and SSE sessions with bookmarking and console integration.
- **Headers**: response headers by default; press `g+Shift+H` to toggle into the sent request headers view (cookies included) and back.
- **Stats**: latency summaries and histograms from `@profile` runs plus step-by-step workflow breakdowns. Press `Shift+J` / `Shift+K` while that view is focused to hop between steps, and Resterm only realigns the viewport if the next step was off screen.
- **Timeline**: per-phase HTTP timings with budget overlays; available whenever tracing is enabled.
- **Diff**: compare the focused pane against the other response pane.
- **History**: chronological responses for the selected request (live updates). Open a full JSON preview with `p` or delete the focused entry with `d`.

When a request opens a stream, the Stream tab becomes available. Use `Ctrl+I` to reveal the WebSocket console inside the Stream tab, `F2` to switch payload modes (text, JSON, base64, file), `Ctrl+S` or `Ctrl+Enter` to send frames, arrow keys to replay recent payloads, `Ctrl+P` to send ping, and `Ctrl+W` to close the session.

Use `Ctrl+V` or `Ctrl+U` to split the response pane. The secondary pane can be pinned so subsequent calls populate only the primary pane, making comparisons easy.

While the response pane is focused, `Ctrl+Shift+C` (or `g y`) copies the entire Pretty, Raw, or Headers tab directly to your clipboard, matching the rendered text (no mouse selection required).

Use `g+g` and `G` to jump to the start or end of the Pretty, Raw, or Headers tabs when the response pane is focused. The same keys jump to the first or last entry in the navigator when you are browsing files or workflows.

Binary responses show size and type hints alongside quick previews. For large binary payloads, the Raw tab starts in a summary view and defers full dumps until requested. While the response pane is focused, press `g+b` to rotate the Raw tab between summary, hex, and base64 views. Press `g+Shift+D` to load the full hex dump immediately. Press `g+Shift+S` to open the Save Response Body prompt, which comes prefilled with a suggested path from your last save or workspace and writes the file after you hit Enter. `g+Shift+E` writes the body to a temporary file and opens it with your default app.

### Pane minimization & zoom

- Toggle the sidebar, editor, or response panes with `g+1`, `g+2`, and `g+3`. Minimized panes collapse into thin frames that display an indicator along with a reminder of the restoring shortcut.
- Status bar badges (`Sidebar:min`, `Editor:min`, `Response:min`) mirror the current state so you can tell when something is hidden even if the stub scrolls out of view.
- Use `g+z` to zoom the currently focused pane and hide the others temporarily; `g+Z` clears zoom and restores the previous layout (including any manual minimize state).
- Resize chords such as `g+h` / `g+l` are disabled while a related pane is hidden or zoomed, preventing accidental layout resets.

### Timeline & tracing

- Add `# @trace` directives to enable HTTP tracing on a request. Budgets use `phase<=duration` notation (`dns<=50ms`, `total<=300ms`, etc.) with an optional `tolerance=` applied to every phase. Supported phases map to `nettrace`: `dns`, `connect`, `tls`, `request_headers`, `request_body`, `ttfb`, `transfer`, and `total`.
- When a traced response arrives, Resterm evaluates budgets, raises status bar warnings for breaches, and unlocks the Timeline tab. Use `Ctrl+Alt+L` or the `g+t` chord to jump straight to it from anywhere.
- The Timeline view renders proportional bars, annotates overruns, and lists budget breaches. Metadata such as cached DNS results or reused sockets appears beneath each phase.
- Scripts can inspect traces through the `trace` binding (`trace.enabled()`, `trace.phases()`, `trace.breaches()`, `trace.withinBudget()`, etc.), allowing automated validations inside Goja test blocks.
- See `_examples/trace.http` for a runnable pair of requests (one within budget, one deliberately breaching) that demonstrate the timeline output and status messaging.
- Configure optional OpenTelemetry export with `RESTERM_TRACE_OTEL_ENDPOINT` (or `--trace-otel-endpoint`). Additional switches: `RESTERM_TRACE_OTEL_INSECURE` / `--trace-otel-insecure`, `RESTERM_TRACE_OTEL_SERVICE` / `--trace-otel-service`, `RESTERM_TRACE_OTEL_TIMEOUT`, and `RESTERM_TRACE_OTEL_HEADERS`. Spans are emitted only while tracing is enabled; HTTP failures and budget breaches mark the span status as `Error`.

### History and globals

- The history pane persists responses along with their request and environment metadata. Entries survive restarts (stored under the config directory; see [Configuration](#configuration)).
- `Ctrl+G` shows current globals (request/file/runtime) with secrets masked. `Ctrl+Shift+G` clears them for the active environment.
- `Ctrl+E` opens the environment picker to switch between `resterm.env.json` (or `rest-client.env.json`) entries.

---

## Workspaces & Files

- Resterm scans the workspace root for `.http` and `.rest` files. Use `--workspace` to set the root or rely on the directory of the file passed via `--file`. Add `--recursive` to traverse subdirectories (hidden directories are skipped).
- The navigator filter sits above the tree: press `/` to focus, type to match files, request/workflow names, URLs, tags, and badges. `m` toggles method badges (single select) for the highlighted request, `t` toggles tag badges, and `Esc` clears text plus any badges.
- The navigator refreshes immediately when a file is saved or reparsed; filtering auto-loads unopened files so cross-workspace matches still appear. Use `Ctrl+Shift+O` (or `g+Shift+O`) to rescan the workspace for new files.
- Resterm watches the active file on disk. If another tool edits or deletes it, a modal appears telling you the file changed or went missing. Your in-memory buffer stays intact. Press the reload shortcut (`g+Shift+R` by default, or whatever you’ve mapped to `reload_file_from_disk`) to pull the on disk version into the editor. If you have unsaved changes, the first press warns that reload will discard them; press reload again to confirm. Dismiss with `Esc` to keep your buffer and continue editing.
- Create a scratch buffer with `Ctrl+T` for ad-hoc experiments. These buffers are not written to disk unless you save them explicitly.

### Inline requests

You can execute simple requests without a `.http` file:

1. Type `GET https://api.example.com/users` (or just the URL) in the editor.
2. Place the cursor on the line and press `Ctrl+Enter`.

Inline requests support full URLs and a limited curl import:

```bash
curl \
  -X POST https://api.example.com/login \
  -H "Content-Type: application/json" \
  -d '{"user":"demo","password":"pass"}'
```

Resterm recognizes common curl flags (`-X`, `--request`, `-H`, `--header`, `-d/--data*`, `--json`, `--url`, `-u/--user`, `--head`, `--compressed`, `-F/--form`) and converts them into a structured request. Multiline commands joined with backslashes are supported.

---

## Variables and Environments

### Environment files

Resterm automatically searches, in order:

1. The directory of the opened file.
2. The workspace root.
3. The current working directory.

It loads the first `resterm.env.json` or `rest-client.env.json` it finds. The JSON can contain nested objects and arrays—they are flattened using dot and bracket notation (`services.api.base`, `plans.addons[0]`).

Example environment (`_examples/resterm.env.json`):

```json
{
  "dev": {
    "settings.http-root-cas": "dev-ca.pem",
    "settings.grpc-insecure": "false",
    "services": {
      "api": {
        "base": "https://httpbin.org/anything/api"
      }
    },
    "auth": {
      "token": "dev-token-123"
    }
  }
}
```

Switch environments with `Ctrl+E`. If multiple environments exist, Resterm defaults to `dev`, `default`, or `local` when available.

#### Dotenv files via `--env-file`

Prefer JSON for multi-environment bundles, but you can point Resterm at a dotenv file when you only need a single workspace:

- Pass `--env-file path/to/.env` (or `.env.prod`, `prod.env`, etc.). Dotenv files are **never** auto-discovered—explicit opt-in avoids surprising overrides.
- Supported syntax matches common `.env` loaders: optional `export` prefixes, `KEY=value` pairs, `#`/`;` comments, single- and double-quoted values (with escapes), and `${VAR}` or `$VAR` interpolation. We expand references using earlier keys from the same file and the current OS environment.
- The environment name is derived from a `workspace` entry (case-insensitive). If that key is missing or blank we fall back to the file name (`.env.prod` → `prod`, `prod.env` → `prod`, bare `.env` → `default`).
- Each dotenv file yields exactly one environment today. If you need multiple environments, stick with `resterm.env.json`.
- Limitations: no multi-workspace support, no auto-discovery, and interpolation only sees keys declared above the current line (plus OS envs).

### Variable resolution order

When expanding `{{variable}}` templates, Resterm looks in:

1. *File constants* (`@const`).
2. Values set by scripts for the current execution (`vars.set` in pre-request or test scripts).
3. *Request-scope* variables (`@var request`, `@capture request`).
4. *Runtime globals* stored via captures or scripts (per environment).
5. *Document globals* (`@global`, `@var global`).
6. *File scope* declarations and `@capture file` values.
7. Selected environment JSON.
8. OS environment variables (case-sensitive with an uppercase fallback).

Dynamic helpers are also available: `{{$uuid}}` (alias `{{$guid}}`), `{{$timestamp}}` (Unix), `{{$timestampISO8601}}`, and `{{$randomInt}}`.

---

## SSH Tunnels

Use `@ssh` to route HTTP/gRPC/WebSocket/SSE traffic through an SSH bastion.

**Syntax:** `# @ssh [scope] [name] key=value ...`

- **TL;DR**:
  - Define a reusable profile: `# @ssh global bastion host=jump.example.com user=ops key=~/.ssh/id_ed25519 persist`
  - Use it in a request: `# @ssh use=bastion`
  - Inline one-off: `# @ssh host=10.0.0.5 user=svc password=env:SSH_PW`

- `scope`: `global`, `file`, or `request` (default request). Global/file scopes define reusable profiles. Requests either reference a profile with `use=` or define inline options.
- `name`: profile tag (default `default`).
- Fields: `host` (required), `port` (default 22), `user`, `password`, `key`, `passphrase`, `agent` (default true when `SSH_AUTH_SOCK` is present), `known_hosts` (default `~/.ssh/known_hosts`), `strict_hostkey` (default true), `persist` (only honored for global/file), `timeout`, `keepalive`, `retries`, `use` (profile selection).
- Values expand templates and support `env:VAR` to prefer terminal env vars before other scopes. Paths for `key` and `known_hosts` expand `~` and environment variables.
- Key is optional: resterm will use your SSH agent (if present) or fall back to default keys (`~/.ssh/id_ed25519`, `id_rsa`, `id_ecdsa`); see "Default key detection" below.
- Global profiles are shared across the workspace; file-scoped profiles override globals when names collide. `use=` resolves file profiles first, then globals.
- Request-level `persist` is ignored to avoid leaking tunnels. Strict host key checking defaults to true; `strict_hostkey=false` is allowed but insecure.

Scopes:

- **Global** (workspace-wide): `# @ssh global bastion host=jump.example.com user=ops key=~/.ssh/id_ed25519 persist`
- **File** (only this `.http`): `# @ssh file edge host=10.0.0.5 user=ops`
- **Request inline** (scope keyword optional because request is default): `# @ssh host=192.168.1.50 user=svc password=env:SSH_PW timeout=12s`
- **Reference** a profile: `# @ssh use=bastion` (picks file-scoped profile first, then global)

Examples:

```http
# @ssh global edge host=env:SSH_BASTION user=ops key=~/.ssh/id_ed25519 persist timeout=30s keepalive=20s
# @global api_host http://10.0.0.10

### List over jump
# @ssh use=edge host={{api_host}} strict_hostkey=false
GET http://{{api_host}}/v1/things
```

Inline request-only:

```http
### Local jump
# @ssh request host=192.168.1.50 user=svc password=env:SSH_PW timeout=12s
POST http://internal.service/api
```

gRPC over SSH:

```http
# @ssh use=edge
# @grpc testservices.inventory.ProjectService/Seed
# @grpc-descriptor ./proto/inventory.protoset
GRPC passthrough:///grpc-internal:8082

{}
```

### How it works

SSH tunneling operates at the transport layer, making it transparent to all other features (`@trace`, `@profile`, `@workflow`, `@sse`, `@websocket`, `@graphql`, `@grpc`, etc.).

```
Your machine                    Bastion (SSH)                  Private VPC
    │                                  │                            │
    │  1. SSH connect                  │                            │
    ├─────────────────────────────────►│                            │
    │                                  │                            │
    │  2. "Dial 10.0.0.100:80"         │  3. TCP connect            │
    │     (through SSH channel)        ├───────────────────────────►│
    │                                  │                            │
    │  4. HTTP request flows through the tunnel                     │
    │◄─────────────────────────────────────────────────────────────►│
```

### Comparison with terminal tunnels

What you'd do manually:

```bash
# Create tunnel in terminal
ssh -L 8080:10.0.0.100:80 ops@bastion.example.com
curl http://localhost:8080/api/users  # in another terminal
```

What resterm does transparently:

```http
# @ssh global tunnel host=bastion.example.com user=ops key=~/.ssh/id_ed25519 persist

### Hit pod directly through tunnel
# @ssh use=tunnel
GET http://10.0.0.100/api/users
```

**Key difference:**

- **Terminal tunnel:** bind a local port, then hit `localhost:port`.
- **Resterm:** hit the **internal IP directly** (`10.0.0.100`); Resterm dials through the SSH tunnel to reach it.

This makes accessing Kubernetes pods, private VPC resources, or any internal service through a bastion host seamless. The `persist` option keeps the SSH connection alive so subsequent requests reuse it without reconnection overhead.

### Default key detection

When no `key` is specified, resterm automatically tries these paths in order:

1. `~/.ssh/id_ed25519`
2. `~/.ssh/id_rsa`
3. `~/.ssh/id_ecdsa`

If `SSH_AUTH_SOCK` is set, the SSH agent is also used by default.

---

## Request File Anatomy

### Separators and comments

- Begin each request with a line that starts with `###`. Everything up to the next separator belongs to the same request.
- Lines prefixed with `#`, `//`, or `--` are treated as comments. Metadata directives live inside these comment blocks.

### Metadata directives

| Directive | Syntax | Description |
| --- | --- | --- |
| `@name` | `# @name identifier` | Friendly name used in the navigator, history, and captures. |
| `@const` | `# @const name value` | Compile-time constant resolved when the file is loaded; immutable and visible to all requests in the document. |
| `@description` / `@desc` | `# @description ...` | Multi-line description (lines concatenate with newline). |
| `@tag` / `@tags` | `# @tag smoke billing` | Tags for grouping and filters (comma- or space-separated). |
| `@trace` | `# @trace dns<=40ms total<=200ms tolerance=25ms` | Enable per-phase tracing and optional latency budgets. |
| `@no-log` | `# @no-log` | Prevents the response body snippet from being stored in history. |
| `@log-sensitive-headers` | `# @log-sensitive-headers [true|false]` | Allow allowlisted sensitive headers (Authorization, Proxy-Authorization, API-token headers such as `X-API-Key`, `X-Access-Token`, `X-Auth-Key`, etc.) to appear in history; omit or set to `false` to keep them masked (default). |
| `@setting` | `# @setting key value` | Generic settings (transport/TLS today: `timeout`, `proxy`, `followredirects`, `insecure`, `http-*`, `grpc-*`). |
| `@settings` | `# @settings key1=val1 key2=val2 ...` | Batch settings on one line; supports the same keys as `@setting` and future prefixes. |
| `@timeout` | `# @timeout 5s` | Equivalent to `@setting timeout 5s`. |

### RestermScript (RST)

RestermScript (RST) powers templates (`{{= ... }}`) and directive expressions. It is separate from Goja `@script` blocks and is the default for directive logic. Full reference: `docs/restermscript.md`.

#### Request-scoped RST directives

| Directive | Syntax | Description |
| --- | --- | --- |
| `@use` | `# @use ./rts/helpers.rts as helpers` | Import an RST module (valid at file or request scope). |
| `@apply` | `# @apply {headers: {"X-Test": "1"}}` | Apply a patch to method/url/headers/query/body/vars before pre-request scripts. |
| `@when` | `# @when vars.has("token")` | Run the request only when the expression is truthy. |
| `@skip-if` | `# @skip-if env.mode == "dry-run"` | Skip the request when the expression is truthy. |
| `@assert` | `# @assert response.statusCode == 200` | Evaluate an assertion after the response arrives. |
| `@for-each` | `# @for-each json.file("users.json") as user` | Repeat the request for each item in a list. |
| `@script pre-request lang=rts` | `# @script pre-request lang=rts` | Run a pre-request RST block with request/vars mutation helpers. |

#### Workflow-only RST directives

| Directive | Syntax | Description |
| --- | --- | --- |
| `@if` / `@elif` / `@else` | `# @if last.statusCode == 200 run=StepOK` | Branch workflow steps based on expressions. |
| `@switch` / `@case` / `@default` | `# @switch last.statusCode` | Choose a workflow branch based on a switch expression. |
| `@for-each` | `# @for-each json.file("users.json") as user` | Repeat a workflow step for each item in a list. |

Notes:

- `@when` / `@skip-if` gate requests; `@if` / `@switch` branch workflows.
- `@for-each` is available in both contexts; it repeats a request or a workflow step depending on scope.

### Transport settings example

```http
### Fast timeout
# @name TimeoutDemo
# @timeout 2s
GET https://httpbin.org/delay/5
```

---

## Compare Runs

Run the same request across multiple environments either inline or from the CLI:

- Add `# @compare dev stage prod base=stage` to a request block to pin the order/baseline inside the file. Provide at least two environments; `base` is optional and defaults to the first entry.
- Supply global defaults with `resterm --compare dev,stage,prod --compare-base stage`, then press `g+c` anywhere in the editor to reuse those targets even if the request lacks `@compare`.
- While a compare run is active Resterm automatically enables a split layout, pins the previous response in the secondary pane, and streams progress in the status bar (`Compare dev✓ stage… prod?`). The new Compare tab renders a table with status/code/duration/diff summaries per environment.
- Each compare sweep writes a bundled history entry (`COMPARE` method) so you can replay the failing environment later; selecting a compare history row loads the run back into the editor, restores the Compare tab, and lets you resend or inspect deltas off-line.
- Navigate the Compare tab with ↑/↓ (or PgUp/PgDn/Home/End) to highlight any environment, then press `Enter` to load that environment’s snapshot into the primary pane while the configured baseline stays pinned in the secondary pane. The Diff tab (and Pretty/Raw/Headers) now reflect “selected ↔ baseline,” so choosing the baseline row yields an “identical” diff, while choosing another environment shows how it diverges from the baseline. To compare against a different reference, rerun with a new `base=` value or load the desired pair from History.

Use `@compare` alongside the usual metadata, e.g. to couple request-scoped variables per environment:

```http
### Smoke workflow
# @name smoke
# @compare dev stage prod base=prod
POST {{services.api.base}}/status
Accept: application/json

{
  "env": "{{services.api.name}}"
}
```

### Variable declarations

`@const`, `@var`, and `@global` provide static values evaluated before the request is sent. Constants resolve immediately when the file is parsed and cannot be overridden by captures or scripts; variables follow the usual resolution order and may be updated at runtime.

| Scope | Syntax | Visibility |
| --- | --- | --- |
| Constant | `# @const api.root https://api.example.com` | Immutable for the lifetime of the document; available to every request in the file. |
| Global | `# @global api.token value` / `# @global-secret api.token value` / `# @var global api.token value` | Visible to every request and every file (per environment). |
| File | `# @file upload.root https://storage.example.com` / `# @file-secret upload.root ...` / `# @var file upload.root ...` | Visible to all requests in the same document only. |
| Request | `# @request trace.id {{$uuid}}` / `# @request-secret trace.id ...` / `# @var request trace.id ...` | Visible only to the current request (useful for tests). |

You can also use shorthand assignments outside comment blocks: `@requestId = {{$uuid}}`. Shorthand defaults to request scope while you're inside a request block and to file scope elsewhere; add a prefix to override (`@global api.token abc`, `@request trace.id {{$uuid}}`, or `@file base.url https://example.com`).

Append `-secret` (`global-secret`, `file-secret`, `request-secret`) to mask stored values in summaries; this works for both comment directives and shorthand lines (`@global-secret token xyz`, `@file-secret base.url ...`, `@request-secret trace.id ...`).

### Captures

`@capture <scope> <name> <expression>` evaluates after the response arrives and stores the result for reuse.

Expressions can reference:

- `{{response.status}}`, `{{response.statuscode}}`
- `{{response.body}}`
- `{{response.headers.<Header-Name>}}`
- `{{response.json.path}}` (dot/bracket navigation into JSON)
- `{{stream.kind}}`, `{{stream.summary.sentCount}}`, `{{stream.events[0].text}}` for streaming transcripts (available when the request used `@sse` or `@websocket`)
- Any template variables resolvable by the current stack

Example:

```http
### Seed session
# @name AnalyticsSeedSession
# @capture global-secret analytics.sessionToken {{response.json.json.sessionToken}}
# @capture file analytics.lastJobId {{response.json.json.jobId}}
# @capture request analytics.trace {{response.json.headers.X-Amzn-Trace-Id}}
POST https://httpbin.org/anything/analytics/sessions
```

### Body content

- **Inline**: everything after the blank line separating headers and body.
- **External file**: `< ./payloads/create-user.json` loads the file relative to the request file. To also search the workspace root / current working directory, set `RESTERM_ENABLE_FALLBACK=1` (opt-in).
- **Inline includes**: lines in the body starting with `@ path/to/file` are replaced with the file contents (useful for multi-part templates).
- **GraphQL**: handled separately (see [GraphQL](#graphql)).

### Profiling requests

Add `# @profile` to any request to run it repeatedly and collect latency statistics without leaving the terminal. Profile runs are recorded in history with aggregated results; hit `p` on the entry to inspect the stored JSON.

```
### Benchmark health check
# @profile count=50 warmup=5 delay=100ms
GET https://httpbin.org/status/200
```

Flags:

- `count` - number of measured runs (defaults to 10).
- `warmup` - optional warmup runs that are executed but excluded from stats.
- `delay` - optional delay between runs (e.g. `250ms`).

When profiling completes the response pane's **Stats** tab shows percentiles, histograms, success/failure counts, and any errors that occurred.

## Workflows

Group existing requests into repeatable workflows using `@workflow` blocks. Each step references a request by name and can override variables or expectations.

```
### Provision account
# @workflow provision-account on-failure=continue
# @step Authenticate using=AuthLogin expect.statuscode=200
# @step CreateProfile using=CreateUser vars.request.name={{vars.workflow.userName}}
# @step FetchProfile using=GetUser

### AuthLogin
POST https://example.com/auth

### CreateUser
POST https://example.com/users

### GetUser
GET https://example.com/users/{{vars.workflow.userId}}
```

Workflows parsed from the current document appear in the **Workflows** list on the left. Select one and press `Enter` (or `Space`) to run it. Resterm executes each step in order, respects `on-failure=continue`, and streams progress in the status bar. When the run completes the **Stats** tab shows a workflow summary (including started/ended timestamps), and a consolidated entry is written to history so you can review results later. While you read through that summary, tap `Shift+J` / `Shift+K` to move between workflow entries.

Key directives and tokens:

- `@workflow <name>` starts a workflow. Add `on-failure=<stop|continue>` to change the default behaviour and attach other tokens (e.g. `region=us-east-1`) which are surfaced under `Workflow.Options` for tooling.
- `@description` / `@tag` lines inside the workflow build the description and tag list shown in the UI and stored in history.
- `@step <optional-alias>` defines an execution step. Supply `using=<RequestName>` (required), `on-failure=<...>` for per-step overrides, `expect.status` / `expect.statuscode`, and any number of `vars.*` assignments.
- `vars.request.*` keys add step-scoped values that are available as `{{vars.request.<name>}}` during that request. They do not rewrite existing `@var` declarations automatically, so reference the namespaced token (or copy it in a pre-request script) when you want the override.
- `vars.workflow.*` keys persist between steps and are available anywhere in the workflow as `{{vars.workflow.<name>}}`, letting later requests reuse or mutate shared context (e.g. `vars.workflow.userId`).
- Unknown tokens on `@workflow` or `@step` are preserved in `Options`, allowing custom scripts or future features to consume them without changing the file format.
- `expect.status` supports quoted or escaped values, so you can write `expect.status="201 Created"` alongside `expect.statuscode=201`.

> **Tip:** Workflow assignments are expanded once when the request executes. If you need helpers such as `{{$uuid}}`, place them directly in the request/template or compute them via a pre-request script before assigning the value.
> **Tip:** Options are parsed like CLI flags; wrap values in quotes or escape spaces (`\ `) to keep text together (e.g. `expect.status="201 Created"`).

Every workflow run is persisted alongside regular requests in History; the newest entry is highlighted automatically so you can open the generated `@workflow` definition and results from the History pane immediately after the run.

## Streaming (SSE & WebSocket)

Streaming sessions surface in the Stream response tab, are captured in history, and can be consumed by captures and scripts.

### Server-Sent Events (`@sse`)

Add `# @sse` to keep an HTTP request open for events:

```http
### Notifications
# @name notifications
# @sse duration=2m idle=15s max-events=250
GET https://api.example.com/notifications
```

`@sse` accepts the following options:

| Token | Description |
| --- | --- |
| `duration` / `timeout` | Maximum lifetime of the stream. Resterm cancels the request once the timer elapses. |
| `idle` / `idle-timeout` | Maximum quiet period between events before the session is closed. |
| `max-events` | Stop reading after N events have been delivered. |
| `max-bytes` / `limit-bytes` | Cap the total payload size and close once the limit is exceeded. |

If the server responds with a non-2xx status or a non-`text/event-stream` content type, Resterm falls back to a standard HTTP response so you can inspect the error. Successful streams produce a transcript (events plus metadata) that appears in the Stream tab and is saved in history. The summary exposed to templates and scripts includes `eventCount`, `byteCount`, `duration`, and `reason` (for example `eof`, `timeout`, `idle-timeout`).

### WebSockets (`@websocket`, `@ws`)

Use `# @websocket` to negotiate an upgrade, then describe scripted interactions with `# @ws` lines:

```http
### Chat session
# @name chatSession
# @websocket timeout=10s idle-timeout=4s subprotocols=chat.v2,json compression=true
# @ws send {"type":"hello"}
# @ws wait 1s
# @ws send-json {"type":"message","text":"Hello from Resterm"}
# @ws ping heartbeat
# @ws close 1000 "client done"
GET wss://chat.example.com/room
```

Available WebSocket options:

| Token | Description |
| --- | --- |
| `timeout` | Handshake deadline (applies until the connection upgrades). |
| `idle-timeout` | Idle timeout once the socket is open. Resets on any send or receive activity (0 leaves it unbounded). |
| `max-message-bytes` | Upper bound on inbound frame sizes. |
| `subprotocols` | Comma-separated list advertised during the handshake. |
| `compression=<true|false>` | Explicitly enable or disable per-message compression. |

Supported `@ws` steps:

| Step | Effect |
| --- | --- |
| `@ws send <text>` | Send a UTF-8 text frame. Templates expand before sending. |
| `@ws send-json <object>` | Encode JSON and send it as text. |
| `@ws send-base64 <data>` | Decode base64 and send the result as binary. |
| `@ws send-file <path>` | Send a file from disk (relative to the request file unless absolute). |
| `@ws ping [payload]` / `@ws pong [payload]` | Emit control frames (payload limited to 125 bytes). |
| `@ws wait <duration>` | Pause for the specified duration (e.g. `500ms`). |
| `@ws close [code] [reason]` | Close the connection with an optional status code (defaults to `1000`). |

Handshake failures surface the HTTP response so upgrade issues are easy to debug. Successful sessions stream events into the UI and history with metadata for direction, opcode, sizes, and close status. The summary exposed to templates and scripts includes `sentCount`, `receivedCount`, `duration`, `closedBy`, `closeCode`, and `closeReason`.

> **Heads-up:** When you keep a WebSocket URL in `@const`, `@global`, or `@var`, write the request line as `GET {{ws.url}}` (or whichever variable you use). The parser needs the explicit method to recognise the line as a WebSocket request before template expansion. Literal `ws://` / `wss://` URLs without a method still work when written directly.

### Stream tab, history, and console

- The Stream tab appears automatically whenever a streaming session is active. Scroll to review frames, press `b` to bookmark important events, and switch tabs with the arrow keys (`Ctrl+H` / `Ctrl+L`).
- Toggle the interactive WebSocket console with `Ctrl+I` while the Stream tab is focused. Cycle payload modes with `F2` (text → JSON → base64 → file), send payloads with `Ctrl+S` or `Ctrl+Enter`, reuse previous payloads with the arrow keys, issue ping frames via `Ctrl+P`, close gracefully with `Ctrl+W`, and clear the live buffer with `Ctrl+L`.
- Completed transcripts are saved alongside the request in history with summary headers (`X-Resterm-Stream-Type`, `X-Resterm-Stream-Summary`). Scripts and captures can access the same data via `stream.*` templates and APIs (see [Scripting](#scripting-api)).

### Authentication directives

| Type | Syntax | Notes |
| --- | --- | --- |
| Basic | `# @auth basic user pass` | Injects `Authorization: Basic …`. Templates expand inside parameters. |
| Bearer | `# @auth bearer {{token}}` | Injects `Authorization: Bearer …`. |
| API key | `# @auth apikey header X-API-Key {{key}}` | `placement` can be `header` or `query`. Defaults to `X-API-Key` header if name omitted. |
| Custom header | `# @auth Authorization CustomValue` | Arbitrary header/value pair. |
| OAuth 2.0 | `# @auth oauth2 token_url=... client_id=...` | Built-in token acquisition and caching (client_credentials/password/authorization_code + PKCE). |

#### OAuth 2.0 parameters

| Parameter | Required | Default | Description |
| --- | --- | --- | --- |
| `token_url` | Yes | - | Token endpoint URL. Must be provided at least once per `cache_key`. |
| `auth_url` | For auth code | — | Authorization endpoint. Required when `grant=authorization_code`. |
| `client_id` | Yes | - | Your application's client ID. |
| `client_secret` | No | - | Client secret (omit for public clients using PKCE). |
| `grant` | No | `client_credentials` | Grant type: `client_credentials`, `password`, or `authorization_code`. |
| `scope` | No | - | Space-separated scopes to request. |
| `audience` | No | - | Target API audience (Auth0, etc.). |
| `resource` | No | - | Resource indicator (Azure AD, etc.). |
| `client_auth` | No | `basic` | How to send credentials: `basic` (Authorization header) or `body` (form fields). Falls back to `body` automatically for public clients. |
| `header` | No | `Authorization` | Which header receives the token. Use this when an API expects tokens in a custom header like `X-Access-Token`. |
| `username` | For password | - | Resource owner username (only for `grant=password`). |
| `password` | For password | - | Resource owner password (only for `grant=password`). |
| `cache_key` | No | auto | Override the cache identity. Useful when multiple requests should share the same token even if their parameters differ slightly. When omitted, Resterm derives the key from token URL, client ID, scope, and other fields. |
| `redirect_uri` | No | auto | Callback URL for authorization code flow. See details below. |
| `code_verifier` | No | auto | PKCE verifier (43-128 characters per RFC 7636). Auto-generated when omitted. |
| `code_challenge_method` | No | `s256` | PKCE method: `s256` (recommended) or `plain`. |
| `state` | No | auto | CSRF protection token. Auto-generated when omitted. |

Any additional `key=value` pairs are forwarded as extra form parameters to both the authorization and token endpoints.

#### How token caching works

Resterm caches tokens per environment and `cache_key`. When a request needs a token:

1. If a valid cached token exists (not expired, with 30-second safety margin), it's reused immediately.
2. If the cached token has a `refresh_token` and is expired, Resterm attempts a refresh.
3. If refresh fails or no token exists, a fresh token is fetched from the token endpoint.

This means you can define full OAuth parameters once, then reference just `cache_key` in subsequent requests:

```http
### First request - seeds the cache
# @auth oauth2 token_url={{oauth.tokenUrl}} client_id={{oauth.clientId}} client_secret={{oauth.clientSecret}} scope="read write" cache_key=myapi
GET {{base.url}}/users

### Later request - reuses cached token
# @auth oauth2 cache_key=myapi
GET {{base.url}}/projects
```

If you skip `token_url` on a follow-up directive and the cache hasn’t been seeded yet, Resterm will error with `@auth oauth2 requires token_url (include it once per cache_key to seed the cache)`.

### Scripting (`@script`)

Add `# @script pre-request` or `# @script test` followed by lines that start with `>`.

```http
# @script pre-request
> var token = vars.global.get("reporting.token") || `script-${Date.now()}`;
> vars.global.set("reporting.token", token, {secret: true});
> request.setHeader("Authorization", `Bearer ${token}`);
> request.setBody(JSON.stringify({ scope: "reports" }, null, 2));
```

Reference external scripts with `> < ./scripts/pre.js`.

See [Scripting API](#scripting-api) for available helpers.

---

## GraphQL

Enable GraphQL handling with `# @graphql` (requests start with it disabled). Resterm packages GraphQL requests according to HTTP method:

- **POST**: body becomes `{ "query": ..., "variables": ..., "operationName": ... }`.
- **GET**: query parameters `query`, `variables`, `operationName` are attached.
- Template variables in the URL are expanded before the GET parameters are attached, so `GET {{graphql.endpoint}}` works even when the host is templated.

Available directives:

| Directive | Description |
| --- | --- |
| `@graphql [true|false]` | Enable/disable GraphQL processing for the request. |
| `@operation` / `@graphql-operation` | Sets the `operationName`. |
| `@variables` | Starts a variables block; inline JSON or `< file.json`. |
| `@query` | Loads the query from a file instead of the inline body. |

Example:

```http
### Inline GraphQL Query
# @graphql
# @operation FetchWorkspace
POST {{graphql.endpoint}}

query FetchWorkspace($id: ID!) {
  workspace(id: $id) {
    id
    name
  }
}

# @variables
{
  "id": "{{graphql.workspaceId}}"
}
```

---

## gRPC

gRPC requests start with a line such as `GRPC host:port`. Metadata directives describe the method and transport options.

| Directive | Description |
| --- | --- |
| `@grpc package.Service/Method` | Fully qualified method to call. |
| `@grpc-descriptor path/to/file.protoset` | Use a compiled descriptor set instead of server reflection. |
| `@grpc-reflection [true|false]` | Toggle server reflection (default `true`). |
| `@grpc-plaintext [true|false]` | Force plaintext or TLS. |
| `@grpc-authority value` | Override the HTTP/2 `:authority` header. |
| `@grpc-metadata key: value` | Add metadata pairs (repeatable). |
| `@setting grpc-root-cas path1,path2` | Extra root CAs (space/comma/semicolon separated). Paths resolve relative to the request file. |
| `@setting grpc-root-mode append|replace` | Control whether extra CAs append to system roots (`append`) or replace them (`replace`, default). |
| `@setting grpc-client-cert path` / `@setting grpc-client-key path` | Client cert/key for mTLS (relative paths allowed). |
| `@setting grpc-insecure true` | Skip TLS verification (off by default). |

Supplying any gRPC TLS setting (roots, client cert/key, insecure) automatically enables TLS unless you explicitly force plaintext with `@grpc-plaintext true`.

The request body contains protobuf JSON. Use `< payload.json` to load from disk. Responses display message JSON, headers, and trailers; history stores method, status, and timing alongside HTTP calls.

Example:

```http
### Generate Report Over gRPC
# @grpc analytics.ReportingService/GenerateReport
# @grpc-reflection true
# @grpc-plaintext true
# @grpc-authority analytics.dev.local
# @grpc-metadata x-trace-id: {{$uuid}}
# @setting grpc-root-cas ./ca.pem
GRPC {{grpc.host}}

{
  "tenantId": "{{tenant.id}}",
  "reportId": "rep-{{$uuid}}"
}
```

---

## Scripting API

Scripts run in an ES5.1-compatible Goja VM.

### Pre-request scripts (`@script pre-request`)

Objects:

- `request`
  - `getURL()`, `setURL(url)`
  - `getMethod()`, `setMethod(method)`
  - `getHeader(name)`, `setHeader(name, value)`, `addHeader(name, value)`, `removeHeader(name)`
  - `setBody(text)`
  - `setQueryParam(name, value)`
- `vars`
  - `get(name)`, `set(name, value)`, `has(name)`
  - `global.get(name)`, `global.set(name, value, options)`, `global.has(name)`, `global.delete(name)` (`options.secret` masks values)
- `console.log/warn/error` (no-op placeholders for compatibility)

Return values from `set*` helpers are ignored; side effects apply to the outgoing request.

### Test scripts (`@script test`)

Objects:

- `client.test(name, fn)` – registers a named test. Exceptions or manual failures mark the test as failed.
- `tests.assert(condition, message)` – add a pass/fail entry.
- `tests.fail(message)` – explicit failure.
- `response`
  - `status`, `statusCode`, `url`, `duration`
  - `body()` (raw string)
  - `json()` (parsed JSON or `null`)
  - `headers.get(name)`, `headers.has(name)`, `headers.all` (lowercase map)
- `stream`
  - `enabled()` – returns `true` when the current response is an SSE or WebSocket transcript.
  - `kind()` – returns `"sse"` or `"websocket"`.
  - `summary()` – copy of the transcript summary (`sentCount`, `receivedCount`, `eventCount`, `duration`, etc.).
  - `events()` – array of event objects (`data`/`comment` for SSE, `type`/`text`/`base64`/`direction` for WebSockets).
  - `onEvent(fn)` – registers a callback invoked for each event after the script runs; useful for assertions over the entire stream.
  - `onClose(fn)` – registers a callback invoked once with the summary after all events replay.
- `vars` – same API as pre-request scripts (allows reading request/file/global values and writing request-scope values for assertions).
- `vars.global` – identical to pre-request usage; changes persist after the script.
- `console.*` – same placeholders as above.

Example test block:

```http
# @script test
> client.test("captures token", function () {
>   var token = vars.get("oauth.manualToken");
>   tests.assert(!!token, "token should be available");
> });
```

---

## Authentication

### Static tokens

Use `@auth bearer {{token}}` or `Authorization: Bearer {{token}}` headers. Combine with `@global` or environment values for reuse.

### Captured tokens

Capture values at runtime and reuse them in subsequent requests:

```http
### Login
# @capture global-secret auth.token {{response.json.token}}
POST {{base.url}}/login

{
  "user": "{{user.email}}",
  "password": "{{user.password}}"
}

### Authorized request
# @auth bearer {{auth.token}}
GET {{base.url}}/profile
```

### OAuth 2.0 directive

Resterm handles the full OAuth 2.0 token lifecycle: fetching tokens, caching them per environment, refreshing when expired, and injecting the `Authorization: Bearer ...` header automatically. Three grant types are supported.

#### Client credentials grant

Best for machine-to-machine authentication where no user is involved.

```http
### Service-to-service call
# @auth oauth2 token_url=https://auth.example.com/oauth/token client_id={{svc.clientId}} client_secret={{svc.clientSecret}} scope="api:read api:write"
GET https://api.example.com/internal/status
```

By default, credentials are sent via HTTP Basic authentication. Use `client_auth=body` to send them as form fields instead (required by some providers):

```http
# @auth oauth2 token_url={{oauth.tokenUrl}} client_id={{oauth.clientId}} client_secret={{oauth.clientSecret}} scope="{{oauth.scope}}" client_auth=body
GET {{base.url}}/resource
```

#### Password grant

For legacy systems that require username/password authentication.

```http
### Resource owner password
# @auth oauth2 token_url=https://auth.example.com/oauth/token client_id={{app.clientId}} client_secret={{app.clientSecret}} grant=password username={{user.email}} password={{user.password}} scope="profile"
GET https://api.example.com/me
```

#### Authorization code + PKCE

When you use `grant=authorization_code`, Resterm handles the entire OAuth automatically:

1. **Browser launch** - Opens your system browser to `auth_url` with the authorization request.
2. **Local callback server** - Spins up a temporary HTTP server on localhost to capture the redirect.
3. **Code exchange** - Exchanges the authorization code for tokens at `token_url`, including the PKCE verifier.
4. **Token injection** - Caches the token and injects it into your request.

##### Redirect URI behavior

The `redirect_uri` controls where the authorization server sends the user after login:

| Configuration | Result |
| --- | --- |
| Omit `redirect_uri` | `http://127.0.0.1:<random-port>/oauth/callback` |
| `redirect_uri=http://127.0.0.1:8080/callback` | Uses port 8080 with path `/callback` |
| `redirect_uri=http://localhost:0/auth` | Random port, custom path `/auth` |

**Constraints:**
- Must use `http://` scheme (not `https://`) - [RFC 8252](https://datatracker.ietf.org/doc/html/rfc8252)
- Host must be `127.0.0.1` or `localhost` - external hosts are rejected
- Register the redirect URI pattern with your OAuth provider (most allow `http://127.0.0.1:*` or similar)

##### PKCE details

PKCE (Proof Key for Code Exchange) protects against authorization code interception. Resterm generates these automatically:

- **code_verifier** - 64 random bytes, base64url-encoded (~86 characters). You can provide your own if needed (must be 43-128 characters per RFC 7636).
- **code_challenge** - SHA-256 hash of the verifier, base64url-encoded.
- **state** - 24 random bytes for CSRF protection.

##### Example: Public client with PKCE

```http
### GitHub OAuth (public client, no secret)
# @auth oauth2 auth_url=https://github.com/login/oauth/authorize token_url=https://github.com/login/oauth/access_token client_id={{github.clientId}} scope="repo read:user" grant=authorization_code
GET https://api.github.com/user
Accept: application/json
```

##### Example: Confidential client

```http
### Auth0 with client secret
# @auth oauth2 auth_url=https://{{auth0.domain}}/authorize token_url=https://{{auth0.domain}}/oauth/token client_id={{auth0.clientId}} client_secret={{auth0.clientSecret}} scope="openid profile" audience={{auth0.audience}} grant=authorization_code
GET {{api.url}}/userinfo
```

##### Timeout behavior

Authorization code flow has a 2-minute timeout by default (to give users time to complete login in the browser). If you need longer, the request's `@timeout` setting is respected as long as it exceeds 2 minutes.

#### Custom token header

Some APIs expect tokens in a non-standard header. Use the `header` parameter to change where the token goes:

```http
### API expecting X-Access-Token header
# @auth oauth2 token_url={{oauth.tokenUrl}} client_id={{oauth.clientId}} client_secret={{oauth.clientSecret}} header=X-Access-Token
GET https://api.example.com/data
```

When `header` is set to something other than `Authorization`, Resterm injects just the raw token (without the "Bearer " prefix). When using the default `Authorization` header, the full `Bearer <token>` format is used.

---

## HTTP Transport & Settings

- Global defaults are passed via CLI flags (`--timeout`, `--follow`, `--insecure`, `--proxy`).
- Per-request overrides use `@setting`, `@settings`, or `@timeout`.
- Requests inherit a shared cookie jar; cookies persist across sessions.
- TLS per request: `# @settings http-root-cas=a.pem http-client-cert=cert.pem http-client-key=key.pem http-insecure=true` for a single line, or `@setting key value` per line (`http-root-cas` accepts space/comma/semicolon separated lists; paths are relative). GraphQL/REST/WebSocket/SSE all share these HTTP settings.
- Use `@no-log` to omit sensitive bodies from history snapshots.
- History is stored in `${RESTERM_CONFIG_DIR}/history.json` (defaults to the platform config directory) and retains up to ~500 entries. Set `RESTERM_CONFIG_DIR` to relocate it.
- Custom root CAs replace system roots by default (strict). Set `http-root-mode append` or `grpc-root-mode append` if you want to keep system roots in addition to your own.
- File-level defaults: place `# @setting key value` or `# @settings key1=val1 ...` before the first request to apply to all requests in that file. Request-level overrides still win.
- Settings are generic. Today the recognized prefixes are transport/TLS (`http-*`, `grpc-*`, `timeout`, `proxy`, `followredirects`, `insecure`). Future features can add more prefixes; unknown keys are ignored for now to stay forward-compatible.
- Environment defaults: `resterm.env.json` can carry global settings under the `settings.` prefix (e.g., `"settings.http-root-cas": "ca-dev.pem"`, `"settings.grpc-insecure": "false"`). Precedence is global (env) < file < request.
- OAuth token exchanges reuse the same HTTP TLS settings (root CAs, client cert/key, `http-insecure`) as the main request.

Body helpers:

- `< path` loads file contents as the body.
- `@ path` inside the body injects file contents inline.
- GraphQL payloads are normalized automatically.

---

## Response History & Diffing

- Every successful request produces a history entry with request text, method, status, duration, and a body snippet (unless `@no-log` is set). Values injected from `-secret` captures and allowlisted sensitive headers (Authorization, Proxy-Authorization, `X-API-Key`, `X-Access-Token`, `X-Auth-Key`, `X-Amz-Security-Token`, etc.) are masked automatically unless you opt-in with `@log-sensitive-headers`.
- History entries are environment-aware; selecting another environment filters the list automatically.
- When focused on the history list, press `Enter` to load a request into the editor without executing it. Use `r`/`Ctrl+R` (or your normal send shortcut such as `Ctrl+Enter` / `Cmd+Enter`) to replay the loaded entry.
- The Diff tab compares focused versus pinned panes, making regression analysis straightforward.
- Compare runs are stored as grouped rows (`COMPARE` method). The preview (`p`) shows the entire bundle, `Enter` loads the failing (or baseline) environment back into the editor, and the Compare tab is automatically repopulated so you can audit deltas offline.

---

## CLI Reference

Run `resterm --help` for the latest list. Core flags:

| Flag | Description |
| --- | --- |
| `--file <path>` | Open a specific `.http`/`.rest` file on launch. |
| `--workspace <dir>` | Workspace root used for file discovery. |
| `--recursive` | Recursively scan the workspace for request files. |
| `--env <name>` | Select environment explicitly. |
| `--env-file <path>` | Provide an explicit environment JSON file. |
| `--timeout <duration>` | Default HTTP timeout (per request). |
| `--insecure` | Skip TLS certificate verification globally. |
| `--follow` | Control redirect following (default on; pass `--follow=false` to disable). |
| `--proxy <url>` | HTTP proxy URL. |
| `--compare <envs>` | Default comma/space-delimited environments for manual compare runs (`g+c`). |
| `--compare-base <env>` | Baseline environment name when `--compare` is set (defaults to the first target). |
| `--from-openapi <spec>` | Generate a `.http` collection from an OpenAPI document. |
| `--http-out <file>` | Destination for the generated `.http` file (defaults to `<spec>.http`). |
| `--openapi-base-var <name>` | Override the base URL variable injected into the generated file (`baseUrl` by default). |
| `--openapi-resolve-refs` | Resolve external `$ref` pointers before generation. |
| `--openapi-include-deprecated` | Keep deprecated operations that are skipped by default. |
| `--openapi-server-index <n>` | Choose which server entry (0-based) seeds the base URL. |

### Importing OpenAPI specs

```bash
resterm \
  --from-openapi openapi-test.yml \
  --http-out openapi-test.http \
  --openapi-resolve-refs \
  --openapi-server-index 1
```

- Loads the OpenAPI document, optionally resolves external `$ref` pointers, and emits a `.http` file that mirrors Resterm metadata and variable conventions.
- Unsupported constructs (for example OpenID Connect) are preserved as `Warning:` lines in the generated header so you know where manual follow-up is required.
- `openapi-test.yml` in the repository is a comprehensive spec that exercises callbacks, complex query styles, and mixed security schemes—perfect for smoke-testing the converter.

> [!NOTE]
> Resterm's converter is powered by [`kin-openapi`](https://github.com/getkin/kin-openapi), which currently validates OpenAPI documents up to **v3.0.1**. Work on v3.1 support is ongoing progress in [getkin/kin-openapi#1102](https://github.com/getkin/kin-openapi/pull/1102).

---

## Configuration

- Config directory: `$HOME/Library/Application Support/resterm` (macOS), `%APPDATA%\resterm` (Windows), or `$HOME/.config/resterm` (Linux/Unix). Override with `RESTERM_CONFIG_DIR`.
- History file: `<config-dir>/history.json` (max ~500 entries by default).
- Settings file: `<config-dir>/settings.toml` (created when you first change preferences such as the default theme).
- Theme directory: `<config-dir>/themes/` (override with `RESTERM_THEMES_DIR`). Drop `.toml` or `.json` files here to make them available in the selector.
- Runtime globals and file captures are scoped per environment and document; they are released when you clear globals or switch environments.

---

## Theming

Resterm lets you override its Lip Gloss styles. Fonts and ultimate colour rendering still come from your terminal emulator; the theme controls which colours Resterm asks the terminal to use.

### Where themes live

- Default directory: `<config-dir>/themes/`
- Override with `RESTERM_THEMES_DIR`
- Sample: `_examples/themes/aurora.toml`
- Switch at runtime with `Ctrl+Alt+T` (or press `g` then `t`). The selection persists in `settings.toml`.

### Theme anatomy

Theme files can be TOML or JSON. Unspecified fields inherit defaults.

```toml
[metadata]
name = "Oceanic"
author = "You"
description = "Cool dusk palette"

[styles.header_title]
foreground = "#5fd1ff"
bold = true

[colors]
pane_border_focus_file = "#1f6feb"
pane_active_foreground = "#f8faff"

[editor_metadata]
comment_marker = "#4c566a"

[[header_segments]]
background = "#5e81ac"
foreground = "#eceff4"

[[command_segments]]
background = "#3b4252"
key = "#88c0d0"
text = "#eceff4"
```

#### Sections and fields

| Section | Keys | Notes |
| --- | --- | --- |
| `[metadata]` | `name`, `description`, `author`, `version`, `tags[]` | Informational only; shown in the selector. |
| `[styles.*]` | `browser_border`, `editor_border`, `response_border`, `navigator_title`, `navigator_title_selected`, `navigator_subtitle`, `navigator_subtitle_selected`, `navigator_badge`, `navigator_tag`, `navigator_detail_title`, `navigator_detail_value`, `navigator_detail_dim`, `app_frame`, `header`, `header_title`, `header_value`, `header_separator`, `status_bar`, `status_bar_key`, `status_bar_value`, `command_bar`, `command_bar_hint`, `response_search_highlight`, `response_search_highlight_active`, `tabs`, `tab_active`, `tab_inactive`, `notification`, `error`, `success`, `header_brand`, `command_divider`, `pane_title`, `pane_title_file`, `pane_title_requests`, `pane_divider`, `editor_hint_box`, `editor_hint_item`, `editor_hint_selected`, `editor_hint_annotation`, `list_item_title`, `list_item_description`, `list_item_selected_title`, `list_item_selected_description`, `list_item_dimmed_title`, `list_item_dimmed_description`, `list_item_filter_match`, `response_content`, `response_content_raw`, `response_content_headers`, `stream_content`, `stream_timestamp`, `stream_direction_send`, `stream_direction_receive`, `stream_direction_info`, `stream_event_name`, `stream_data`, `stream_binary`, `stream_summary`, `stream_error`, `stream_console_title`, `stream_console_mode`, `stream_console_status`, `stream_console_prompt`, `stream_console_input`, `stream_console_input_focused` | Accept `foreground`, `background`, `border_color`, `border_background`, `border_style` (`normal`, `rounded`, `thick`, `double`, `ascii`, `block`), plus booleans `bold`, `italic`, `underline`, `faint`, `strikethrough`, and `align` (`left`, `center`, `right`). |
| `[colors]` | `pane_border_focus_file`, `pane_border_focus_requests`, `pane_active_foreground`, `method_get`, `method_post`, `method_put`, `method_patch`, `method_delete`, `method_head`, `method_options`, `method_grpc`, `method_ws`, `method_default` | Frequently reused colours for pane borders, active text, and method badges. |
| `[editor_metadata]` | `comment_marker`, `directive_default`, `value`, `setting_key`, `setting_value`, `request_line`, `request_separator`, `[editor_metadata.directive_colors]` | Controls metadata highlighting inside the editor. |
| `[[header_segments]]` | `background`, `foreground`, `border`, `accent` | Rotating header chips; add multiple tables for rotation. |
| `[[command_segments]]` | `background`, `border`, `key`, `text` | Colour sets for command bar hint capsules. |

Setting `editor_metadata.directive_default` recolours every built-in directive (`@name`, `@tag`, etc.) that still uses the inherited default. Specify entries inside `[editor_metadata.directive_colors]` only when you need a directive to diverge from that default.

Set `editor_metadata.request_line` to recolour the full request line (`POST https://…`). If you omit it, Resterm falls back to the directive default.
Use `editor_metadata.request_separator` for the `###` section dividers and `editor_metadata.comment_marker` for the `#` / `//` prefixes at the start of comment lines.

`styles.stream_*` keys control the transcript viewer (events, timestamps, direction badges). `styles.stream_console_*` tweak the interactive WebSocket console (prompt, status line, input field).

`navigator_*` styles control the unified sidebar tree, and `styles.list_item_*` keys continue to power history/picker list rows. `styles.response_content`, `styles.response_content_raw`, and `styles.response_content_headers` colour the response panes for Raw and Headers (with the general key applied first, then the tab-specific override).

### Testing a theme

```bash
export RESTERM_THEMES_DIR="$(pwd)/_examples/themes"
resterm
```

Inside Resterm, press `g` then `t` (or `Ctrl+Alt+T`) and pick “Aurora” for a dark setup or “Daybreak” for light terminals. Quit and restart to confirm the theme persists. If a theme fails to parse, Resterm logs the error and falls back to the default palette.


---

## Examples

Explore `_examples/` for ready-to-run:

- `basic.http` - simple GET/POST with bearer auth.
- `scopes.http` - demonstrates global/file/request captures.
- `scripts.http` - pre-request and test scripting patterns.
- `graphql.http` - inline and file-based GraphQL requests.
- `grpc.http` - gRPC reflection and descriptor usage.
- `oauth2.http` - manual capture vs using the `@auth oauth2` directive.
- `transport.http` - timeout, proxy, and `@no-log` samples.
- `compare.http` - demonstrates `@compare` directives and CLI-triggered multi-environment sweeps.
- `workflows.http` - end-to-end workflow with captures, overrides, and expectations.

Open one in Resterm, switch to the appropriate environment (`resterm.env.json`), and send requests to see each feature in action.

---

## Troubleshooting & Tips

- Use `Ctrl+P` to force a reparse if the navigator seems out of sync with editor changes.
- If a template fails to expand (undefined variable), Resterm leaves the placeholder intact and surfaces an error banner.
- Combine `@capture request ...` with test scripts to assert on response headers without cluttering file/global scopes.
- Inline curl import works best with single commands; complex shell pipelines may need manual cleanup.
- `Ctrl+Shift+V` pins the focused response pane-ideal for diffing the last good response against the current attempt.
- Keep secrets in environment files or runtime globals marked as `-secret`. Remember that history stores the raw response unless you add `@no-log` or redact the payload yourself.

For additional questions or feature requests, open an issue on GitHub.
