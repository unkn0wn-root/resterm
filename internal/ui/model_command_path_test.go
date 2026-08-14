package ui

import "testing"

func TestSourceEqualsPathCompletionKeepsTheFlag(t *testing.T) {
	input := "mock start --source=us"
	arg, ok := exCommands.pathArg(input, len(input))
	if !ok {
		t.Fatal("path argument missing")
	}

	arg.edit.Text = "users.http"
	got, _, err := arg.edit.Apply(input)
	if err != nil {
		t.Fatal(err)
	}
	if got != "mock start --source=users.http" {
		t.Fatalf("completed line = %q", got)
	}
}

func TestExCommandParsesQuotedPath(t *testing.T) {
	cmd := exCommands.Parse(`edit "fixtures/user mocks.http"`)
	if cmd.kind != exCommandEdit || len(cmd.args) != 1 || cmd.args[0] != "fixtures/user mocks.http" {
		t.Fatalf("command = %+v", cmd)
	}

	cmd = exCommands.Parse(`edit "fixtures/user mocks.http`)
	if cmd.kind != exCommandInvalid {
		t.Fatalf("unfinished command = %+v", cmd)
	}
}
