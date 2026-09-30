# Kubernetes port-forwards

Use `@k8s` to route HTTP/gRPC/WebSocket/SSE traffic through a Kubernetes API port-forward managed by Resterm.

**Syntax:** `# @k8s [scope] [name] key=value ...`

- **TL;DR**:
  - Define a reusable profile: `# @k8s global cluster-api namespace=default service=api port=http context=dev persist`
  - Use it in a request: `# @k8s use=cluster-api`
  - Inline one-off: `# @k8s deployment=payments port=https container=api`

- `scope`: `global`, `file`, or `request` (default request). Global/file scopes define reusable profiles. Requests either reference a profile with `use=` or define inline options.
- `name`: profile tag (default `default`).
- Target fields:
  - `target=` accepts `pod:<name>`, `service:<name>`, `deployment:<name>`, `statefulset:<name>`.
  - Aliases: `pod=`, `service=` (`svc=`), `deployment=` (`deploy=`), `statefulset=` (`sts=`).
  - Exactly one target is allowed.
- Transport fields: `namespace` (`ns`), `port` (number or named port), `container`, `local_port`, `address`, `pod_running_timeout`, `retries`, `persist` (only honored for global/file), `context`, `kubeconfig`, `use`.
- Values expand templates and support `env:VAR` to prefer terminal env vars before other scopes.
- `use=` resolves file-scoped profile first, then globals.
- Request-level `persist` is ignored to avoid leaking background forwarders.
- `@ssh` and `@k8s` are mutually exclusive on a request.

Scopes:

- **Global** (workspace-wide): `# @k8s global cluster namespace=default service=api port=http context=kind-dev persist`
- **File** (only this `.http`): `# @k8s file edge namespace=payments deployment=api port=https`
- **Request inline** (scope keyword optional because request is default): `# @k8s service=catalog port=http`
- **Reference** a profile: `# @k8s use=cluster`

Examples:

```http
# @k8s global cluster-api namespace=default service=api port=http context=kind-dev persist retries=2

### API through service
# @k8s use=cluster-api
GET http://api.default.svc.cluster.local/v1/health
```

Named port with deployment target:

```http
### Deployment target
# @k8s deployment=payments-api port=https container=api pod_running_timeout=30s
POST https://payments.internal/v1/charge
Content-Type: application/json

{"amount": 100}
```

gRPC over Kubernetes port-forward:

```http
# @k8s namespace=default pod=grpc-api-0 port=grpc
# @grpc testservices.inventory.ProjectService/Seed
# @grpc-descriptor ./proto/inventory.protoset
GRPC passthrough:///grpc-api.default.svc.cluster.local:8082

{}
```

How target resolution works:

- `pod`: forwards to that pod directly.
- `service`: resolves service selectors and chooses a deterministic pod (running/ready preferred, then by name).
- `deployment` / `statefulset`: resolves selectors from the workload and picks a deterministic pod with the same policy.
- For named ports:
  - pod/workload targets resolve against container ports (`container` can disambiguate).
  - service targets resolve service port or targetPort, then map to container port when needed.

Authentication and authorization:

- Resterm uses `client-go` kubeconfig loading (`$KUBECONFIG`/default config, optional `kubeconfig=` override).
- `context=` selects a kube context explicitly.
- Cluster auth plugins and exec credentials follow your kubeconfig with Resterm's configured exec policy.
- RBAC still applies. You need permission to read target resources/pods and open pod port-forward sessions in the namespace.

Troubleshooting:

- `k8s target ... has no running pods`: check selectors, pod readiness, and namespace.
- `does not expose named port`: verify container/service port names and set `container=` if multiple containers export the same name.
- `service ... has no selector`: selector-less services cannot auto-resolve pods.
- Use a higher `pod_running_timeout` when pods are still starting.
