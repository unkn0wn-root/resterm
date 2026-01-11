package curl

import (
	"strings"
	"testing"
)

func TestSplitCommandsBlankLine(t *testing.T) {
	src := "curl https://a.test\n\ncurl https://b.test"
	cmds := SplitCommands(src)
	if len(cmds) != 2 {
		t.Fatalf("expected 2 commands, got %d", len(cmds))
	}
	if !strings.Contains(cmds[0], "https://a.test") {
		t.Fatalf("unexpected first command %q", cmds[0])
	}
	if !strings.Contains(cmds[1], "https://b.test") {
		t.Fatalf("unexpected second command %q", cmds[1])
	}
}

func TestSplitCommandsCurlStart(t *testing.T) {
	src := "curl https://a.test\ncurl https://b.test"
	cmds := SplitCommands(src)
	if len(cmds) != 2 {
		t.Fatalf("expected 2 commands, got %d", len(cmds))
	}
}

func TestSplitCommandsSudoUser(t *testing.T) {
	src := "sudo -u root curl https://a.test\n\ncurl https://b.test"
	cmds := SplitCommands(src)
	if len(cmds) != 2 {
		t.Fatalf("expected 2 commands, got %d", len(cmds))
	}
}

func TestSplitCommandsEnvUnset(t *testing.T) {
	src := "env -u FOO curl https://a.test"
	cmds := SplitCommands(src)
	if len(cmds) != 1 {
		t.Fatalf("expected 1 command, got %d", len(cmds))
	}
}

func TestSplitCommandsTimePrefix(t *testing.T) {
	src := "time -p curl https://a.test"
	cmds := SplitCommands(src)
	if len(cmds) != 1 {
		t.Fatalf("expected 1 command, got %d", len(cmds))
	}
}

func TestSplitCommandsCommandPrefix(t *testing.T) {
	src := "command -p curl https://a.test"
	cmds := SplitCommands(src)
	if len(cmds) != 1 {
		t.Fatalf("expected 1 command, got %d", len(cmds))
	}
}

func TestSplitCommandsMultilineBody(t *testing.T) {
	src := "curl https://a.test -d '{\n\n}'\n\ncurl https://b.test"
	cmds := SplitCommands(src)
	if len(cmds) != 2 {
		t.Fatalf("expected 2 commands, got %d", len(cmds))
	}
	if !strings.Contains(cmds[0], "\n\n") {
		t.Fatalf("expected blank line preserved in first command: %q", cmds[0])
	}
}

func TestSplitCommandsAnsiQuote(t *testing.T) {
	src := "curl https://a.test -d $'one\\n\\'two\\n'\n\ncurl https://b.test"
	cmds := SplitCommands(src)
	if len(cmds) != 2 {
		t.Fatalf("expected 2 commands, got %d", len(cmds))
	}
}
