# SSH tunnels

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

## How it works

SSH tunneling operates at the transport layer, making it transparent to all other features (`@trace`, `@profile`, `@workflow`, `@sse`, `@websocket`, `@graphql`, `@grpc`, etc.).

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

## Default key detection

When no `key` is specified, resterm automatically tries these paths in order:

1. `~/.ssh/id_ed25519`
2. `~/.ssh/id_rsa`
3. `~/.ssh/id_ecdsa`

If `SSH_AUTH_SOCK` is set, the SSH agent is also used by default.
