# GraphQL

Mark a request as GraphQL, optionally select an operation, and provide variables as JSON. Add `@sse` or `@websocket` to run a subscription. GraphQL cannot be combined with a `GRPC` request line.

```http
# @graphql
# @operation FetchUser
POST https://api.example.com/graphql

query FetchUser { viewer { id name } }
```

Use `@variables` for an inline or file-backed variables object and `@query` to load the query itself from a file.
