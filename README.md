<h1 align="center">Resterm</h1>

<p align="center">
  <em>a terminal-based REST client.</em>
</p>

<p align="center">
  <img src="_media/resterm.png" alt="Screenshot of resterm TUI" width="720" />
</p>

## Features
- **Workspace explorer.** Filters `.http`/`.rest` files, respects workspace roots, and keeps the file pane navigable with incremental search.
- **Editor with modal workflow.** Starts in view mode, supports Vim-style motions, visual selections with inline highlighting, clipboard yank/cut, `Shift+F` search, and an `i` / `Esc` toggle for insert mode.
- **Inline requests.** Type `https://api.example.com` or `GET https://api.example.com` directly in the editor and press `Ctrl+Enter` - no `.http`/`.rest` file required.
- **Curl command parsing (limited).** supports basic `curl` invocations (method, headers, data flags) - more in the road-map.
- **Status-aware response pane.** Pill-style header calls out workspace, environment, active request, and script/test outcomes; response tabs cover Pretty, Raw, Headers, and History, plus request previews.
- **Split response views with diffing.** Snap responses into vertical or horizontal splits, pin panes, and compare Pretty/Raw/Headers tabs side-by-side with a unified diff view.
- **Auth & variable helpers.** `@auth` directives cover basic, bearer, API key, and custom headers; variable resolution spans request, file, environment, and OS layers with helpers like `{{$timestamp}}` and `{{$uuid}}`.
- **Pre-request & test scripting.** JavaScript (goja) hooks mutate outgoing requests, assert on responses, and surface pass/fail summaries inline.
- **GraphQL tooling.** `@graphql` and `@variables` directives produce proper payloads, attach operation names, and keep previews/history readable.
- **gRPC client.** `GRPC host:port` requests with `@grpc` metadata build messages from descriptor sets or reflection, stream metadata/trailers, and log history entries beside HTTP calls.
- **Session persistence.** Cookie jar, history store, and environment-aware entries survive restarts; `@no-log` can redact bodies.
- **Configurable transport.** Flag-driven timeout, TLS, redirect, and proxy settings alongside environment file discovery (`resterm.env.json` or legacy `rest-client.env.json`).

> [!WARNING]
> Resterm is still in early stages so bugs and undesired behaviors can be expected.

## Request File Structure

Resterm reads plain-text `.http`/`.rest` files. Each request follows the same conventions so the editor, parser, and history can reason about it consistently.

```http
### get user
# @name getUser
# @description Fetch a user profile
GET https://{{baseUrl}}/users/{{userId}}
Authorization: Bearer {{token}}
X-Debug: {{$timestamp}}

{
  "verbose": true
}

### create user
POST https://{{baseUrl}}/users
Content-Type: application/json

< ./payloads/create-user.json
```

