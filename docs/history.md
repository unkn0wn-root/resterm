# History and diffing

- Every successful request saves its request text, method, status, duration, and a body snippet to history. `@no-log` omits the body snippet. Values from `-secret` captures and sensitive headers on Resterm's list (such as Authorization, Proxy-Authorization, `X-API-Key`, `X-Access-Token`, `X-Auth-Key`, and `X-Amz-Security-Token`) are masked unless you opt in with `@log-sensitive-headers`.
- History entries remember their environment, and switching environments filters the list. Grouped entries save both the display label and the individual group selections. Replaying an entry restores those selections if they still exist.

  If a group or profile has been removed, Resterm keeps the current selection, shows a warning, and blocks the immediate resend. This prevents a replay from silently using different credentials.
- With the history list focused, press `Enter` to load a request into the editor without sending it. Use `r`/`Ctrl+R` (or your normal send shortcut such as `Ctrl+Enter` / `Cmd+Enter`) to replay the loaded entry.
- The Diff tab compares the focused pane with the pinned one, which helps you spot regressions.
- Compare runs are stored as grouped rows (`COMPARE` method), including the varied group, target profile, and full selection for each row. The preview (`p`) shows the entire bundle, `Enter` loads the failing (or baseline) environment back into the editor, and the Compare tab fills in again so you can review the differences offline.

JSON reports stay at schema version `1`. Grouped runs add `environmentSelection` and `compare.group`, while the existing `envName` and `environment` strings keep the full display label. Text and JUnit output only use those labels.
