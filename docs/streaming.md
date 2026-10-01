# WebSocket and SSE

Streaming sessions surface in the Stream response tab, are captured in history, and can be consumed by captures and scripts.

## Server-Sent Events (`@sse`)

Add `# @sse` to keep an HTTP request open for events:

```http
### Notifications
# @name notifications
# @sse duration=2m idle=15s max-events=250
GET https://api.example.com/notifications
```

`@sse` accepts the following options:

| Token | Description |
| --- | --- |
| `duration` / `timeout` | Maximum lifetime of the stream. Resterm cancels the request once the timer elapses. |
| `idle` / `idle-timeout` | Longest time to wait for more data. Resterm cancels the request if no data arrives before this time. This limit is separate from `duration`. |
| `max-events` | Stop after this many events. |
| `max-bytes` / `limit-bytes` | Maximum amount of stream data to read. The stream ends when it reaches this limit. |
| `max-line-bytes` | Largest allowed line. The default is 4 MiB, or whatever `@setting sse-max-line-bytes` sets. A larger line stops the stream with an error. |
| `max-event-bytes` | Largest allowed event, counting every line it is built from. The default is 8 MiB, or whatever `@setting sse-max-event-bytes` sets. A larger event stops the stream with an error. |

If the server returns a non-2xx status or a content type other than `text/event-stream`, Resterm shows a normal HTTP response so you can inspect it. For a successful stream, Resterm shows the events and their details in the Stream tab and saves them in history. Templates and scripts can read `eventCount`, `byteCount`, `duration`, `reason`, `error`, `errorClass`, and `dropped` from the summary. A non-zero `dropped` value means the retained transcript is incomplete. `reason` has one of these values:

| Reason | Meaning |
| --- | --- |
| `eof` | The server closed the stream. |
| `timeout:idle` | The stream went quiet for longer than `idle`. |
| `timeout:total` | The stream ran for longer than `duration`. |
| `limit:max_events` | `max-events` was reached. |
| `limit:max_bytes` | `max-bytes` was reached. |
| `limit:line_bytes` | One line was larger than `max-line-bytes`. |
| `limit:event_bytes` | One event was larger than `max-event-bytes`. |
| `context_canceled` | The run was cancelled. |
| `context_deadline` | The run's deadline expired before the stream reached one of its own limits. |
| `error` | The stream failed. `summary.error` contains the error message and `summary.errorClass` names what kind of failure it was. |

