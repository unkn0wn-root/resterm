package theme

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

type Metadata struct {
	Name        string   `json:"name"        toml:"name"`
	Description string   `json:"description" toml:"description"`
	Author      string   `json:"author"      toml:"author"`
	Version     string   `json:"version"     toml:"version"`
	Tags        []string `json:"tags"        toml:"tags"`
}

type ThemeSpec struct {
	Metadata        *Metadata            `json:"metadata"         toml:"metadata"`
	Styles          StylesSpec           `json:"styles"           toml:"styles"`
	Colors          ColorsSpec           `json:"colors"           toml:"colors"`
	HeaderSegments  []HeaderSegmentSpec  `json:"header_segments"  toml:"header_segments"`
	CommandSegments []CommandSegmentSpec `json:"command_segments" toml:"command_segments"`
	EditorMetadata  *EditorMetadataSpec  `json:"editor_metadata"  toml:"editor_metadata"`
}

type StylesSpec struct {
	BrowserBorder                 *StyleSpec `json:"browser_border"                   toml:"browser_border"`
	EditorBorder                  *StyleSpec `json:"editor_border"                    toml:"editor_border"`
	ResponseBorder                *StyleSpec `json:"response_border"                  toml:"response_border"`
	NavigatorTitle                *StyleSpec `json:"navigator_title"                  toml:"navigator_title"`
	NavigatorTitleSelected        *StyleSpec `json:"navigator_title_selected"         toml:"navigator_title_selected"`
	NavigatorSubtitle             *StyleSpec `json:"navigator_subtitle"               toml:"navigator_subtitle"`
	NavigatorSubtitleSelected     *StyleSpec `json:"navigator_subtitle_selected"      toml:"navigator_subtitle_selected"`
	NavigatorBadge                *StyleSpec `json:"navigator_badge"                  toml:"navigator_badge"`
	NavigatorTag                  *StyleSpec `json:"navigator_tag"                    toml:"navigator_tag"`
	AppFrame                      *StyleSpec `json:"app_frame"                        toml:"app_frame"`
	Header                        *StyleSpec `json:"header"                           toml:"header"`
	HeaderTitle                   *StyleSpec `json:"header_title"                     toml:"header_title"`
	HeaderValue                   *StyleSpec `json:"header_value"                     toml:"header_value"`
	HeaderSeparator               *StyleSpec `json:"header_separator"                 toml:"header_separator"`
	StatusBar                     *StyleSpec `json:"status_bar"                       toml:"status_bar"`
	StatusBarKey                  *StyleSpec `json:"status_bar_key"                   toml:"status_bar_key"`
	StatusBarValue                *StyleSpec `json:"status_bar_value"                 toml:"status_bar_value"`
	CommandBar                    *StyleSpec `json:"command_bar"                      toml:"command_bar"`
	CommandBarHint                *StyleSpec `json:"command_bar_hint"                 toml:"command_bar_hint"`
	ResponseSearchHighlight       *StyleSpec `json:"response_search_highlight"        toml:"response_search_highlight"`
	ResponseSearchHighlightActive *StyleSpec `json:"response_search_highlight_active" toml:"response_search_highlight_active"`
	Tabs                          *StyleSpec `json:"tabs"                             toml:"tabs"`
	TabActive                     *StyleSpec `json:"tab_active"                       toml:"tab_active"`
	TabInactive                   *StyleSpec `json:"tab_inactive"                     toml:"tab_inactive"`
	Notification                  *StyleSpec `json:"notification"                     toml:"notification"`
	Error                         *StyleSpec `json:"error"                            toml:"error"`
	Success                       *StyleSpec `json:"success"                          toml:"success"`
	HeaderBrand                   *StyleSpec `json:"header_brand"                     toml:"header_brand"`
	CommandDivider                *StyleSpec `json:"command_divider"                  toml:"command_divider"`
	PaneTitle                     *StyleSpec `json:"pane_title"                       toml:"pane_title"`
	PaneTitleFile                 *StyleSpec `json:"pane_title_file"                  toml:"pane_title_file"`
	PaneTitleRequests             *StyleSpec `json:"pane_title_requests"              toml:"pane_title_requests"`
	PaneDivider                   *StyleSpec `json:"pane_divider"                     toml:"pane_divider"`
	EditorHintBox                 *StyleSpec `json:"editor_hint_box"                  toml:"editor_hint_box"`
	EditorHintItem                *StyleSpec `json:"editor_hint_item"                 toml:"editor_hint_item"`
	EditorHintSelected            *StyleSpec `json:"editor_hint_selected"             toml:"editor_hint_selected"`
	EditorHintAnnotation          *StyleSpec `json:"editor_hint_annotation"           toml:"editor_hint_annotation"`
	ListItemTitle                 *StyleSpec `json:"list_item_title"                  toml:"list_item_title"`
	ListItemDescription           *StyleSpec `json:"list_item_description"            toml:"list_item_description"`
	ListItemSelectedTitle         *StyleSpec `json:"list_item_selected_title"         toml:"list_item_selected_title"`
	ListItemSelectedDescription   *StyleSpec `json:"list_item_selected_description"   toml:"list_item_selected_description"`
	ListItemDimmedTitle           *StyleSpec `json:"list_item_dimmed_title"           toml:"list_item_dimmed_title"`
	ListItemDimmedDescription     *StyleSpec `json:"list_item_dimmed_description"     toml:"list_item_dimmed_description"`
	ListItemFilterMatch           *StyleSpec `json:"list_item_filter_match"           toml:"list_item_filter_match"`
	ResponseContent               *StyleSpec `json:"response_content"                 toml:"response_content"`
	ResponseContentRaw            *StyleSpec `json:"response_content_raw"             toml:"response_content_raw"`
	ResponseContentHeaders        *StyleSpec `json:"response_content_headers"         toml:"response_content_headers"`
	StreamContent                 *StyleSpec `json:"stream_content"                   toml:"stream_content"`
	StreamTimestamp               *StyleSpec `json:"stream_timestamp"                 toml:"stream_timestamp"`
	StreamDirectionSend           *StyleSpec `json:"stream_direction_send"            toml:"stream_direction_send"`
	StreamDirectionReceive        *StyleSpec `json:"stream_direction_receive"         toml:"stream_direction_receive"`
	StreamDirectionInfo           *StyleSpec `json:"stream_direction_info"            toml:"stream_direction_info"`
	StreamEventName               *StyleSpec `json:"stream_event_name"                toml:"stream_event_name"`
	StreamData                    *StyleSpec `json:"stream_data"                      toml:"stream_data"`
	StreamBinary                  *StyleSpec `json:"stream_binary"                    toml:"stream_binary"`
	StreamSummary                 *StyleSpec `json:"stream_summary"                   toml:"stream_summary"`
	StreamError                   *StyleSpec `json:"stream_error"                     toml:"stream_error"`
	StreamConsoleTitle            *StyleSpec `json:"stream_console_title"             toml:"stream_console_title"`
	StreamConsoleMode             *StyleSpec `json:"stream_console_mode"              toml:"stream_console_mode"`
	StreamConsoleStatus           *StyleSpec `json:"stream_console_status"            toml:"stream_console_status"`
	StreamConsolePrompt           *StyleSpec `json:"stream_console_prompt"            toml:"stream_console_prompt"`
	StreamConsoleInput            *StyleSpec `json:"stream_console_input"             toml:"stream_console_input"`
	StreamConsoleInputFocused     *StyleSpec `json:"stream_console_input_focused"     toml:"stream_console_input_focused"`
}

