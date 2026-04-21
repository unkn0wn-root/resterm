<h1 align="center">
  <img src="_media/resterm_logo.png" alt="Resterm" width="200" />
  <br>
  Resterm
</h1>

<p align="center">
  <em>A terminal client for REST, GraphQL, gRPC, WebSocket, and SSE.</em>
</p>

<p align="center">
  <img src="_media/resterm_base.png" alt="Screenshot of Resterm TUI base" width="720" />
</p>

Resterm is a **keyboard-driven** API client that lives in your terminal and keeps everything local. It stores requests as plain files, supports **SSH tunnels**, **Kubernetes port-forwarding**, **OAuth 2.0**, and **command-backed auth**, and gives you a fast feedback loop with `history`, `diffs`, `tracing`, and `profiling`.

Quick links: [Screenshots](#screenshot-tour), [Installation](#installation), [Quick Start](#quick-start), [Features](#overview) and [Documentation](#documentation).

## Screenshot tour

<details>
<summary>See the UI in action</summary>

<p align="center">
  <strong>Light Theme</strong>
</p>

<p align="center">
  <img src="_media/resterm-lighttheme.png" alt="Screenshot of Resterm Light Theme" width="720" />
</p>

<p align="center">
  <strong>Workflows</strong>
</p>

<p align="center">
  <img src="_media/resterm_workflow.png" alt="Screenshot of Resterm with Workflow" width="720" />
</p>

<p align="center">
  <strong>Trace and Timeline</strong>
</p>

<p align="center">
  <img src="_media/resterm_trace_timeline.png" alt="Screenshot of Resterm with timeline" width="720" />
</p>

<p align="center">
  <strong>Profiler</strong>
</p>

<p align="center">
  <img src="_media/resterm_profiler.png" alt="Screenshot of Resterm profiler" width="720" />
</p>

<p align="center">
  <strong>Explain</strong>
</p>

<p align="center">
  <img src="_media/resterm_explain.png" alt="Screenshot of Resterm Explain Tab" width="720" />
</p>

<p align="center">
  <strong>RestermScript</strong>
</p>

<p align="center">
  <img src="_media/resterm_script.png" alt="Screenshot of Resterm with RestermScript" width="720" />
</p>

<p align="center">
  <strong>OAuth browser demo (old UI design)</strong>
</p>

<p align="center">
  <img src="_media/oauth.gif" alt="Resterm OAuth flow" width="720" />
</p>

</details>

## Why Resterm

- Requests live in plain `.http` / `.rest` files.
- **Conditional logic** - `@when`, `@skip-if`, `@if`/`@elif`/`@else`, `@switch`/`@case`, and `@for-each`.
- **Multi-step workflows** with `@workflow` / `@step`.
- **Captures, variables, and assertions** (`@capture`, `@var`, `@assert`).
- **RestermScript** - a small, safe expression language purpose.
- **OAuth 2.0** (client credentials, password, auth code + PKCE), **command-backed auth** via existing CLIs, **SSH tunnels**, and **Kubernetes port-forwards** are built in - no extra tools.
- **CLI** with `resterm run` for requests, workflows, JSON/JUnit output, and reusable run artifacts.
- **Timeline tracing**, **profiling**, and **compare runs** across environments.
- **Streaming transcripts** and an interactive console for WebSocket and SSE sessions.
- No cloud sync, no accounts, no telemetry. Everything stays local.
- There is no AI integration and there will never be.

## Headless

Resterm ships with the engine API, so you can build your own headless runner for Resterm and embed request execution in your own Go tooling/code, CI flows or internal automation.

The public Go API lives in the [`headless`](./headless) package and can run requests, workflows, assertions, compare runs, and profiles without the TUI.

> [!NOTE]
> If you want an "off-the-shelf" runner instead of building your own, check out [resterm-runner](https://github.com/unkn0wn-root/resterm-runner).

## CLI

Resterm also ships with a built-in CLI: `resterm run`.

Use `resterm run` when you want to execute `.http` / `.rest` files directly from the terminal without opening the TUI.

**Example**:

```bash
mkdir my-api && cd my-api
resterm init
resterm run --request Echo requests.http
```

`resterm init` creates `requests.http` with sample requests and `resterm.env.json` with a `dev` environment that points at `httpbin.org`. The command above runs the **Echo** request which POSTs a JSON body and prints the response.

> [!NOTE]
> This is different from the `headless` package. The `headless` package is the embeddable Go API for building your own runner or CI integration, while `resterm run` is the built-in CLI on top of the same execution engine.

See the full [CLI documentation](docs/cli.md) for usage, selectors, output formats, and examples.


## Quick Start

1. Install Resterm using the command that matches your OS.

   ```bash
   # Linux/macOS (Homebrew)
   brew install resterm

   # Linux (install script)
   curl -fsSL https://raw.githubusercontent.com/unkn0wn-root/resterm/main/install.sh | bash
   ```

   ```powershell
   # Windows (PowerShell)
   iwr -useb https://raw.githubusercontent.com/unkn0wn-root/resterm/main/install.ps1 | iex
   ```

2. Bootstrap a tiny workspace.

   ```bash
   mkdir my-api && cd my-api
   resterm init
   ```

3. Run it and send your first request.

   ```bash
   resterm
   ```

   Press `Ctrl+Enter` in the editor to send the highlighted request.

If you do not want files yet, just run `resterm`, type a URL in the editor, and press `Ctrl+Enter`.
If you already have a curl command, paste it into the editor and press `Ctrl+Enter` to import it.

## Key files

These are the files you will see most often in a Resterm workspace.

- `.http` / `.rest` files store your requests, metadata, and named steps.
- `resterm.env.json` holds environment variables and secrets for one or more environments.
- `resterm.env.example.json` is a safe template you can commit and share.
- `.rts` files contain RestermScript helpers for reuse and small bits of logic.
- `RESTERM.md` is optional project notes created by `resterm init`.

## Overview

Protocols and transports cover the following.

- `HTTP`, `GraphQL`, `gRPC`, `WebSocket`, and `SSE` are supported out of the box.
- `SSH tunnels` include host key verification, keep-alives, retries, and persistent sessions.
- `Kubernetes port-forwards` target pods, services, deployments, and statefulsets with named port resolution, pod readiness checks, retries, and persistent sessions.

Workspace and files focus on the following.

- `.http` / `.rest` files are discovered automatically and the request list stays in sync as you edit.
- The request editor includes inline syntax highlighting, search, and contextual metadata hints.
- Environment files are treated as first-class input. `resterm init` adds _resterm.env.json_ to _.gitignore_ but any custom env files passed via the CLI are **not** ignored automatically.

Automation and reuse include the following.

- Variables, captures, constants, and multi-step workflows.
- RestermScript (RTS) and optional JavaScript hooks handle request-time logic.
- Curl commands and OpenAPI specs can be imported directly into request collections.

Debugging and inspection provide the following.

- `Pretty/raw/header/diff/history` views are available with split panes.
- Streaming transcripts are recorded and an interactive console is available for WebSocket and SSE.
- Timeline tracing and profiling help find slow or flaky endpoints.

Security and auth cover the following.

- OAuth 2.0 client credentials, password grant, and authorization code + PKCE are supported.
- Command-backed auth can reuse existing CLIs without wrapping a shell.
- Tokens can be cached per environment, and OAuth tokens are refreshed automatically.

Customization includes the following.

- Custom themes and keybindings.

## Navigation & layout cheat sheet

A few keys that make Resterm feel “native” quickly:

- **Pane focus & layout**
  - `Tab` / `Shift+Tab` - move focus between sidebar, editor, and response.
  - `g+r` - jump to **Requests** (sidebar).
  - `g+i` - jump to **Editor**.
  - `g+p` - jump to **Response**.
  - `g+h` / `g+l` - adjust layout:
    - When the **left pane** (sidebar) is focused: change sidebar width.
    - Otherwise: change editor/response split.
  - `g+v` / `g+s` - toggle response pane between inline and stacked layout.
  - `g+1`, `g+2`, `g+3` + minimize/restore sidebar, editor, response.
  - `g+z` / `g+Z` - zoom the focused pane / clear zoom.

- **Environments & globals**
  - `Ctrl+E` - switch environments.
  - `Ctrl+G` - inspect captured globals.

- **Working with responses**
  - `Ctrl+V` / `Ctrl+U` - split the response pane for side-by-side comparison.
  - When response pane is focused:
    - `Ctrl+Shift+C` or `g y` - copy the entire Pretty/Raw/Headers tab
      to the clipboard (no mouse selection needed).
  - `g x` - build an Explain preview for the active request without sending it.

---

> [!TIP]
> **If you only remember three shortcuts…**
> - `Ctrl+Enter` - send request
> - `Tab` / `Shift+Tab` - switch panes
> - `g+p` - jump to response

## Installation

**Linux / macOS (Homebrew)**

```bash
brew install resterm
```

> [!NOTE]
> Homebrew installs should be updated with Homebrew (`brew upgrade resterm`). The built-in `resterm --update` command is for binaries installed from GitHub releases or install scripts.

**Linux / macOS (Shell script)**

> [!IMPORTANT]
> Pre-built Linux binaries depend on glibc **2.32 or newer**. If you run an older distro, build from source on a machine with a newer glibc toolchain or upgrade glibc before using the release archives.

```bash
curl -fsSL https://raw.githubusercontent.com/unkn0wn-root/resterm/main/install.sh | bash
```

or with `wget`:

```bash
wget -qO- https://raw.githubusercontent.com/unkn0wn-root/resterm/main/install.sh | bash
```

**Windows (PowerShell)**

```powershell
iwr -useb https://raw.githubusercontent.com/unkn0wn-root/resterm/main/install.ps1 | iex
```

These scripts detect your architecture, download the latest release, and install the binary.

### Manual installation

> [!NOTE]
> The manual install helper uses `curl` and `jq`. Install `jq` with your package manager (`brew install jq`, `sudo apt install jq`, etc.).

**Linux / macOS**

```bash
# Detect latest tag
LATEST_TAG=$(curl -fsSL https://api.github.com/repos/unkn0wn-root/resterm/releases/latest | jq -r .tag_name)

# Download the matching binary (Darwin/Linux + amd64/arm64)
curl -fL -o resterm "https://github.com/unkn0wn-root/resterm/releases/download/${LATEST_TAG}/resterm_$(uname -s)_$(uname -m)"

# Make it executable and move it onto your PATH
chmod +x resterm
sudo install -m 0755 resterm /usr/local/bin/resterm
```

**Windows (PowerShell)**

```powershell
$latest = Invoke-RestMethod https://api.github.com/repos/unkn0wn-root/resterm/releases/latest
$asset  = $latest.assets | Where-Object { $_.name -like 'resterm_Windows_*' } | Select-Object -First 1
Invoke-WebRequest -Uri $asset.browser_download_url -OutFile resterm.exe
# Optionally relocate to a directory on PATH, e.g.:
Move-Item resterm.exe "$env:USERPROFILE\bin\resterm.exe"
```

### From source

```bash
go install github.com/unkn0wn-root/resterm/cmd/resterm@latest
```

## Update

```bash
resterm --check-update
resterm --update
```

The first command reports whether a newer release is available. The second downloads and installs it (Windows users receive a staged binary to swap on restart).

## Quick Configuration Overview

- Environments are JSON files (`resterm.env.json`) discovered in the request directory, workspace root, or CWD. Dotenv files (`.env`, `.env.*`) are opt-in via `--env-file` and are single-workspace. Prefer JSON when you need multiple environments in one file.
- Flags you probably reach for most are `--workspace`, `--file`, `--env`, `--env-file`, `--timeout`, `--insecure`, `--follow`, `--proxy`, `--recursive`, `--from-curl`, `--from-openapi`, and `--http-out`.
- Config is stored at `$HOME/Library/Application Support/resterm`, `%APPDATA%\resterm`, or `$HOME/.config/resterm` and can be overridden with `RESTERM_CONFIG_DIR`.

## Collections

You can export a workspace into a Git-friendly Resterm bundle. The exported bundle always includes a `manifest.json` with checksums, so imports can verify file integrity before writing anything.

When Resterm sees `resterm.env.example.json` in your workspace, it includes that file as-is. If only `resterm.env.json` exists, Resterm generates `resterm.env.example.json` automatically and replaces values with `REPLACE_ME` placeholders so secrets are not exported.

### Export a bundle

```bash
resterm collection export \
  --workspace ./my-api \
  --out ./shared/my-api-bundle \
  --recursive \
  --name "my-api-v1"
```

### Import a bundle

```bash
resterm collection import \
  --in ./shared/my-api-bundle \
  --workspace ./my-local-api
```

If you want to preview actions first, you can run:

```bash
resterm collection import \
  --in ./shared/my-api-bundle \
  --workspace ./my-local-api \
  --dry-run
```

If files already exist and should be replaced intentionally, add `--force`.

## Inline curl import

Paste a curl command into the editor and press `Ctrl+Enter` to convert it into a structured request. Resterm understands common flags, merges repeated data segments, and keeps multipart uploads intact.

```bash
curl \
  --compressed \
  --url "https://httpbin.org/post?source=resterm&case=multipart" \
  --request POST \
  -H "Accept: application/json" \
  -H "X-Client: resterm-dev" \
  --user resterm:test123 \
  -F file=@README.md \
  --form-string memo='Testing resterm inline curl
with multiline value' \
  --form-string meta='{"env":"test","attempt":1}'
```

If you copied the command from a shell, prefixes like `sudo` or `$` are ignored automatically.

## RestermScript

RestermScript (RTS) is a small expression language built specifically for Resterm. Because it targets Resterm features directly, it can evolve alongside Resterm and stay symbiotic with the request format, workflows, and directives. JavaScript hooks are still available when you want them, but RTS is the default because it is readable, predictable, and focused on Resterm’s domain.

Quick example (RTS module + request):

```rts
// rts/helpers.rts
module helpers
export fn authHeader(token) {
  return token ? "Bearer " + token : ""
}
```

```http
# @use ./rts/helpers.rts
# @when env.has("feature")
# @assert response.statusCode == 200
GET https://api.example.com/users/{{= vars.get("user") }}
Authorization: {{= helpers.authHeader(vars.get("auth.token")) }}
```

Full reference: [`docs/restermscript.md`](docs/restermscript.md).

## Deep dive

#### OAuth 2.0

Resterm supports client credentials, password grant, and authorization code + PKCE. For auth code flows, it opens your system browser, spins up a local callback server on `127.0.0.1`, captures the redirect, and exchanges the code automatically. Tokens are cached per environment and refreshed when they expire. Docs: [`docs/resterm.md#oauth-20-directive`](./docs/resterm.md#oauth-20-directive) and `_examples/oauth2.http`.

#### Workflows and scripting

Chain requests with `@workflow` and `@step`, pass data between steps, and add lightweight JS hooks where needed. Docs + sample: [`docs/resterm.md#workflows-multi-step-workflows`](./docs/resterm.md#workflows-multi-step-workflows) and `_examples/workflows.http`.

#### Compare runs

Run the same request across environments with `@compare` or `--compare`, then diff responses side by side with `g+c`. Docs: [`docs/resterm.md#compare-runs`](./docs/resterm.md#compare-runs).

#### Tracing and timeline

Add `@trace` with budgets to capture DNS, connect, TLS, TTFB, and transfer timings. Resterm visualizes overruns and can export spans to OpenTelemetry. Docs: [`docs/resterm.md#timeline--tracing`](./docs/resterm.md#timeline--tracing).

#### Streaming (WebSocket and SSE)

Use `@websocket` with `@ws` steps or `@sse` to script and record streams. The Stream tab keeps transcripts and includes an interactive console. Docs: [`docs/resterm.md#streaming-sse--websocket`](./docs/resterm.md#streaming-sse--websocket).

#### gRPC

Resterm supports unary and streaming calls with transcripts, metadata, and body expansion for gRPC files. Docs: [`docs/resterm.md#grpc`](./docs/resterm.md#grpc).

#### OpenAPI import

Convert OpenAPI 3 specs into Resterm-ready `.http` collections from the CLI with `--from-openapi`. Docs: [`docs/resterm.md#importing-openapi-specs`](./docs/resterm.md#importing-openapi-specs).

#### Collection sharing

Export a portable Resterm-native bundle with `resterm collection export` and import it into another workspace with `resterm collection import`. Docs: [`docs/resterm.md#collection-sharing`](./docs/resterm.md#collection-sharing).

#### Curl import

Convert curl commands into `.http` files from the CLI with `--from-curl`. Docs: [`docs/resterm.md#importing-curl-commands`](./docs/resterm.md#importing-curl-commands).

#### SSH tunnels

Route HTTP, gRPC, WebSocket, and SSE traffic through bastions with `@ssh` profiles. Docs: [`docs/resterm.md#ssh-tunnels`](./docs/resterm.md#ssh-tunnels) and `_examples/ssh.http`.

#### Kubernetes port-forwards

Route HTTP, gRPC, WebSocket, and SSE traffic through Kubernetes with `@k8s` profiles targeting pods/services/deployments/statefulsets. Docs: [`docs/resterm.md#kubernetes-port-forwards`](./docs/resterm.md#kubernetes-port-forwards) and `_examples/k8s.http`.

#### Theming and bindings

Customize colors and keybindings via `themes/*.toml` and `bindings.toml` or `bindings.json` under the config directory. Docs: [`docs/resterm.md#theming`](./docs/resterm.md#theming) and [`docs/resterm.md#custom-bindings`](./docs/resterm.md#custom-bindings).

## Documentation

The full reference, including request syntax, metadata, directive tables, scripting APIs, transport settings, and advanced workflows, lives in [`docs/resterm.md`](./docs/resterm.md).
The command-line guide for `resterm run`, importers, collection tooling, and history maintenance lives in [`docs/cli.md`](./docs/cli.md).
