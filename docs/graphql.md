# GraphQL

Enable GraphQL handling with `# @graphql` (requests start with it disabled). Resterm packages GraphQL requests according to HTTP method:

- **POST**: body becomes `{ "query": ..., "variables": ..., "operationName": ... }`.
- **GET**: query parameters `query`, `variables`, `operationName` are attached.
- Template variables in the URL are expanded before the GET parameters are attached, so `GET {{graphql.endpoint}}` works even when the host is templated.

Available directives:

| Directive | Description |
| --- | --- |
| `@graphql [boolean]` | Enable/disable GraphQL processing for the request. |
| `@operation` / `@graphql-operation` | Sets the `operationName`. |
| `@variables` | Starts a variables block; inline JSON or `< file.json`. |
| `@query` | Loads the query from a file instead of the inline body. |

`@graphql` accepts the standard boolean values, including `true`/`false`, `yes`/`no`, `on`/`off`, `1`/`0`, and `t`/`f`. It also accepts `disable` and `disabled` as false values; other values are reported as errors. Switching GraphQL off discards the operation, variables, and query collected so far, allowing the request to declare them again after GraphQL is re-enabled:

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