type ColorsSpec struct {
	PaneBorderFocusFile     *string `json:"pane_border_focus_file"     toml:"pane_border_focus_file"`
	PaneBorderFocusRequests *string `json:"pane_border_focus_requests" toml:"pane_border_focus_requests"`
	PaneActiveForeground    *string `json:"pane_active_foreground"     toml:"pane_active_foreground"`
	MethodGET               *string `json:"method_get"                 toml:"method_get"`
	MethodPOST              *string `json:"method_post"                toml:"method_post"`
	MethodPUT               *string `json:"method_put"                 toml:"method_put"`
	MethodPATCH             *string `json:"method_patch"               toml:"method_patch"`
	MethodDELETE            *string `json:"method_delete"              toml:"method_delete"`
	MethodHEAD              *string `json:"method_head"                toml:"method_head"`
	MethodOPTIONS           *string `json:"method_options"             toml:"method_options"`
	MethodGRPC              *string `json:"method_grpc"                toml:"method_grpc"`
	MethodWS                *string `json:"method_ws"                  toml:"method_ws"`
	MethodDefault           *string `json:"method_default"             toml:"method_default"`
}

type HeaderSegmentSpec struct {
	Background *string `json:"background" toml:"background"`
	Border     *string `json:"border"     toml:"border"`
	Foreground *string `json:"foreground" toml:"foreground"`
	Accent     *string `json:"accent"     toml:"accent"`
}

type CommandSegmentSpec struct {
	Background *string `json:"background" toml:"background"`
	Border     *string `json:"border"     toml:"border"`
	Key        *string `json:"key"        toml:"key"`
	Text       *string `json:"text"       toml:"text"`
}

