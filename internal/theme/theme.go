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

type EditorMetadataPalette struct {
	CommentMarker     lipgloss.Color
	DirectiveDefault  lipgloss.Color
	Value             lipgloss.Color
	SettingKey        lipgloss.Color
	SettingValue      lipgloss.Color
	RequestLine       lipgloss.Color
	RequestSeparator  lipgloss.Color
	RTSKeywordDefault lipgloss.Color
	RTSKeywordDecl    lipgloss.Color
	RTSKeywordControl lipgloss.Color
	RTSKeywordLiteral lipgloss.Color
	RTSKeywordLogical lipgloss.Color
	DirectiveColors   map[string]lipgloss.Color
}

type Theme struct {
	BrowserBorder                 lipgloss.Style
	EditorBorder                  lipgloss.Style
	ResponseBorder                lipgloss.Style
	NavigatorTitle                lipgloss.Style
	NavigatorTitleSelected        lipgloss.Style
	NavigatorSubtitle             lipgloss.Style
	NavigatorSubtitleSelected     lipgloss.Style
	NavigatorBadge                lipgloss.Style
	NavigatorTag                  lipgloss.Style
	AppFrame                      lipgloss.Style
	Header                        lipgloss.Style
	HeaderTitle                   lipgloss.Style
	HeaderValue                   lipgloss.Style
	HeaderSeparator               lipgloss.Style
	StatusBar                     lipgloss.Style
	StatusBarKey                  lipgloss.Style
	StatusBarValue                lipgloss.Style
	CommandBar                    lipgloss.Style
	CommandBarHint                lipgloss.Style
	ResponseSearchHighlight       lipgloss.Style
	ResponseSearchHighlightActive lipgloss.Style
	Tabs                          lipgloss.Style
	TabActive                     lipgloss.Style
	TabInactive                   lipgloss.Style
	Notification                  lipgloss.Style
	Error                         lipgloss.Style
	Success                       lipgloss.Style
	HeaderBrand                   lipgloss.Style
	HeaderSegments                []HeaderSegmentStyle
	CommandSegments               []CommandSegmentStyle
	CommandDivider                lipgloss.Style
	PaneTitle                     lipgloss.Style
	PaneTitleFile                 lipgloss.Style
	PaneTitleRequests             lipgloss.Style
	PaneDivider                   lipgloss.Style
	PaneBorderFocusFile           lipgloss.Color
	PaneBorderFocusRequests       lipgloss.Color
	PaneActiveForeground          lipgloss.Color
	EditorMetadata                EditorMetadataPalette
	EditorHintBox                 lipgloss.Style
	EditorHintItem                lipgloss.Style
	EditorHintSelected            lipgloss.Style
	EditorHintAnnotation          lipgloss.Style
	MethodColors                  MethodColors
	ListItemTitle                 lipgloss.Style
	ListItemDescription           lipgloss.Style
	ListItemSelectedTitle         lipgloss.Style
	ListItemSelectedDescription   lipgloss.Style
	ListItemDimmedTitle           lipgloss.Style
	ListItemDimmedDescription     lipgloss.Style
	ListItemFilterMatch           lipgloss.Style
	ResponseContent               lipgloss.Style
	ResponseContentRaw            lipgloss.Style
	ResponseContentHeaders        lipgloss.Style
	StreamContent                 lipgloss.Style
	StreamTimestamp               lipgloss.Style
	StreamDirectionSend           lipgloss.Style
	StreamDirectionReceive        lipgloss.Style
	StreamDirectionInfo           lipgloss.Style
	StreamEventName               lipgloss.Style
	StreamData                    lipgloss.Style
	StreamBinary                  lipgloss.Style
	StreamSummary                 lipgloss.Style
	StreamError                   lipgloss.Style
	StreamConsoleTitle            lipgloss.Style
	StreamConsoleMode             lipgloss.Style
	StreamConsoleStatus           lipgloss.Style
	StreamConsolePrompt           lipgloss.Style
	StreamConsoleInput            lipgloss.Style
	StreamConsoleInputFocused     lipgloss.Style
}

type MethodColors struct {
	GET     lipgloss.Color
	POST    lipgloss.Color
	PUT     lipgloss.Color
	PATCH   lipgloss.Color
	DELETE  lipgloss.Color
	HEAD    lipgloss.Color
	OPTIONS lipgloss.Color
	GRPC    lipgloss.Color
	WS      lipgloss.Color
	Default lipgloss.Color
}

