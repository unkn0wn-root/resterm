# resterm mock

Serve mock response blocks from one request file or a workspace:

```bash
resterm mock ./api.http
resterm mock --recursive --addr 127.0.0.1:9090 ./requests
resterm mock --source users.http --source payments.http
resterm mock --source users.http,payments.http ./workspace
```

| Flag | Short | Description |
| --- | --- | --- |
| `--addr <host:port>` | `-a` | Listen address (default `127.0.0.1:8080`). |
| `--source <file>` | `-s` | Serve only these request files. Repeatable, and accepts comma-separated lists. |
| `--cors <policy>` |  | `auto`, `off`, `*`, or a comma-separated origin allowlist. |
| `--tls-cert <file>` |  | Serve HTTPS using this PEM certificate (requires `--tls-key`). |
| `--tls-key <file>` |  | PEM private key for `--tls-cert`. |
| `--recursive` | `-r` | Scan nested workspace directories. |
| `--watch` | `-w` | Reload source files and referenced body fixtures (enabled by default). |
| `--quiet` | `-q` | Hide per-request access summaries. |
| `--sequence-key-limit <n>` |  | Maximum distinct keys kept by each keyed sequence (default `10000`). |
| `--journal-entries <n>` |  | Maximum requests kept for verification (default `2000`). |
| `--journal-bytes <size>` |  | Total size limit for data kept in the verification journal (default `16MiB`). |
| `--journal-body-limit <size>` |  | Body bytes kept per journaled request (default `64KiB`). |

`--source` entries must be `.http` or `.rest` files. They resolve against the positional directory (default `.`) and must stay inside it, because file-based response bodies are confined to that root. `--source` cannot be combined with `--recursive`.

When listening on a loopback address such as localhost, `--cors=auto` allows browser requests from any origin and handles CORS preflights automatically. On other addresses, it disables CORS and warns that the server is exposed. Use `--cors=off`, `--cors='*'`, or a comma-separated list of allowed origins to override it.

Reloads replace the routes in one operation. If an edit is invalid, Resterm reports it and keeps the last valid routes serving. Stop the server with `Ctrl+C` or `SIGTERM`; requests already in progress get a short grace period to finish.

The server exits `0` when you stop it. It exits `2` for invalid flags, arguments, or `--source` values, and `1` when the request files fail to load, contain no `# @mock` blocks, or the server cannot start.

`--tls-cert` and `--tls-key` switch the server to HTTPS. Resterm does not generate certificates. Bring your own pair, for example with [mkcert](https://github.com/FiloSottile/mkcert):

```bash
mkcert -install
mkcert 127.0.0.1 localhost
resterm mock --tls-cert ./127.0.0.1+1.pem --tls-key ./127.0.0.1+1-key.pem ./requests
```

`mkcert -install` adds the local CA to supported system trust stores, which Resterm uses by default. For explicit per-request trust, find the public `rootCA.pem` in the directory printed by `mkcert -CAROOT`, copy it next to the request file, and add:

```http
# @setting http-root-cas ./rootCA.pem
```

Relative CA paths resolve from the request file. Do not copy or share `rootCA-key.pem`.

## Mock operations

Use the commands below to reset sequences, clear logs, or verify calls on a standalone mock server. They connect through a control channel reachable only from loopback. This channel is for Resterm's commands and never exposes raw journal entries. The TUI's mock server does not enable it.

The path `/.resterm/` is reserved for these operations. Mock routes cannot use it. While the control channel is enabled, its endpoints take priority over wildcard routes that would match the same paths.

```bash
# Reset all sequences, or every sequence named polling.
resterm mock reset
resterm mock reset polling

# Clear the verification journal and request logs.
resterm mock clear

# Check # @expect declarations loaded from a file or workspace.
resterm mock verify payments.http
resterm mock verify --recursive .
resterm mock verify --source users.http,payments.http
```

The operations take these flags:

| Flag | Short | Description |
| --- | --- | --- |
| `--url <url>` | `-u` | Mock server URL (default `http://127.0.0.1:8080`). |
| `--timeout <duration>` | `-t` | Timeout for each control request (default `5s`). |
| `--insecure` | `-k` | Skip HTTPS certificate verification. |
| `--recursive` | `-r` | `verify` only. Scan nested workspace directories. |
| `--source <file>` | `-s` | `verify` only. Check only these request files. Repeatable, and accepts comma-separated lists. |

Flags can go on either side of the optional sequence or source argument, for example:

```bash
resterm mock reset polling --url http://127.0.0.1:9090
resterm mock verify payments.http --url https://localhost:9443 --insecure
```

The URL must contain only the `http` or `https` scheme and host. These commands do not support proxy base paths, by design. Source files or directories named `reset`, `clear`, or `verify` should be passed with an explicit path such as `./reset` so they are not interpreted as operations.

`reset` and `clear` exit `0` on success, `1` when the server cannot be reached or `reset` names a sequence that does not exist, and `2` for invalid usage. `verify` exits `0` when every exact call count passes, `1` for mismatches, an incomplete journal, or a connection failure, and `2` for invalid usage, an invalid source, or a missing `@expect` declaration. Operational requests are excluded from both request counts and access logs.

See [Mock Servers](../mock-servers.md) for the response-block syntax, matching rules, selectors, and TUI commands.