type EditorMetadataSpec struct {
	CommentMarker     *string           `json:"comment_marker"      toml:"comment_marker"`
	DirectiveDefault  *string           `json:"directive_default"   toml:"directive_default"`
	Value             *string           `json:"value"               toml:"value"`
	SettingKey        *string           `json:"setting_key"         toml:"setting_key"`
	SettingValue      *string           `json:"setting_value"       toml:"setting_value"`
	RequestLine       *string           `json:"request_line"        toml:"request_line"`
	RequestSeparator  *string           `json:"request_separator"   toml:"request_separator"`
	RTSKeywordDefault *string           `json:"rts_keyword_default" toml:"rts_keyword_default"`
	RTSKeywordDecl    *string           `json:"rts_keyword_decl"    toml:"rts_keyword_decl"`
	RTSKeywordControl *string           `json:"rts_keyword_control" toml:"rts_keyword_control"`
	RTSKeywordLiteral *string           `json:"rts_keyword_literal" toml:"rts_keyword_literal"`
	RTSKeywordLogical *string           `json:"rts_keyword_logical" toml:"rts_keyword_logical"`
	DirectiveColors   map[string]string `json:"directive_colors"    toml:"directive_colors"`
}

type StyleSpec struct {
	Foreground       *string `json:"foreground"        toml:"foreground"`
	Background       *string `json:"background"        toml:"background"`
	BorderColor      *string `json:"border_color"      toml:"border_color"`
	BorderBackground *string `json:"border_background" toml:"border_background"`
	BorderStyle      *string `json:"border_style"      toml:"border_style"`
	Bold             *bool   `json:"bold"              toml:"bold"`
	Italic           *bool   `json:"italic"            toml:"italic"`
	Underline        *bool   `json:"underline"         toml:"underline"`
	Faint            *bool   `json:"faint"             toml:"faint"`
	Strikethrough    *bool   `json:"strikethrough"     toml:"strikethrough"`
	Align            *string `json:"align"             toml:"align"`
}

