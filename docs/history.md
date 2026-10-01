# History and diffing

- Every successful request produces a history entry with request text, method, status, duration, and a body snippet (unless `@no-log` is set). Values injected from `-secret` captures and allowlisted sensitive headers (Authorization, Proxy-Authorization, `X-API-Key`, `X-Access-Token`, `X-Auth-Key`, `X-Amz-Security-Token`, etc.) are masked automatically unless you opt-in with `@log-sensitive-headers`.
- History entries are environment-aware; selecting another environment filters the list automatically. Grouped entries store the display label together with the structured selection, and replaying one restores that selection when it still resolves. If a group or profile no longer exists, Resterm keeps the current selection, shows a warning, and refuses an immediate resend rather than silently running with different credentials.
- When focused on the history list, press `Enter` to load a request into the editor without executing it. Use `r`/`Ctrl+R` (or your normal send shortcut such as `Ctrl+Enter` / `Cmd+Enter`) to replay the loaded entry.
- The Diff tab compares focused versus pinned panes, making regression analysis straightforward.
- Compare runs are stored as grouped rows (`COMPARE` method), including the varied group, target profile, and full selection for each row. The preview (`p`) shows the entire bundle, `Enter` loads the failing (or baseline) environment back into the editor, and the Compare tab is automatically repopulated so you can audit deltas offline.

JSON reports stay at schema version `1`. Grouped runs add `environmentSelection` and `compare.group`, while the existing `envName` and `environment` strings keep the full display label. Text and JUnit output only use those labels.
