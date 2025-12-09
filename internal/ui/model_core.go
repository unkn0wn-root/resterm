package ui

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/cursor"
	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/unkn0wn-root/resterm/internal/bindings"
	"github.com/unkn0wn-root/resterm/internal/config"
	"github.com/unkn0wn-root/resterm/internal/filesvc"
	"github.com/unkn0wn-root/resterm/internal/grpcclient"
	"github.com/unkn0wn-root/resterm/internal/history"
	"github.com/unkn0wn-root/resterm/internal/httpclient"
	"github.com/unkn0wn-root/resterm/internal/oauth"
	"github.com/unkn0wn-root/resterm/internal/parser"
	"github.com/unkn0wn-root/resterm/internal/restfile"
	"github.com/unkn0wn-root/resterm/internal/scripts"
	"github.com/unkn0wn-root/resterm/internal/ssh"
	"github.com/unkn0wn-root/resterm/internal/stream"
	"github.com/unkn0wn-root/resterm/internal/theme"
	"github.com/unkn0wn-root/resterm/internal/ui/navigator"
	"github.com/unkn0wn-root/resterm/internal/ui/textarea"
	"github.com/unkn0wn-root/resterm/internal/update"
	"github.com/unkn0wn-root/resterm/internal/vars"
)

var _ tea.Model = (*Model)(nil)

type paneFocus int

const (
	focusFile paneFocus = iota
	focusRequests
	focusWorkflows
	focusEditor
	focusResponse
)

type responseTab int

const (
	responseTabPretty responseTab = iota
	responseTabRaw
	responseTabHeaders
	responseTabStream
	responseTabStats
	responseTabTimeline
	responseTabCompare
	responseTabDiff
	responseTabHistory
)

type responseSplitOrientation int

const (
	responseSplitVertical responseSplitOrientation = iota
	responseSplitHorizontal
)

type mainSplitOrientation int

const (
	mainSplitVertical mainSplitOrientation = iota
	mainSplitHorizontal
)

type paneRegion int

const (
	paneRegionSidebar paneRegion = iota
	paneRegionEditor
	paneRegionResponse
)

type searchTarget int

const (
	searchTargetEditor searchTarget = iota
	searchTargetResponse
)

const (
	noResponseMessage         = "░█▀▄░█▀▀░█▀▀░▀█▀░█▀▀░█▀▄░█▄█\n░█▀▄░█▀▀░▀▀█░░█░░█▀▀░█▀▄░█░█\n░▀░▀░▀▀▀░▀▀▀░░▀░░▀▀▀░▀░▀░▀░▀"
	historySnippetPlaceholder = "[HTML content omitted]"
	historySnippetMaxLines    = 24
	tabIndicatorPrefix        = "▸ "
)

const (
	sidebarWidthDefault   = 0.2
	sidebarWidthStep      = 0.05
	minSidebarWidthRatio  = 0.05
	maxSidebarWidthRatio  = 0.3
	minSidebarWidthPixels = 20
	sidebarSplitDefault   = 0.5
	sidebarSplitStep      = 0.05
)

const (
	requestCompactSwitch = 10
	minWorkflowSplit     = 0.3
	maxWorkflowSplit     = 0.7
	workflowSplitDefault = 0.5
	workflowSplitStep    = 0.05
)

const (
	editorSplitDefault           = 0.6
	editorSplitStep              = 0.05
	minEditorSplit               = 0.3
	maxEditorSplit               = 0.63
	minEditorPaneWidth           = 30
	minResponsePaneWidth         = 40
	minResponseSplitWidth        = 24
	responseSplitSeparatorWidth  = 1
	minResponseSplitHeight       = 6
	responseSplitSeparatorHeight = 1
	minEditorPaneHeight          = 10
	minResponsePaneHeight        = 6
)

const (
	collapsedSidebarWidthPx = 5
	collapsedPaneWidthPx    = 8
	collapsedPaneHeightRows = 4
)

