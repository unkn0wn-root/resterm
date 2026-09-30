# UI tour

## Layout

- **Sidebar**: unified navigator tree for files, requests, and workflows with a filter bar and tag/method chips. `→`/`Space` expand files, `g+k`/`g+j` expand or collapse the current branch, and `g+Shift+K`/`g+Shift+J` expand or collapse all. A detail well beneath the list shows the selected request/workflow summary. When focused, `g+h` shrinks and `g+l` expands the sidebar.
- **Editor**: middle pane with modal editing (view mode by default, `i` to insert, `Esc` to return to view). Inline syntax highlighting marks metadata, headers, and bodies.
- **Response panes**: right-hand side displays the most recent response, with optional splits for side-by-side comparisons.
- **Header bar**: shows workspace, active environment, current request, test summaries, the latest transport status and RTT, and the Help shortcut.
- **Command bar & status**: contextual hints, progress, and notifications. Long messages are shortened to preserve the file, focus, and mode sections. Errors and long warnings open a popup with the complete message; press `Esc` or `Enter` to dismiss it, or `j`/`k` to scroll. Press `g .` to inspect current document warnings, or to reopen the current status message when there are none. Run summaries and confirmation prompts remain in the bar because their details or next action are available elsewhere.

## Core shortcuts

| Action | Shortcut |
| --- | --- |
| Send active request | `Ctrl+Enter` / `Cmd+Enter` / `Alt+Enter` / `Ctrl+J` / `Ctrl+M` |
| Toggle help overlay | `?` |
| Toggle editor insert mode | `i` / `Esc` |
| Cycle focus (navigator -> editor -> response) | `Tab` / `Shift+Tab` |
| Focus navigator / editor / response panes | `g+r` / `g+i` / `g+p` |
| Open environment selector | `Ctrl+E` |
| Save file | `Ctrl+S` |
| Open file/workspace popup | `Ctrl+O` |
| Quit | `Ctrl+Q` (or `Ctrl+D`) |

[Key bindings](key-bindings.md) lists every shortcut and shows how to change them.

The editor supports familiar Vim motions (`h`, `j`, `k`, `l`, `w`, `b`, `gg`, `G`, etc.), insert entries (`i`, `a`, `I`, `A`, `o`, `O`; `I` moves to the first non-blank character), visual selections with `v` / `V`, yank and delete/change operations, undo/redo (`u` / `Ctrl+r`), and a search palette (`Shift+F` or `/`, toggle regex with `Ctrl+R` and `n` moves cursor forward and `p` backwards).

Press `:` from normal mode panes to open a Vim-style command line. Supported actions include `:w`, `:q`, `:q!`, `:wq`, `:x`, `:e [path]`, `:help`, `:man`, `:docs`, `:noh`, and the `:mock` command family. Bare `:e` opens the path prompt; giving it a path opens that file or workspace directly.

## Finding help

Resterm keeps concise documentation inside the binary, so the first layer of help works offline and matches the installed version:

- Press `?` for the searchable help index. Type `/` to filter shortcuts and topic contents, then use `Esc` to clear the filter or close help.
- Run `:help <topic>` to open an embedded topic directly; `:man <topic>` is an alias. Run either command without a topic for the index.
- In editor normal mode, put the cursor on an `@directive`, an HTTP/protocol keyword, or inside `{{ ... }}`, then press `K` for the relevant topic. If no exact topic is available, Resterm leaves the editor open and shows a short recovery hint.
- Press `o` from an embedded topic, or run `:docs <topic>`, to open the matching page on resterm.app. Bare `:docs` opens the documentation index. If the browser cannot be started, Resterm shows the URL so it can be copied manually.

The command line suggests commands, topics, `:mock` subcommands, and filesystem paths where the active argument accepts one. `Up` / `Down` (or `Ctrl+P` / `Ctrl+N`) selects a suggestion, `Tab` completes it without running, and `Enter` accepts and runs an explicit selection. `Tab` on a directory descends into it; `Enter` also descends when the command requires a file. If no row has been selected, `Enter` runs the text currently in the prompt. Paths containing whitespace are quoted automatically.

`Ctrl+O` opens the same filesystem picker as a standalone “Open File or Workspace” popup. Type a relative, absolute, or `~` path; use `Up` / `Down` (or `Ctrl+P` / `Ctrl+N`) to select, `Tab` to complete or descend, and `Enter` to open the selected supported file or directory as a workspace.

The bottom command bar adapts to the focused pane, editor mode, and response tab. Its contextual keys use a flat presentation by default; themes can add keycap backgrounds through `command_segments`. The global Help shortcut remains visible in the header, and configured shortcuts are reflected in both contextual hints and the help overlay.

