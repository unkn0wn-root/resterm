package ui

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/unkn0wn-root/resterm/internal/httpclient"
)

func TestFormatHistorySnippetStripsHTMLAndLimitsLines(t *testing.T) {
	snippet := "<html><head><style>body{color:red}</style></head><body><h1>Hello</h1><p>World</p><p>More content here.</p><p>Line4</p><p>Line5</p><p>Line6</p><p>Line7</p><p>Line8</p><p>Line9</p><p>Line10</p><p>Line11</p><p>Line12</p><p>Line13</p><p>Line14</p><p>Line15</p><p>Line16</p><p>Line17</p><p>Line18</p><p>Line19</p><p>Line20</p><p>Line21</p><p>Line22</p><p>Line23</p><p>Line24</p><p>Line25</p></body></html>"

	formatted := formatHistorySnippet(snippet, 40)

	if strings.Contains(formatted, "body{") {
		t.Fatalf("expected style content to be removed, got %q", formatted)
	}
	if strings.Contains(formatted, "<") || strings.Contains(formatted, ">") {
		t.Fatalf("expected HTML tags to be stripped, got %q", formatted)
	}

	lines := strings.Split(formatted, "\n")
	if len(lines) != historySnippetMaxLines+1 {
		t.Fatalf("expected %d lines plus truncation, got %d", historySnippetMaxLines+1, len(lines))
	}
	if !strings.HasSuffix(lines[len(lines)-1], "(truncated)") {
		t.Fatalf("expected truncation marker, got %q", lines[len(lines)-1])
	}
}

func TestFormatHistorySnippetHandlesStyleOnly(t *testing.T) {
	snippet := "<style>body{color:red}</style>"
	formatted := formatHistorySnippet(snippet, 40)
	if formatted != historySnippetPlaceholder {
		t.Fatalf("expected placeholder for empty html snippet, got %q", formatted)
	}
}

func TestConsumeHTTPResponseSchedulesAsyncRender(t *testing.T) {
	vp := viewport.New(80, 10)
	model := Model{
		responseViewport: vp,
		activeTab:        responseTabPretty,
	}

	resp := &httpclient.Response{
		Status:       "200 OK",
		StatusCode:   200,
		Headers:      http.Header{"Content-Type": []string{"text/html"}},
		Body:         []byte("<html><body><p>Hello</p></body></html>"),
		Duration:     150 * time.Millisecond,
		EffectiveURL: "https://example.com",
	}

	cmd := model.consumeHTTPResponse(resp, nil, nil)
	if cmd == nil {
		t.Fatalf("expected consumeHTTPResponse to return render command")
	}
	if !model.responseLoading {
		t.Fatalf("expected responseLoading to be true after scheduling render")
	}
	if model.responseRenderToken == "" {
		t.Fatalf("expected responseRenderToken to be assigned")
	}
	if content := model.responseViewport.View(); !strings.HasPrefix(content, responseFormattingBase) {
		t.Fatalf("expected viewport to show formatting message prefix, got %q", content)
	}

	queue := collectMsgs(cmd)
	for i := 0; i < len(queue); i++ {
		switch typed := queue[i].(type) {
		case responseRenderedMsg:
			if typed.token != model.responseRenderToken {
				t.Fatalf("expected render token to match model state")
			}
			if follow := model.handleResponseRendered(typed); follow != nil {
				queue = append(queue, collectMsgs(follow)...)
			}
		case responseWrapMsg:
			if follow := model.handleResponseWrap(typed); follow != nil {
				queue = append(queue, collectMsgs(follow)...)
			}
		case tea.Cmd:
			queue = append(queue, collectMsgs(typed)...)
		case responseLoadingTickMsg:
			if follow := model.handleResponseLoadingTick(); follow != nil {
				queue = append(queue, collectMsgs(follow)...)
			}
		case nil:
			// ignore
		default:
			t.Fatalf("unexpected message type %T", typed)
		}
	}

	if model.responseLoading {
		t.Fatalf("expected responseLoading to be false after render completes")
	}
	if model.prettyView == "" {
		t.Fatalf("expected prettyView to be populated")
	}
	viewportContent := model.responseViewport.View()
	if !strings.Contains(viewportContent, "Status:") {
		t.Fatalf("expected viewport content to include response summary, got %q", viewportContent)
	}
}

func collectMsgs(cmd tea.Cmd) []tea.Msg {
	if cmd == nil {
		return nil
	}
	msg := cmd()
	if msg == nil {
		return nil
	}
	if batch, ok := msg.(tea.BatchMsg); ok {
		msgs := make([]tea.Msg, len(batch))
		for i, item := range batch {
			msgs[i] = item
		}
		return msgs
	}
	return []tea.Msg{msg}
}
