package ui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/unkn0wn-root/resterm/internal/helpdoc"
)

type urlOpener interface {
	Open(string) error
}

type docsOpenedMsg struct {
	title string
	url   string
	err   error
}

func (m *Model) openDocsQuery(args []string) tea.Cmd {
	query := strings.Join(args, " ")
	if query == "" {
		return m.openDoc("Resterm manual", helpdoc.Manual())
	}

	topic, ok := helpdoc.Lookup(query)
	if !ok {
		return statusCmd(statusWarn, fmt.Sprintf("Unknown docs topic %q; try :help %s", query, query))
	}
	return m.openTopicDoc(topic)
}

func (m *Model) openTopicDoc(topic helpdoc.Topic) tea.Cmd {
	return m.openDoc(topic.Title, topic.Doc)
}

func (m *Model) openDoc(title string, ref helpdoc.DocRef) tea.Cmd {
	opener := m.docsOpener
	link := ref.URL(m.docsRef)
	return func() tea.Msg {
		return docsOpenedMsg{title: title, url: link, err: opener.Open(link)}
	}
}

func (m *Model) handleDocsOpened(msg docsOpenedMsg) {
	if msg.err != nil {
		m.setStatusMessage(statusMsg{
			level: statusError,
			text: fmt.Sprintf(
				"Could not open documentation: %v\n\nOpen this URL manually:\n%s",
				msg.err,
				msg.url,
			),
		})
		return
	}
	m.setStatusMessage(statusMsg{
		level: statusInfo,
		text:  "Opened documentation for " + msg.title,
	})
}
