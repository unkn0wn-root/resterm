package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/unkn0wn-root/resterm/internal/config"
	"github.com/unkn0wn-root/resterm/internal/grpcclient"
	"github.com/unkn0wn-root/resterm/internal/history"
	"github.com/unkn0wn-root/resterm/internal/httpclient"
	"github.com/unkn0wn-root/resterm/internal/theme"
	"github.com/unkn0wn-root/resterm/internal/ui"
	"github.com/unkn0wn-root/resterm/internal/vars"
)

var (
	version = "dev"
	commit  = "unknown"
	date    = "unknown"
)

func main() {
	var (
		filePath    string
		envName     string
		envFile     string
		workspace   string
		timeout     time.Duration
		insecure    bool
		follow      bool
		proxyURL    string
		recursive   bool
		showVersion bool
	)

	follow = true

	flag.StringVar(&filePath, "file", "", "Path to .http/.rest file to open")
	flag.StringVar(&envName, "env", "", "Environment name to use")
	flag.StringVar(&envFile, "env-file", "", "Path to environment file")
	flag.StringVar(&workspace, "workspace", "", "Workspace directory to scan for request files")
	flag.DurationVar(&timeout, "timeout", 30*time.Second, "Request timeout")
	flag.BoolVar(&insecure, "insecure", false, "Skip TLS certificate verification")
	flag.BoolVar(&follow, "follow", true, "Follow redirects")
	flag.StringVar(&proxyURL, "proxy", "", "HTTP proxy URL")
	flag.BoolVar(&recursive, "recursive", false, "Recursively scan workspace for request files")
	flag.BoolVar(&recursive, "recurisve", false, "(deprecated) Recursively scan workspace for request files")
	flag.BoolVar(&showVersion, "version", false, "Show resterm version")
	flag.Parse()

	if showVersion {
		fmt.Printf("resterm %s\n", version)
		fmt.Printf("  commit: %s\n", commit)
		fmt.Printf("  built:  %s\n", date)
		os.Exit(0)
	}

	if filePath == "" && flag.NArg() > 0 {
		filePath = flag.Arg(0)
	}

	var initialContent string
	if filePath != "" {
		data, err := os.ReadFile(filePath)
		if err != nil {
			log.Fatalf("read file: %v", err)
		}

		filePath = filepath.Clean(filePath)
		initialContent = string(data)
	}

	if workspace == "" {
		if filePath != "" {
			workspace = filepath.Dir(filePath)
		} else if wd, err := os.Getwd(); err == nil {
			workspace = wd
		} else {
			workspace = "."
		}
	} else {
		if abs, err := filepath.Abs(workspace); err == nil {
			workspace = abs
		}
	}

	envSet, resolvedEnvFile := loadEnvironment(envFile, filePath, workspace)
	var envFallback string
	if envName == "" && len(envSet) > 0 {
		selected, notify := selectDefaultEnvironment(envSet)
		if selected != "" {
			envName = selected
			if notify {
				envFallback = selected
			}
		}
	}

	client := httpclient.NewClient(nil)
	httpOpts := httpclient.Options{
		Timeout:            timeout,
		FollowRedirects:    follow,
		InsecureSkipVerify: insecure,
		ProxyURL:           proxyURL,
	}
	if filePath != "" {
		httpOpts.BaseDir = filepath.Dir(filePath)
	}

	grpcOpts := grpcclient.Options{
		DefaultPlaintext:    true,
		DefaultPlaintextSet: true,
	}

	historyStore := history.NewStore(config.HistoryPath(), 500)
	if err := historyStore.Load(); err != nil {
		log.Printf("history load error: %v", err)
	}

	settings, settingsHandle, err := config.LoadSettings()
	if err != nil {
		log.Printf("settings load error: %v", err)
		settings = config.Settings{}
		settingsHandle = config.SettingsHandle{
			Path:   filepath.Join(config.Dir(), "settings.toml"),
			Format: config.SettingsFormatTOML,
		}
	}

	themeCatalog, themeErr := theme.LoadCatalog([]string{config.ThemeDir()})
	if themeErr != nil {
		log.Printf("theme load error: %v", themeErr)
	}

	th := theme.DefaultTheme()
	activeThemeKey := strings.TrimSpace(strings.ToLower(settings.DefaultTheme))
	if activeThemeKey == "" {
		activeThemeKey = "default"
	}
	if def, ok := themeCatalog.Get(activeThemeKey); ok {
		th = def.Theme
		activeThemeKey = def.Key
		settings.DefaultTheme = def.Key
	} else {
		if settings.DefaultTheme != "" {
			log.Printf("theme %q not found; using built-in default", settings.DefaultTheme)
		}
		if def, ok := themeCatalog.Get("default"); ok {
			th = def.Theme
			activeThemeKey = def.Key
		} else {
			th = theme.DefaultTheme()
			activeThemeKey = "default"
		}
		settings.DefaultTheme = ""
	}
	model := ui.New(ui.Config{
		FilePath:            filePath,
		InitialContent:      initialContent,
		Client:              client,
		Theme:               &th,
		ThemeCatalog:        themeCatalog,
		ActiveThemeKey:      activeThemeKey,
		Settings:            settings,
		SettingsHandle:      settingsHandle,
		EnvironmentSet:      envSet,
		EnvironmentName:     envName,
		EnvironmentFile:     resolvedEnvFile,
		EnvironmentFallback: envFallback,
		HTTPOptions:         httpOpts,
		GRPCOptions:         grpcOpts,
		History:             historyStore,
		WorkspaceRoot:       workspace,
		Recursive:           recursive,
	})

	program := tea.NewProgram(model, tea.WithAltScreen())
	if _, err := program.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func loadEnvironment(explicit string, filePath string, workspace string) (vars.EnvironmentSet, string) {
	if explicit != "" {
		envs, err := vars.LoadEnvironmentFile(explicit)
		if err != nil {
			log.Printf("failed to load environment file %s: %v", explicit, err)
			return nil, ""
		}
		return envs, explicit
	}

	var searchPaths []string
	if filePath != "" {
		searchPaths = append(searchPaths, filepath.Dir(filePath))
	}
	if workspace != "" {
		searchPaths = append(searchPaths, workspace)
	}
	if cwd, err := os.Getwd(); err == nil {
		searchPaths = append(searchPaths, cwd)
	}

	envs, path, err := vars.ResolveEnvironment(searchPaths)
	if err != nil {
		return nil, ""
	}
	return envs, path
}

func selectDefaultEnvironment(envs vars.EnvironmentSet) (string, bool) {
	if len(envs) == 0 {
		return "", false
	}
	preferred := []string{"dev", "default", "local"}
	for _, name := range preferred {
		if _, ok := envs[name]; ok {
			return name, len(envs) > 1
		}
	}
	names := make([]string, 0, len(envs))
	for name := range envs {
		names = append(names, name)
	}
	sort.Strings(names)
	return names[0], len(envs) > 1
}
