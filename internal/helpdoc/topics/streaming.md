# Streaming

Resterm records Server-Sent Events and WebSocket sessions in the Stream response tab.

```http
# @sse duration=2m idle=15s
GET https://example.com/events
```

Use `@websocket` for a session and `@ws` steps such as `send`, `send-json`, `wait`, `ping`, and `close` to script it. In the Stream tab `Ctrl+Space` pauses live follow and `g w` opens WebSocket commands.
