// Package headless runs Resterm requests and workflows without the TUI.
//
// The package exposes a stable library API for CI/CD, automation, and other
// non-interactive integrations. The primary workflow is:
//
//  1. Construct Options.
//  2. Call Options.Validate for early configuration feedback when needed.
//  3. Call Run to execute a request or workflow file.
//  4. Inspect the returned Report, Result, and Step values.
//  5. Serialize the report with Report.Encode using JSON, JUnit, or Text.
//
// WriteJSON, WriteJUnit, and WriteText remain available as convenience helpers
// over Encode. Programmatic format selection is available through the Format
// constants JSON, JUnit, and Text, and ParseFormat converts user-provided names
// into a Format value.
//
// Invalid library inputs are reported as UsageError values. Use IsUsageError to
// classify them, and errors.Is to match specific sentinels such as ErrNoFilePath
// or ErrTooFewTargets.
package headless
