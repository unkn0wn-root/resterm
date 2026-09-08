package intellisense

import "github.com/unkn0wn-root/resterm/internal/directive"

func addTransportArgs(c argumentCatalog) {
	c.add(args{named: sseArgs}, directive.SSE)
	c.add(args{named: webSocketArgs}, directive.WebSocket)
	c.add(args{named: wsArgs}, directive.WS)
	c.add(args{named: sshArgs}, directive.SSH)
	c.add(args{named: k8sArgs}, directive.K8s)
	c.add(
		args{value: directiveValue(filePath("descriptor", "Descriptor set file", PathAny, pathLoad))},
		directive.GRPCDescriptor,
	)
	c.add(
		args{value: directiveValue(flag("enabled", "Toggle gRPC reflection")), single: true},
		directive.GRPCReflection,
	)
	c.add(args{value: directiveValue(flag("enabled", "Force plaintext gRPC")), single: true}, directive.GRPCPlaintext)
}

var sseArgs = []argument{
	opt("timeout", "Total stream timeout", "30s").alias("duration"),
	opt("idle", "Idle timeout between events", "10s").alias("idle-timeout"),
	opt("max-events", "Stop after N events", "100"),
	opt("max-bytes", "Stop after N bytes", "1mb").alias("limit-bytes"),
	opt("max-line-bytes", "Largest SSE line to buffer", "1mb"),
	opt("max-event-bytes", "Largest SSE event to buffer", "1mb"),
	word("off", "Disable SSE for this request"),
}

var webSocketArgs = []argument{
	opt("timeout", "Handshake deadline", "10s"),
	opt("idle-timeout", "Idle timeout (resets on any activity)", "5s").alias("idle"),
	opt("max-message-bytes", "Max inbound frame size", "1mb"),
	opt("subprotocols", "Comma-separated subprotocols", "chat,json"),
	flag("compression", "Toggle WebSocket compression"),
}

var wsArgs = []argument{
	word("send", "Send a text frame"),
	word("send-json", "Send a JSON frame"),
	word("send-base64", "Send base64-decoded binary data"),
	word("send-file", "Send file contents").takes(pathValue(PathAny, pathLoad)),
	word("ping", "Send a ping frame"),
	word("pong", "Send a pong frame"),
	word("wait", "Wait for a duration (e.g. 500ms)").withExample("500ms"),
	word("close", "Close the connection with code and reason"),
}

var sshArgs = []argument{
	opt("host", "Jump host (supports env:VAR and templates)", "env:SSH_HOST"),
	opt("port", "Port (default 22)", "22"),
	opt("user", "SSH user", "ops"),
	opt("password", "Password auth", "env:SSH_PW").alias("pass"),
	filePath("key", "Private key path", PathAny, pathWord).home(),
	opt("passphrase", "Key passphrase", "env:SSH_KEY_PW"),
	flag("agent", "Use the SSH agent"),
	filePath("known_hosts", "Known hosts file", PathAny, pathWord).home().alias("known-hosts"),
	flag("strict_hostkey", "Toggle host key checking").alias("strict-hostkey", "strict_host_key"),
	flag("persist", "Keep the tunnel open"),
	opt("timeout", "SSH dial timeout", "15s"),
	opt("keepalive", "Server keepalive interval", "30s"),
	opt("retries", "Retry count for tunnel attach", "2"),
	optValue("use", "Reference a named SSH profile", namesValue(func(_ Context, sc Scope) ([]string, string) {
		return sc.Profiles.SSH, "ssh profile"
	})),
}

var k8sArgs = []argument{
	opt("target", "Target ref (pod:/service:/deployment:/statefulset:)", "pod:api-server"),
	opt("namespace", "Kubernetes namespace (default: default)", "default"),
	opt("pod", "Pod name for port-forward target", "api-server"),
	opt("service", "Service name for target pod selection", "api"),
	opt("deployment", "Deployment name for target pod selection", "api"),
	opt("statefulset", "StatefulSet name for target pod selection", "db"),
	opt("port", "Remote port (number or named port)", "8080"),
	opt("context", "Kubeconfig context override", "dev-cluster"),
	filePath("kubeconfig", "Kubeconfig path override", PathAny, pathWord).home().alias("config"),
	opt("container", "Container name in selected pod", "api"),
	opt("local_port", "Local port to bind (optional)", "18080"),
	opt("address", "Local bind address", "127.0.0.1"),
	opt("pod_running_timeout", "Wait timeout for running pod", "20s"),
	opt("retries", "Retry count for forward attach", "2"),
	optValue("use", "Reference a named Kubernetes profile", namesValue(func(_ Context, sc Scope) ([]string, string) {
		return sc.Profiles.K8s, "k8s profile"
	})),
	flag("persist", "Keep the forwarder open"),
}
