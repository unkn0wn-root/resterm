# Compare runs

Run the same request across several environments, either from the request file or from the CLI:

- Add `# @compare dev stage prod base=stage` to a request block to set the order and baseline in the file. Provide at least two environments. `base` is optional and defaults to the first entry. `baseline`, `primary`, and `ref` are alternate names for `base`, so only one of them may carry a value. Writing `base=` or `group=` with nothing after it is reported rather than treated as omitted.
- Supply global defaults with `resterm --compare dev,stage,prod --compare-base stage`, then press `g+c` anywhere in the editor to reuse those targets even if the request lacks `@compare`.
- While a compare run is active, Resterm switches to a split layout, pins the previous response in the secondary pane, and shows progress in the status bar (`Compare dev✓ stage… prod?`). The Compare tab shows a table with the status, code, duration, and diff summary for each environment.
- Each compare sweep writes one bundled history entry (`COMPARE` method). [History and diffing](history.md) covers replaying it.
- In the Compare tab, use ↑/↓ (or PgUp/PgDn/Home/End) to highlight an environment. Press `Enter` to load its saved response into the primary pane. The baseline stays pinned in the secondary pane, so Diff, Pretty, Raw, and Headers compare the selected environment with it.

  Selecting the baseline row shows no differences. Select another row to see what changed. To use a different baseline, run again with a new `base=` value or load the pair from History.

With grouped environments, a compare run changes one group and keeps the other selections fixed. This example compares API profiles while keeping the same app and credentials:

```http
# @compare group=api dev uat prod base=dev
GET {{services.api.base}}/health
```

The CLI equivalent is:

```bash
resterm --compare dev,uat,prod --compare-group api --compare-base dev
```

Profiles with spaces can be quoted inline, or comma-separated on the CLI as [`resterm run`](cli/run.md) shows. Compare on a grouped file requires a group. Unknown profiles or a baseline that is not one of the targets fail before the first network request.

Use `@compare` together with the usual metadata, for example to pair request-scoped variables with each environment:

```http
### Smoke workflow
# @name smoke
# @compare dev stage prod base=prod
POST {{services.api.base}}/status
Accept: application/json

{
  "env": "{{services.api.name}}"
}
```
