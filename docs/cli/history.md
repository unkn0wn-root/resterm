# resterm history

The history commands operate on persisted history storage. [Configuration](../configuration.md) says where it lives, and [History and diffing](../history.md) covers history in the TUI.

| Command | What it does |
| --- | --- |
| `resterm history export --out <path>` | Export persisted history as JSON. |
| `resterm history import --in <path>` | Import history from JSON. |
| `resterm history backup --out <path>` | Create a SQLite-consistent backup. |
| `resterm history stats` | Print schema version, row counts, and sizes. |
| `resterm history check [--full]` | Run integrity checks. |
| `resterm history compact` | Checkpoint and compact `history.db`. |
