# Security

## Request files can run commands and scripts

A `.http` or `.rest` file can:

- run a command through `@auth command`
- read files inside or outside the workspace
- run JavaScript from script and test blocks
- write files through `response.saveBody`
- connect to hosts through a proxy, SSH tunnel, or Kubernetes port-forward

Resterm does not keep these actions inside the workspace. Review files you did not write before running them. Only run request files, imported collections, and `.rts` files from sources you trust.

## Safety limits

- **Scripts.** Each script block stops after 30 seconds or when the run is canceled. Script memory has no limit. See [JavaScript hooks](scripting.md).
- **Redirects.** When a redirect goes to another origin, Resterm removes credential headers and any custom headers named by `@auth`. It also removes the path and query from the `Referer`, so a key in the query does not travel to the new origin. If the chain moves from HTTPS to HTTP, Resterm stops sending credentials for the rest of that chain. Resterm cannot remove secrets from a request body. A 307 or 308 redirect can send the same body to the new target. OAuth token requests cannot leave the token endpoint's origin. See [HTTP transport and settings](http-settings.md).
- **Streams.** A stream can end normally when it reaches a configured limit. If it ends because of an error, the request fails. Resterm still saves and reports the events it collected. See [WebSocket and SSE](streaming.md#limits).
- **Response size.** Response bodies, SSE lines and events, WebSocket messages, and saved stream data have size limits. See [HTTP transport and settings](http-settings.md#other-http-settings).
- **Secrets in output.** Resterm masks secrets it knows about and credential headers in history, explain output, and errors. [Telemetry](#telemetry) has its own rules.

## Telemetry

When `@trace` sends data to an OTLP collector, the collector receives the host and path of each request. Resterm replaces query values with `REDACTED` and removes usernames, passwords, and fragments from URLs. It applies the same rules to URLs in errors, including redirect targets. Request and response bodies are never sent. Use a collector you trust.
