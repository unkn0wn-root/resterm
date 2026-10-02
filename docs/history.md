# History and diffing

- Every successful request produces a history entry with request text, method, status, duration, and a body snippet (unless `@no-log` is set). Values injected from `-secret` captures and allowlisted sensitive headers (such as Authorization, Proxy-Authorization, `X-API-Key`, `X-Access-Token`, `X-Auth-Key`, and `X-Amz-Security-Token`) are masked unless you opt in with `@log-sensitive-headers`.
- History entries remember their environment. Selecting another environment filters the list. Grouped entries store the display label together with the structured selection, and replaying one restores that selection when it still resolves. If a group or profile no longer exists, Resterm keeps the current selection, shows a warning, and refuses to resend right away instead of quietly running with different credentials.
- With the history list focused, press `Enter` to load a request into the editor without sending it. Use `r`/`Ctrl+R` (or your normal send shortcut such as `Ctrl+Enter` / `Cmd+Enter`) to replay the loaded entry.
- The Diff tab compares the focused pane with the pinned one, which helps you spot regressions.
- Compare runs are stored as grouped rows (`COMPARE` method), including the varied group, target profile, and full selection for each row. The preview (`p`) shows the entire bundle, `Enter` loads the failing (or baseline) environment back into the editor, and the Compare tab fills in again so you can review the differences offline.

JSON reports stay at schema version `1`. Grouped runs add `environmentSelection` and `compare.group`, while the existing `envName` and `environment` strings keep the full display label. Text and JUnit output only use those labels.