func DefaultTheme() Theme {
	accent := lipgloss.Color("#7D56F4")
	base := lipgloss.NewStyle().Foreground(lipgloss.Color("#dcd7ff"))
	directiveAccent := lipgloss.Color("#56A9DD")

	return Theme{
		BrowserBorder: base.BorderStyle(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("#A78BFA")),
		EditorBorder: base.BorderStyle(lipgloss.RoundedBorder()).BorderForeground(accent),
		ResponseBorder: base.BorderStyle(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("#5FB3B3")),
		NavigatorTitle: lipgloss.NewStyle().Foreground(lipgloss.Color("#E6E1FF")),
		NavigatorTitleSelected: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#0F111A")).
			Background(lipgloss.Color("#FFD46A")).
			Bold(true),
		NavigatorSubtitle: lipgloss.NewStyle().Foreground(lipgloss.Color("#6E6A86")),
		NavigatorSubtitleSelected: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#0F111A")).
			Background(lipgloss.Color("#FFD46A")),
		NavigatorBadge: lipgloss.NewStyle().Padding(0, 1).Bold(true),
		NavigatorTag:   lipgloss.NewStyle().Foreground(lipgloss.Color("#A6A1BB")),
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
		ResponseSearchHighlight: lipgloss.NewStyle().
			Background(lipgloss.Color("#2C1E3A")).
			Foreground(lipgloss.Color("#E9E6FF")),
		ResponseSearchHighlightActive: lipgloss.NewStyle().
			Background(lipgloss.Color("#FFD46A")).
			Foreground(lipgloss.Color("#1A1020")).
			Bold(true),
		Tabs: lipgloss.NewStyle().Foreground(lipgloss.Color("#A6A1BB")).Padding(0, 1),
		TabActive: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FDFBFF")).
			Background(lipgloss.Color("#7D56F4")).
			Bold(true).
			Padding(0, 2),
		TabInactive: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#5E5A72")).
			Padding(0, 1),
		Notification: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#E0DEF4")).
			Background(lipgloss.Color("#433C59")).
			Padding(0, 1),
		Error:   lipgloss.NewStyle().Foreground(lipgloss.Color("#FF6E6E")),
		Success: lipgloss.NewStyle().Foreground(lipgloss.Color("#6EF17E")),
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
				Background: lipgloss.Color("#9CD6FF"),
				Border:     lipgloss.Color("#B9E1FF"),
				Foreground: lipgloss.Color("#0D2C3D"),
				Accent:     lipgloss.Color("#134158"),
			},
			{
				Background: lipgloss.Color("#B8F5C9"),
				Border:     lipgloss.Color("#D3FBE0"),
				Foreground: lipgloss.Color("#0F2E1A"),
				Accent:     lipgloss.Color("#18472A"),
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
		CommandDivider: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#403B59")).
			Bold(true),
		PaneTitle: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#A6A1BB")).
			Bold(true),
		PaneTitleFile: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#7D56F4")).
			Bold(true),
		PaneTitleRequests: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#15AABF")).
			Bold(true),
		PaneDivider:             lipgloss.NewStyle().Foreground(lipgloss.Color("#3A3547")),
		PaneBorderFocusFile:     lipgloss.Color("#7D56F4"),
		PaneBorderFocusRequests: lipgloss.Color("#15AABF"),
		PaneActiveForeground:    lipgloss.Color("#F5F2FF"),
		EditorMetadata: EditorMetadataPalette{
			CommentMarker:    lipgloss.Color("#5E5A72"),
			DirectiveDefault: directiveAccent,
			Value:            lipgloss.Color("#E6E1FF"),
			SettingKey:       lipgloss.Color("#FFD46A"),
			SettingValue:     lipgloss.Color("#FFEBC5"),
			RequestLine:      lipgloss.Color("#FF6E6E"),
			RequestSeparator: lipgloss.Color(
				"#626166",
			), // still debating with myself if i want this
			RTSKeywordDefault: directiveAccent,
			RTSKeywordDecl:    directiveAccent,
			RTSKeywordControl: lipgloss.Color("#FFD46A"),
			RTSKeywordLiteral: lipgloss.Color("#6EF17E"),
			RTSKeywordLogical: lipgloss.Color("#FF8B39"),
			DirectiveColors: map[string]lipgloss.Color{
				"name":              directiveAccent,
				"description":       directiveAccent,
				"desc":              directiveAccent,
				"tag":               directiveAccent,
				"auth":              directiveAccent,
				"graphql":           directiveAccent,
				"graphql-operation": directiveAccent,
				"operation":         directiveAccent,
				"variables":         directiveAccent,
				"graphql-variables": directiveAccent,
				"query":             directiveAccent,
				"graphql-query":     directiveAccent,
				"grpc":              directiveAccent,
				"grpc-descriptor":   directiveAccent,
				"grpc-reflection":   directiveAccent,
				"grpc-plaintext":    directiveAccent,
				"grpc-authority":    directiveAccent,
				"grpc-metadata":     directiveAccent,
				"setting":           directiveAccent,
				"timeout":           directiveAccent,
				"script":            directiveAccent,
				"no-log":            directiveAccent,
			},
		},
		EditorHintBox: lipgloss.NewStyle().
			BorderStyle(lipgloss.RoundedBorder()).
			BorderForeground(accent).
			Padding(0, 1).
			Foreground(lipgloss.Color("#E6E1FF")),
		EditorHintItem: lipgloss.NewStyle().Foreground(lipgloss.Color("#D8D4F1")),
		EditorHintSelected: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#1A1020")).
			Background(lipgloss.Color("#FFD46A")).
			Bold(true),
		EditorHintAnnotation:        lipgloss.NewStyle().Foreground(lipgloss.Color("#A6A1BB")),
		ListItemTitle:               lipgloss.NewStyle().Foreground(lipgloss.Color("#E6E1FF")),
		ListItemDescription:         lipgloss.NewStyle().Foreground(lipgloss.Color("#7d7b87")),
		ListItemSelectedTitle:       lipgloss.Style{},
		ListItemSelectedDescription: lipgloss.Style{},
		ListItemDimmedTitle:         lipgloss.NewStyle().Foreground(lipgloss.Color("#5E5A72")),
		ListItemDimmedDescription:   lipgloss.NewStyle().Foreground(lipgloss.Color("#4A4760")),
		ListItemFilterMatch: lipgloss.NewStyle().
			Underline(true).
			Foreground(lipgloss.Color("#B9A5FF")),
		MethodColors: MethodColors{
			GET:     lipgloss.Color("#34d399"),
			POST:    lipgloss.Color("#60a5fa"),
			PUT:     lipgloss.Color("#f59e0b"),
			PATCH:   lipgloss.Color("#14b8a6"),
			DELETE:  lipgloss.Color("#f87171"),
			HEAD:    lipgloss.Color("#a1a1aa"),
			OPTIONS: lipgloss.Color("#c084fc"),
			GRPC:    lipgloss.Color("#22d3ee"),
			WS:      lipgloss.Color("#fb923c"),
			Default: lipgloss.Color("#9ca3af"),
		},
		ResponseContent:        lipgloss.NewStyle(),
		ResponseContentRaw:     lipgloss.NewStyle().Foreground(lipgloss.Color("#E6E1FF")),
		ResponseContentHeaders: lipgloss.NewStyle().Foreground(lipgloss.Color("#C7C4E0")),
		StreamContent:          lipgloss.NewStyle(),
		StreamTimestamp:        lipgloss.NewStyle().Foreground(lipgloss.Color("#6E6A86")),
		StreamDirectionSend: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FF6E6E")).
			Bold(true),
		StreamDirectionReceive: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#6EF17E")).
			Bold(true),
		StreamDirectionInfo: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FFD46A")).
			Bold(true),
		StreamEventName: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#7D56F4")).
			Bold(true),
		StreamData:   lipgloss.NewStyle().Foreground(lipgloss.Color("#EAEAEA")),
		StreamBinary: lipgloss.NewStyle().Foreground(lipgloss.Color("#FFC078")),
		StreamSummary: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#A6A1BB")).
			Italic(true),
		StreamError: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FF6E6E")).
			Bold(true),
		StreamConsoleTitle: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#7D56F4")).
			Bold(true),
		StreamConsoleMode: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#33C481")).
			Bold(true),
		StreamConsoleStatus: lipgloss.NewStyle().Foreground(lipgloss.Color("#FFD46A")),
		StreamConsolePrompt: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#7D56F4")).
			Bold(true),
		StreamConsoleInput: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#EAEAEA")).
			BorderStyle(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("#5E5A72")).
			Padding(0, 1),
		StreamConsoleInputFocused: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FDFBFF")).
			BorderStyle(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("#7D56F4")).
			Padding(0, 1),
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
