# SSH Tunnels

Route HTTP, gRPC, WebSocket, or SSE traffic through a jump host with `@ssh`. Resterm dials internal addresses through the tunnel, so requests target them directly.

```http
# @ssh request host=bastion.example.com user=ops key=~/.ssh/id_ed25519
GET http://internal-api/health
```

Define reusable profiles at `global` or `file` scope and reference them with `@ssh use=profile-name`. Add `persist` to keep the connection alive between requests.
