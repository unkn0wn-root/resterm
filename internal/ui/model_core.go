package ui

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/charmbracelet/bubbles/cursor"
	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/unkn0wn-root/resterm/internal/filesvc"
	"github.com/unkn0wn-root/resterm/internal/grpcclient"
	"github.com/unkn0wn-root/resterm/internal/history"
	"github.com/unkn0wn-root/resterm/internal/httpclient"
	"github.com/unkn0wn-root/resterm/internal/oauth"
	"github.com/unkn0wn-root/resterm/internal/parser"
	"github.com/unkn0wn-root/resterm/internal/restfile"
	"github.com/unkn0wn-root/resterm/internal/scripts"
	"github.com/unkn0wn-root/resterm/internal/theme"
	"github.com/unkn0wn-root/resterm/internal/ui/textarea"
	"github.com/unkn0wn-root/resterm/internal/vars"
)

var _ tea.Model = (*Model)(nil)

type paneFocus int

const (
	focusFile paneFocus = iota
	focusRequests
	focusEditor
	focusResponse
)

type responseTab int

const (
	responseTabPretty responseTab = iota
	responseTabRaw
	responseTabHeaders
	responseTabStats
	responseTabDiff
	responseTabHistory
)

type responseSplitOrientation int

const (
	responseSplitVertical responseSplitOrientation = iota
	responseSplitHorizontal
)

type searchTarget int

const (
	searchTargetEditor searchTarget = iota
	searchTargetResponse
)

const noResponseMessage = "░█▀▄░█▀▀░█▀▀░▀█▀░█▀▀░█▀▄░█▄█\n░█▀▄░█▀▀░▀▀█░░█░░█▀▀░█▀▄░█░█\n░▀░▀░▀▀▀░▀▀▀░░▀░░▀▀▀░▀░▀░▀░▀"
const historySnippetPlaceholder = "[HTML content omitted]"
const historySnippetMaxLines = 24
const tabIndicatorPrefix = "▸ "

const (
	sidebarSplitDefault = 0.5
	sidebarSplitStep    = 0.05
	minSidebarSplit     = 0.2
	maxSidebarSplit     = 0.8
	minSidebarFiles     = 6
	minSidebarRequests  = 4
	sidebarSplitPadding = 1
)

const (
	editorSplitDefault           = 0.6
	editorSplitStep              = 0.05
	minEditorSplit               = 0.3
	maxEditorSplit               = 0.70
	minEditorPaneWidth           = 30
	minResponsePaneWidth         = 40
	minResponseSplitWidth        = 24
	responseSplitSeparatorWidth  = 1
	minResponseSplitHeight       = 6
	responseSplitSeparatorHeight = 1
)

type Config struct {
	FilePath            string
	InitialContent      string
	Client              *httpclient.Client
	Theme               *theme.Theme
	EnvironmentSet      vars.EnvironmentSet
	EnvironmentName     string
	EnvironmentFile     string
	EnvironmentFallback string
	HTTPOptions         httpclient.Options
	GRPCOptions         grpcclient.Options
	History             *history.Store
	WorkspaceRoot       string
	Recursive           bool
}

type operatorState struct {
	active     bool
	operator   string
	anchor     cursorPosition
	motionKeys []string
}

type Model struct {
	cfg                Config
	theme              theme.Theme
	client             *httpclient.Client
	grpcClient         *grpcclient.Client
	grpcOptions        grpcclient.Options
	workspaceRoot      string
	workspaceRecursive bool

	fileList                 list.Model
	requestList              list.Model
	editor                   requestEditor
	responsePanes            [2]responsePaneState
	responseSplit            bool
	responseSplitRatio       float64
	responseSplitOrientation responseSplitOrientation
	responsePaneFocus        responsePaneID
	responsePaneChord        bool
	historyList              list.Model
	envList                  list.Model

	responseLatest         *responseSnapshot
	responsePrevious       *responseSnapshot
	responsePending        *responseSnapshot
	responseTokens         map[string]*responseSnapshot
	responseLastFocused    responsePaneID
	focus                  paneFocus
	showEnvSelector        bool
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
	lastResponse     *httpclient.Response
	lastGRPC         *grpcclient.Response
	lastError        error

	scriptRunner *scripts.Runner
	testResults  []scripts.TestResult
	scriptError  error
	globals      *globalStore
	fileVars     *fileStore
	oauth        *oauth.Manager

	responseRenderToken  string
	responseLoading      bool
	responseLoadingFrame int

	historyStore        *history.Store
	historyEntries      []history.Entry
	historySelectedID   string
	historyJumpToLatest bool
	requestItems        []requestListItem

	width                 int
	height                int
	paneContentHeight     int
	frameWidth            int
	frameHeight           int
	sidebarSplit          float64
	sidebarFilesHeight    int
	sidebarRequestsHeight int
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
	activeRequestTitle string
	activeRequestKey   string
}