type Config struct {
	FilePath            string
	InitialContent      string
	Client              *httpclient.Client
	Theme               *theme.Theme
	ThemeCatalog        theme.Catalog
	ActiveThemeKey      string
	Settings            config.Settings
	SettingsHandle      config.SettingsHandle
	EnvironmentSet      vars.EnvironmentSet
	EnvironmentName     string
	EnvironmentFile     string
	EnvironmentFallback string
	HTTPOptions         httpclient.Options
	GRPCOptions         grpcclient.Options
	SSHManager          *ssh.Manager
	History             *history.Store
	WorkspaceRoot       string
	Recursive           bool
	Version             string
	UpdateClient        update.Client
	EnableUpdate        bool
	CompareTargets      []string
	CompareBase         string
	Bindings            *bindings.Map
}

type operatorState struct {
	active     bool
	operator   string
	anchor     cursorPosition
	motionKeys []string
}

type Model struct {
	cfg                Config
	bindingsMap        *bindings.Map
	theme              theme.Theme
	themeCatalog       theme.Catalog
	client             *httpclient.Client
	grpcClient         *grpcclient.Client
	grpcOptions        grpcclient.Options
	sshMgr             *ssh.Manager
	sshGlobals         *sshStore
	workspaceRoot      string
	workspaceRecursive bool

	fileList                 list.Model
	requestList              list.Model
	workflowList             list.Model
	navigator                *navigator.Model[any]
	navigatorFilter          textinput.Model
	navigatorCompact         bool
	pendingCrossFileID       string
	docCache                 map[string]navDocCache
	editor                   requestEditor
	responsePanes            [2]responsePaneState
	responseSplit            bool
	responseSplitRatio       float64
	responseSplitOrientation responseSplitOrientation
	responsePaneFocus        responsePaneID
	responsePaneChord        bool
	sidebarCollapsed         bool
	editorCollapsed          bool
	responseCollapsed        bool
	zoomActive               bool
	zoomRegion               paneRegion
	mainSplitOrientation     mainSplitOrientation
	reqCompact               *bool
	wfCompact                *bool
	editorContentHeight      int
	responseContentHeight    int
	historyList              list.Model
	envList                  list.Model
	themeList                list.Model

	responseLatest         *responseSnapshot
	responsePrevious       *responseSnapshot
	responsePending        *responseSnapshot
	responseTokens         map[string]*responseSnapshot
	responseLastFocused    responsePaneID
	focus                  paneFocus
	compareSnapshots       map[string]*responseSnapshot
	compareRowIndex        int
	compareSelectedEnv     string
	compareFocusedEnv      string
	showEnvSelector        bool
	showThemeSelector      bool
	showHelp               bool
	helpJustOpened         bool
	showNewFileModal       bool
	showOpenModal          bool
	showErrorModal         bool
	errorModalMessage      string
	showHistoryPreview     bool
	historyPreviewContent  string
	historyPreviewTitle    string
	historyPreviewViewport *viewport.Model
	helpViewport           *viewport.Model
	suppressNextErrorModal bool

	showSearchPrompt   bool
	searchInput        textinput.Model
	searchIsRegex      bool
	searchJustOpened   bool
	searchTarget       searchTarget
	searchResponsePane responsePaneID

	statusMessage    statusMsg
	statusPulseBase  string
	statusPulseFrame int
	statusPulseSeq   int
	lastResponse     *httpclient.Response
	lastGRPC         *grpcclient.Response
	lastError        error

	scriptRunner    *scripts.Runner
	testResults     []scripts.TestResult
	scriptError     error
	globals         *globalStore
	fileVars        *fileStore
	oauth           *oauth.Manager
	updateClient    update.Client
	updateVersion   string
	updateEnabled   bool
	updateBusy      bool
	updateAnnounce  string
	updateInfo      *update.Result
	updateLastErr   string
	updateLastCheck time.Time

	responseRenderToken  string
	responseLoading      bool
	responseLoadingFrame int

	activeThemeKey      string
	settingsHandle      config.SettingsHandle
	historyStore        *history.Store
	historyEntries      []history.Entry
	historySelectedID   string
	historyJumpToLatest bool
	historyWorkflowName string
	requestItems        []requestListItem
	workflowItems       []workflowListItem
	showWorkflow        bool

	width                 int
	height                int
	paneContentHeight     int
	frameWidth            int
	frameHeight           int
	sidebarWidth          float64
	sidebarWidthPx        int
	responseWidthPx       int
	sidebarSplit          float64
	sidebarFilesHeight    int
	sidebarRequestsHeight int
	workflowSplit         float64
	editorSplit           float64
	pendingChord          string
	pendingChordMsg       tea.KeyMsg
	hasPendingChord       bool
	repeatChordPrefix     string
	repeatChordActive     bool
	operator              operatorState
	suppressListKey       bool
	ready                 bool
	dirty                 bool
	sending               bool
	sendCancel            context.CancelFunc
	suppressEditorKey     bool
	editorInsertMode      bool
	editorWriteKeyMap     textarea.KeyMap
	editorViewKeyMap      textarea.KeyMap
	newFileInput          textinput.Model
	newFileExtIndex       int
	newFileError          string
	newFileFromSave       bool
	openPathInput         textinput.Model
	openPathError         string

	doc                *restfile.Document
	currentFile        string
	currentRequest     *restfile.Request
	profileRun         *profileState
	workflowRun        *workflowState
	compareRun         *compareState
	lastCompareResults []compareResult
	lastCompareSpec    *restfile.CompareSpec
	compareBundle      *compareBundle
	activeRequestTitle string
	activeRequestKey   string
	activeWorkflowKey  string

	streamMgr          *stream.Manager
	streamMsgChan      chan tea.Msg
	streamBatchWindow  time.Duration
	streamMaxEvents    int
	liveSessions       map[string]*liveSession
	wsSenders          map[string]*httpclient.WebSocketSender
	sessionHandles     map[string]*stream.Session
	wsConsole          *websocketConsole
	streamFilterActive bool
	streamFilterInput  textinput.Model
	requestSessions    map[*restfile.Request]string
	sessionRequests    map[string]*restfile.Request
	requestKeySessions map[string]string
}

