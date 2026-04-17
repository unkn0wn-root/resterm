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
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/unkn0wn-root/resterm/internal/bindings"
	"github.com/unkn0wn-root/resterm/internal/cli"
	"github.com/unkn0wn-root/resterm/internal/config"
	curl "github.com/unkn0wn-root/resterm/internal/curl/importer"
	histdb "github.com/unkn0wn-root/resterm/internal/history/sqlite"
	"github.com/unkn0wn-root/resterm/internal/openapi"
	"github.com/unkn0wn-root/resterm/internal/openapi/generator"
	"github.com/unkn0wn-root/resterm/internal/openapi/parser"
	"github.com/unkn0wn-root/resterm/internal/openapi/writer"
	"github.com/unkn0wn-root/resterm/internal/rtfmt"
	"github.com/unkn0wn-root/resterm/internal/theme"
	"github.com/unkn0wn-root/resterm/internal/ui"
	"github.com/unkn0wn-root/resterm/internal/update"
	str "github.com/unkn0wn-root/resterm/internal/util"
)

var (
	version = "dev"
	commit  = "unknown"
	date    = "unknown"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		if msg := str.Trim(err.Error()); msg != "" {
			_, _ = fmt.Fprintln(os.Stderr, msg)
		}
		os.Exit(cli.ExitCode(err))
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
	if ok, err := handleRunSubcommand(a); ok {
		return err
	}

	var (
		filePath                 string
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
	)

	exec := cli.NewExecFlags()
	fs := cli.NewFlagSet("resterm")
	fs.StringVar(&filePath, "file", "", "Path to .http/.rest file to open")
	exec.Bind(fs)
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
	if err := fs.Parse(a); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			printMainUsage(os.Stderr, fs)
			return nil
		}
		return cli.ExitErr{Err: err, Code: 2}
	}

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
	if err := exec.ValidateEnvFlag(); err != nil {
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
	filePath = cli.CleanExecPath(filePath)

	var initialContent string
	if filePath != "" {
		data, err := os.ReadFile(filePath)
		if err != nil {
			return fmt.Errorf("read file: %w", err)
		}
		initialContent = string(data)
	}

	cfg, err := exec.Resolve(filePath)
	if err != nil {
		return err
	}

	client, shutdown, err := cli.NewExecClient(version, exec)
	if err != nil {
		if exec.TelemetryConfig(version).Enabled() {
			log.Printf("telemetry init error: %v", err)
		}
	} else if shutdown != nil {
		defer func() {
			if shutdownErr := shutdown(); shutdownErr != nil {
				log.Printf("telemetry shutdown: %v", shutdownErr)
			}
		}()
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
	activeThemeKey := str.Trim(strings.ToLower(settings.DefaultTheme))
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
		EnvironmentSet:      cfg.EnvSet,
		EnvironmentName:     cfg.EnvName,
		EnvironmentFile:     cfg.EnvFile,
		EnvironmentFallback: cfg.EnvFallback,
		HTTPOptions:         cfg.HTTPOpts,
		GRPCOptions:         cfg.GRPCOpts,
		History:             historyStore,
		WorkspaceRoot:       cfg.Workspace,
		Recursive:           cfg.Recursive,
		Version:             version,
		UpdateClient:        uc,
		EnableUpdate:        updateEnabled,
		UpdateCmd:           ucmd,
		CompareTargets:      cfg.CompareTargets,
		CompareBase:         cfg.CompareBase,
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
	if _, err := fmt.Fprintln(w, "Subcommands:"); err != nil {
		return
	}
	if _, err := fmt.Fprintln(w, "  run         Execute request files without the TUI"); err != nil {
		return
	}
	if _, err := fmt.Fprintln(w, "  init        Bootstrap a new workspace"); err != nil {
		return
	}
	if _, err := fmt.Fprintln(w, "  collection  Export or import request bundles"); err != nil {
		return
	}
	if _, err := fmt.Fprintln(w, "  history     Manage persisted history"); err != nil {
		return
	}
	if _, err := fmt.Fprintln(w); err != nil {
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
	if str.Trim(opts.HeaderComment) == "" {
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
	if str.Trim(opts.Generate.BaseURLVariable) == "" {
		opts.Generate.BaseURLVariable = openapi.DefaultBaseURLVariable
	}
	if str.Trim(opts.Write.HeaderComment) == "" {
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
	src = str.Trim(src)
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
	src = str.Trim(src)
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