func ApplySpec(base Theme, spec ThemeSpec) (Theme, error) {
	cloned := cloneTheme(base)

	apply := func(name string, target *lipgloss.Style, override *StyleSpec) error {
		if override == nil {
			return nil
		}
		next, err := override.apply(*target)
		if err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
		*target = next
		return nil
	}

	// Don't judge me.
	// @david - rethink is this is how you want this to look because it's ugly as f.
	if err := apply(
		"browser_border",
		&cloned.BrowserBorder,
		spec.Styles.BrowserBorder,
	); err != nil {
		return Theme{}, err
	}
	if err := apply("editor_border", &cloned.EditorBorder, spec.Styles.EditorBorder); err != nil {
		return Theme{}, err
	}
	if err := apply(
		"response_border",
		&cloned.ResponseBorder,
		spec.Styles.ResponseBorder,
	); err != nil {
		return Theme{}, err
	}
	if err := apply(
		"navigator_title",
		&cloned.NavigatorTitle,
		spec.Styles.NavigatorTitle,
	); err != nil {
		return Theme{}, err
	}
	if err := apply(
		"navigator_title_selected",
		&cloned.NavigatorTitleSelected,
		spec.Styles.NavigatorTitleSelected,
	); err != nil {
		return Theme{}, err
	}
	if err := apply(
		"navigator_subtitle",
		&cloned.NavigatorSubtitle,
		spec.Styles.NavigatorSubtitle,
	); err != nil {
		return Theme{}, err
	}
	if err := apply(
		"navigator_subtitle_selected",
		&cloned.NavigatorSubtitleSelected,
		spec.Styles.NavigatorSubtitleSelected,
	); err != nil {
		return Theme{}, err
	}
	if err := apply(
		"navigator_badge",
		&cloned.NavigatorBadge,
		spec.Styles.NavigatorBadge,
	); err != nil {
		return Theme{}, err
	}
	if err := apply("navigator_tag", &cloned.NavigatorTag, spec.Styles.NavigatorTag); err != nil {
		return Theme{}, err
	}
	if err := apply("app_frame", &cloned.AppFrame, spec.Styles.AppFrame); err != nil {
		return Theme{}, err
	}
	if err := apply("header", &cloned.Header, spec.Styles.Header); err != nil {
		return Theme{}, err
	}
	if err := apply("header_title", &cloned.HeaderTitle, spec.Styles.HeaderTitle); err != nil {
		return Theme{}, err
	}
	if err := apply("header_value", &cloned.HeaderValue, spec.Styles.HeaderValue); err != nil {
		return Theme{}, err
	}
	if err := apply(
		"header_separator",
		&cloned.HeaderSeparator,
		spec.Styles.HeaderSeparator,
	); err != nil {
		return Theme{}, err
	}
	if err := apply("status_bar", &cloned.StatusBar, spec.Styles.StatusBar); err != nil {
		return Theme{}, err
	}
	if err := apply("status_bar_key", &cloned.StatusBarKey, spec.Styles.StatusBarKey); err != nil {
		return Theme{}, err
	}
	if err := apply(
		"status_bar_value",
		&cloned.StatusBarValue,
		spec.Styles.StatusBarValue,
	); err != nil {
		return Theme{}, err
	}
	if err := apply("command_bar", &cloned.CommandBar, spec.Styles.CommandBar); err != nil {
		return Theme{}, err
	}
	if err := apply(
		"command_bar_hint",
		&cloned.CommandBarHint,
		spec.Styles.CommandBarHint,
	); err != nil {
		return Theme{}, err
	}
	if err := apply(
		"response_search_highlight",
		&cloned.ResponseSearchHighlight,
		spec.Styles.ResponseSearchHighlight,
	); err != nil {
		return Theme{}, err
	}
	if err := apply(
		"response_search_highlight_active",
		&cloned.ResponseSearchHighlightActive,
		spec.Styles.ResponseSearchHighlightActive,
	); err != nil {
		return Theme{}, err
	}
	if err := apply("tabs", &cloned.Tabs, spec.Styles.Tabs); err != nil {
		return Theme{}, err
	}
	if err := apply("tab_active", &cloned.TabActive, spec.Styles.TabActive); err != nil {
		return Theme{}, err
	}
	if err := apply("tab_inactive", &cloned.TabInactive, spec.Styles.TabInactive); err != nil {
		return Theme{}, err
	}
	if err := apply("notification", &cloned.Notification, spec.Styles.Notification); err != nil {
		return Theme{}, err
	}
	if err := apply("error", &cloned.Error, spec.Styles.Error); err != nil {
		return Theme{}, err
	}
	if err := apply("success", &cloned.Success, spec.Styles.Success); err != nil {
		return Theme{}, err
	}
	if err := apply("header_brand", &cloned.HeaderBrand, spec.Styles.HeaderBrand); err != nil {
		return Theme{}, err
	}
	if err := apply(
		"command_divider",
		&cloned.CommandDivider,
		spec.Styles.CommandDivider,
	); err != nil {
		return Theme{}, err
	}
	if err := apply("pane_title", &cloned.PaneTitle, spec.Styles.PaneTitle); err != nil {
		return Theme{}, err
	}
	if err := apply(
		"pane_title_file",
		&cloned.PaneTitleFile,
		spec.Styles.PaneTitleFile,
	); err != nil {
		return Theme{}, err
	}
	if err := apply(
		"pane_title_requests",
		&cloned.PaneTitleRequests,
		spec.Styles.PaneTitleRequests,
	); err != nil {
		return Theme{}, err
	}
	if err := apply("pane_divider", &cloned.PaneDivider, spec.Styles.PaneDivider); err != nil {
		return Theme{}, err
	}
	if err := apply(
		"editor_hint_box",
		&cloned.EditorHintBox,
		spec.Styles.EditorHintBox,
	); err != nil {
		return Theme{}, err
	}
	if err := apply(
		"editor_hint_item",
		&cloned.EditorHintItem,
		spec.Styles.EditorHintItem,
	); err != nil {
		return Theme{}, err
	}
	if err := apply(
		"editor_hint_selected",
		&cloned.EditorHintSelected,
		spec.Styles.EditorHintSelected,
	); err != nil {
		return Theme{}, err
	}
	if err := apply(
		"editor_hint_annotation",
		&cloned.EditorHintAnnotation,
		spec.Styles.EditorHintAnnotation,
	); err != nil {
		return Theme{}, err
	}
	if err := apply(
		"list_item_title",
		&cloned.ListItemTitle,
		spec.Styles.ListItemTitle,
	); err != nil {
		return Theme{}, err
	}
	if err := apply(
		"list_item_description",
		&cloned.ListItemDescription,
		spec.Styles.ListItemDescription,
	); err != nil {
		return Theme{}, err
	}
	if err := apply(
		"list_item_selected_title",
		&cloned.ListItemSelectedTitle,
		spec.Styles.ListItemSelectedTitle,
	); err != nil {
		return Theme{}, err
	}
	if err := apply(
		"list_item_selected_description",
		&cloned.ListItemSelectedDescription,
		spec.Styles.ListItemSelectedDescription,
	); err != nil {
		return Theme{}, err
	}
	if err := apply(
		"list_item_dimmed_title",
		&cloned.ListItemDimmedTitle,
		spec.Styles.ListItemDimmedTitle,
	); err != nil {
		return Theme{}, err
	}
	if err := apply(
		"list_item_dimmed_description",
		&cloned.ListItemDimmedDescription,
		spec.Styles.ListItemDimmedDescription,
	); err != nil {
		return Theme{}, err
	}
	if err := apply(
		"list_item_filter_match",
		&cloned.ListItemFilterMatch,
		spec.Styles.ListItemFilterMatch,
	); err != nil {
		return Theme{}, err
	}
	if err := apply(
		"response_content",
		&cloned.ResponseContent,
		spec.Styles.ResponseContent,
	); err != nil {
		return Theme{}, err
	}
	if err := apply(
		"response_content_raw",
		&cloned.ResponseContentRaw,
		spec.Styles.ResponseContentRaw,
	); err != nil {
		return Theme{}, err
	}
	if err := apply(
		"response_content_headers",
		&cloned.ResponseContentHeaders,
		spec.Styles.ResponseContentHeaders,
	); err != nil {
		return Theme{}, err
	}
	if err := apply(
		"stream_content",
		&cloned.StreamContent,
		spec.Styles.StreamContent,
	); err != nil {
		return Theme{}, err
	}
	if err := apply(
		"stream_timestamp",
		&cloned.StreamTimestamp,
		spec.Styles.StreamTimestamp,
	); err != nil {
		return Theme{}, err
	}
	if err := apply(
		"stream_direction_send",
		&cloned.StreamDirectionSend,
		spec.Styles.StreamDirectionSend,
	); err != nil {
		return Theme{}, err
	}
	if err := apply(
		"stream_direction_receive",
		&cloned.StreamDirectionReceive,
		spec.Styles.StreamDirectionReceive,
	); err != nil {
		return Theme{}, err
	}
	if err := apply(
		"stream_direction_info",
		&cloned.StreamDirectionInfo,
		spec.Styles.StreamDirectionInfo,
	); err != nil {
		return Theme{}, err
	}
	if err := apply(
		"stream_event_name",
		&cloned.StreamEventName,
		spec.Styles.StreamEventName,
	); err != nil {
		return Theme{}, err
	}
	if err := apply("stream_data", &cloned.StreamData, spec.Styles.StreamData); err != nil {
		return Theme{}, err
	}
	if err := apply("stream_binary", &cloned.StreamBinary, spec.Styles.StreamBinary); err != nil {
		return Theme{}, err
	}
	if err := apply(
		"stream_summary",
		&cloned.StreamSummary,
		spec.Styles.StreamSummary,
	); err != nil {
		return Theme{}, err
	}
	if err := apply("stream_error", &cloned.StreamError, spec.Styles.StreamError); err != nil {
		return Theme{}, err
	}
	if err := apply(
		"stream_console_title",
		&cloned.StreamConsoleTitle,
		spec.Styles.StreamConsoleTitle,
	); err != nil {
		return Theme{}, err
	}
	if err := apply(
		"stream_console_mode",
		&cloned.StreamConsoleMode,
		spec.Styles.StreamConsoleMode,
	); err != nil {
		return Theme{}, err
	}
	if err := apply(
		"stream_console_status",
		&cloned.StreamConsoleStatus,
		spec.Styles.StreamConsoleStatus,
	); err != nil {
		return Theme{}, err
	}
	if err := apply(
		"stream_console_prompt",
		&cloned.StreamConsolePrompt,
		spec.Styles.StreamConsolePrompt,
	); err != nil {
		return Theme{}, err
	}
	if err := apply(
		"stream_console_input",
		&cloned.StreamConsoleInput,
		spec.Styles.StreamConsoleInput,
	); err != nil {
		return Theme{}, err
	}
	if err := apply(
		"stream_console_input_focused",
		&cloned.StreamConsoleInputFocused,
		spec.Styles.StreamConsoleInputFocused,
	); err != nil {
		return Theme{}, err
	}

	if spec.Colors.PaneBorderFocusFile != nil {
		color, err := toColor("pane_border_focus_file", *spec.Colors.PaneBorderFocusFile)
		if err != nil {
			return Theme{}, err
		}
		cloned.PaneBorderFocusFile = color
	}
	if spec.Colors.PaneBorderFocusRequests != nil {
		color, err := toColor("pane_border_focus_requests", *spec.Colors.PaneBorderFocusRequests)
		if err != nil {
			return Theme{}, err
		}
		cloned.PaneBorderFocusRequests = color
	}
	if spec.Colors.PaneActiveForeground != nil {
		color, err := toColor("pane_active_foreground", *spec.Colors.PaneActiveForeground)
		if err != nil {
			return Theme{}, err
		}
		cloned.PaneActiveForeground = color
	}
	if spec.Colors.MethodGET != nil {
		color, err := toColor("method_get", *spec.Colors.MethodGET)
		if err != nil {
			return Theme{}, err
		}
		cloned.MethodColors.GET = color
	}
	if spec.Colors.MethodPOST != nil {
		color, err := toColor("method_post", *spec.Colors.MethodPOST)
		if err != nil {
			return Theme{}, err
		}
		cloned.MethodColors.POST = color
	}
	if spec.Colors.MethodPUT != nil {
		color, err := toColor("method_put", *spec.Colors.MethodPUT)
		if err != nil {
			return Theme{}, err
		}
		cloned.MethodColors.PUT = color
	}
	if spec.Colors.MethodPATCH != nil {
		color, err := toColor("method_patch", *spec.Colors.MethodPATCH)
		if err != nil {
			return Theme{}, err
		}
		cloned.MethodColors.PATCH = color
	}
	if spec.Colors.MethodDELETE != nil {
		color, err := toColor("method_delete", *spec.Colors.MethodDELETE)
		if err != nil {
			return Theme{}, err
		}
		cloned.MethodColors.DELETE = color
	}
	if spec.Colors.MethodHEAD != nil {
		color, err := toColor("method_head", *spec.Colors.MethodHEAD)
		if err != nil {
			return Theme{}, err
		}
		cloned.MethodColors.HEAD = color
	}
	if spec.Colors.MethodOPTIONS != nil {
		color, err := toColor("method_options", *spec.Colors.MethodOPTIONS)
		if err != nil {
			return Theme{}, err
		}
		cloned.MethodColors.OPTIONS = color
	}
	if spec.Colors.MethodGRPC != nil {
		color, err := toColor("method_grpc", *spec.Colors.MethodGRPC)
		if err != nil {
			return Theme{}, err
		}
		cloned.MethodColors.GRPC = color
	}
	if spec.Colors.MethodWS != nil {
		color, err := toColor("method_ws", *spec.Colors.MethodWS)
		if err != nil {
			return Theme{}, err
		}
		cloned.MethodColors.WS = color
	}
	if spec.Colors.MethodDefault != nil {
		color, err := toColor("method_default", *spec.Colors.MethodDefault)
		if err != nil {
			return Theme{}, err
		}
		cloned.MethodColors.Default = color
	}

	if len(spec.HeaderSegments) > 0 {
		segments, err := applyHeaderSegments(cloned.HeaderSegments, spec.HeaderSegments)
		if err != nil {
			return Theme{}, err
		}
		cloned.HeaderSegments = segments
	}

	if len(spec.CommandSegments) > 0 {
		segments, err := applyCommandSegments(cloned.CommandSegments, spec.CommandSegments)
		if err != nil {
			return Theme{}, err
		}
		cloned.CommandSegments = segments
	}

	if spec.EditorMetadata != nil {
		if err := applyEditorMetadata(&cloned.EditorMetadata, *spec.EditorMetadata); err != nil {
			return Theme{}, err
		}
	}

	return cloned, nil
}

