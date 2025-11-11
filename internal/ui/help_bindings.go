package ui

import (
	"strings"

	"github.com/unkn0wn-root/resterm/internal/bindings"
)

var helpTokenMap = map[string]string{
	"tab":    "Tab",
	"enter":  "Enter",
	"space":  "Space",
	"home":   "Home",
	"end":    "End",
	"pgup":   "PgUp",
	"pgdown": "PgDn",
	"up":     "Up",
	"down":   "Down",
	"left":   "Left",
	"right":  "Right",
	"esc":    "Esc",
}

func (m *Model) helpActionKey(action bindings.ActionID, fallback string) string {
	if label := m.helpBindingLabel(action); label != "" {
		return label
	}
	return fallback
}

func (m *Model) helpCombinedKey(actions []bindings.ActionID, fallback string) string {
	var labels []string
	for _, action := range actions {
		if label := m.helpBindingLabel(action); label != "" {
			labels = append(labels, label)
		}
	}
	if len(labels) == 0 {
		return fallback
	}
	return strings.Join(labels, " / ")
}

func (m *Model) helpBindingLabel(action bindings.ActionID) string {
	if m.bindingsMap == nil {
		return ""
	}
	bindings := m.bindingsMap.Bindings(action)
	if len(bindings) == 0 {
		return ""
	}
	labels := make([]string, 0, len(bindings))
	for _, binding := range bindings {
		if label := formatHelpBinding(binding); label != "" {
			labels = append(labels, label)
		}
	}
	return strings.Join(labels, " / ")
}

func formatHelpBinding(binding bindings.Binding) string {
	if len(binding.Steps) == 0 {
		return ""
	}
	steps := make([]string, len(binding.Steps))
	for i, step := range binding.Steps {
		steps[i] = formatHelpBindingStep(step)
	}
	return strings.Join(steps, " ")
}

func formatHelpBindingStep(step string) string {
	trimmed := strings.TrimSpace(step)
	if trimmed == "" {
		return ""
	}
	if trimmed == "shift+/" {
		return "?"
	}
	parts := strings.Split(trimmed, "+")
	hasModifier := len(parts) > 1
	for i, part := range parts {
		parts[i] = formatHelpToken(part, hasModifier)
	}
	return strings.Join(parts, "+")
}

func formatHelpToken(token string, capitalizeSingle bool) string {
	lowered := strings.ToLower(strings.TrimSpace(token))
	if lowered == "" {
		return ""
	}
	switch lowered {
	case "ctrl":
		return "Ctrl"
	case "alt":
		return "Alt"
	case "shift":
		return "Shift"
	case "cmd":
		return "Cmd"
	case "option":
		return "Option"
	case "meta":
		return "Meta"
	}
	if mapped, ok := helpTokenMap[lowered]; ok {
		return mapped
	}
	if len(lowered) == 1 {
		if capitalizeSingle {
			return strings.ToUpper(lowered)
		}
		return lowered
	}
	return strings.ToUpper(lowered[:1]) + lowered[1:]
}
