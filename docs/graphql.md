# GraphQL

Turn on GraphQL with `# @graphql`. It is off by default for every request. Resterm builds the GraphQL request based on the HTTP method:

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
