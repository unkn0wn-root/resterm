# gRPC, GraphQL, WebSocket and SSE

Read this when a request is not plain HTTP. Captures, checks and `resterm run` work the same way for every protocol, apart from the gRPC status rule below.

## gRPC

A gRPC request uses a `GRPC host:port` line and names the method with `# @grpc package.Service/Method`. The body is the request message as protobuf JSON.

```http
### Get user over gRPC
# @name grpcUser
# @grpc users.v1.UserService/GetUser
# @grpc-plaintext true
# @grpc-metadata x-request-id: {{$uuid}}
# @assert response.json().id == "42"
GRPC {{grpcHost}}

{"id": "42"}
```

- Resterm asks the server for its schema through reflection by default. Without reflection, use `# @grpc-proto ./proto/users.proto`, with `# @grpc-import-path ./proto` for imports, or a compiled descriptor set with `# @grpc-descriptor ./users.protoset`. A request can use `@grpc-proto` or `@grpc-descriptor`, not both.
- `# @grpc-plaintext true` turns off TLS. `# @grpc-authority` sets the `:authority` header.
- `# @grpc-metadata key: value` adds metadata and may repeat. `@auth` works too and is sent as metadata.
- Directives that start with `@grpc` work only on a `GRPC` request line. `@graphql`, `@sse` and `@websocket` are parse errors there.
- For client or bidirectional streams, the body is a JSON array of messages.
- A status other than `OK` fails the request before its checks run, so `@assert` cannot expect an error status. The exit code follows the status: `23` for `Unauthenticated` and `PermissionDenied`, `21` for `Unavailable`, `20` for `DeadlineExceeded`, `130` for `Canceled`, and `26` for any other status. `results[].grpc` still shows the status. In checks, `response.json()` is the reply message.
- In the JSON report, `results[].grpc` has `code`, `statusCode`, `statusMessage` and, when the server sent them, `statusDetails`. `statusCode` is left out for an OK call, because it is `0`.

## GraphQL

`# @graphql` turns GraphQL on for one request. The body is the query. A `# @variables` comment below the query starts the variables JSON.

```http
### Find a repository
# @name findRepo
# @graphql
# @operation FindRepo
# @assert response.json().errors == null
POST {{graphqlUrl}}
Authorization: Bearer {{token}}

query FindRepo($name: String!) {
  repository(name: $name) {
    id
    stars
  }
}

# @variables
{"name": "resterm"}
```

- `# @operation` sets the operation name. `# @query < ./query.graphql` and `# @variables < ./vars.json` load them from files.
- With POST, Resterm sends a JSON body with `query`, `variables` and `operationName`. With GET, it adds them as query parameters.
- GraphQL errors do not fail the request. Check `response.json().errors` yourself, as above.
- Add `@sse` or `@websocket` to run a subscription. Over WebSocket, Resterm speaks only `graphql-transport-ws` and sends no payload in `connection_init`, so pass credentials in headers.

## Server-Sent Events

`# @sse` keeps a request open and records events.

```http
### Watch notifications
# @name notifications
# @sse duration=30s idle=10s max-events=20
# @assert stream.summary().eventCount > 0
GET {{baseUrl}}/notifications
Accept: text/event-stream
```

- Options: `duration` limits the whole stream, `idle` the wait for more data, `max-events` and `max-bytes` the amount read.
- Reaching `duration`, `idle`, `max-events` or `max-bytes` ends the stream normally. A read error or an expired run deadline fails the request.
- `stream.summary().reason` says why the stream ended, such as `eof`, `timeout:idle` or `limit:max_events`.
- A response that is not `2xx` or not `text/event-stream` is reported as a normal HTTP response.

## WebSocket

`# @websocket` opens a WebSocket, and `# @ws` lines script the session in order.

```http
### Chat session
# @name chat
# @websocket timeout=10s idle-timeout=5s
# @ws send-json {"type": "hello"}
# @ws wait 2s
# @ws close 1000 "done"
# @assert stream.summary().receivedCount > 0
GET wss://chat.example.com/socket
```

- Steps: `@ws send <text>`, `@ws send-json <json>`, `@ws send-base64 <data>`, `@ws send-file <path>`, `@ws ping`, `@ws pong`, `@ws wait <duration>` and `@ws close [code] [reason]`.
- `timeout` limits the handshake. `idle-timeout` ends a quiet session, and `0` means no limit. `max-message-bytes` raises the 32 KiB limit for one incoming message.
- `stream.summary().closedBy` is `server`, `client`, `timeout`, `canceled` or `error`. `error`, `canceled`, and a timeout caused by the run deadline fail the request.
- When the URL comes from a variable, keep the method on the request line, as in `GET {{wsUrl}}`.

## Streams in captures, checks and reports

- `stream.summary()` and `stream.events()` read the transcript in `@capture`, `@assert` and test blocks. A non-zero `stream.summary().dropped` means some events were not kept.
- `--artifact-dir <dir>` writes each transcript under `<dir>/streams/`, and the JSON report gives its path in `results[].stream.transcriptPath`.
- A failed stream exits with the code for its `errorClass`: `20` for a timeout, `21` for the network, `22` for TLS, `25` for a file it could not read and `26` for a protocol error.