func New(cfg Config) Model {
	th := theme.DefaultTheme()
	if cfg.Theme != nil {
		th = *cfg.Theme
	}

	client := cfg.Client
	if client == nil {
		client = httpclient.NewClient(nil)
		cfg.Client = client
	}
	grpcExec := grpcclient.NewClient()

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
			text:  fmt.Sprintf("Environment defaulted to %q; press Ctrl+E to change.", cfg.EnvironmentFallback),
			level: statusInfo,
		}
	}

	items := makeFileItems(entries)
	delegate := list.NewDefaultDelegate()
	delegate.ShowDescription = false
	fileList := list.New(items, delegate, 0, 0)
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

	primaryViewport := viewport.New(0, 0)
	primaryViewport.SetContent(centerContent(noResponseMessage, 0, 0))
	secondaryViewport := viewport.New(0, 0)
	secondaryViewport.SetContent(centerContent(noResponseMessage, 0, 0))

	reqDelegate := list.NewDefaultDelegate()
	reqDelegate.ShowDescription = true
	reqDelegate.SetHeight(3)
	requestList := list.New(nil, reqDelegate, 0, 0)
	requestList.Title = "Requests"
	requestList.SetShowStatusBar(false)
	requestList.SetShowHelp(false)
	requestList.SetFilteringEnabled(true)
	requestList.SetShowTitle(false)
	requestList.DisableQuitKeybindings()

	histDelegate := list.NewDefaultDelegate()
	histDelegate.ShowDescription = true
	histDelegate.SetHeight(3)
	historyList := list.New(nil, histDelegate, 0, 0)
	historyList.SetShowStatusBar(false)
	historyList.SetShowHelp(false)
	historyList.SetShowTitle(false)
	historyList.DisableQuitKeybindings()

	envItems := makeEnvItems(cfg.EnvironmentSet)
	envDelegate := list.NewDefaultDelegate()
	envDelegate.ShowDescription = false
	envList := list.New(envItems, envDelegate, 0, 0)
	envList.Title = "Environments"
	envList.SetShowStatusBar(false)
	envList.SetShowHelp(false)
	envList.SetFilteringEnabled(false)
	envList.DisableQuitKeybindings()

	previewViewport := viewport.New(0, 0)
	previewViewport.SetContent("")

	model := Model{
		cfg:                    cfg,
		theme:                  th,
		client:                 client,
		grpcClient:             grpcExec,
		grpcOptions:            cfg.GRPCOptions,
		workspaceRoot:          workspace,
		workspaceRecursive:     cfg.Recursive,
		fileList:               fileList,
		requestList:            requestList,
		editor:                 editor,
		historyList:            historyList,
		envList:                envList,
		historyPreviewViewport: &previewViewport,
		responsePanes: [2]responsePaneState{
			newResponsePaneState(primaryViewport, true),
			newResponsePaneState(secondaryViewport, false),
		},
		responsePaneFocus:        responsePanePrimary,
		responseSplitRatio:       0.5,
		responseSplitOrientation: responseSplitVertical,
		responseTokens:           make(map[string]*responseSnapshot),
		responseLastFocused:      responsePanePrimary,
		focus:                    focusFile,
		sidebarSplit:             sidebarSplitDefault,
		editorSplit:              editorSplitDefault,
		historyStore:             cfg.History,
		currentFile:              cfg.FilePath,
		statusMessage:            initialStatus,
		scriptRunner:             scripts.NewRunner(nil),
		globals:                  newGlobalStore(),
		fileVars:                 newFileStore(),
		oauth:                    oauth.NewManager(client),
		editorInsertMode:         false,
		editorWriteKeyMap:        writeKeyMap,
		editorViewKeyMap:         viewKeyMap,
		newFileInput:             newFileInput,
		openPathInput:            openPathInput,
		searchInput:              searchInput,
		searchTarget:             searchTargetEditor,
	}
	model.setInsertMode(false, false)

	model.doc = parser.Parse(cfg.FilePath, []byte(cfg.InitialContent))
	model.syncRequestList(model.doc)
	if model.historyStore != nil {
		_ = model.historyStore.Load()
	}
	model.syncHistory()
	model.setLivePane(responsePanePrimary)

	return model
}
