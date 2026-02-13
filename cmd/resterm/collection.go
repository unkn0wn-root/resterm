package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/unkn0wn-root/resterm/internal/collection"
)

func handleCollectionSubcommand(args []string) (bool, error) {
	if len(args) == 0 || args[0] != "collection" {
		return false, nil
	}
	if len(args) == 1 && collectionTargetExists() {
		return true, fmt.Errorf(
			"collection: found file named \"collection\" in the current directory; use `resterm -- collection` or `resterm ./collection` to open it, or pass a subcommand like `resterm collection export --workspace . --out ./bundle`",
		)
	}
	return true, runCollection(args[1:])
}

func collectionTargetExists() bool {
	info, err := os.Stat("collection")
	if err != nil {
		return false
	}
	return !info.IsDir()
}

func runCollection(args []string) error {
	if len(args) == 0 {
		return errors.New(collectionUsageText())
	}
	op := strings.TrimSpace(strings.ToLower(args[0]))
	switch op {
	case "-h", "--help", "help":
		if err := writeln(os.Stdout, collectionUsageText()); err != nil {
			return fmt.Errorf("collection: write output: %w", err)
		}
		return nil
	case "export":
		return runCollectionExport(args[1:])
	case "import":
		return runCollectionImport(args[1:])
	case "pack":
		return runCollectionPack(args[1:])
	case "unpack":
		return runCollectionUnpack(args[1:])
	default:
		return fmt.Errorf("collection: unknown subcommand %q\n\n%s", op, collectionUsageText())
	}
}

func runCollectionExport(args []string) error {
	fs := newSubcommandFlagSet("collection export")
	var workspace string
	var out string
	var name string
	var recursive bool
	var force bool

	fs.StringVar(&workspace, "workspace", "", "Workspace directory to export")
	fs.StringVar(&out, "out", "", "Output bundle directory path")
	fs.StringVar(&name, "name", "", "Optional bundle name")
	fs.BoolVar(&recursive, "recursive", false, "Recursively scan workspace for request files")
	fs.BoolVar(&force, "force", false, "Overwrite existing output directory")

	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return fmt.Errorf("collection export: %w", err)
	}
	if len(fs.Args()) > 0 {
		return fmt.Errorf(
			"collection export: unexpected args: %s",
			strings.Join(fs.Args(), " "),
		)
	}

	workspace = strings.TrimSpace(workspace)
	if workspace == "" {
		return errors.New("collection export: --workspace is required")
	}
	out = strings.TrimSpace(out)
	if out == "" {
		return errors.New("collection export: --out is required")
	}
	name = strings.TrimSpace(name)

	res, err := collection.ExportBundle(collection.ExportOptions{
		Workspace: workspace,
		OutDir:    out,
		Name:      name,
		Recursive: recursive,
		Force:     force,
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

func runCollectionImport(args []string) error {
	fs := newSubcommandFlagSet("collection import")
	var in string
	var workspace string
	var force bool
	var dry bool

	fs.StringVar(&in, "in", "", "Input bundle directory path")
	fs.StringVar(&workspace, "workspace", "", "Destination workspace directory")
	fs.BoolVar(&force, "force", false, "Overwrite existing destination files")
	fs.BoolVar(&dry, "dry-run", false, "Plan import without writing files")

	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return fmt.Errorf("collection import: %w", err)
	}
	if len(fs.Args()) > 0 {
		return fmt.Errorf(
			"collection import: unexpected args: %s",
			strings.Join(fs.Args(), " "),
		)
	}

	in = strings.TrimSpace(in)
	if in == "" {
		return errors.New("collection import: --in is required")
	}
	workspace = strings.TrimSpace(workspace)
	if workspace == "" {
		return errors.New("collection import: --workspace is required")
	}

	res, err := collection.ImportBundle(collection.ImportOptions{
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

func runCollectionPack(args []string) error {
	fs := newSubcommandFlagSet("collection pack")
	var in string
	var out string
	var force bool

	fs.StringVar(&in, "in", "", "Input bundle directory path")
	fs.StringVar(&out, "out", "", "Output archive (.zip) path")
	fs.BoolVar(&force, "force", false, "Overwrite existing archive file")

	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return fmt.Errorf("collection pack: %w", err)
	}
	if len(fs.Args()) > 0 {
		return fmt.Errorf(
			"collection pack: unexpected args: %s",
			strings.Join(fs.Args(), " "),
		)
	}

	in = strings.TrimSpace(in)
	if in == "" {
		return errors.New("collection pack: --in is required")
	}
	out = strings.TrimSpace(out)
	if out == "" {
		return errors.New("collection pack: --out is required")
	}

	res, err := collection.PackBundle(collection.PackOptions{
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

func runCollectionUnpack(args []string) error {
	fs := newSubcommandFlagSet("collection unpack")
	var in string
	var out string
	var force bool

	fs.StringVar(&in, "in", "", "Input archive (.zip) path")
	fs.StringVar(&out, "out", "", "Output bundle directory path")
	fs.BoolVar(&force, "force", false, "Overwrite existing output directory")

	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return fmt.Errorf("collection unpack: %w", err)
	}
	if len(fs.Args()) > 0 {
		return fmt.Errorf(
			"collection unpack: unexpected args: %s",
			strings.Join(fs.Args(), " "),
		)
	}

	in = strings.TrimSpace(in)
	if in == "" {
		return errors.New("collection unpack: --in is required")
	}
	out = strings.TrimSpace(out)
	if out == "" {
		return errors.New("collection unpack: --out is required")
	}

	res, err := collection.UnpackBundle(collection.UnpackOptions{
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
