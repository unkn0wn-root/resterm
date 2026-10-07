# Kubernetes port-forwards

Use `@k8s` to send HTTP, gRPC, WebSocket, and SSE traffic through a Kubernetes API port-forward that Resterm manages.

The syntax is `# @k8s [scope] [name] key=value ...`. For example:

- Define a reusable profile: `# @k8s global cluster-api namespace=default service=api port=http context=dev persist`
- Use it in a request: `# @k8s use=cluster-api`
- Inline one-off: `# @k8s deployment=payments port=https container=api`

Options and rules:

- `scope`: `global`, `file`, or `request` (default request). Global and file scopes define reusable profiles. Requests either reference a profile with `use=` or define inline options.
- `name`: profile tag (default `default`).
- Target fields:
  - `target=` accepts `pod:<name>`, `service:<name>`, `deployment:<name>`, `statefulset:<name>`.
  - Aliases: `pod=`, `service=` (`svc=`), `deployment=` (`deploy=`), `statefulset=` (`sts=`).
  - Exactly one target is allowed.
- Transport fields: `namespace` (`ns`), `port` (number or named port), `container`, `local_port` (`local-port`, `localport`), `address` (`bind`), `pod_running_timeout` (`pod-running-timeout`, `podwait`), `retries`, `persist` (only used at global and file scope), `context` (`kube_context`, `kube-context`), `kubeconfig` (`config`), `use`.
- Values expand templates and support `env:VAR`, which checks your shell environment variables before other scopes.
- `use=` checks file-scoped profiles first, then global ones.
- Request-level `persist` is ignored to avoid leaking background forwarders.
- `@ssh` and `@k8s` are mutually exclusive on a request.

Scopes:

- **Global** (workspace-wide): `# @k8s global cluster namespace=default service=api port=http context=kind-dev persist`
- **File** (only this `.http`): `# @k8s file edge namespace=payments deployment=api port=https`
- **Request inline** (the scope keyword is optional because request is the default): `# @k8s service=catalog port=http`
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
- `service`: uses the service selectors and picks a pod in a fixed order (running and ready pods first, then by name).
- `deployment` / `statefulset`: uses the workload's selectors and picks a pod the same way.
- For named ports:
  - Pod and workload targets look up container ports. Set `container` when more than one container could match.
  - Service targets look up the service port or targetPort, then map it to a container port when needed.

Authentication and authorization:

- Resterm loads kubeconfig the same way `client-go` does, from `$KUBECONFIG` or the default config. `kubeconfig=` overrides the path.
- `context=` selects a kube context explicitly.
- Cluster auth plugins and exec credentials follow your kubeconfig with Resterm's configured exec policy.
- RBAC still applies. You need permission to read the target resources and pods and to open port-forward sessions in the namespace.

Troubleshooting:

- `k8s target ... has no running pods`: check selectors, pod readiness, and namespace.
- `does not expose named port`: check the container and service port names and set `container=` if multiple containers export the same name.
- `service ... has no selector`: selector-less services cannot auto-resolve pods.
- Use a higher `pod_running_timeout` when pods are still starting.