A configured `duration` is a normal limit. An expired run deadline is a `timeout` failure. [Limits](#limits) lists which endings fail the request.

## WebSockets (`@websocket`, `@ws`)

Use `# @websocket` to negotiate an upgrade, then describe scripted interactions with `# @ws` lines:

```http
### Chat session
# @name chatSession
# @websocket timeout=10s idle-timeout=4s subprotocols=chat.v2,json compression=true
# @ws send {"type":"hello"}
# @ws wait 1s
# @ws send-json {"type":"message","text":"Hello from Resterm"}
# @ws ping heartbeat
# @ws close 1000 "client done"
GET wss://chat.example.com/room
```

Available WebSocket options:

| Token | Description |
| --- | --- |
| `timeout` | Handshake deadline (applies until the connection upgrades). |
| `idle-timeout` | Idle timeout once the socket is open. Resets on any send or receive activity (0 leaves it unbounded). |
| `max-message-bytes` | Upper bound on inbound frame sizes. |
| `subprotocols` | Comma-separated list advertised during the handshake. |
| `compression=<true\|false>` | Explicitly enable or disable per-message compression. |

Supported `@ws` steps:

| Step | Effect |
| --- | --- |
| `@ws send <text>` | Send a UTF-8 text frame. Templates expand before sending. |
| `@ws send-json <object>` | Encode JSON and send it as text. |
| `@ws send-base64 <data>` | Decode base64 and send the result as binary. |
| `@ws send-file <path>` | Send a file from disk (relative to the request file unless absolute). |
| `@ws ping [payload]` / `@ws pong [payload]` | Emit control frames (payload limited to 125 bytes). |
| `@ws wait <duration>` | Pause for the specified duration (e.g. `500ms`). |
| `@ws close [code] [reason]` | Close the connection with an optional status code (defaults to `1000`). |

When the handshake fails, Resterm shows the HTTP response to help you find the problem. During a successful session, events appear in the UI and history together with their direction, opcode, size, and close status. Templates and scripts can read `sentCount`, `receivedCount`, `duration`, `closedBy`, `closeCode`, `closeReason`, `errorClass`, and `dropped` from the summary. `closedBy` has one of these values:

| Value | Meaning |
| --- | --- |
| `server` | The server closed the connection. |
| `client` | Resterm closed the connection through `@ws close` or after the last step. |
| `timeout` | The idle limit was reached, or the run's deadline expired. An idle timeout is a normal ending. An expired run deadline is a failure and sets `errorClass` to `timeout`. |
| `canceled` | The run was cancelled. |
| `error` | The session failed. `closeReason` contains the error message and `errorClass` names what kind of failure it was. |

`error`, `canceled`, and a `timeout` with an `errorClass` cause the request to fail. Resterm keeps the transcript in every case.

> **Heads-up:** When you keep a WebSocket URL in `@const`, `@global`, or `@var`, write the request line as `GET {{ws.url}}` (or whichever variable you use). The parser needs the explicit method to recognise the line as a WebSocket request before template expansion. Literal `ws://` / `wss://` URLs without a method still work when written directly.

## Limits

- SSE lines are limited to 4 MiB and SSE events to 8 MiB. Change these limits with `@sse max-line-bytes` and `@sse max-event-bytes`. A larger line or event stops the stream with an error naming the limit to raise. Reaching `@sse max-bytes` ends the stream without an error. WebSocket messages are limited to 32 KiB unless `@websocket max-message-bytes` sets another limit.
- To change these limits for more than one request, use the `sse-max-line-bytes`, `sse-max-event-bytes`, and `ws-max-message-bytes` settings. They take a size such as `8mb` and set the default every request starts from, so an environment or a file can raise a limit once instead of repeating it. A `@sse` or `@websocket` directive on the request still wins. These settings do not accept `none`, because a stream with no line limit has nothing to stop it.

  ```http
  # @setting sse-max-line-bytes 16mb
  # @setting ws-max-message-bytes 1mb
  ```
- Most events arrive as a single `data:` line, so the line limit is the one they reach first. Raise `max-line-bytes` along with `max-event-bytes` when a single line carries the whole payload. Base64 adds about a third to a payload, so a 3 MiB file needs roughly 4 MiB of headroom.
- Resterm limits how much stream data it keeps in memory. An SSE session keeps up to 1024 events and 16 MiB, or twice `max-event-bytes` when that is larger. The size of an SSE event includes its data, comment, id, name, and other saved fields. A WebSocket session and its saved transcript each have an 8 MiB limit. The Stream pane keeps up to 5000 events or 16 MiB. If Resterm removes older events, the summary counts them in `dropped`. The Stream tab shows `Transcript incomplete` when its view is missing events.
- Reaching `max-events`, `max-bytes`, `idle`, or `duration` is a normal way for a stream to end. The saved data includes everything read before the limit was reached. Other problems fail the request. These include an expired run deadline, a read error, an SSE line or event that exceeds its limit, and a WebSocket session that ends with `closedBy: error`. Resterm still saves and reports the transcript it collected. With detailed exit codes, a cancelled run returns `130` and a stream error returns the code for the failure named in `summary.errorClass`: `20` for `timeout`, `21` for `network`, `22` for `tls`, `25` for `filesystem` such as an `@ws send-file` payload Resterm could not read, and `26` for `protocol`. A stream that failed for a reason Resterm cannot name reports `protocol`.

## Stream tab, history, and console

- The Stream tab appears automatically whenever a streaming session is active. Scroll to review frames, press `b` to bookmark important events, and switch tabs with the arrow keys (`Ctrl+H` / `Ctrl+L`).
- While the Stream tab is focused, use `g+w` then `i` to toggle the interactive WebSocket console, `p` to send ping, `c` to close gracefully, or `l` to clear the live buffer. If the console is focused for typing, press `Esc` first. Inside the console, cycle payload modes with `F2`, send payloads with `Ctrl+S` or `Ctrl+Enter`, and reuse previous payloads with the arrow keys.
- Completed transcripts are saved alongside the request in history with summary headers (`X-Resterm-Stream-Type`, `X-Resterm-Stream-Summary`). Scripts and captures can access the same data via `stream.*` templates and APIs (see [Scripting](scripting.md)).
