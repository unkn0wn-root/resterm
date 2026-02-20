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
	curl "github.com/unkn0wn-root/resterm/internal/curl/importer"
	"github.com/unkn0wn-root/resterm/internal/grpcclient"
	histdb "github.com/unkn0wn-root/resterm/internal/history/sqlite"
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

type cliExitErr struct {
	err  error
	code int
}

type exitCoder interface {
	ExitCode() int
}

func (e cliExitErr) Error() string {
	if e.err == nil {
		return ""
	}
	return e.err.Error()
}

func (e cliExitErr) Unwrap() error {
	return e.err
}

func (e cliExitErr) ExitCode() int {
	if e.code == 0 {
		return 1
	}
	return e.code
}

func main() {
	if err := run(os.Args[1:]); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(exitCode(err))
	}
}

func run(a []string) error {
	if ok, err := handleCollectionSubcommand(a); ok {
		return err
	}
	if ok, err := handleHistorySubcommand(a); ok {
		return err
	}
	if ok, err := handleInitSubcommand(a); ok {
		return err
	}

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
		curlSrc                  string
		openapiSpec              string
		httpOut                  string
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

	tc := telemetry.ConfigFromEnv(os.Getenv)
	traceOTEndpoint = tc.Endpoint
	traceOTInsecure = tc.Insecure
	traceOTService = tc.ServiceName

	fs := flag.NewFlagSet("resterm", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.StringVar(&filePath, "file", "", "Path to .http/.rest file to open")
	fs.StringVar(&envName, "env", "", "Environment name to use")
	fs.StringVar(&envFile, "env-file", "", "Path to environment file")
	fs.StringVar(&workspace, "workspace", "", "Workspace directory to scan for request files")
	fs.DurationVar(&timeout, "timeout", 30*time.Second, "Request timeout")
	fs.BoolVar(&insecure, "insecure", false, "Skip TLS certificate verification")
	fs.BoolVar(&follow, "follow", true, "Follow redirects")
	fs.StringVar(&proxyURL, "proxy", "", "HTTP proxy URL")
	fs.BoolVar(&recursive, "recursive", false, "Recursively scan workspace for request files")
	fs.BoolVar(
		&recursive,
		"recurisve",
		false,
		"(deprecated) Recursively scan workspace for request files",
	)
	fs.BoolVar(&showVersion, "version", false, "Show resterm version")
	fs.BoolVar(&checkUpdate, "check-update", false, "Check for newer releases and exit")
	fs.BoolVar(
		&doUpdate,
		"update",
		false,
		"Download and install the latest release, if available",
	)
	fs.StringVar(
		&curlSrc,
		"from-curl",
		"",
		"Curl command or file path to convert",
	)
	fs.StringVar(
		&openapiSpec,
		"from-openapi",
		"",
		"Path to OpenAPI specification file to convert",
	)
	fs.StringVar(&httpOut, "http-out", "", "Destination path for generated .http file")
	fs.StringVar(
		&openapiBase,
		"openapi-base-var",
		openapi.DefaultBaseURLVariable,
		"Variable name for the generated base URL",
	)
	fs.BoolVar(
		&openapiResolveRefs,
		"openapi-resolve-refs",
		false,
		"Resolve external $ref references during OpenAPI import",
	)
	fs.BoolVar(
		&openapiIncludeDeprecated,
		"openapi-include-deprecated",
		false,
		"Include deprecated operations when generating requests",
	)
	fs.IntVar(
		&openapiServerIndex,
		"openapi-server-index",
		0,
		"Preferred server index (0-based) from the spec to use as the base URL",
	)
	fs.StringVar(
		&traceOTEndpoint,
		"trace-otel-endpoint",
		traceOTEndpoint,
		"OTLP collector endpoint used when @trace is enabled",
	)
	fs.BoolVar(
		&traceOTInsecure,
		"trace-otel-insecure",
		traceOTInsecure,
		"Disable TLS for OTLP trace export",
	)
	fs.StringVar(
		&traceOTService,
		"trace-otel-service",
		traceOTService,
		"Override service.name resource attribute for exported spans",
	)
	fs.StringVar(
		&compareTargetsRaw,
		"compare",
		"",
		"Default environments for manual compare runs (comma/space separated)",
	)
	fs.StringVar(
		&compareBaseline,
		"compare-base",
		"",
		"Baseline environment when --compare is used (defaults to first target)",
	)
	if err := fs.Parse(a); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			printMainUsage(os.Stderr, fs)
			return nil
		}
		return cliExitErr{err: err, code: 2}
	}

	tc.Endpoint = strings.TrimSpace(traceOTEndpoint)
	tc.Insecure = traceOTInsecure
	tc.ServiceName = strings.TrimSpace(traceOTService)
	tc.Version = version

	if showVersion {
		fmt.Printf("resterm %s\n", version)
		fmt.Printf("  commit: %s\n", commit)
		fmt.Printf("  built:  %s\n", date)
		if sum, err := executableChecksum(); err == nil {
			fmt.Printf("  sha256: %s\n", sum)
		} else {
			fmt.Printf("  sha256: unavailable (%v)\n", err)
		}
		return nil
	}
	if err := validateReservedEnvironment(envName, "--env"); err != nil {
		return err
	}

	hc := &http.Client{Timeout: 60 * time.Second}
	uc, err := update.NewClient(hc, updateRepo)
	if err != nil {
		return fmt.Errorf("update client: %w", err)
	}

	src := installSrc()
	ucmd := updCmd(src)

	if checkUpdate || doUpdate {
		if doUpdate && src == srcBrew {
			return errors.New(updBlock(ucmd))
		}
		u := newCLIUpdater(uc, version)
		ctx := context.Background()
		res, ok, err := u.check(ctx)
		if err != nil {
			if errors.Is(err, errUpdateDisabled) {
				_ = rtfmt.Fprintln(
					os.Stdout,
					rtfmt.LogHandler(log.Printf, "update notice write failed: %v"),
					"Update checks are disabled for dev builds.",
				)
				return nil
			}
			return fmt.Errorf("update check failed: %w", err)
		}
		if !ok {
			u.printNoUpdate()
			return nil
		}
		u.printAvailable(res)
		u.printChangelog(res)
		if !doUpdate {
			_ = rtfmt.Fprintln(
				os.Stdout,
				rtfmt.LogHandler(log.Printf, "update hint write failed: %v"),
				updHint(ucmd),
			)
			return nil
		}
		if _, err := u.apply(ctx, res); err != nil && !errors.Is(err, update.ErrPendingSwap) {
			return fmt.Errorf("update failed: %w", err)
		}
		return nil
	}

	if curlSrc != "" && openapiSpec != "" {
		return errors.New("import error: choose either --from-curl or --from-openapi")
	}

	if curlSrc != "" {
		cmd, err := readCurlCommand(curlSrc)
		if err != nil {
			return fmt.Errorf("curl import error: %w", err)
		}

		targetOut := httpOut
		if targetOut == "" {
			targetOut = defaultCurlOutputPath(curlSrc)
		}

		opts := curl.WriterOptions{
			HeaderComment:     fmt.Sprintf("Generated by resterm %s", version),
			OverwriteExisting: true,
		}

		if err := convertCurlCommand(
			context.Background(),
			cmd,
			targetOut,
			version,
			opts); err != nil {
			return fmt.Errorf("curl import error: %w", err)
		}

		_ = rtfmt.Fprintf(os.Stdout, "Generated %s from curl\n", nil, targetOut)
		return nil
	}

	if openapiSpec != "" {
		targetOut := httpOut
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
			return fmt.Errorf("openapi import error: %w", err)
		}

		_ = rtfmt.Fprintf(os.Stdout, "Generated %s from %s\n", nil, targetOut, openapiSpec)
		return nil
	}

	if filePath == "" && fs.NArg() > 0 {
		filePath = fs.Arg(0)
	}

	var initialContent string
	if filePath != "" {
		data, err := os.ReadFile(filePath)
		if err != nil {
			return fmt.Errorf("read file: %w", err)
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

	provider, err := telemetry.New(tc)
	if err != nil {
		if tc.Enabled() {
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

	historyStore := histdb.New(config.HistoryPath())
	// History failures should never block the UI startup path.
	// We log issues and keep running with an empty in-memory view.
	if err := historyStore.Load(); err != nil {
		log.Printf("history load error: %v", err)
	} else if rec := historyStore.RecoveryInfo(); rec != nil {
		log.Printf("history db recovered: %s -> %s", rec.Path, rec.Backup)
	}
	// Migration is also best effort at startup so existing workflows
	// can continue even when legacy files are malformed.
	if n, err := historyStore.MigrateJSON(config.LegacyHistoryPath()); err != nil {
		log.Printf("history migration error: %v", err)
	} else if n > 0 {
		log.Printf(
			"history migration imported %d entries from %s",
			n,
			config.LegacyHistoryPath(),
		)
	}
	defer func() {
		if err := historyStore.Close(); err != nil {
			log.Printf("history close error: %v", err)
		}
	}()

	compareTargets, compareErr := parseCompareTargets(compareTargetsRaw)
	if compareErr != nil {
		return fmt.Errorf("invalid --compare value: %w", compareErr)
	}
	compareBaseline = strings.TrimSpace(compareBaseline)
	if err := validateReservedEnvironment(compareBaseline, "--compare-base"); err != nil {
		return fmt.Errorf("invalid --compare-base value: %w", err)
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
		UpdateClient:        uc,
		EnableUpdate:        updateEnabled,
		UpdateCmd:           ucmd,
		CompareTargets:      compareTargets,
		CompareBase:         compareBaseline,
		Bindings:            bindingMap,
	})

	program := tea.NewProgram(model, tea.WithAltScreen())
	if _, err := program.Run(); err != nil {
		return fmt.Errorf("error: %w", err)
	}
	return nil
}

func printMainUsage(w io.Writer, fs *flag.FlagSet) {
	if _, err := fmt.Fprintf(w, "Usage: %s [flags] [file]\n\n", fs.Name()); err != nil {
		return
	}
	if _, err := fmt.Fprintln(w, "Flags:"); err != nil {
		return
	}
	out := fs.Output()
	fs.SetOutput(w)
	defer fs.SetOutput(out)
	fs.PrintDefaults()
}

func exitCode(err error) int {
	if err == nil {
		return 0
	}
	var ex exitCoder
	if errors.As(err, &ex) {
		return ex.ExitCode()
	}
	return 1
}

func loadEnvironment(
	explicit string,
	filePath string,
	workspace string,
) (vars.EnvironmentSet, string) {
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
		if vars.IsReservedEnvironment(field) {
			return nil, fmt.Errorf("environment %q is reserved for shared defaults", field)
		}
		lower := strings.ToLower(field)
		if _, ok := seen[lower]; ok {
			continue
		}
		seen[lower] = struct{}{}
		targets = append(targets, field)
	}

	if len(targets) < 2 {
		return nil, fmt.Errorf("expected at least two environments, got %d", len(targets))
	}
	return targets, nil
}

func validateReservedEnvironment(value, flagName string) error {
	if vars.IsReservedEnvironment(value) {
		return fmt.Errorf(
			"%s %q is reserved for shared defaults; choose a concrete environment",
			flagName,
			value,
		)
	}
	return nil
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

func convertCurlCommand(
	ctx context.Context,
	cmd, outputPath, version string,
	opts curl.WriterOptions,
) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if outputPath == "" {
		outputPath = "curl.http"
	}
	if strings.TrimSpace(opts.HeaderComment) == "" {
		opts.HeaderComment = fmt.Sprintf("Generated by resterm %s", version)
	}
	svc := curl.Service{
		Writer: curl.NewFileWriter(),
	}
	return svc.GenerateHTTPFile(ctx, cmd, outputPath, opts)
}

func convertOpenAPISpec(
	ctx context.Context,
	specPath, outputPath, version string,
	opts openapi.GenerateOptions,
) error {
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

func readCurlCommand(src string) (string, error) {
	src = strings.TrimSpace(src)
	if src == "" {
		return "", fmt.Errorf("curl source is empty")
	}
	if src == "-" {
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			return "", err
		}
		return string(data), nil
	}
	if info, err := os.Stat(src); err == nil {
		if info.IsDir() {
			return "", fmt.Errorf("curl source %s is a directory", src)
		}
		data, err := os.ReadFile(src)
		if err != nil {
			return "", err
		}
		return string(data), nil
	}
	return src, nil
}

func defaultCurlOutputPath(src string) string {
	src = strings.TrimSpace(src)
	if src == "" || src == "-" {
		return "curl.http"
	}
	if info, err := os.Stat(src); err == nil && !info.IsDir() {
		return defaultHTTPOutputPath(src)
	}
	return "curl.http"
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
