# gRPC

Use `@grpc` with a fully qualified service method. Reflection is enabled unless a descriptor set is supplied with `@grpc-descriptor`.

```http
# @grpc inventory.ProjectService/Get
# @grpc-metadata x-trace-id: {{$uuid}}
GRPC localhost:50051

{"id":"42"}
```

The body is protobuf JSON. Client and bidi streaming send a JSON array of messages, and streaming responses appear in the Stream tab.

`@auth` is honoured and sent as metadata. `apikey` with `placement query` is rejected, since gRPC has no query string.

`@timeout` covers connecting, resolving descriptors, and unary calls. A timeout on the request itself also bounds the whole stream, so `# @timeout 2m` ends a server stream with `DeadlineExceeded`. Timeouts inherited from file settings, an environment, or the app default do not apply to streams, and a stream without one runs until the server ends it or you cancel it from the Stream tab.

Raise the 4MB message cap with `@setting grpc-max-recv-size 16MB`.
