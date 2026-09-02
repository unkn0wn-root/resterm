package theme

import "github.com/charmbracelet/lipgloss"

type CommandSegmentStyle struct {
	Background lipgloss.Color
	Border     lipgloss.Color
	Key        lipgloss.Color
	Text       lipgloss.Color
}

type GitColors struct {
	Branch    lipgloss.Color
	Modified  lipgloss.Color
	Added     lipgloss.Color
	Untracked lipgloss.Color
	Deleted   lipgloss.Color
	Renamed   lipgloss.Color
	Conflict  lipgloss.Color
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
	RTSFunction       lipgloss.Color
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
	StatusBarPalette              StatusBarPalette
	StatusBarInfo                 lipgloss.Style
	StatusBarKey                  lipgloss.Style
	StatusBarValue                lipgloss.Style
	CommandBar                    lipgloss.Style
	CommandBarHint                lipgloss.Style
	CLIRunPicker                  lipgloss.Style
	CLIRunPickerSelected          lipgloss.Style
	CLIRunPickerCursor            lipgloss.Style
	CLIRunPickerCursorSelected    lipgloss.Style
	ResponseSearchHighlight       lipgloss.Style
	ResponseSearchHighlightActive lipgloss.Style
	Tabs                          lipgloss.Style
	TabActive                     lipgloss.Style
	TabInactive                   lipgloss.Style
	Notification                  lipgloss.Style
	Error                         lipgloss.Style
	Success                       lipgloss.Style
	HeaderBrand                   lipgloss.Style
	HeaderIcon                    lipgloss.Style
	HeaderLabel                   lipgloss.Style
	HeaderHelp                    lipgloss.Style
	HeaderWarn                    lipgloss.Style
	CommandSegments               []CommandSegmentStyle
	CommandDivider                lipgloss.Style
	PaneTitle                     lipgloss.Style
	PaneTitleFile                 lipgloss.Style
	PaneTitleRequests             lipgloss.Style
	PaneTitleEditor               lipgloss.Style
	PaneTitleResponse             lipgloss.Style
	PaneDivider                   lipgloss.Style
	PaneBorderFocusFile           lipgloss.Color
	PaneBorderFocusRequests       lipgloss.Color
	PaneBorderFocusEditor         lipgloss.Color
	PaneBorderFocusResponse       lipgloss.Color
	PaneActiveForeground          lipgloss.Color
	ModalBackdrop                 lipgloss.Color
	ModalInputBackground          lipgloss.Color
	ModalOption                   lipgloss.Color
	GitColors                     GitColors
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
	ResponseSelection             lipgloss.Style
	ResponseCursor                lipgloss.Style
	ExplainLabel                  lipgloss.Style
	ExplainValue                  lipgloss.Style
	ExplainMuted                  lipgloss.Style
	ExplainSectionTitle           lipgloss.Style
	ExplainSectionBorder          lipgloss.Style
	ExplainBadgeReady             lipgloss.Style
	ExplainBadgeSkipped           lipgloss.Style
	ExplainBadgeError             lipgloss.Style
	ExplainStageOK                lipgloss.Style
	ExplainStageSkipped           lipgloss.Style
	ExplainStageError             lipgloss.Style
	ExplainChangeAdd              lipgloss.Style
	ExplainChangeRemove           lipgloss.Style
	ExplainChangeUpdate           lipgloss.Style
	ExplainWarning                lipgloss.Style
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
	editorAccent := lipgloss.Color("#56A9DD")
	responseAccent := lipgloss.Color("#33C481")
	muted := lipgloss.Color("#8B86A8")
	base := lipgloss.NewStyle().Foreground(lipgloss.Color("#dcd7ff"))
	directiveAccent := editorAccent

	return Theme{
		BrowserBorder: base.BorderStyle(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("#A78BFA")),
		EditorBorder: base.BorderStyle(lipgloss.RoundedBorder()).
			BorderForeground(accent),
		ResponseBorder: base.BorderStyle(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("#5FB3B3")),
		NavigatorTitle: lipgloss.NewStyle().Foreground(lipgloss.Color("#E6E1FF")),
		NavigatorTitleSelected: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#F3ECFF")).
			Background(lipgloss.Color("#2A2140")).
			Bold(true),
		NavigatorSubtitle: lipgloss.NewStyle().Foreground(lipgloss.Color("#6E6A86")),
		NavigatorSubtitleSelected: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#C9B8FF")).
			Background(lipgloss.Color("#2A2140")),
		NavigatorBadge: lipgloss.NewStyle().Padding(0, 1).Bold(true),
		NavigatorTag:   lipgloss.NewStyle().Foreground(lipgloss.Color("#A6A1BB")),
		AppFrame: lipgloss.NewStyle().
			BorderStyle(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("#403B59")),
		Header:           lipgloss.NewStyle().Foreground(lipgloss.Color("#E5E1FF")).Padding(0, 1),
		HeaderTitle:      lipgloss.NewStyle().Foreground(lipgloss.Color("#7D56F4")).Bold(true),
		HeaderValue:      lipgloss.NewStyle().Foreground(lipgloss.Color("#D1CFF6")),
		HeaderSeparator:  lipgloss.NewStyle().Foreground(lipgloss.Color("#6E6A86")),
		StatusBar:        lipgloss.NewStyle().Foreground(lipgloss.Color("#A6A1BB")).Padding(0, 1),
		StatusBarPalette: DefaultStatusBarPalette(),
		StatusBarKey:     lipgloss.NewStyle().Foreground(lipgloss.Color("#FF8B39")).Bold(true),
		StatusBarValue:   lipgloss.NewStyle().Foreground(lipgloss.Color("#EAEAEA")),
		CommandBar:       lipgloss.NewStyle().Foreground(lipgloss.Color("#C2C0D9")).Padding(0, 1),
		CommandBarHint:   lipgloss.NewStyle().Foreground(lipgloss.Color("#7D56F4")).Bold(true),
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
			Background(accent).
			Bold(true).
			Padding(0, 2),
		TabInactive: lipgloss.NewStyle().
			Foreground(muted).
			Padding(0, 1),
		Notification: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#E0DEF4")).
			Background(lipgloss.Color("#433C59")).
			Padding(0, 1),
		Error:       lipgloss.NewStyle().Foreground(lipgloss.Color("#FF6E6E")),
		Success:     lipgloss.NewStyle().Foreground(lipgloss.Color("#6EF17E")),
		HeaderBrand: lipgloss.NewStyle().Foreground(lipgloss.Color("#FBC859")).Bold(true),
		HeaderIcon:  lipgloss.NewStyle().Foreground(lipgloss.Color("#9683D8")),
		HeaderLabel: lipgloss.NewStyle().Foreground(lipgloss.Color("#A6A1BB")),
		HeaderHelp:  lipgloss.NewStyle().Foreground(lipgloss.Color("#7DD3FC")).Bold(true),
		HeaderWarn:  lipgloss.NewStyle().Foreground(lipgloss.Color("#FFD46A")),
		CommandSegments: []CommandSegmentStyle{
			{
				Background: lipgloss.Color(""),
				Border:     lipgloss.Color("#7D56F4"),
				Key:        lipgloss.Color("#F6E3FF"),
				Text:       muted,
			},
			{
				Background: lipgloss.Color(""),
				Border:     lipgloss.Color("#15AABF"),
				Key:        lipgloss.Color("#A7F2FF"),
				Text:       muted,
			},
			{
				Background: lipgloss.Color(""),
				Border:     lipgloss.Color("#FF7A45"),
				Key:        lipgloss.Color("#FFE0D3"),
				Text:       muted,
			},
			{
				Background: lipgloss.Color(""),
				Border:     lipgloss.Color("#33C481"),
				Key:        lipgloss.Color("#C0F5DF"),
				Text:       muted,
			},
			{
				Background: lipgloss.Color(""),
				Border:     lipgloss.Color("#FFB61E"),
				Key:        lipgloss.Color("#FFECC0"),
				Text:       muted,
			},
		},
		CommandDivider: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#403B59")).
			Bold(true),
		PaneTitle:               lipgloss.NewStyle().Bold(true),
		PaneTitleFile:           lipgloss.NewStyle().Bold(true),
		PaneTitleRequests:       lipgloss.NewStyle().Bold(true),
		PaneTitleEditor:         lipgloss.NewStyle().Bold(true),
		PaneTitleResponse:       lipgloss.NewStyle().Bold(true),
		PaneDivider:             lipgloss.NewStyle().Foreground(lipgloss.Color("#5B526E")),
		PaneBorderFocusFile:     lipgloss.Color("#FFD46A"),
		PaneBorderFocusRequests: lipgloss.Color("#FFD46A"),
		PaneBorderFocusEditor:   editorAccent,
		PaneBorderFocusResponse: responseAccent,
		PaneActiveForeground:    lipgloss.Color("#F5F2FF"),
		GitColors: GitColors{
			Branch:    lipgloss.Color("#A78BFA"),
			Modified:  lipgloss.Color("#FFD46A"),
			Added:     lipgloss.Color("#6EF17E"),
			Untracked: lipgloss.Color("#56A9DD"),
			Deleted:   lipgloss.Color("#FF6E6E"),
			Renamed:   lipgloss.Color("#C9B8FF"),
			Conflict:  lipgloss.Color("#FF8B39"),
		},
		EditorMetadata: EditorMetadataPalette{
			CommentMarker:    lipgloss.Color("#8B86A8"),
			DirectiveDefault: directiveAccent,
			Value:            lipgloss.Color("#E6E1FF"),
			SettingKey:       lipgloss.Color("#9EC1DE"),
			SettingValue:     lipgloss.Color("#FFEBC5"),
			RequestLine:      lipgloss.Color("#FF6E6E"),
			RequestSeparator: lipgloss.Color(
				"#626166",
			), // still debating with myself if i want this
			RTSKeywordDefault: directiveAccent,
			RTSKeywordDecl:    directiveAccent,
			RTSKeywordControl: lipgloss.Color("#D48AD0"),
			RTSKeywordLiteral: lipgloss.Color("#6EF17E"),
			RTSKeywordLogical: lipgloss.Color("#FF8B39"),
			RTSFunction:       lipgloss.Color("#B9A5FF"),
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
			BorderForeground(editorAccent).
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
		ResponseSelection:      lipgloss.NewStyle().Background(lipgloss.Color("#3A2B52")),
		ResponseCursor: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#A6A1BB")).
			Bold(true),
		ExplainLabel:        lipgloss.NewStyle().Foreground(lipgloss.Color("#8FD3FF")).Bold(true),
		ExplainValue:        lipgloss.NewStyle().Foreground(lipgloss.Color("#F3F0FF")),
		ExplainMuted:        lipgloss.NewStyle().Foreground(lipgloss.Color("#A6A1BB")),
		ExplainSectionTitle: lipgloss.NewStyle().Foreground(lipgloss.Color("#7D56F4")).Bold(true),
		ExplainSectionBorder: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#4F4670")),
		ExplainBadgeReady: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#062311")).
			Background(lipgloss.Color("#6EF17E")).
			Bold(true).
			Padding(0, 1),
		ExplainBadgeSkipped: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#2B1C00")).
			Background(lipgloss.Color("#FFD46A")).
			Bold(true).
			Padding(0, 1),
		ExplainBadgeError: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#280B0B")).
			Background(lipgloss.Color("#FF8A8A")).
			Bold(true).
			Padding(0, 1),
		ExplainStageOK: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#6EF17E")).
			Bold(true),
		ExplainStageSkipped: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FFD46A")).
			Bold(true),
		ExplainStageError: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FF6E6E")).
			Bold(true),
		ExplainChangeAdd: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#6EF17E")).
			Bold(true),
		ExplainChangeRemove: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FF6E6E")).
			Bold(true),
		ExplainChangeUpdate: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#7D56F4")).
			Bold(true),
		ExplainWarning: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FFD46A")).
			Bold(true),
		StreamContent:   lipgloss.NewStyle(),
		StreamTimestamp: lipgloss.NewStyle().Foreground(lipgloss.Color("#6E6A86")),
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

func (t Theme) CommandSegment(idx int) CommandSegmentStyle {
	if len(t.CommandSegments) == 0 {
		return CommandSegmentStyle{
			Background: lipgloss.Color(""),
			Border:     lipgloss.Color("#7D56F4"),
			Key:        lipgloss.Color("#F6E3FF"),
			Text:       lipgloss.Color("#E5E1FF"),
		}
	}
	return t.CommandSegments[idx%len(t.CommandSegments)]
}
