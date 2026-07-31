# Kubernetes Port-Forwards

Use `@k8s` to create and reuse a port-forward for a pod, service, deployment, or statefulset.

```http
# @k8s namespace=default service=api port=http
GET http://api/health
```

`port` accepts a number or a named port and `context` selects a kube context. Define profiles at `global` or `file` scope and reference them with `@k8s use=profile-name`. `@ssh` and `@k8s` cannot be combined on one request.
