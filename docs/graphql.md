# GraphQL

Turn on GraphQL with `# @graphql`. It is off by default for every request. GraphQL can run over `@sse` or `@websocket`, as [Subscriptions](#subscriptions) describes, but not on a `GRPC` request line. Over HTTP and SSE, Resterm builds the GraphQL request based on the HTTP method:

- **POST**: body becomes `{ "query": ..., "variables": ..., "operationName": ... }`.
- **GET**: the `query`, `variables`, and `operationName` query parameters are added.
- Template variables in the URL are expanded before the GET parameters are attached, so `GET {{graphql.endpoint}}` works even when the host is templated.

Available directives:

| Directive | Description |
| --- | --- |
| `@graphql [boolean]` | Turn GraphQL on or off for the request. |
| `@operation` / `@graphql-operation` | Sets the `operationName`. |
| `@variables` / `@graphql-variables` | Starts a variables block. Use inline JSON or `< file.json`. |
| `@query` / `@graphql-query` | Loads the query from a file instead of the inline body. |

`@graphql` accepts the standard boolean values, including `true`/`false`, `yes`/`no`, `on`/`off`, `1`/`0`, and `t`/`f`. It also accepts `disable` and `disabled` as false values. Other values are reported as errors. Switching GraphQL off discards the operation, variables, and query collected so far, so the request can declare them again after GraphQL is turned back on:

```http
### Reconfigured
# @graphql
# @operation First
# @query query First { a }
POST {{graphql.endpoint}}
# @graphql off
# @graphql
# @operation Second
# @query query Second { b }
```

Example:

```http
### Inline GraphQL Query
# @graphql
# @operation FetchWorkspace
POST {{graphql.endpoint}}

query FetchWorkspace($id: ID!) {
  workspace(id: $id) {
    id
    name
  }
}

# @variables
{
  "id": "{{graphql.workspaceId}}"
}
```

## Subscriptions

A subscription runs over Server-Sent Events or a WebSocket. Add `@sse` or `@websocket` to a `@graphql` request. `@operation`, `@variables`, and `@query` work the same way as over HTTP.

Over SSE, Resterm sends the request described above and reads the response as an event stream. This is the distinct connections mode of the GraphQL over SSE protocol. The stream ends at the first `complete` event, and the summary reason is `complete`. A stream that closes before `complete` ends with the reason `eof`. Each `next` event must carry a JSON object. A `next` event that does not fails the request with the reason `error`. Other events stay in the transcript.

```http
### Audit events over SSE
# @name auditStream
# @graphql
# @operation AuditEvents
# @sse duration=20s idle=10s max-events=50
POST {{graphql.endpoint}}
Authorization: Bearer {{token}}

subscription AuditEvents($project: String) {
  auditEvents(project: $project) { type summary }
}

# @variables
{ "project": "{{project.id}}" }
```

Over WebSocket, Resterm uses the `graphql-transport-ws` subprotocol. It sends `connection_init`, waits for `connection_ack`, and then sends the operation in a `subscribe` message. It answers each `ping` with a `pong`. When the server sends `complete` or `error` for the operation, Resterm closes the socket with code `1000`. GraphQL messages that arrive after that stay in the transcript but do not change the result. The transcript shows these messages, and the ones Resterm sent are labeled `graphql`.

```http
### Audit events over WebSocket
# @name auditSocket
# @graphql
# @operation AuditEvents
# @query < ./queries/audit.graphql
# @variables < ./queries/audit.variables.json
# @websocket max-message-bytes=1mb
# @ws wait 20s
# @assert stream.summary().closedBy == "client"
GET {{graphql.wsEndpoint}}
Authorization: Bearer {{token}}
```

`@ws` steps run after the subscription starts, and Resterm closes the socket when the last step ends. Without steps, the session runs until the operation ends, the server closes the socket, `idle` passes, or the request times out. A server that sends GraphQL `ping` messages resets `idle`, so use `@ws wait` to listen for a fixed time shorter than the request timeout. The request fails if the server closes the socket, or `idle` passes, before `connection_ack`, because the operation never started.

Resterm supports only `graphql-transport-ws`. Leave `subprotocols` unset or set it to `graphql-transport-ws`. The request fails when the server does not answer with that subprotocol. `connection_init` carries no payload, so send credentials in request headers.

GraphQL errors do not fail the request, the same as over HTTP. They arrive as `next` data or as an `error` message. After `connection_ack`, a server can also close the socket with any code, such as `4403`, and that does not fail the request either. Check them with `@assert` on `stream.summary()` or on the events. A message that breaks the protocol fails the request with `closedBy` set to `error` and `errorClass` set to `protocol`, even when the server closes the socket right after it. Resterm then closes the socket with code `4400`. Examples are invalid JSON, a message without the `id` or `payload` its type needs, an `error` message whose payload is not a list of GraphQL errors, and `next` before `connection_ack`.
