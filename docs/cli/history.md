# resterm history

Use the history commands to back up, inspect, or move saved history. [Configuration](../configuration.md) lists its location, and [History and diffing](../history.md) covers browsing and replaying it in the TUI.

| Command | What it does |
| --- | --- |
| `resterm history export --out <path>` | Export persisted history as JSON. |
| `resterm history import --in <path>` | Import history from JSON. |
| `resterm history backup --out <path>` | Create a SQLite-consistent backup. |
| `resterm history stats` | Print schema version, row counts, and sizes. |
| `resterm history check [--full]` | Run integrity checks. |
| `resterm history compact` | Checkpoint and compact `history.db`. |

`resterm history vacuum` is another name for `compact`. These commands exit `0` on success, `2` for a missing or unknown flag, extra arguments, or an unknown subcommand, and `1` for any other error.
