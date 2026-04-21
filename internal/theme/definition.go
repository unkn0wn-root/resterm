package theme

import "strings"

type Appearance int

const (
	AppearanceUnknown Appearance = iota
	AppearanceDark
	AppearanceLight
)

func (a Appearance) String() string {
	switch a {
	case AppearanceDark:
		return "dark"
	case AppearanceLight:
		return "light"
	default:
		return "unknown"
	}
}

func (m Metadata) HasTag(tag string) bool {
	tag = strings.ToLower(strings.TrimSpace(tag))
	if tag == "" {
		return false
	}
	for _, item := range m.Tags {
		if strings.EqualFold(strings.TrimSpace(item), tag) {
			return true
		}
	}
	return false
}

func (m Metadata) Appearance() Appearance {
	switch {
	case m.HasTag("light"):
		return AppearanceLight
	case m.HasTag("dark"):
		return AppearanceDark
	default:
		return AppearanceUnknown
	}
}

func (d Definition) Appearance() Appearance {
	return d.Metadata.Appearance()
}

func DefaultDefinition() Definition {
	return Definition{
		Key:         "default",
		DisplayName: "Default",
		Metadata: Metadata{
			Name: "Default",
			Tags: []string{"dark"},
		},
		Theme:  DefaultTheme(),
		Source: SourceBuiltin,
		Format: FormatBuiltin,
	}
}