- **Request separators.** Start a new request with a line beginning `###` (an optional label after the hashes is ignored by the parser but is handy for readability).
- **Metadata directives.** Comment lines (`#` or `//`) before the request line can include directives such as `@name`, `@description`, `@tag`, `@auth`, `@graphql`, `@grpc`, `@variables`, and `@script`. See [Request Metadata & Settings](#request-metadata--settings) for the full list.
- **Request line.** The first non-comment line specifies the verb and target. HTTP calls use `<METHOD> <URL>`, whereas gRPC calls begin with `GRPC host:port` followed by `@grpc package.Service/Method` metadata.
- **Headers.** Subsequent lines of the form `Header-Name: value` are sent verbatim after variable substitution.
- **Body.** A blank line separates headers from the body. You can inline JSON/text, use heredoc-style scripts, or include external files with `< ./path/to/file`.
- **Inline variables.** Placeholders like `{{userId}}` or `{{token}}` are resolved using the variable stack (request variables, file-level variables, selected environment, then OS environment). Helpers such as `{{$uuid}}` and `{{$timestamp}}` are available out of the box.

## Getting Started

### Install prebuilt binaries

Download from [GitHub Releases](https://github.com/unkn0wn-root/resterm/releases). You can use the snippets below to automatically detect the latest tag and fetch the matching binary for your platform.

> [!NOTE]
> The examples require `curl` and `jq`. Install `jq` with your package manager (e.g. `brew install jq`, `sudo apt install jq` etc.).

#### Linux / macOS

```bash
# Detect latest version
LATEST_TAG=$(curl -fsSL https://api.github.com/repos/unkn0wn-root/resterm/releases/latest | jq -r .tag_name)

# Download the appropriate binary (Darwin/Linux + amd64/arm64)
curl -fL -o resterm "https://github.com/unkn0wn-root/resterm/releases/download/${LATEST_TAG}/resterm_$(uname -s)_$(uname -m)"

# Make it executable and install into your PATH
chmod +x resterm
sudo install -m 0755 resterm /usr/local/bin/resterm
```

If your system does not include `install`, replace the final line with `sudo mv resterm /usr/local/bin/resterm`.

#### Windows (PowerShell)

```powershell
# Detect latest version
$release = Invoke-RestMethod -Uri "https://api.github.com/repos/unkn0wn-root/resterm/releases/latest"
$tag = $release.tag_name

# Map PROCESSOR_ARCHITECTURE to the release naming convention
$arch = if ($env:PROCESSOR_ARCHITECTURE -eq 'ARM64') { 'arm64' } else { 'x86_64' }
$asset = "resterm_Windows_$arch"

# Download and rename to resterm.exe
Invoke-WebRequest -Uri "https://github.com/unkn0wn-root/resterm/releases/download/$tag/$asset" -OutFile $asset
Rename-Item $asset resterm.exe -Force

# Move into a directory on your PATH (update the destination to your preference)
Move-Item resterm.exe "$env:USERPROFILE\bin\resterm.exe" -Force
```

Ensure the destination directory is on your `PATH`. If `%USERPROFILE%\bin` does not exist, create it and add it to your environment variables.

### Build from source

```bash
# build the binary
go build ./cmd/resterm

# run with a sample file
./resterm --file examples/basic.http
```

### Workspace Mode

By default `resterm` scans the opened file’s directory (or the current working directory) for request files. Use `--workspace` to pick a different root:

```bash
./resterm --workspace ./samples --file samples/basic.http
```

### Key Bindings

| Action | Shortcut |
|--------|----------|
| Cycle focus between panes | `Tab` / `Shift+Tab` |
| Send active editor request | `Ctrl+Enter` |
| Run selected request from the palette | `Enter` (Requests list) |
| Preview selected request in the editor | `Space` |
| Toggle editor insert mode | `i` (enter insert) / `Esc` (return to view) |
| Toggle help overlay | `?` |
| Open environment selector | `Ctrl+E` |
| Save current file | `Ctrl+S` |
| Reparse document | `Ctrl+T` |
| Refresh workspace file list | `Ctrl+O` |
| Adjust files/requests split | `gk` / `gj` |
| Adjust editor width | `gh` / `gl` |
| Split responses vertically | `Ctrl+V` |
| Split responses horizontally | `Ctrl+U` |
| Pin/unpin focused response pane | `Ctrl+Shift+V` |
| Target response pane for new responses | `Ctrl+F`, then `← / →` or `h / l` |
| Replay highlighted history entry | `Enter` (History tab) |
| Quit | `Ctrl+Q` (`Ctrl+D` also works) |

Note: resize chords (`gh`, `gl`, `gj`, `gk`) stay active until you press another key, so tap any non-resize key when you want to return to normal navigation.

#### Response panes, pinning & diff
- Split the response area vertically (`Ctrl+V`) or horizontally (`Ctrl+U`). Press the shortcut again to collapse the split.
- Use `Ctrl+F` followed by `← / →` (or `h / l`) to choose which pane receives the next response. The other pane is pinned automatically so you can compare results.
- `Ctrl+Shift+V` toggles whether the focused pane tracks new responses (live) or stays pinned to its current snapshot.
- Each pane has **Pretty**, **Raw**, **Headers**, **Diff**, and **History** tabs. Open **Diff** in either pane to compare its current base tab (Pretty/Raw/Headers) against the other pane.

#### Editor motions & search
- `h`, `j`, `k`, `l` - move left/down/up/right
- `w`, `b`, `e` - jump by words (`e` lands on word ends)
- `0`, `$`, `^` - start/end/first non-blank of line
- `gg`, `G` - top/bottom of buffer
- `Ctrl+f` / `Ctrl+b` - page down/up (`Ctrl+d` / `Ctrl+u` half-page)
- `v`, `V`, `y`, `d` - visual select, visual line select, yank selection, delete via current selection or motion (`dw`, `db`, `dk`, `dgg`, `dG`, etc.)
- `u` - undo last edit
- `Ctrl+r` - redo previously undone edit
- `dd`, `D`, `x`, `c`, `a` - delete line, delete to end of line, delete character, change line, append after cursor (enter insert mode)
- `p`, `P` - paste after/before cursor
- `f`, `t`, `T` - find character (forward / forward before target / backward before target)
- `Shift+F` - open search prompt; `Ctrl+R` toggles regex while open
- `n` - jump to the next match (wraps around)

### CLI Flags
- `--file`: path to a `.http`/`.rest` file to open.
- `--workspace`: directory to scan for request files.
- `--env`: named environment from the environment set.
- `--env-file`: explicit path to an environment JSON file.
- `--timeout`: request timeout (default `30s`).
- `--insecure`: skip TLS certificate verification.
- `--follow`: follow redirects (default `true`).
- `--proxy`: HTTP proxy URL.
- `--recurisve`: recursively scan the workspace for `.http`/`.rest` files.

Environment files are simple JSON maps keyed by environment name, for example:

```json
{
  "dev": {
    "baseUrl": "https://api.dev.local",
    "token": "dev-token"
  },
  "prod": {
    "baseUrl": "https://api.example.com"
  }
}
```

## Request Metadata & Settings

- `@name <identifier>` - names the request for the file explorer and history.
- `@description <text>` / `@desc <text>` - attaches multi-line prose notes that travel with the request.
- `@tag <tag1> <tag2>` - assigns tags for quick filtering (stored even if the current UI doesn’t surface them yet (it is in the roadmap)).
- `@auth` - injects authentication automatically. Supported forms:
  - `@auth basic <user> <password>`
  - `@auth bearer <token>`
  - `@auth apikey <header|query> <name> <value>`
  - `@auth Authorization <value>` (custom header)
- `@setting <key> <value>` - per-request overrides. Recognised keys (`timeout`, `proxy`, `followredirects`, `insecure`), and `@timeout <duration>` is accepted as a shorthand.
- `@no-log` - skip storing the response body snippet for that request in history.
- `@script <kind>` followed by lines beginning with `>` - executes JavaScript either as `pre-request` (mutate method/url/headers/body/variables) or `test` blocks whose assertions appear in the UI and history. Use `> < ./path/to/file.js` to load a script from disk (paths resolve relative to the request file unless absolute).

### GraphQL

Enable GraphQL handling by adding `@graphql` to the request’s comment block. The request body captures the query, and an optional `@variables` directive switches the subsequent body lines to JSON variables (or `< file.json` to load from disk). `@operation <name>` sets the `operationName` field. Example:

```
# @graphql
# @operation FetchUser
POST https://api.example.com/graphql

query FetchUser($id: ID!) {
  user(id: $id) {
    name
  }
}

# @variables
{
  "id": "{{userId}}"
}
```

`resterm` packages this as `{ "query": ..., "variables": ... }` for POST requests (or as query parameters for GET), sets `Content-Type: application/json` when needed, and preserves the query/variables layout in previews and history.

**GraphQL metadata**
- `@graphql [true|false]` - enable (default) or turn off GraphQL processing for the request.
- `@operation <name>` (alias: `@graphql-operation`) - populate the `operationName` field.
- `@variables [< file.json]` - start a variables block. Lines following the directive are treated as JSON until another directive is encountered; use `< file.json` to load from disk.
- `@query < file.graphql>` - optional helper if you prefer to load the main query from a file instead of inlining it.

### gRPC

Switch a request into gRPC mode by starting the request line with `GRPC host:port` and declaring the method using `@grpc <package.Service>/<Method>`. Optionally provide a compiled descriptor set (`@grpc-descriptor descriptors/service.protoset`) or rely on server reflection (`@grpc-reflection true`, the default). The request body should contain protobuf JSON for the request message, or use `< payload.json` to load from disk. Example:

```
# @grpc my.pkg.UserService/GetUser
# @grpc-descriptor descriptors/user.protoset
GRPC localhost:50051

{
  "id": "{{userId}}"
}
```

Headers and `@grpc-metadata key: value` directives attach gRPC metadata. `resterm` resolves templates before invoking the call, displays headers/trailers and the JSON response, and records each invocation in history with the gRPC status code.

**gRPC metadata**
- `@grpc <package.Service>/<Method>` - specify the fully-qualified method name (package optional).
- `@grpc-descriptor <path>` - path to a compiled descriptor set (`protoc --descriptor_set_out`).
- `@grpc-reflection [true|false]` - toggle server reflection (default `true`).
- `@grpc-plaintext [true|false]` - override TLS usage for the channel.
- `@grpc-authority <value>` - set the :authority pseudo-header for HTTP/2.
- `@grpc-metadata <key>: <value>` - add unary call metadata (repeat for multiple entries).

Inline, request-, and file-level variables resolve against the selected environment file (`resterm.env.json` or `rest-client.env.json`), then fall back to OS environment variables.

## Development

Pre-requisites: Go 1.22 or newer.

History is stored in `~/.config/resterm/history.json` (using the platform-appropriate config directory). Override the location via the `RESTERM_CONFIG_DIR` environment variable.

```bash
go test ./...
go run ./cmd/resterm --file _examples/basic.http
```

## Roadmap
- Command palette & keymap customisation
- Richer response tooling (streaming previews, save-to-file, diffing)
- Better scripting support (shared helpers, setup/teardown, better assertions)
- Themes & layout configuration
- Support more curl flags
