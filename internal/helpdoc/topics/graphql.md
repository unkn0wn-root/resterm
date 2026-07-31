# GraphQL

Mark a request as GraphQL, optionally select an operation, and provide variables as JSON.

```http
# @graphql
# @operation FetchUser
POST https://api.example.com/graphql

query FetchUser { viewer { id name } }
```

Use `@variables` for an inline or file-backed variables object and `@query` to load the query itself from a file.