type navDocCache struct {
	doc *restfile.Document
	mod time.Time
}

func New(cfg Config) Model {
	th := theme.DefaultTheme()
	if cfg.Theme != nil {
		th = *cfg.Theme
	}
	activeTheme := strings.TrimSpace(cfg.ActiveThemeKey)
	if activeTheme == "" {
		activeTheme = "default"
	}

	reqCompact := false
	wfCompact := false

	client := cfg.Client
	if client == nil {
		client = httpclient.NewClient(nil)
		cfg.Client = client
	}
	grpcExec := grpcclient.NewClient()
	bindingMap := cfg.Bindings
	if bindingMap == nil {
		bindingMap = bindings.DefaultMap()
	}

	workspace := cfg.WorkspaceRoot
	if workspace == "" {
		if cfg.FilePath != "" {
			workspace = filepath.Dir(cfg.FilePath)
		} else if wd, err := os.Getwd(); err == nil {
			workspace = wd
		} else {
			workspace = "."
		}
	}

	entries, err := filesvc.ListRequestFiles(workspace, cfg.Recursive)
	var initialStatus statusMsg
	if err != nil {
		initialStatus = statusMsg{text: fmt.Sprintf("workspace error: %v", err), level: statusWarn}
		entries = nil
	}
	if initialStatus.text == "" && cfg.EnvironmentFallback != "" {
		initialStatus = statusMsg{
			text:  fmt.Sprintf("Environment defaulted to %q - press Ctrl+E to change.", cfg.EnvironmentFallback),
			level: statusInfo,
		}
	}

	items := makeFileItems(entries)
	fileList := list.New(items, listDelegateForTheme(th, false, 0), 0, 0)
	fileList.Title = "Files"
	fileList.SetShowStatusBar(false)
	fileList.SetShowHelp(false)
	fileList.SetFilteringEnabled(true)
	fileList.SetShowTitle(false)
	fileList.DisableQuitKeybindings()
	if cfg.FilePath != "" {
		for i, entry := range entries {
			if filepath.Clean(entry.Path) == filepath.Clean(cfg.FilePath) {
				fileList.Select(i)
				break
			}
		}
	}

	editor := newRequestEditor()
	editor.SetRuneStyler(newMetadataRuneStyler(th.EditorMetadata))
	editor.Placeholder = "Write HTTP requests here..."
	editor.SetValue(cfg.InitialContent)
	editor.moveToBufferTop()
	editor.ShowLineNumbers = true
	writeKeyMap := editor.KeyMap
	viewKeyMap := makeReadOnlyKeyMap(editor.KeyMap)
	editor.KeyMap = viewKeyMap
	editor.Cursor.SetMode(cursor.CursorStatic)

	newFileInput := textinput.New()
	newFileInput.Placeholder = "new-request"
	newFileInput.CharLimit = 0
	newFileInput.Prompt = ""
	newFileInput.SetCursor(0)

	openPathInput := textinput.New()
	openPathInput.Placeholder = "./examples/basic.http"
	openPathInput.CharLimit = 0
	openPathInput.Prompt = ""
	openPathInput.SetCursor(0)

	searchInput := textinput.New()
	searchInput.Placeholder = "pattern"
	searchInput.CharLimit = 0
	searchInput.Prompt = "/"
	searchInput.SetCursor(0)
	searchInput.Blur()

	navFilter := textinput.New()
	navFilter.Placeholder = "filter"
	navFilter.CharLimit = 0
	navFilter.Prompt = ""
	navFilter.SetCursor(0)
	navFilter.Blur()

	primaryViewport := viewport.New(0, 0)
	primaryViewport.SetContent(centerContent(noResponseMessage, 0, 0))
	secondaryViewport := viewport.New(0, 0)
	secondaryViewport.SetContent(centerContent(noResponseMessage, 0, 0))

	reqDelegate := listDelegateForTheme(th, true, 3)
	requestList := list.New(nil, reqDelegate, 0, 0)
	requestList.Title = "Requests"
	requestList.SetShowStatusBar(false)
	requestList.SetShowHelp(false)
	requestList.SetFilteringEnabled(true)
	requestList.SetShowTitle(false)
	requestList.DisableQuitKeybindings()

	workflowDelegate := listDelegateForTheme(th, true, 3)
	workflowList := list.New(nil, workflowDelegate, 0, 0)
	workflowList.Title = "Workflows"
	workflowList.SetShowStatusBar(false)
	workflowList.SetShowHelp(false)
	workflowList.SetFilteringEnabled(true)
	workflowList.SetShowTitle(false)
	workflowList.DisableQuitKeybindings()

	histDelegate := listDelegateForTheme(th, true, 3)
	historyList := list.New(nil, histDelegate, 0, 0)
	historyList.SetShowStatusBar(false)
	historyList.SetShowHelp(false)
	historyList.SetShowTitle(false)
	historyList.DisableQuitKeybindings()

	envItems := makeEnvItems(cfg.EnvironmentSet)
	envList := list.New(envItems, listDelegateForTheme(th, false, 0), 0, 0)
	envList.Title = "Environments"
	envList.SetShowStatusBar(false)
	envList.SetShowHelp(false)
	envList.SetFilteringEnabled(false)
	envList.DisableQuitKeybindings()

	themeItems := makeThemeItems(cfg.ThemeCatalog, activeTheme)
	themeDelegate := listDelegateForTheme(th, true, 3)
	themeList := list.New(themeItems, themeDelegate, 0, 0)
	themeList.Title = "Themes"
	themeList.SetShowStatusBar(false)
	themeList.SetShowHelp(false)
	themeList.SetFilteringEnabled(true)
	themeList.SetShowTitle(false)
	themeList.DisableQuitKeybindings()
	if len(themeItems) > 0 {
		selected := false
		for i, item := range themeItems {
			if t, ok := item.(themeItem); ok && t.key == activeTheme {
				themeList.Select(i)
				selected = true
				break
			}
		}
		if !selected {
			themeList.Select(0)
		}
	}

	previewViewport := viewport.New(0, 0)
	previewViewport.SetContent("")

	helpViewport := viewport.New(0, 0)
	helpViewport.SetContent("")

	sshMgr := cfg.SSHManager
	if sshMgr == nil {
		sshMgr = ssh.NewManager()
	}
	sshGlobals := newSSHStore()

	updateVersion := strings.TrimSpace(cfg.Version)
	updateEnabled := cfg.EnableUpdate && updateVersion != "" && updateVersion != "dev" && cfg.UpdateClient.Ready()

	model := Model{
		cfg:                    cfg,
		bindingsMap:            bindingMap,
		theme:                  th,
		themeCatalog:           cfg.ThemeCatalog,
		client:                 client,
		grpcClient:             grpcExec,
		grpcOptions:            cfg.GRPCOptions,
		sshMgr:                 sshMgr,
		sshGlobals:             sshGlobals,
		workspaceRoot:          workspace,
		workspaceRecursive:     cfg.Recursive,
		fileList:               fileList,
		requestList:            requestList,
		workflowList:           workflowList,
		navigatorFilter:        navFilter,
		docCache:               make(map[string]navDocCache),
		editor:                 editor,
		historyList:            historyList,
		envList:                envList,
		themeList:              themeList,
		historyPreviewViewport: &previewViewport,
		helpViewport:           &helpViewport,
		activeThemeKey:         activeTheme,
		settingsHandle:         cfg.SettingsHandle,
		responsePanes: [2]responsePaneState{
			newResponsePaneState(primaryViewport, true),
			newResponsePaneState(secondaryViewport, false),
		},
		responsePaneFocus:        responsePanePrimary,
		responseSplitRatio:       0.5,
		responseSplitOrientation: responseSplitVertical,
		mainSplitOrientation:     mainSplitVertical,
		reqCompact:               &reqCompact,
		wfCompact:                &wfCompact,
		responseTokens:           make(map[string]*responseSnapshot),
		responseLastFocused:      responsePanePrimary,
		focus:                    focusFile,
		sidebarWidth:             sidebarWidthDefault,
		sidebarSplit:             sidebarSplitDefault,
		workflowSplit:            workflowSplitDefault,
		editorSplit:              editorSplitDefault,
		historyStore:             cfg.History,
		currentFile:              cfg.FilePath,
		statusMessage:            initialStatus,
		scriptRunner:             scripts.NewRunner(nil),
		globals:                  newGlobalStore(),
		fileVars:                 newFileStore(),
		oauth:                    oauth.NewManager(client),
		updateClient:             cfg.UpdateClient,
		updateVersion:            updateVersion,
		updateEnabled:            updateEnabled,
		editorInsertMode:         false,
		editorWriteKeyMap:        writeKeyMap,
		editorViewKeyMap:         viewKeyMap,
		newFileInput:             newFileInput,
		openPathInput:            openPathInput,
		searchInput:              searchInput,
		searchTarget:             searchTargetEditor,
		streamMgr:                stream.NewManager(),
		streamMsgChan:            make(chan tea.Msg, 128),
		streamBatchWindow:        defaultStreamBatchWindow,
		streamMaxEvents:          defaultStreamMaxEvents,
		sessionHandles:           make(map[string]*stream.Session),
		liveSessions:             make(map[string]*liveSession),
		wsSenders:                make(map[string]*httpclient.WebSocketSender),
		streamFilterInput: func() textinput.Model {
			ti := textinput.New()
			ti.Placeholder = "filter"
			ti.Prompt = "Filter: "
			ti.CharLimit = 0
			ti.SetCursor(0)
			ti.Blur()
			return ti
		}(),
		requestSessions:    make(map[*restfile.Request]string),
		sessionRequests:    make(map[string]*restfile.Request),
		requestKeySessions: make(map[string]string),
		compareSnapshots:   make(map[string]*responseSnapshot),
	}
	model.setInsertMode(false, false)

	model.doc = parser.Parse(cfg.FilePath, []byte(cfg.InitialContent))
	model.syncSSHGlobals(model.doc)
	model.syncRequestList(model.doc)
	model.rebuildNavigator(entries)
	if model.historyStore != nil {
		_ = model.historyStore.Load()
	}
	model.syncHistory()
	model.setLivePane(responsePanePrimary)
	model.applyThemeToLists()

	return model
}
