# gRPC

Start a gRPC request with a line such as `GRPC host:port`. Use the directives below to choose the method, provide service descriptors, and configure the connection.

| Directive | Description |
| --- | --- |
| `@grpc package.Service/Method` | Fully qualified method to call. |
| `@grpc-descriptor path/to/file.protoset` | Use a compiled descriptor set instead of server reflection. |
| `@grpc-proto path/to/service.proto` | Load the schema from a `.proto` file instead of server reflection (repeatable). See [Proto files](#proto-files). |
| `@grpc-import-path path/to/dir` | Add a directory to search for imports of `@grpc-proto` files (repeatable). |
| `@grpc-reflection [true\|false]` | Toggle server reflection (default `true`). |
| `@grpc-plaintext [true\|false]` | Force plaintext or TLS. |
| `@grpc-authority value` | Override the HTTP/2 `:authority` header. |
| `@grpc-metadata key: value` | Add a metadata pair (repeatable). Invalid lines are reported. |
| `@setting grpc-root-cas path1,path2` | Extra root CAs (space/comma/semicolon separated). Paths resolve relative to the request file. |
| `@setting grpc-root-mode append\|replace` | Control whether extra CAs append to system roots (`append`) or replace them (`replace`, default). |
| `@setting grpc-client-cert path` / `@setting grpc-client-key path` | Client cert/key for mTLS (relative paths allowed). |
| `@setting grpc-insecure true` | Skip TLS verification (off by default). |
| `@setting grpc-max-recv-size 16MB` | Raise the maximum response message size (gRPC defaults to 4MB). |
| `@setting grpc-max-send-size 16MB` | Raise the maximum request message size. |
| `@setting grpc-compression gzip\|none` | Compress request messages. Compressed responses are always accepted. |

Any gRPC TLS setting (roots, client cert/key, insecure) turns on TLS, unless you force plaintext with `@grpc-plaintext true`.

gRPC manages some metadata keys itself, including `grpc-*`, `content-type`, `user-agent`, and `te`. Resterm rejects these reserved keys in `@grpc-metadata` and in gRPC headers.

Metadata keys ending in `-bin` carry binary values. Write the raw bytes in `@grpc-metadata`. gRPC base64-encodes them on the wire, so the request metadata pane shows the encoded form, not the literal you typed.

`@auth` works on gRPC requests. `basic`, `bearer`, `apikey` and `header` auth are sent as metadata, as are `command` and `oauth2`. `apikey` with `placement query` is rejected, because gRPC has no query string. Resterm does not support Digest auth for gRPC requests. Explicit `authorization` metadata takes precedence over `@auth`.

Descriptor sets, proto files, import paths, and message files resolve relative to the request file, then against the fallback roots used for HTTP body files.

The request body contains protobuf JSON. Use `< payload.json` to load from disk, and add `# @body expand` if the file includes templates. Responses show the message JSON, headers, and trailers, with `-bin` metadata base64-encoded as it travels on the wire. A failing call also shows any `google.rpc.*` status details the server attached. History stores the method, status, and timing next to HTTP calls.

Resterm supports server, client, and bidirectional (bidi) streaming. Use one JSON object for a unary call or a server stream. For client and bidirectional streams, send a JSON array of message objects. Streaming responses return a JSON array, and the Stream tab shows each message and a summary.

`@timeout` / `@setting timeout` limits connecting, resolving descriptors, and unary calls. A timeout set on the request itself also limits the whole stream, so `# @timeout 2m` ends a server stream after two minutes with `DeadlineExceeded`. Streams without one run until the server ends them or you cancel from the Stream tab. A timeout inherited from file settings, an environment, or the app default does not apply to streams.

Example:

```http
### Generate Report Over gRPC
# @timeout 5s
# @grpc analytics.ReportingService/GenerateReport
# @grpc-reflection true
# @grpc-plaintext true
# @grpc-authority analytics.dev.local
# @grpc-metadata x-trace-id: {{$uuid}}
# @setting grpc-root-cas ./ca.pem
GRPC {{grpc.host}}

{
  "tenantId": "{{tenant.id}}",
  "reportId": "rep-{{$uuid}}"
}
```

Streaming example:

```http
### Bidi Stream Chat
# @grpc chat.ChatService/Stream
# @grpc-plaintext true
GRPC {{grpc.host}}

[
  {"message": "hello"},
  {"message": "again"}
]
```

## Proto files

`@grpc-proto` reads the schema from `.proto` files, so the server does not need reflection and you do not need `protoc` to build a descriptor set. Edits to the files apply on the next send. A request can use `@grpc-proto` or `@grpc-descriptor`, but not both.

Without `@grpc-import-path`, the folders of the listed files are the import paths. Add import paths when imports start from a root folder, as with `protoc -I`. Resterm searches them in order, and each listed file must be inside one of them. You can also give a file by its path inside an import path, as in `@grpc-proto acme/v1/greeter.proto`. The standard `google/protobuf/*.proto` files are built in. Others, such as `google/api/annotations.proto`, must be on an import path.

```http
### Say Hello
# @grpc acme.v1.Greeter/SayHello
# @grpc-proto ./proto/acme/v1/greeter.proto
# @grpc-import-path ./proto
# @grpc-import-path ./vendor
GRPC {{grpc.host}}

{"name": "Ada"}
```

Here `greeter.proto` imports `acme/v1/types.proto` from `proto/` and `google/api/annotations.proto` from `vendor/`. Compile errors name the file, line, and column.