func (s *StyleSpec) apply(base lipgloss.Style) (lipgloss.Style, error) {
	if s == nil {
		return base, nil
	}
	current := base
	if s.Foreground != nil {
		color, err := toColor("foreground", *s.Foreground)
		if err != nil {
			return lipgloss.Style{}, err
		}
		current = current.Foreground(color)
	}
	if s.Background != nil {
		color, err := toColor("background", *s.Background)
		if err != nil {
			return lipgloss.Style{}, err
		}
		current = current.Background(color)
	}
	if s.BorderColor != nil {
		color, err := toColor("border_color", *s.BorderColor)
		if err != nil {
			return lipgloss.Style{}, err
		}
		current = current.BorderForeground(color)
	}
	if s.BorderBackground != nil {
		color, err := toColor("border_background", *s.BorderBackground)
		if err != nil {
			return lipgloss.Style{}, err
		}
		current = current.BorderBackground(color)
	}
	if s.BorderStyle != nil {
		normalized := strings.ToLower(strings.TrimSpace(*s.BorderStyle))
		if normalized == "inherit" {
		} else {
			border, err := parseBorderStyle(normalized)
			if err != nil {
				return lipgloss.Style{}, err
			}
			current = current.BorderStyle(border)
		}
	}
	if s.Bold != nil {
		current = current.Bold(*s.Bold)
	}
	if s.Italic != nil {
		current = current.Italic(*s.Italic)
	}
	if s.Underline != nil {
		current = current.Underline(*s.Underline)
	}
	if s.Faint != nil {
		current = current.Faint(*s.Faint)
	}
	if s.Strikethrough != nil {
		current = current.Strikethrough(*s.Strikethrough)
	}
	if s.Align != nil {
		align, err := parseAlign(*s.Align)
		if err != nil {
			return lipgloss.Style{}, err
		}
		current = current.Align(align)
	}
	return current, nil
}

