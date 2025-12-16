package bindings

import "fmt"

var (
	// Action identifiers exposed via KnownActions for docs/config validation.
	ActionCycleFocusNext          ActionID = "cycle_focus_next"
	ActionCycleFocusPrev          ActionID = "cycle_focus_prev"
	ActionOpenEnvSelector         ActionID = "open_env_selector"
	ActionShowGlobals             ActionID = "show_globals"
	ActionClearGlobals            ActionID = "clear_globals"
	ActionSaveFile                ActionID = "save_file"
	ActionSaveLayout              ActionID = "save_layout"
	ActionToggleResponseSplitVert ActionID = "toggle_response_split_vertical"
	ActionToggleResponseSplitHorz ActionID = "toggle_response_split_horizontal"
	ActionTogglePaneFollowLatest  ActionID = "toggle_pane_follow_latest"
	ActionToggleHelp              ActionID = "toggle_help"
	ActionShowRequestDetails      ActionID = "show_request_details"
	ActionOpenPathModal           ActionID = "open_path_modal"
	ActionReloadWorkspace         ActionID = "reload_workspace"
	ActionOpenNewFileModal        ActionID = "open_new_file_modal"
	ActionOpenThemeSelector       ActionID = "open_theme_selector"
	ActionOpenTempDocument        ActionID = "open_temp_document"
	ActionReparseDocument         ActionID = "reparse_document"
	ActionReloadFileFromDisk      ActionID = "reload_file_from_disk"
	ActionSelectTimelineTab       ActionID = "select_timeline_tab"
	ActionQuitApp                 ActionID = "quit_app"
	ActionSidebarWidthDecrease    ActionID = "sidebar_width_decrease"
	ActionSidebarWidthIncrease    ActionID = "sidebar_width_increase"
	ActionSidebarHeightDecrease   ActionID = "sidebar_height_decrease"
	ActionSidebarHeightIncrease   ActionID = "sidebar_height_increase"
	ActionWorkflowHeightIncrease  ActionID = "workflow_height_increase"
	ActionWorkflowHeightDecrease  ActionID = "workflow_height_decrease"
	ActionFocusRequests           ActionID = "focus_requests"
	ActionFocusResponse           ActionID = "focus_response"
	ActionFocusEditorNormal       ActionID = "focus_editor_normal"
	ActionSetMainSplitHorizontal  ActionID = "set_main_split_horizontal"
	ActionSetMainSplitVertical    ActionID = "set_main_split_vertical"
	ActionStartCompareRun         ActionID = "start_compare_run"
	ActionToggleWebsocketConsole  ActionID = "toggle_ws_console"
	ActionToggleSidebarCollapse   ActionID = "toggle_sidebar_collapse"
	ActionToggleEditorCollapse    ActionID = "toggle_editor_collapse"
	ActionToggleResponseCollapse  ActionID = "toggle_response_collapse"
	ActionToggleZoom              ActionID = "toggle_zoom"
	ActionClearZoom               ActionID = "clear_zoom"
	ActionSendRequest             ActionID = "send_request"
	ActionCancelRun               ActionID = "cancel_run"
	ActionCopyResponseTab         ActionID = "copy_response_tab"
	ActionToggleHeaderPreview     ActionID = "toggle_header_preview"
	ActionCycleRawView            ActionID = "cycle_raw_view"
	ActionScrollResponseTop       ActionID = "scroll_response_top"
	ActionScrollResponseBottom    ActionID = "scroll_response_bottom"
	ActionSaveResponseBody        ActionID = "save_response_body"
	ActionOpenResponseExternally  ActionID = "open_response_externally"
)

type definition struct {
	id         ActionID
	repeatable bool
	defaults   [][]string
}

