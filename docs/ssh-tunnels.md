# SSH tunnels

Use `@ssh` to send HTTP, gRPC, WebSocket, and SSE traffic through an SSH bastion.

The syntax is `# @ssh [scope] [name] key=value ...`. For example:

- Define a reusable profile: `# @ssh global bastion host=jump.example.com user=ops key=~/.ssh/id_ed25519 persist`
- Use it in a request: `# @ssh use=bastion`
- Inline one-off: `# @ssh host=10.0.0.5 user=svc password=env:SSH_PW`

Options and rules:

- `scope`: `global`, `file`, or `request` (default request). Global and file scopes define reusable profiles. Requests either reference a profile with `use=` or define inline options.
- `name`: profile tag (default `default`).
- Fields: `host` (required), `port` (default 22), `user`, `password`, `key`, `passphrase`, `agent` (default true when `SSH_AUTH_SOCK` is present), `known_hosts` (default `~/.ssh/known_hosts`), `strict_hostkey` (default true), `persist` (only used at global and file scope), `timeout`, `keepalive`, `retries`, `use` (profile selection).
- Values expand templates and support `env:VAR`, which checks your shell environment variables before other scopes. Paths for `key` and `known_hosts` expand `~` and environment variables.
- `key` is optional. Resterm uses your SSH agent if there is one, or falls back to the default keys (`~/.ssh/id_ed25519`, `id_rsa`, `id_ecdsa`). See [Default key detection](#default-key-detection) below.
- Global profiles are shared across the workspace. File-scoped profiles override global ones when the names match. `use=` resolves file profiles first, then globals.
- Request-level `persist` is ignored to avoid leaking tunnels. Strict host key checking is on by default. `strict_hostkey=false` is allowed but insecure.

Scopes:

- **Global** (workspace-wide): `# @ssh global bastion host=jump.example.com user=ops key=~/.ssh/id_ed25519 persist`
- **File** (only this `.http`): `# @ssh file edge host=10.0.0.5 user=ops`
- **Request inline** (the scope keyword is optional because request is the default): `# @ssh host=192.168.1.50 user=svc password=env:SSH_PW timeout=12s`
- **Reference** a profile: `# @ssh use=bastion` (picks the file-scoped profile first, then the global one)

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

## How it works

SSH tunneling works at the transport layer, so other features work over it as usual. That includes `@trace`, `@profile`, `@workflow`, `@sse`, `@websocket`, `@graphql`, and `@grpc`.

```text
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

## Comparison with terminal tunnels

What you would do by hand:

```bash
# Create tunnel in terminal
ssh -L 8080:10.0.0.100:80 ops@bastion.example.com
curl http://localhost:8080/api/users  # in another terminal
```

What Resterm does for you:

```http
# @ssh global tunnel host=bastion.example.com user=ops key=~/.ssh/id_ed25519 persist

### Hit pod directly through tunnel
# @ssh use=tunnel
GET http://10.0.0.100/api/users
```

The difference:

- With a terminal tunnel, you bind a local port, then call `localhost:port`.
- With Resterm, you call the internal IP directly (`10.0.0.100`). Resterm dials through the SSH tunnel to reach it.

You can reach Kubernetes pods, private VPC resources, or any other internal service behind a bastion host the same way. The `persist` option keeps the SSH connection open, so later requests reuse it instead of reconnecting.

## Default key detection

When no `key` is set, Resterm tries these paths in order:

1. `~/.ssh/id_ed25519`
2. `~/.ssh/id_rsa`
3. `~/.ssh/id_ecdsa`

If `SSH_AUTH_SOCK` is set, the SSH agent is also used by default.
