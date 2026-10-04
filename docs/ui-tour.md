# UI tour

## Layout

- **Sidebar**: one navigator tree for files, requests, and workflows, with a filter bar and tag and method chips. `→`/`Space` expand files, `g+k`/`g+j` expand or collapse the current branch, and `g+Shift+K`/`g+Shift+J` expand or collapse all. A detail panel under the list shows a summary of the selected request or workflow. When the sidebar is focused, `g+h` shrinks it and `g+l` widens it.
- **Editor**: the middle pane, with modal editing (view mode by default, `i` to insert, `Esc` to go back to view mode). Syntax highlighting marks metadata, headers, and bodies.
- **Response panes**: the right side shows the most recent response. You can split it to compare responses side by side.
- **Header bar**: shows the workspace, active environment, current request, test summaries, the latest transport status and RTT, and the Help shortcut.
- **Command bar & status**: contextual hints, progress, and notifications. Long messages are shortened so the file, focus, and mode sections stay visible. Errors and long warnings open a popup with the full message. Press `Esc` or `Enter` to close it, or `j`/`k` to scroll. Press `g .` to see warnings for the current document, or to reopen the current status message when there are none. Run summaries and confirmation prompts stay in the bar, because their details or next step are available elsewhere.

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

The editor supports common Vim motions such as `h`, `j`, `k`, `l`, `w`, `b`, `gg`, and `G`. Enter insert mode with `i`, `a`, `I`, `A`, `o`, or `O` (`I` moves to the first non-blank character). It also has visual selections with `v` / `V`, yank and delete/change operations, and undo/redo (`u` / `Ctrl+r`). Press `Shift+F` or `/` to search. `Ctrl+R` toggles regex, `n` jumps to the next match, and `p` to the previous one.

Press `:` in a normal mode pane to open a Vim-style command line. Supported commands include `:w`, `:q`, `:q!`, `:wq`, `:x`, `:e [path]`, `:help`, `:man`, `:docs`, `:noh`, and the `:mock` commands. `:e` without a path opens the path prompt. With a path, it opens that file or workspace directly.

## Finding help

Resterm keeps short docs inside the binary, so the built-in help works offline and matches the version you have installed:

- Press `?` for the searchable help index. Type `/` to filter shortcuts and topics, then press `Esc` to clear the filter or close help.
- Run `:help <topic>` to open a built-in topic directly. `:man <topic>` does the same. Run either command without a topic to see the index.
- In editor normal mode, put the cursor on an `@directive`, an HTTP or protocol keyword, or inside `{{ ... }}`, then press `K` to open the matching topic. If there is no exact match, Resterm stays in the editor and shows a short hint.
- Press `o` in a built-in topic, or run `:docs <topic>`, to open the matching page on resterm.app. `:docs` on its own opens the docs index. If the browser can't be opened, Resterm shows the URL so you can copy it.

The command line suggests commands, topics, `:mock` subcommands, and file paths when the current argument takes one. `Up` / `Down` (or `Ctrl+P` / `Ctrl+N`) selects a suggestion, `Tab` completes it without running it, and `Enter` runs the selected one. `Tab` on a directory moves into it. `Enter` also moves into a directory when the command needs a file. If you haven't selected a row, `Enter` runs whatever is in the prompt. Paths with spaces are quoted automatically.

`Ctrl+O` opens the same file picker as its own "Open File or Workspace" popup. Type a relative, absolute, or `~` path. Use `Up` / `Down` (or `Ctrl+P` / `Ctrl+N`) to select, `Tab` to complete or move into a directory, and `Enter` to open a supported file, or a directory as a workspace.

The command bar at the bottom changes with the focused pane, editor mode, and response tab. Its key hints are plain text by default. Themes can add keycap backgrounds with `command_segments`. The Help shortcut always stays visible in the header. If you change a shortcut, the new key shows up in the hints and in the help overlay.

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
| `vars.` and `vars.global.` in scripts, `{{= ... }}` expressions, and RTS directives such as `@assert` | Methods such as `get`, `set`, and `interpolate`. `require` is only available in RTS. Expressions and directives are read-only, so `set` and `delete` are not suggested |
| `@compare` arguments | Environment names or profiles from the selected group. Baseline suggestions use the targets already chosen |
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
- **Raw**: the exact payload text.
- **Stream**: a live transcript of WebSocket and SSE sessions, with bookmarks and a console.
- **Headers**: the response headers and the request headers that were sent, with a switcher in the pane. Press `Enter` or `Space` on the Headers tab to switch between them. The request headers include cookies.
- **Profile** / **Workflow**: live results for profile and workflow runs. Profile results show progress, latency statistics, a histogram, and failures. On narrow panes, the sections stack vertically. Workflow results show a summary, a step list, and details for the selected step. The tab label follows the current run type. Use `j` / `k` or the arrow keys to move between steps. Press `Enter` or `Space` to focus the details of the selected step, then `j` / `k` or `PageUp` / `PageDown` to scroll them. `Esc`, `Enter`, or `Space` takes you back to the step list.
- **Timeline**: HTTP timings for each phase, with budget overlays. Available when tracing is enabled.
- **Diff**: compare the focused pane against the other response pane.
- **History**: past responses for the selected request in time order, updated live. Press `p` for a full JSON preview or `d` to delete the focused entry.

