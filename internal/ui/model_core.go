package ui

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/charmbracelet/bubbles/cursor"
	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/unkn0wn-root/resterm/internal/filesvc"
	"github.com/unkn0wn-root/resterm/internal/grpcclient"
	"github.com/unkn0wn-root/resterm/internal/history"
	"github.com/unkn0wn-root/resterm/internal/httpclient"
	"github.com/unkn0wn-root/resterm/internal/parser"
	"github.com/unkn0wn-root/resterm/internal/scripts"
	"github.com/unkn0wn-root/resterm/internal/theme"
	"github.com/unkn0wn-root/resterm/internal/vars"
	"github.com/unkn0wn-root/resterm/pkg/restfile"
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
	responseTabHistory
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
	sidebarSplitPadding = 3
)

type Config struct {
	FilePath        string
	InitialContent  string
	Client          *httpclient.Client
	Theme           *theme.Theme
	EnvironmentSet  vars.EnvironmentSet
	EnvironmentName string
	EnvironmentFile string
	HTTPOptions     httpclient.Options
	GRPCOptions     grpcclient.Options
	History         *history.Store
	WorkspaceRoot   string
	Recursive       bool
}

type Model struct {
	cfg                Config
	theme              theme.Theme
	client             *httpclient.Client
	grpcClient         *grpcclient.Client
	grpcOptions        grpcclient.Options
	workspaceRoot      string
	workspaceRecursive bool

	fileList         list.Model
	requestList      list.Model
	editor           textarea.Model
	responseViewport viewport.Model
	historyList      list.Model
	envList          list.Model

	responseTabs     []string
	activeTab        responseTab
	focus            paneFocus
	showEnvSelector  bool
	showHelp         bool
	helpJustOpened   bool
	showNewFileModal bool
	showOpenModal    bool

	statusMessage    statusMsg
	statusPulseBase  string
	statusPulseFrame int
	lastResponse     *httpclient.Response
	lastGRPC         *grpcclient.Response
	lastError        error

	scriptRunner *scripts.Runner
	testResults  []scripts.TestResult
	scriptError  error

	prettyView  string
	rawView     string
	headersView string

	responseRenderToken  string
	responseLoading      bool
	responseLoadingFrame int
	prettyWrapCache      cachedWrap
	rawWrapCache         cachedWrap
	headersWrapCache     cachedWrap

	historyStore        *history.Store
	historyEntries      []history.Entry
	historySelectedID   string
	historyJumpToLatest bool
	requestItems        []requestListItem

	width                 int
	height                int
	frameWidth            int
	frameHeight           int
	sidebarSplit          float64
	sidebarFilesHeight    int
	sidebarRequestsHeight int
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
	openPathInput         textinput.Model
	openPathError         string

	doc                *restfile.Document
	currentFile        string
	currentRequest     *restfile.Request
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

	editor := textarea.New()
	editor.Placeholder = "Write HTTP requests here..."
	editor.SetValue(cfg.InitialContent)
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

	response := viewport.New(0, 0)
	response.SetContent(centerContent(noResponseMessage, 0, 0))

	reqDelegate := list.NewDefaultDelegate()
	reqDelegate.ShowDescription = true
	requestList := list.New(nil, reqDelegate, 0, 0)
	requestList.Title = "Requests"
	requestList.SetShowStatusBar(false)
	requestList.SetShowHelp(false)
	requestList.SetFilteringEnabled(true)
	requestList.SetShowTitle(false)
	requestList.DisableQuitKeybindings()

	historyList := list.New(nil, list.NewDefaultDelegate(), 0, 0)
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

	model := Model{
		cfg:                cfg,
		theme:              th,
		client:             client,
		grpcClient:         grpcExec,
		grpcOptions:        cfg.GRPCOptions,
		workspaceRoot:      workspace,
		workspaceRecursive: cfg.Recursive,
		fileList:           fileList,
		requestList:        requestList,
		editor:             editor,
		responseViewport:   response,
		historyList:        historyList,
		envList:            envList,
		responseTabs:       []string{"Pretty", "Raw", "Headers", "History"},
		activeTab:          responseTabPretty,
		focus:              focusFile,
		sidebarSplit:       sidebarSplitDefault,
		historyStore:       cfg.History,
		currentFile:        cfg.FilePath,
		statusMessage:      initialStatus,
		scriptRunner:       scripts.NewRunner(),
		editorInsertMode:   false,
		editorWriteKeyMap:  writeKeyMap,
		editorViewKeyMap:   viewKeyMap,
		newFileInput:       newFileInput,
		openPathInput:      openPathInput,
	}
	model.setInsertMode(false, false)

	model.doc = parser.Parse(cfg.FilePath, []byte(cfg.InitialContent))
	model.syncRequestList(model.doc)
	if model.historyStore != nil {
		_ = model.historyStore.Load()
	}
	model.syncHistory()

	return model
}
