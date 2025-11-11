package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/unkn0wn-root/resterm/internal/bindings"
	"github.com/unkn0wn-root/resterm/internal/config"
	"github.com/unkn0wn-root/resterm/internal/grpcclient"
	"github.com/unkn0wn-root/resterm/internal/history"
	"github.com/unkn0wn-root/resterm/internal/httpclient"
	"github.com/unkn0wn-root/resterm/internal/openapi"
	"github.com/unkn0wn-root/resterm/internal/openapi/generator"
	"github.com/unkn0wn-root/resterm/internal/openapi/parser"
	"github.com/unkn0wn-root/resterm/internal/openapi/writer"
	"github.com/unkn0wn-root/resterm/internal/rtfmt"
	"github.com/unkn0wn-root/resterm/internal/telemetry"
	"github.com/unkn0wn-root/resterm/internal/theme"
	"github.com/unkn0wn-root/resterm/internal/ui"
	"github.com/unkn0wn-root/resterm/internal/update"
	"github.com/unkn0wn-root/resterm/internal/vars"
)

var (
	version = "dev"
	commit  = "unknown"
	date    = "unknown"
)

func main() {
	var (
		filePath                 string
		envName                  string
		envFile                  string
		workspace                string
		timeout                  time.Duration
		insecure                 bool
		follow                   bool
		proxyURL                 string
		recursive                bool
		showVersion              bool
		checkUpdate              bool
		doUpdate                 bool
		openapiSpec              string
		openapiOut               string
		openapiBase              string
		openapiResolveRefs       bool
		openapiIncludeDeprecated bool
		openapiServerIndex       int
		traceOTEndpoint          string
		traceOTInsecure          bool
		traceOTService           string
		compareTargetsRaw        string
		compareBaseline          string
	)

	telemetryCfg := telemetry.ConfigFromEnv(os.Getenv)
	traceOTEndpoint = telemetryCfg.Endpoint
	traceOTInsecure = telemetryCfg.Insecure
	traceOTService = telemetryCfg.ServiceName

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
	flag.BoolVar(&checkUpdate, "check-update", false, "Check for newer releases and exit")
	flag.BoolVar(&doUpdate, "update", false, "Download and install the latest release, if available")
	flag.StringVar(&openapiSpec, "from-openapi", "", "Path to OpenAPI specification file to convert")
	flag.StringVar(&openapiOut, "http-out", "", "Destination path for generated .http file")
	flag.StringVar(&openapiBase, "openapi-base-var", openapi.DefaultBaseURLVariable, "Variable name for the generated base URL")
	flag.BoolVar(&openapiResolveRefs, "openapi-resolve-refs", false, "Resolve external $ref references during OpenAPI import")
	flag.BoolVar(&openapiIncludeDeprecated, "openapi-include-deprecated", false, "Include deprecated operations when generating requests")
	flag.IntVar(&openapiServerIndex, "openapi-server-index", 0, "Preferred server index (0-based) from the spec to use as the base URL")
	flag.StringVar(&traceOTEndpoint, "trace-otel-endpoint", traceOTEndpoint, "OTLP collector endpoint used when @trace is enabled")
	flag.BoolVar(&traceOTInsecure, "trace-otel-insecure", traceOTInsecure, "Disable TLS for OTLP trace export")
	flag.StringVar(&traceOTService, "trace-otel-service", traceOTService, "Override service.name resource attribute for exported spans")
	flag.StringVar(&compareTargetsRaw, "compare", "", "Default environments for manual compare runs (comma/space separated)")
	flag.StringVar(&compareBaseline, "compare-base", "", "Baseline environment when --compare is used (defaults to first target)")
	flag.Parse()

	telemetryCfg.Endpoint = strings.TrimSpace(traceOTEndpoint)
	telemetryCfg.Insecure = traceOTInsecure
	telemetryCfg.ServiceName = strings.TrimSpace(traceOTService)
	telemetryCfg.Version = version

	if showVersion {
		fmt.Printf("resterm %s\n", version)
		fmt.Printf("  commit: %s\n", commit)
		fmt.Printf("  built:  %s\n", date)
		if sum, err := executableChecksum(); err == nil {
			fmt.Printf("  sha256: %s\n", sum)
		} else {
			fmt.Printf("  sha256: unavailable (%v)\n", err)
		}
		os.Exit(0)
	}

	updateHTTP := &http.Client{Timeout: 60 * time.Second}
	upClient, err := update.NewClient(updateHTTP, updateRepo)
	if err != nil {
		log.Fatalf("update client: %v", err)
	}

	if checkUpdate || doUpdate {
		u := newCLIUpdater(upClient, version)
		ctx := context.Background()
		res, ok, err := u.check(ctx)
		if err != nil {
			if errors.Is(err, errUpdateDisabled) {
				_ = rtfmt.Fprintln(
					os.Stdout,
					rtfmt.LogHandler(log.Printf, "update notice write failed: %v"),
					"Update checks are disabled for dev builds.",
				)
				os.Exit(0)
			}
			_ = rtfmt.Fprintf(
				os.Stderr,
				"update check failed: %v\n",
				rtfmt.LogHandler(log.Printf, "update check error write failed: %v"),
				err,
			)
			os.Exit(1)
		}
		if !ok {
			u.printNoUpdate()
			os.Exit(0)
		}
		u.printAvailable(res)
		u.printChangelog(res)
		if !doUpdate {
			_ = rtfmt.Fprintln(
				os.Stdout,
				rtfmt.LogHandler(log.Printf, "update hint write failed: %v"),
				"Run `resterm --update` to install.",
			)
			os.Exit(0)
		}
		_, err = u.apply(ctx, res)
		if err != nil && !errors.Is(err, update.ErrPendingSwap) {
			_ = rtfmt.Fprintf(
				os.Stderr,
				"update failed: %v\n",
				rtfmt.LogHandler(log.Printf, "update failure write failed: %v"),
				err,
			)
			os.Exit(1)
		}
		os.Exit(0)
	}

	if openapiSpec != "" {
		targetOut := openapiOut
		if targetOut == "" {
			targetOut = defaultHTTPOutputPath(openapiSpec)
		}

		opts := openapi.GenerateOptions{
			Parse: openapi.ParseOptions{ResolveExternalRefs: openapiResolveRefs},
			Generate: openapi.GeneratorOptions{
				BaseURLVariable:      openapiBase,
				IncludeDeprecated:    openapiIncludeDeprecated,
				PreferredServerIndex: openapiServerIndex,
			},
			Write: openapi.WriterOptions{
				HeaderComment:     fmt.Sprintf("Generated by resterm %s", version),
				OverwriteExisting: true,
			},
		}

		if err := convertOpenAPISpec(
			context.Background(),
			openapiSpec,
			targetOut,
			version,
			opts); err != nil {
			_ = rtfmt.Fprintf(
				os.Stderr,
				"openapi import error: %v\n",
				nil,
				err,
			)
			os.Exit(1)
		}

		_ = rtfmt.Fprintf(os.Stdout, "Generated %s from %s\n", nil, targetOut, openapiSpec)
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

	provider, err := telemetry.New(telemetryCfg)
	if err != nil {
		if telemetryCfg.Enabled() {
			log.Printf("telemetry init error: %v", err)
		}
	} else {
		client.SetTelemetry(provider)
		defer func() {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if shutdownErr := provider.Shutdown(ctx); shutdownErr != nil {
				log.Printf("telemetry shutdown: %v", shutdownErr)
			}
		}()
	}

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

	compareTargets, compareErr := parseCompareTargets(compareTargetsRaw)
	if compareErr != nil {
		log.Printf("invalid --compare value: %v", compareErr)
	}
	compareBaseline = strings.TrimSpace(compareBaseline)

	settings, settingsHandle, err := config.LoadSettings()
	if err != nil {
		log.Printf("settings load error: %v", err)
		settings = config.Settings{}
		settingsHandle = config.SettingsHandle{
			Path:   filepath.Join(config.Dir(), "settings.toml"),
			Format: config.SettingsFormatTOML,
		}
	}

	bindingMap, _, bindingErr := bindings.Load(config.Dir())
	if bindingErr != nil {
		log.Printf("bindings load error: %v", bindingErr)
		bindingMap = bindings.DefaultMap()
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
	updateEnabled := version != "dev"

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
		Version:             version,
		UpdateClient:        upClient,
		EnableUpdate:        updateEnabled,
		CompareTargets:      compareTargets,
		CompareBase:         compareBaseline,
		Bindings:            bindingMap,
	})

	program := tea.NewProgram(model, tea.WithAltScreen())
	if _, err := program.Run(); err != nil {
		handler := rtfmt.LogHandler(log.Printf, "program.Run() write failed: %v")
		_ = rtfmt.Fprintf(os.Stderr, "error: %v\n", handler, err)
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

func parseCompareTargets(raw string) ([]string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}

	replacer := strings.NewReplacer(",", " ", ";", " ")
	clean := replacer.Replace(raw)
	fields := strings.Fields(clean)
	if len(fields) == 0 {
		return nil, nil
	}

	seen := make(map[string]struct{}, len(fields))
	targets := make([]string, 0, len(fields))
	for _, field := range fields {
		value := strings.TrimSpace(field)
		if value == "" {
			continue
		}
		lower := strings.ToLower(value)
		if _, ok := seen[lower]; ok {
			continue
		}
		seen[lower] = struct{}{}
		targets = append(targets, value)
	}

	if len(targets) < 2 {
		return nil, fmt.Errorf("expected at least two environments, got %d", len(targets))
	}
	return targets, nil
}

func executableChecksum() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	exe, err = filepath.EvalSymlinks(exe)
	if err != nil {
		return "", err
	}
	f, err := os.Open(exe)
	if err != nil {
		return "", err
	}
	defer func() {
		_ = f.Close()
	}()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func convertOpenAPISpec(ctx context.Context, specPath, outputPath, version string, opts openapi.GenerateOptions) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if outputPath == "" {
		outputPath = defaultHTTPOutputPath(specPath)
	}
	if strings.TrimSpace(opts.Generate.BaseURLVariable) == "" {
		opts.Generate.BaseURLVariable = openapi.DefaultBaseURLVariable
	}
	if strings.TrimSpace(opts.Write.HeaderComment) == "" {
		opts.Write.HeaderComment = fmt.Sprintf("Generated by resterm %s", version)
	}
	svc := openapi.Service{
		Parser:    parser.NewLoader(),
		Generator: generator.NewBuilder(),
		Writer:    writer.NewFileWriter(),
	}
	return svc.GenerateHTTPFile(ctx, specPath, outputPath, opts)
}

func defaultHTTPOutputPath(specPath string) string {
	ext := filepath.Ext(specPath)
	if ext == "" {
		return specPath + ".http"
	}
	return strings.TrimSuffix(specPath, ext) + ".http"
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