func applyHeaderSegments(
	base []HeaderSegmentStyle,
	overrides []HeaderSegmentSpec,
) ([]HeaderSegmentStyle, error) {
	if len(overrides) == 0 {
		return base, nil
	}
	if len(base) == 0 {
		base = []HeaderSegmentStyle{{}}
	}
	result := make([]HeaderSegmentStyle, len(overrides))
	for i, spec := range overrides {
		template := base[i%len(base)]
		if spec.Background != nil {
			color, err := toColor("header_segments.background", *spec.Background)
			if err != nil {
				return nil, err
			}
			template.Background = color
		}
		if spec.Border != nil {
			color, err := toColor("header_segments.border", *spec.Border)
			if err != nil {
				return nil, err
			}
			template.Border = color
		}
		if spec.Foreground != nil {
			color, err := toColor("header_segments.foreground", *spec.Foreground)
			if err != nil {
				return nil, err
			}
			template.Foreground = color
		}
		if spec.Accent != nil {
			color, err := toColor("header_segments.accent", *spec.Accent)
			if err != nil {
				return nil, err
			}
			template.Accent = color
		}
		result[i] = template
	}
	return result, nil
}

func applyCommandSegments(
	base []CommandSegmentStyle,
	overrides []CommandSegmentSpec,
) ([]CommandSegmentStyle, error) {
	if len(overrides) == 0 {
		return base, nil
	}
	if len(base) == 0 {
		base = []CommandSegmentStyle{{}}
	}
	result := make([]CommandSegmentStyle, len(overrides))
	for i, spec := range overrides {
		template := base[i%len(base)]
		if spec.Background != nil {
			color, err := toColor("command_segments.background", *spec.Background)
			if err != nil {
				return nil, err
			}
			template.Background = color
		}
		if spec.Border != nil {
			color, err := toColor("command_segments.border", *spec.Border)
			if err != nil {
				return nil, err
			}
			template.Border = color
		}
		if spec.Key != nil {
			color, err := toColor("command_segments.key", *spec.Key)
			if err != nil {
				return nil, err
			}
			template.Key = color
		}
		if spec.Text != nil {
			color, err := toColor("command_segments.text", *spec.Text)
			if err != nil {
				return nil, err
			}
			template.Text = color
		}
		result[i] = template
	}
	return result, nil
}