## Editor completions (IntelliSense)

In insert mode, Resterm suggests completions at the caret using the open file and
active environment. It makes no network calls while you type.

| Context | What completes |
| --- | --- |
| Start of a request line | HTTP methods plus `WS` / `WSS` / `GRPC` |
| Start of a request URL | Schemes: `http://`, `https://`, `ws://`, `wss://` |
| `@` at the start of a line, with or without a comment marker | Directives, option keys, and values such as booleans, OAuth grants, HTTP/TLS modes, and workflow failure modes |
| Header section (after the request line, before the blank line) | Header names, then values for well-known headers such as `Content-Type` |
| Inside `{{ ... }}` | Variables in scope (file/global/request, `@const`, current-environment keys) and dynamic builtins (`$uuid`, `$timestamp`, ...) |
| `@compare` arguments | Environment names or profiles from the selected group; baseline suggestions use the targets already chosen |
| `use=` on `@apply` / `@ssh` / `@k8s` | Matching `@patch` / `@ssh` / `@k8s` profile names |
| `using=` / `run=` on workflow steps and branches | Named requests from the current document |
| File paths | Files and directories for `@use`, descriptors, GraphQL/JSON inputs, TLS/SSH/Kubernetes options, request bodies, and script or body includes |

Press `Ctrl+N` to show all completions valid at the caret. If there are none, it does
nothing. In editor insert mode this key always controls completion, regardless of
the New Request binding. Outside editor insert mode, the configured New Request
shortcut applies.

In the popup, `Up` / `Down` or `Ctrl+P` / `Ctrl+N` selects an item. `Right` or `?`
opens the details preview, `Ctrl+L` toggles it, and `Left` or `Esc` closes it.
`Enter` or `Tab` accepts an item. `Esc` dismisses the popup when the preview is
closed. Use the `editor_hint_*` theme keys to style the popup.

After you accept a completion, the popup shows what can follow it. For example,
`@auth` opens auth modes, `oauth2` opens its options, and `grant=` opens grant values.
Headers open value suggestions, and directories open their contents. Options that
can appear only once disappear after use, along with their aliases, such as `base=`
and `baseline=`. You can repeat `@apply use=`, but profiles already selected are
omitted.

File suggestions use the request file's directory, or the workspace for temporary
documents. They are filtered by file type where needed: `.rts` for `@use`, GraphQL
for `@query`, JSON for `@variables`, and script files for script includes. Directory
listings are cached while browsing. In directives that split arguments on spaces,
paths with spaces are quoted. File references that use the rest of the line stay
unquoted. Only SSH and Kubernetes path options expand `~` to the home directory.

Accepting a directive typed without a comment marker adds `# ` automatically. For
example, completing `@na` produces `# @name `. The marker is added only when you
accept a suggestion, so you can still type variables such as `@name = value`.

Many suggestions insert an example value, such as `@setting timeout=5s` or `@mock
latency=random(100ms,500ms)`. Accepting one leaves the example selected, so the next
keystroke replaces it. Press `Tab` to keep the example and move past the inserted
text, including any closing parenthesis. `Left` and `Right` also keep the example
and move to the start or end of the selected text.

Dynamic helpers with call examples insert the call and select only its arguments.
Variable completion adds any missing closing braces, so completing `{{ho`, `{{ho}`,
and `{{ho}}` with `host` produces `{{host}}` in each case. Inside `{{= ... }}`
expressions and helper arguments, completion leaves the closing braces unchanged.

Completion does not use gRPC reflection, descriptors, or GraphQL schemas to suggest
service, method, or field names.

## Response panes

- **Pretty**: formatted JSON (or best-effort formatting for other types).
- **Raw**: exact payload text.
- **Stream**: live transcript viewer for WebSocket and SSE sessions with bookmarking and console integration.
- **Headers**: response and request header subviews with a visible in-pane switcher. Press `Enter` or `Space` while focused on the Headers tab to switch between the response headers and the sent request headers (cookies included).
- **Profile** / **Workflow**: live results for profile and workflow runs. Profile results show progress, latency statistics, a histogram, and failures. On narrow panes, the sections stack vertically. Workflow results show a summary, a step list, and details for the selected step. The tab label follows the current run type. Use `j` / `k` or arrow keys to move between steps, `Enter` or `Space` to focus the selected step detail, `j` / `k` or `PageUp` / `PageDown` to scroll that detail, and `Esc`, `Enter`, or `Space` to return to the step list.
- **Timeline**: per-phase HTTP timings with budget overlays; available whenever tracing is enabled.
- **Diff**: compare the focused pane against the other response pane.
- **History**: chronological responses for the selected request (live updates). Open a full JSON preview with `p` or delete the focused entry with `d`.

