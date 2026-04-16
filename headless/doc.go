// Package headless runs Resterm requests and workflows without the TUI.
//
// The package exposes a public API for running resterm .http/.rest files in CI/CD. Call Run
// with Options to execute a request file, then inspect the returned Report or
// serialize it with JSON, text or JUnit encoders.
package headless
