package theme

import "github.com/charmbracelet/lipgloss"

type HeaderSegmentStyle struct {
	Background lipgloss.Color
	Border     lipgloss.Color
	Foreground lipgloss.Color
	Accent     lipgloss.Color
}

type CommandSegmentStyle struct {
	Background lipgloss.Color
	Border     lipgloss.Color
	Key        lipgloss.Color
	Text       lipgloss.Color
}

type Theme struct {
	BrowserBorder           lipgloss.Style
	EditorBorder            lipgloss.Style
	ResponseBorder          lipgloss.Style
	AppFrame                lipgloss.Style
	Header                  lipgloss.Style
	HeaderTitle             lipgloss.Style
	HeaderValue             lipgloss.Style
	HeaderSeparator         lipgloss.Style
	StatusBar               lipgloss.Style
	StatusBarKey            lipgloss.Style
	StatusBarValue          lipgloss.Style
	CommandBar              lipgloss.Style
	CommandBarHint          lipgloss.Style
	Tabs                    lipgloss.Style
	TabActive               lipgloss.Style
	TabInactive             lipgloss.Style
	Notification            lipgloss.Style
	Error                   lipgloss.Style
	Success                 lipgloss.Style
	HeaderBrand             lipgloss.Style
	HeaderSegments          []HeaderSegmentStyle
	CommandSegments         []CommandSegmentStyle
	CommandDivider          lipgloss.Style
	PaneTitle               lipgloss.Style
	PaneTitleFile           lipgloss.Style
	PaneTitleRequests       lipgloss.Style
	PaneDivider             lipgloss.Style
	PaneBorderFocusFile     lipgloss.Color
	PaneBorderFocusRequests lipgloss.Color
	PaneActiveForeground    lipgloss.Color
}

