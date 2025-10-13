package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	"github.com/unkn0wn-root/resterm/internal/restfile"
)

type workflowListItem struct {
	workflow *restfile.Workflow
	index    int
}

func (i workflowListItem) Title() string {
	if i.workflow == nil {
		return ""
	}
	name := strings.TrimSpace(i.workflow.Name)
	if name == "" {
		name = fmt.Sprintf("Workflow %d", i.index+1)
	}
	base := []string{name}
	if tags := joinTags(i.workflow.Tags, 3); tags != "" {
		base = append(base, tags)
	}
	return strings.Join(base, " ")
}

func (i workflowListItem) Description() string {
	if i.workflow == nil {
		return ""
	}
	desc := strings.TrimSpace(i.workflow.Description)
	if desc != "" {
		desc = condense(desc, 80)
	}
	count := len(i.workflow.Steps)
	info := fmt.Sprintf("%d step", count)
	if count != 1 {
		info += "s"
	}
	if desc == "" {
		return info
	}
	return strings.Join([]string{desc, info}, "\n")
}

func (i workflowListItem) FilterValue() string {
	if i.workflow == nil {
		return ""
	}
	parts := []string{
		i.workflow.Name,
		i.workflow.Description,
		strings.Join(i.workflow.Tags, " "),
	}
	for _, step := range i.workflow.Steps {
		parts = append(parts, step.Name, step.Using)
	}
	return strings.Join(parts, " ")
}

func buildWorkflowItems(doc *restfile.Document) ([]workflowListItem, []list.Item) {
	if doc == nil || len(doc.Workflows) == 0 {
		return nil, nil
	}
	items := make([]workflowListItem, len(doc.Workflows))
	listItems := make([]list.Item, len(doc.Workflows))
	for idx := range doc.Workflows {
		workflow := &doc.Workflows[idx]
		item := workflowListItem{workflow: workflow, index: idx}
		items[idx] = item
		listItems[idx] = item
	}
	return items, listItems
}

func workflowKey(s *restfile.Workflow) string {
	if s == nil {
		return ""
	}
	if name := strings.TrimSpace(s.Name); name != "" {
		return strings.ToLower(name)
	}
	if s.LineRange.Start > 0 {
		return fmt.Sprintf("line:%d", s.LineRange.Start)
	}
	return fmt.Sprintf("idx:%d", len(s.Steps))
}