func applyEditorMetadata(dst *EditorMetadataPalette, spec EditorMetadataSpec) error {
	if spec.CommentMarker != nil {
		color, err := toColor("editor_metadata.comment_marker", *spec.CommentMarker)
		if err != nil {
			return err
		}
		dst.CommentMarker = color
	}
	if spec.DirectiveDefault != nil {
		color, err := toColor("editor_metadata.directive_default", *spec.DirectiveDefault)
		if err != nil {
			return err
		}
		oldDefault := dst.DirectiveDefault
		dst.DirectiveDefault = color
		if oldDefault != "" && len(dst.DirectiveColors) > 0 {
			updated := make(map[string]lipgloss.Color, len(dst.DirectiveColors))
			for key, value := range dst.DirectiveColors {
				if value == oldDefault {
					updated[key] = color
					continue
				}
				updated[key] = value
			}
			dst.DirectiveColors = updated
		}
	}
	if spec.Value != nil {
		color, err := toColor("editor_metadata.value", *spec.Value)
		if err != nil {
			return err
		}
		dst.Value = color
	}
	if spec.SettingKey != nil {
		color, err := toColor("editor_metadata.setting_key", *spec.SettingKey)
		if err != nil {
			return err
		}
		dst.SettingKey = color
	}
	if spec.SettingValue != nil {
		color, err := toColor("editor_metadata.setting_value", *spec.SettingValue)
		if err != nil {
			return err
		}
		dst.SettingValue = color
	}
	if spec.RequestLine != nil {
		color, err := toColor("editor_metadata.request_line", *spec.RequestLine)
		if err != nil {
			return err
		}
		dst.RequestLine = color
	}
	if spec.RequestSeparator != nil {
		color, err := toColor("editor_metadata.request_separator", *spec.RequestSeparator)
		if err != nil {
			return err
		}
		dst.RequestSeparator = color
	}
	if spec.RTSKeywordDefault != nil {
		color, err := toColor("editor_metadata.rts_keyword_default", *spec.RTSKeywordDefault)
		if err != nil {
			return err
		}
		dst.RTSKeywordDefault = color
	}
	if spec.RTSKeywordDecl != nil {
		color, err := toColor("editor_metadata.rts_keyword_decl", *spec.RTSKeywordDecl)
		if err != nil {
			return err
		}
		dst.RTSKeywordDecl = color
	}
	if spec.RTSKeywordControl != nil {
		color, err := toColor("editor_metadata.rts_keyword_control", *spec.RTSKeywordControl)
		if err != nil {
			return err
		}
		dst.RTSKeywordControl = color
	}
	if spec.RTSKeywordLiteral != nil {
		color, err := toColor("editor_metadata.rts_keyword_literal", *spec.RTSKeywordLiteral)
		if err != nil {
			return err
		}
		dst.RTSKeywordLiteral = color
	}
	if spec.RTSKeywordLogical != nil {
		color, err := toColor("editor_metadata.rts_keyword_logical", *spec.RTSKeywordLogical)
		if err != nil {
			return err
		}
		dst.RTSKeywordLogical = color
	}
	if len(spec.DirectiveColors) > 0 {
		combined := make(
			map[string]lipgloss.Color,
			len(dst.DirectiveColors)+len(spec.DirectiveColors),
		)
		for key, value := range dst.DirectiveColors {
			combined[key] = value
		}
		for key, value := range spec.DirectiveColors {
			normalized := strings.ToLower(strings.TrimSpace(key))
			if normalized == "" {
				return fmt.Errorf(
					"editor_metadata.directive_colors: directive name may not be empty",
				)
			}
			color, err := toColor("editor_metadata.directive_colors", value)
			if err != nil {
				return err
			}
			combined[normalized] = color
		}
		dst.DirectiveColors = combined
	}
	return nil
}