func DefaultTheme() Theme {
	accent := lipgloss.Color("#7D56F4")
	base := lipgloss.NewStyle().Foreground(lipgloss.Color("#dcd7ff"))

	return Theme{
		BrowserBorder:  base.Copy().BorderStyle(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("#A78BFA")),
		EditorBorder:   base.Copy().BorderStyle(lipgloss.RoundedBorder()).BorderForeground(accent),
		ResponseBorder: base.Copy().BorderStyle(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("#5FB3B3")),
		AppFrame: lipgloss.NewStyle().
			BorderStyle(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("#403B59")),
		Header:          lipgloss.NewStyle().Foreground(lipgloss.Color("#E5E1FF")).Padding(0, 1),
		HeaderTitle:     lipgloss.NewStyle().Foreground(lipgloss.Color("#7D56F4")).Bold(true),
		HeaderValue:     lipgloss.NewStyle().Foreground(lipgloss.Color("#D1CFF6")),
		HeaderSeparator: lipgloss.NewStyle().Foreground(lipgloss.Color("#867CC1")).Bold(true),
		StatusBar:       lipgloss.NewStyle().Foreground(lipgloss.Color("#A6A1BB")).Padding(0, 1),
		StatusBarKey:    lipgloss.NewStyle().Foreground(lipgloss.Color("#FF8B39")).Bold(true),
		StatusBarValue:  lipgloss.NewStyle().Foreground(lipgloss.Color("#EAEAEA")),
		CommandBar:      lipgloss.NewStyle().Foreground(lipgloss.Color("#C2C0D9")).Padding(0, 1),
		CommandBarHint:  lipgloss.NewStyle().Foreground(lipgloss.Color("#7D56F4")).Bold(true),
		Tabs:            lipgloss.NewStyle().Foreground(lipgloss.Color("#A6A1BB")).Padding(0, 1),
		TabActive: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FDFBFF")).
			Background(lipgloss.Color("#7D56F4")).
			Bold(true).
			Padding(0, 2),
		TabInactive: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#5E5A72")).
			Padding(0, 1),
		Notification: lipgloss.NewStyle().Foreground(lipgloss.Color("#E0DEF4")).Background(lipgloss.Color("#433C59")).Padding(0, 1),
		Error:        lipgloss.NewStyle().Foreground(lipgloss.Color("#FF6E6E")),
		Success:      lipgloss.NewStyle().Foreground(lipgloss.Color("#6EF17E")),
		HeaderBrand: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#1A1020")).
			Background(lipgloss.Color("#FBC859")).
			Bold(true).
			Padding(0, 1).
			BorderStyle(lipgloss.Border{
				Top:         "",
				Bottom:      "",
				Left:        "┃",
				Right:       "┃",
				TopLeft:     "",
				TopRight:    "",
				BottomLeft:  "",
				BottomRight: "",
			}).
			BorderForeground(lipgloss.Color("#FFE29B")),
		HeaderSegments: []HeaderSegmentStyle{
			{
				Background: lipgloss.Color("#7D56F4"),
				Border:     lipgloss.Color("#A58CFF"),
				Foreground: lipgloss.Color("#F5F2FF"),
				Accent:     lipgloss.Color("#FFFFFF"),
			},
			{
				Background: lipgloss.Color("#15AABF"),
				Border:     lipgloss.Color("#2EC6D6"),
				Foreground: lipgloss.Color("#EFFDFF"),
				Accent:     lipgloss.Color("#FFFFFF"),
			},
			{
				Background: lipgloss.Color("#FF7A45"),
				Border:     lipgloss.Color("#FF9F70"),
				Foreground: lipgloss.Color("#1F0F0A"),
				Accent:     lipgloss.Color("#301B15"),
			},
			{
				Background: lipgloss.Color("#33C481"),
				Border:     lipgloss.Color("#5EE0A0"),
				Foreground: lipgloss.Color("#052817"),
				Accent:     lipgloss.Color("#06331D"),
			},
			{
				Background: lipgloss.Color("#FFB61E"),
				Border:     lipgloss.Color("#FFD46A"),
				Foreground: lipgloss.Color("#1F1500"),
				Accent:     lipgloss.Color("#332300"),
			},
		},
		CommandSegments: []CommandSegmentStyle{
			{
				Background: lipgloss.Color("#2C1E3A"),
				Border:     lipgloss.Color("#7D56F4"),
				Key:        lipgloss.Color("#F6E3FF"),
				Text:       lipgloss.Color("#E5E1FF"),
			},
			{
				Background: lipgloss.Color("#102B33"),
				Border:     lipgloss.Color("#15AABF"),
				Key:        lipgloss.Color("#A7F2FF"),
				Text:       lipgloss.Color("#D6F7FF"),
			},
			{
				Background: lipgloss.Color("#32160E"),
				Border:     lipgloss.Color("#FF7A45"),
				Key:        lipgloss.Color("#FFE0D3"),
				Text:       lipgloss.Color("#FFD4C2"),
			},
			{
				Background: lipgloss.Color("#0F2F20"),
				Border:     lipgloss.Color("#33C481"),
				Key:        lipgloss.Color("#C0F5DF"),
				Text:       lipgloss.Color("#D6F9E8"),
			},
			{
				Background: lipgloss.Color("#332408"),
				Border:     lipgloss.Color("#FFB61E"),
				Key:        lipgloss.Color("#FFECC0"),
				Text:       lipgloss.Color("#FFF3D8"),
			},
		},
		CommandDivider:          lipgloss.NewStyle().Foreground(lipgloss.Color("#403B59")).Bold(true),
		PaneTitle:               lipgloss.NewStyle().Foreground(lipgloss.Color("#A6A1BB")).Bold(true),
		PaneTitleFile:           lipgloss.NewStyle().Foreground(lipgloss.Color("#7D56F4")).Bold(true),
		PaneTitleRequests:       lipgloss.NewStyle().Foreground(lipgloss.Color("#15AABF")).Bold(true),
		PaneDivider:             lipgloss.NewStyle().Foreground(lipgloss.Color("#3A3547")),
		PaneBorderFocusFile:     lipgloss.Color("#7D56F4"),
		PaneBorderFocusRequests: lipgloss.Color("#15AABF"),
		PaneActiveForeground:    lipgloss.Color("#F5F2FF"),
	}
}

func (t Theme) HeaderSegment(idx int) HeaderSegmentStyle {
	if len(t.HeaderSegments) == 0 {
		return HeaderSegmentStyle{
			Background: lipgloss.Color("#3B355D"),
			Border:     lipgloss.Color("#5F5689"),
			Foreground: lipgloss.Color("#F5F2FF"),
			Accent:     lipgloss.Color("#FFFFFF"),
		}
	}
	return t.HeaderSegments[idx%len(t.HeaderSegments)]
}

func (t Theme) CommandSegment(idx int) CommandSegmentStyle {
	if len(t.CommandSegments) == 0 {
		return CommandSegmentStyle{
			Background: lipgloss.Color("#2C1E3A"),
			Border:     lipgloss.Color("#7D56F4"),
			Key:        lipgloss.Color("#F6E3FF"),
			Text:       lipgloss.Color("#E5E1FF"),
		}
	}
	return t.CommandSegments[idx%len(t.CommandSegments)]
}
