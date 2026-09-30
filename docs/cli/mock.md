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
| `--sequence-key-limit <n>` |  | Maximum distinct keys retained by each keyed sequence (default `10000`). |
| `--journal-entries <n>` |  | Maximum requests retained for verification (default `2000`). |
| `--journal-bytes <size>` |  | Total retained-data budget for the verification journal (default `16MiB`). |
| `--journal-body-limit <size>` |  | Body bytes retained per journaled request (default `64KiB`). |

`--source` entries must be `.http` or `.rest` files. They resolve against the positional directory (default `.`) and must stay inside it, because file-based response bodies are confined to that root. `--source` cannot be combined with `--recursive`.

On loopback, `--cors=auto` enables wildcard CORS and automatic preflight responses. On a non-loopback bind it disables CORS and prints an exposure warning. Use `--cors=off`, `--cors='*'`, or a comma-separated origin allowlist to override it. Reloads are atomic: invalid edits are reported and the last valid route set stays live. Stop the server with `Ctrl+C` or `SIGTERM`. In-flight requests get a short grace period to finish.

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

A running standalone mock server exposes a narrow loopback-only control channel for Resterm's own operational commands. It is not a general mock administration API. The TUI-owned server does not enable it, and it never exposes raw journal entries. The literal `/.resterm/` path namespace is reserved for these endpoints: mocks cannot declare routes inside it, and wildcard routes that overlap it are shadowed while the control channel is enabled.

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

The operations connect to `http://127.0.0.1:8080` by default. Each accepts `--url`, `--timeout`, and `--insecure`, and `verify` also accepts `--recursive` and `--source`. Flags may go on either side of the optional sequence or source argument, for example:

```bash
resterm mock reset polling --url http://127.0.0.1:9090
resterm mock verify payments.http --url https://localhost:9443 --insecure
```

The URL must contain only the `http` or `https` scheme and host. Operational commands intentionally do not support proxy base paths. Source files or directories named `reset`, `clear`, or `verify` should be passed with an explicit path such as `./reset` so they are not interpreted as operations.

`verify` exits `0` when every exact call count passes, `1` for mismatches, an incomplete journal, or a connection failure, and `2` for invalid usage, an invalid source, or a missing `@expect` declaration. Operational requests are excluded from both request counts and access logs.

See [Mock Servers](../mock-servers.md) for the response-block syntax, matching rules, selectors, and TUI commands.