When a request opens a stream, the Stream tab becomes available. [Stream tab, history, and console](streaming.md#stream-tab-history-and-console) covers its keys and the WebSocket console.

Use `Alt+V` or `Alt+H` to split the response pane. The secondary pane can be pinned so subsequent calls populate only the primary pane, making comparisons easy.

While the response pane is focused, `Ctrl+Shift+C` (or `g y`) copies the entire Pretty, Raw, or Headers tab directly to your clipboard, matching the rendered text (no mouse selection required).

Use `g+g` and `G` to jump to the start or end of the Pretty, Raw, or Headers tabs when the response pane is focused. The same keys jump to the first or last entry in the navigator when you are browsing files or workflows.

Binary responses show size and type hints alongside quick previews. For large binary payloads, the Raw tab starts in a summary view and defers full dumps until requested. While the response pane is focused, press `g+b` to rotate the Raw tab between summary, hex, and base64 views. Press `g+Shift+D` to load the full hex dump immediately. Press `g+Shift+S` to open the Save Response Body prompt, which comes prefilled with a suggested path from your last save or workspace and writes the file after you hit Enter.

Press `g+Shift+E` to open the body in your default app. Resterm only opens types from a fixed list: images, PDF, text, JSON, CSV, audio, video, archives, and Office files without macros. HTML, SVG, XML, and Markdown open as plain text so no scripts inside them can run. It finds the type from the `Content-Type` header, then the file name the server sent, then the body itself. The file extension always comes from that type, so a PDF sent as `invoice.exe` opens as `invoice.pdf`. If no type fits, Resterm shows a warning and you can save the body with `g+Shift+S` instead. Opened files are kept in a temporary folder that Resterm deletes when it exits.

## Pane minimization & zoom

- Toggle the sidebar, editor, or response panes with `g+1`, `g+2`, and `g+3`. Minimized panes collapse into thin frames that display an indicator along with a reminder of the restoring shortcut.
- Status bar badges (`Sidebar:min`, `Editor:min`, `Response:min`) mirror the current state so you can tell when something is hidden even if the stub scrolls out of view.
- Use `g+z` to zoom the currently focused pane and hide the others temporarily; `g+Z` clears zoom and restores the previous layout (including any manual minimize state).
- Resize chords such as `g+h` / `g+l` and `g+j` / `g+k` are disabled while a related pane is hidden or zoomed, preventing accidental layout resets.

## Timeline & tracing

- Add `# @trace` directives to enable HTTP tracing on a request. Budgets use `phase<=duration` notation (`dns<=50ms`, `total<=300ms`, etc.) with an optional `tolerance=` applied to every phase. Supported phases map to `nettrace`: `dns`, `connect`, `tls`, `request_headers`, `request_body`, `ttfb`, `transfer`, and `total`.
- When a traced response arrives, Resterm evaluates budgets, raises status bar warnings for breaches, and unlocks the Timeline tab. Use `Ctrl+Alt+L` or the `g+t` chord to jump straight to it from anywhere.
- The Timeline view renders proportional bars, annotates overruns, and lists budget breaches. Metadata such as cached DNS results or reused sockets appears beneath each phase, followed by Connection and TLS panels (protocol, reuse, proxy/SSH, resolved IPs, cipher/ALPN, cert chain, SANs, issuer, expiry).
- Scripts can inspect traces through the `trace` binding (`trace.enabled()`, `trace.phases()`, `trace.connection()`, `trace.tls()`, `trace.breaches()`, `trace.withinBudget()`, etc.), allowing automated validations inside Goja test blocks.
- See `_examples/trace.http` for a runnable pair of requests (one within budget, one deliberately breaching) that demonstrate the timeline output and status messaging.
- Configure optional OpenTelemetry export with `RESTERM_TRACE_OTEL_ENDPOINT` (or `--trace-otel-endpoint`). Additional switches: `RESTERM_TRACE_OTEL_INSECURE` / `--trace-otel-insecure`, `RESTERM_TRACE_OTEL_SERVICE` / `--trace-otel-service`, `RESTERM_TRACE_OTEL_TIMEOUT`, and `RESTERM_TRACE_OTEL_HEADERS`. Spans are emitted only while tracing is enabled; HTTP failures and budget breaches mark the span status as `Error`.

## History and globals

- The history pane persists responses along with their request and environment metadata. Entries survive restarts (stored under the config directory; see [Configuration](configuration.md)).
- `Ctrl+G` shows current globals (request/file/runtime) with secrets masked. `Ctrl+Shift+G` (or `g Shift+G`) clears globals and cookies for the active environment.
- `Ctrl+E` opens the environment picker to switch between `resterm.env.json` (or `rest-client.env.json`) entries.