var definitions = []definition{
	def(ActionCycleFocusNext, false, "tab"),
	def(ActionCycleFocusPrev, false, "shift+tab"),
	def(ActionOpenEnvSelector, false, "ctrl+e"),
	def(ActionShowGlobals, false, "ctrl+g"),
	def(ActionClearGlobals, false, "ctrl+shift+g"),
	def(ActionSaveFile, false, "ctrl+s"),
	def(ActionSaveLayout, false, "g shift+l"),
	def(ActionToggleResponseSplitVert, false, "ctrl+v"),
	def(ActionToggleResponseSplitHorz, false, "ctrl+u"),
	def(ActionTogglePaneFollowLatest, false, "ctrl+shift+v"),
	def(ActionToggleHelp, false, "?"),
	def(ActionShowRequestDetails, false, "g ,"),
	def(ActionOpenPathModal, false, "ctrl+o"),
	def(ActionReloadWorkspace, false, "ctrl+shift+o", "g shift+o"),
	def(ActionOpenNewFileModal, false, "ctrl+n"),
	def(ActionOpenThemeSelector, false, "ctrl+alt+t", "g m", "g shift+t"),
	def(ActionOpenTempDocument, false, "ctrl+t"),
	def(ActionReparseDocument, false, "ctrl+p", "ctrl+alt+p", "ctrl+shift+t"),
	def(ActionReloadFileFromDisk, false, "g shift+r"),
	def(ActionSelectTimelineTab, false, "ctrl+alt+l", "g t"),
	def(ActionQuitApp, false, "ctrl+q", "ctrl+d"),
	def(ActionSidebarWidthDecrease, true, "g h"),
	def(ActionSidebarWidthIncrease, true, "g l"),
	def(ActionSidebarHeightDecrease, true, "g j"),
	def(ActionSidebarHeightIncrease, true, "g k"),
	def(ActionWorkflowHeightIncrease, true, "g shift+j"),
	def(ActionWorkflowHeightDecrease, true, "g shift+k"),
	def(ActionFocusRequests, false, "g r"),
	def(ActionFocusResponse, false, "g p"),
	def(ActionFocusEditorNormal, false, "g i"),
	def(ActionSetMainSplitHorizontal, false, "g s"),
	def(ActionSetMainSplitVertical, false, "g v"),
	def(ActionStartCompareRun, false, "g c"),
	def(ActionToggleWebsocketConsole, false, "g w"),
	def(ActionToggleSidebarCollapse, false, "g 1"),
	def(ActionToggleEditorCollapse, false, "g 2"),
	def(ActionToggleResponseCollapse, false, "g 3"),
	def(ActionToggleZoom, false, "g z"),
	def(ActionClearZoom, false, "g shift+z"),
	def(ActionSendRequest, false, "ctrl+enter", "cmd+enter", "alt+enter", "ctrl+j", "ctrl+m"),
	def(ActionCancelRun, false, "ctrl+c"),
	def(ActionCopyResponseTab, false, "ctrl+shift+c", "g y"),
	def(ActionToggleHeaderPreview, false, "g shift+h"),
	def(ActionCycleRawView, false, "g b"),
	def(ActionScrollResponseTop, false, "g g"),
	def(ActionScrollResponseBottom, false, "shift+g"),
	def(ActionSaveResponseBody, false, "g shift+s"),
	def(ActionOpenResponseExternally, false, "g shift+e"),
}

var definitionLookup = func() map[ActionID]definition {
	lookup := make(map[ActionID]definition, len(definitions))
	for _, def := range definitions {
		lookup[def.id] = def
	}
	return lookup
}()

func def(id ActionID, repeatable bool, specs ...string) definition {
	seqs := make([][]string, 0, len(specs))
	for _, spec := range specs {
		seqs = append(seqs, mustSequence(spec))
	}
	return definition{id: id, repeatable: repeatable, defaults: seqs}
}

func mustSequence(spec string) []string {
	seq, err := parseSequence(spec)
	if err != nil {
		panic(fmt.Sprintf("invalid default shortcut %q: %v", spec, err))
	}
	return seq
}