func cloneTheme(src Theme) Theme {
	clone := src
	if len(src.HeaderSegments) > 0 {
		clone.HeaderSegments = append([]HeaderSegmentStyle(nil), src.HeaderSegments...)
	}
	if len(src.CommandSegments) > 0 {
		clone.CommandSegments = append([]CommandSegmentStyle(nil), src.CommandSegments...)
	}
	if src.EditorMetadata.DirectiveColors != nil {
		clone.EditorMetadata.DirectiveColors = make(
			map[string]lipgloss.Color,
			len(src.EditorMetadata.DirectiveColors),
		)
		for k, v := range src.EditorMetadata.DirectiveColors {
			clone.EditorMetadata.DirectiveColors[k] = v
		}
	}
	return clone
}

func toColor(field string, value string) (lipgloss.Color, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return "", fmt.Errorf("%s: colour value may not be empty", field)
	}
	return lipgloss.Color(trimmed), nil
}

func parseAlign(value string) (lipgloss.Position, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "left", "start", "default", "":
		return lipgloss.Left, nil
	case "center", "centre", "middle":
		return lipgloss.Center, nil
	case "right", "end":
		return lipgloss.Right, nil
	default:
		return lipgloss.Left, fmt.Errorf("align: unknown alignment %q", value)
	}
}

func parseBorderStyle(value string) (lipgloss.Border, error) {
	switch value {
	case "":
		return lipgloss.Border{}, fmt.Errorf("border_style: value may not be empty")
	case "none", "hidden", "off":
		return lipgloss.Border{}, nil
	case "normal", "single":
		return lipgloss.NormalBorder(), nil
	case "rounded":
		return lipgloss.RoundedBorder(), nil
	case "thick", "heavy":
		return lipgloss.ThickBorder(), nil
	case "double":
		return lipgloss.DoubleBorder(), nil
	case "ascii":
		return lipgloss.Border{
			Top:         "-",
			Bottom:      "-",
			Left:        "|",
			Right:       "|",
			TopLeft:     "+",
			TopRight:    "+",
			BottomLeft:  "+",
			BottomRight: "+",
		}, nil
	case "block":
		return lipgloss.BlockBorder(), nil
	default:
		return lipgloss.Border{}, fmt.Errorf("border_style: unknown border style %q", value)
	}
}
