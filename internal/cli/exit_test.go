package cli

import (
	"errors"
	"fmt"
	"testing"
)

func TestIsExitCodeOnly(t *testing.T) {
	if !IsExitCodeOnly(ExitErr{Code: 1}) {
		t.Fatal("expected exit-code-only ExitErr to be silent")
	}
	if !IsExitCodeOnly(&ExitErr{Code: 1}) {
		t.Fatal("expected pointer exit-code-only ExitErr to be silent")
	}
	if IsExitCodeOnly(ExitErr{Err: errors.New("failed"), Code: 1}) {
		t.Fatal("expected ExitErr with error payload to render")
	}
	if IsExitCodeOnly(fmt.Errorf("run: %w", ExitErr{Code: 1})) {
		t.Fatal("expected wrapped ExitErr to render wrapper context")
	}
}