When a request opens a stream, the Stream tab becomes available. [Stream tab, history, and console](streaming.md#stream-tab-history-and-console) covers its keys and the WebSocket console.

Use `Alt+V` or `Alt+H` to split the response pane. You can pin the second pane so later responses only fill the first one, which makes comparing easy.

When the response pane is focused, `Ctrl+Shift+C` (or `g y`) copies the whole Pretty, Raw, or Headers tab to your clipboard, exactly as it is shown. You don't need to select anything with the mouse.

Use `g+g` and `G` to jump to the start or end of the Pretty, Raw, or Headers tabs when the response pane is focused. The same keys jump to the first or last entry in the navigator when you are browsing files or workflows.

Binary responses show the size and type next to a short preview. For large binary payloads, the Raw tab starts in a summary view and loads the full dump only when you ask for it. With the response pane focused, press `g+b` to cycle the Raw tab through summary, hex, and base64 views. Press `g+Shift+D` to load the full hex dump right away. Press `g+Shift+S` to open the Save Response Body prompt. It suggests a path based on your last save or the workspace, and writes the file when you press Enter.

Press `g+Shift+E` to open the body in your default app. Resterm only opens types from a fixed list: images, PDF, text, JSON, CSV, audio, video, archives, and Office files without macros. HTML, SVG, XML, and Markdown open as plain text so no scripts inside them can run. It finds the type from the `Content-Type` header, then the file name the server sent, then the body itself. The file extension always comes from that type, so a PDF sent as `invoice.exe` opens as `invoice.pdf`. If no type fits, Resterm shows a warning and you can save the body with `g+Shift+S` instead. Opened files are kept in a temporary folder that Resterm deletes when it exits.

## Pane minimization & zoom

- Toggle the sidebar, editor, or response panes with `g+1`, `g+2`, and `g+3`. Minimized panes shrink to thin frames that show an indicator and the shortcut to restore them.
- Status bar badges (`Sidebar:min`, `Editor:min`, `Response:min`) show which panes are minimized, so you can tell something is hidden even if its frame scrolls out of view.
- Use `g+z` to zoom the focused pane and hide the others for a while. `g+Z` clears the zoom and restores the previous layout, including any panes you minimized.
- Resize chords such as `g+h` / `g+l` and `g+j` / `g+k` are disabled while a related pane is hidden or zoomed, so you can't reset the layout by accident.

## Timeline & tracing

- Add a `# @trace` directive to turn on HTTP tracing for a request. Budgets use `phase<=duration`, such as `dns<=50ms` or `total<=300ms`, with an optional `tolerance=` that applies to every phase. The supported phases map to `nettrace`: `dns`, `connect`, `tls`, `request_headers`, `request_body`, `ttfb`, `transfer`, and `total`.
- When a traced response arrives, Resterm checks the budgets, shows status bar warnings for breaches, and enables the Timeline tab. Press `Ctrl+Alt+L` or `g+t` to jump to it from anywhere.
- The Timeline view draws proportional bars, marks overruns, and lists budget breaches. Details such as cached DNS results or reused sockets appear under each phase, followed by Connection and TLS panels (protocol, reuse, proxy/SSH, resolved IPs, cipher/ALPN, cert chain, SANs, issuer, expiry).
- Scripts can read trace data through the `trace` binding, with calls such as `trace.enabled()`, `trace.phases()`, `trace.connection()`, `trace.tls()`, `trace.breaches()`, and `trace.withinBudget()`. Goja test blocks can use them to check timings automatically.
- `_examples/trace.http` has two requests you can run, one within budget and one that breaks it on purpose, to show the timeline and status messages.
- To export traces to OpenTelemetry, set `RESTERM_TRACE_OTEL_ENDPOINT` (or `--trace-otel-endpoint`). Other options are `RESTERM_TRACE_OTEL_INSECURE` / `--trace-otel-insecure`, `RESTERM_TRACE_OTEL_SERVICE` / `--trace-otel-service`, `RESTERM_TRACE_OTEL_TIMEOUT`, and `RESTERM_TRACE_OTEL_HEADERS`. Spans are only exported while tracing is enabled. HTTP failures and budget breaches set the span status to `Error`.

## History and globals

- The history pane saves responses together with their request and environment details. Entries are kept across restarts in the config directory. See [Configuration](configuration.md).
- `Ctrl+G` shows current globals (request/file/runtime) with secrets masked. `Ctrl+Shift+G` (or `g Shift+G`) clears globals and cookies for the active environment.
- `Ctrl+E` opens the environment picker to switch between `resterm.env.json` (or `rest-client.env.json`) entries.
