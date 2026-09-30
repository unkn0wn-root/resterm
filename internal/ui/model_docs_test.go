package ui

import (
	"errors"
	"strings"
	"testing"
)

type recordingOpener struct {
	links []string
	err   error
}

func (o *recordingOpener) Open(link string) error {
	o.links = append(o.links, link)
	return o.err
}

func TestDocsCommandOpensTopicPage(t *testing.T) {
	opener := &recordingOpener{}
	model := New(Config{})
	model.launcher = opener

	msg, ok := model.openDocsQuery([]string{"grpc"})().(docsOpenedMsg)
	if !ok {
		t.Fatal("expected documentation result message")
	}
	if len(opener.links) != 1 || opener.links[0] != msg.url {
		t.Fatalf("opener links = %+v, message URL = %q", opener.links, msg.url)
	}
	if msg.url != "https://resterm.app/docs/grpc/" {
		t.Fatalf("expected gRPC documentation URL, got %q", msg.url)
	}

	model.handleDocsOpened(msg)
	if model.statusMessage.level != statusInfo || !strings.Contains(model.statusMessage.text, "gRPC") {
		t.Fatalf("unexpected success status: %+v", model.statusMessage)
	}
}

func TestBareDocsCommandOpensDocsIndex(t *testing.T) {
	opener := &recordingOpener{}
	model := New(Config{})
	model.launcher = opener

	msg := model.openDocsQuery(nil)().(docsOpenedMsg)
	if msg.url != "https://resterm.app/docs/" {
		t.Fatalf("expected docs index URL, got %q", msg.url)
	}
}

func TestDocsCommandReportsUnknownTopic(t *testing.T) {
	model := New(Config{})

	status, ok := statusMsgFromCmd(model.openDocsQuery([]string{"not-a-topic"}))
	if !ok || status.level != statusWarn {
		t.Fatalf("expected warning status, got %+v (ok=%v)", status, ok)
	}
	if !strings.Contains(status.text, "try :help not-a-topic") {
		t.Fatalf("expected recovery hint, got %q", status.text)
	}
}

func TestDocsCommandFailureShowsCopyableURL(t *testing.T) {
	opener := &recordingOpener{err: errors.New("browser unavailable")}
	model := New(Config{})
	model.launcher = opener

	msg := model.openDocsQuery([]string{"requests"})().(docsOpenedMsg)
	model.handleDocsOpened(msg)

	if !model.showStatusModal {
		t.Fatal("expected browser failure to open an error modal")
	}
	for _, want := range []string{"browser unavailable", msg.url, "Open this URL manually"} {
		if !strings.Contains(model.statusModalMessage, want) {
			t.Fatalf("expected error modal to contain %q, got %q", want, model.statusModalMessage)
		}
	}
}
