// Package headless runs Resterm requests and workflows without the TUI.
//
// The package exposes a library API for CI/CD, automation, and other
// non-interactive integrations. Choose the entrypoint based on how you run:
//
//   - Call Run for a one-shot execution.
//   - Call Build and then RunPlan when you want to prepare once and reuse the
//     same validated plan across retries, repeated runs, or concurrent calls.
//
// The primary workflow is:
//
//  1. Construct Options.
//  2. Call Build to prepare and validate a reusable Plan.
//  3. Call RunPlan to execute the prepared plan, or call Run for a one-shot run.
//  4. Inspect the returned Report, Result, and Step values.
//  5. Serialize the report with Report.Encode using JSON, JUnit, or Text.
//
// WriteJSON, WriteJUnit, and WriteText remain available as helpers
// over Encode. Programmatic format selection is available through the Format
// constants JSON, JUnit, and Text, and ParseFormat converts user-provided names
// into a Format value.
//
// Invalid library inputs are reported as UsageError values. Use IsUsageError to
// classify them, and errors.Is to match specific sentinels such as ErrNoSourcePath
// or ErrTooFewTargets.
package headless
