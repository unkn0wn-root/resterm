# Compatibility

Starting with v1.0.0, Resterm follows semantic versioning. The compatibility promise applies to the public Go API as well as the file formats and command-line interfaces that users rely on.

## Stable throughout v1

The following interfaces remain compatible across all v1 releases:

- Every exported identifier in the `headless` package. This is Resterm's only public Go package. Packages under `internal/` are private and may change in any release.
- The names, arguments, and behavior of request file directives, including `@name`, `@profile`, `@compare`, `@for-each`, `@expect`, and `@setting`.
- Existing CLI flags and their behavior.
- Machine-readable headless output, including JSON field names, JUnit structure, and exit codes.
- Configuration identifiers, including key binding action IDs, theme keys, and settings keys. Binding and theme files reject unknown keys, so removing a key would prevent an existing configuration from loading.

## RestermScript deprecations

RestermScript builtins and reserved words have a narrower compatibility policy. They may be removed during v1, but only through the following deprecation process:

- A builtin scheduled for removal is deprecated in one minor release and removed no earlier than the following minor release.
- A deprecated builtin continues to work. The parser reports each use as an editor warning and shows the full warning in the Explain pane.
- The release notes identify every removal and its replacement.

## Outside the compatibility promise

Presentation details may change in any release. This includes status message wording, rendered layout, colors, and log output. Code under `internal/` is also not covered.

Minor releases may add directives, builtins, flags, and configuration keys. Older versions ignore unknown `@setting` keys, so a newly added setting alone will not prevent a request file from loading.
