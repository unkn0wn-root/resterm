package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/unkn0wn-root/resterm/internal/cli"
	"github.com/unkn0wn-root/resterm/internal/collection"
	"github.com/unkn0wn-root/resterm/internal/protocol/grpcx"
	"github.com/unkn0wn-root/resterm/internal/restfile"
	"github.com/unkn0wn-root/resterm/internal/runx/fail"
)

func handleCollectionSubcommand(args []string) (bool, error) {
	if len(args) == 0 || args[0] != "collection" {
		return false, nil
	}
	if len(args) == 1 && cli.HasFileConflict("collection") {
		return true, cli.CommandFileConflict(
			"resterm",
			"collection",
			"pass a subcommand like `resterm collection export --workspace . --out ./bundle`",
		)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return true, runCollection(ctx, args[1:])
}

func runCollection(ctx context.Context, args []string) error {
	if len(args) == 0 {
		_ = writeln(os.Stderr, collectionUsageText())
		return cli.UsageError(errors.New("collection: subcommand is required"))
	}
	if cli.IsHelpArg(args[0]) {
		if err := writeln(os.Stdout, collectionUsageText()); err != nil {
			return fmt.Errorf("collection: write output: %w", err)
		}
		return nil
	}
	op := strings.TrimSpace(strings.ToLower(args[0]))
	var err error
	switch op {
	case "export":
		err = runCollectionExport(ctx, args[1:])
	case "import":
		err = runCollectionImport(ctx, args[1:])
	case "pack":
		err = runCollectionPack(ctx, args[1:])
	case "unpack":
		err = runCollectionUnpack(ctx, args[1:])
	default:
		_ = writeln(os.Stderr, collectionUsageText())
		return cli.UsageError(fmt.Errorf("collection: unknown subcommand %q", op))
	}
	if errors.Is(err, context.Canceled) {
		return cli.ExitErr{Err: err, Code: runfail.ExitCanceled}
	}
	return err
}

func runCollectionExport(ctx context.Context, args []string) error {
	fs := cli.NewSubcommandFlagSet("resterm", "collection export", os.Stderr)
	var workspace string
	var out string
	var name string
	var recursive bool
	var force bool

	fs.StringVar(&workspace, "workspace", "", "Workspace directory to export")
	fs.StringVar(&out, "out", "", "Output bundle directory path")
	fs.StringVar(&name, "name", "", "Optional bundle name")
	fs.BoolVar(&recursive, "recursive", false, "Recursively scan workspace for request files")
	fs.BoolVar(&force, "force", false, "Replace an existing bundle")

	if done, err := parseSubcommand(fs, args); done || err != nil {
		return err
	}

	if workspace == "" {
		return cli.UsageError(errors.New("collection export: --workspace is required"))
	}
	if out == "" {
		return cli.UsageError(errors.New("collection export: --out is required"))
	}

	res, err := collection.ExportBundle(ctx, collection.ExportOptions{
		Workspace: workspace,
		OutDir:    out,
		Name:      name,
		Recursive: recursive,
		Force:     force,
		ProtoSources: func(gr *restfile.GRPCRequest, dir string) ([]string, error) {
			return grpcx.ProtoSources(gr, grpcx.Options{BaseDir: dir, NoFallback: true})
		},
	})
	if err != nil {
		return fmt.Errorf("collection export: %w", err)
	}

	if err := writeCollectionOutput(
		"collection export",
		"Exported %d files to %s\n",
		res.FileCount,
		res.OutDir,
	); err != nil {
		return err
	}
	if err := writeCollectionOutput(
		"collection export",
		"Manifest: %s\n",
		res.ManifestPath,
	); err != nil {
		return err
	}
	return nil
}

func runCollectionImport(ctx context.Context, args []string) error {
	fs := cli.NewSubcommandFlagSet("resterm", "collection import", os.Stderr)
	var in string
	var workspace string
	var force bool
	var dry bool

	fs.StringVar(&in, "in", "", "Input bundle directory path")
	fs.StringVar(&workspace, "workspace", "", "Destination workspace directory")
	fs.BoolVar(&force, "force", false, "Overwrite existing destination files")
	fs.BoolVar(&dry, "dry-run", false, "Plan import without writing files")

	if done, err := parseSubcommand(fs, args); done || err != nil {
		return err
	}

	if in == "" {
		return cli.UsageError(errors.New("collection import: --in is required"))
	}
	if workspace == "" {
		return cli.UsageError(errors.New("collection import: --workspace is required"))
	}

	res, err := collection.ImportBundle(ctx, collection.ImportOptions{
		BundleDir: in,
		Workspace: workspace,
		Force:     force,
		DryRun:    dry,
	})
	if err != nil {
		return fmt.Errorf("collection import: %w", err)
	}

	if dry {
		if err := writeCollectionOutput(
			"collection import",
			"Dry-run: planned %d file operations (%d create, %d overwrite)\n",
			res.FileCount,
			res.Created,
			res.Overwritten,
		); err != nil {
			return err
		}
		return nil
	}
	if err := writeCollectionOutput(
		"collection import",
		"Imported %d files into %s (%d create, %d overwrite)\n",
		res.FileCount,
		res.Workspace,
		res.Created,
		res.Overwritten,
	); err != nil {
		return err
	}
	return nil
}

func runCollectionPack(ctx context.Context, args []string) error {
	fs := cli.NewSubcommandFlagSet("resterm", "collection pack", os.Stderr)
	var in string
	var out string
	var force bool

	fs.StringVar(&in, "in", "", "Input bundle directory path")
	fs.StringVar(&out, "out", "", "Output archive (.zip) path")
	fs.BoolVar(&force, "force", false, "Replace an existing file")

	if done, err := parseSubcommand(fs, args); done || err != nil {
		return err
	}

	if in == "" {
		return cli.UsageError(errors.New("collection pack: --in is required"))
	}
	if out == "" {
		return cli.UsageError(errors.New("collection pack: --out is required"))
	}

	res, err := collection.PackBundle(ctx, collection.PackOptions{
		BundleDir: in,
		OutFile:   out,
		Force:     force,
	})
	if err != nil {
		return fmt.Errorf("collection pack: %w", err)
	}

	if err := writeCollectionOutput(
		"collection pack",
		"Packed %d files into %s\n",
		res.FileCount,
		res.OutFile,
	); err != nil {
		return err
	}
	return nil
}

func runCollectionUnpack(ctx context.Context, args []string) error {
	fs := cli.NewSubcommandFlagSet("resterm", "collection unpack", os.Stderr)
	var in string
	var out string
	var force bool

	fs.StringVar(&in, "in", "", "Input archive (.zip) path")
	fs.StringVar(&out, "out", "", "Output bundle directory path")
	fs.BoolVar(&force, "force", false, "Replace an existing bundle")

	if done, err := parseSubcommand(fs, args); done || err != nil {
		return err
	}

	if in == "" {
		return cli.UsageError(errors.New("collection unpack: --in is required"))
	}
	if out == "" {
		return cli.UsageError(errors.New("collection unpack: --out is required"))
	}

	res, err := collection.UnpackBundle(ctx, collection.UnpackOptions{
		InFile: in,
		OutDir: out,
		Force:  force,
	})
	if err != nil {
		return fmt.Errorf("collection unpack: %w", err)
	}

	if err := writeCollectionOutput(
		"collection unpack",
		"Unpacked %d files to %s\n",
		res.FileCount,
		res.OutDir,
	); err != nil {
		return err
	}
	return nil
}

func writeCollectionOutput(op, format string, args ...any) error {
	if err := writef(os.Stdout, format, args...); err != nil {
		return fmt.Errorf("%s: write output: %w", op, err)
	}
	return nil
}

func collectionUsageText() string {
	return strings.TrimSpace(`
Usage: resterm collection <export|import|pack|unpack> [flags]

Subcommands:
  export --workspace <dir> --out <dir> [--name <name>] [--recursive] [--force]
      Export a Git-friendly collection bundle directory.
  import --in <dir> --workspace <dir> [--force] [--dry-run]
      Import a collection bundle into a workspace.
  pack --in <dir> --out <file.zip> [--force]
      Pack a bundle directory into a zip archive.
  unpack --in <file.zip> --out <dir> [--force]
      Unpack and validate a bundle archive into a directory.
`)
}
