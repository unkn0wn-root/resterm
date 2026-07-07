package cli

import (
	"cmp"
	"flag"
	"fmt"
	"io"
	"os"
	"reflect"
	"slices"
	"strconv"
	"strings"

	"github.com/mattn/go-runewidth"
	"golang.org/x/term"
)

const (
	rowIndent      = "  "
	columnGap      = "  "
	defaultWidth   = 80
	minHelpDivisor = 3
)

// MaxTextWidth caps wrapped CLI text output at a readable line length.
const MaxTextWidth = 100

func PrintFlagSetUsage(w io.Writer, app string, fs *flag.FlagSet) {
	_, _ = fmt.Fprintf(w, "Usage: %s %s [flags]\n\n", app, fs.Name())
	_, _ = fmt.Fprintln(w, "Flags:")
	PrintFlagDefaults(w, fs)
}

func PrintFlagDefaults(w io.Writer, fs *flag.FlagSet) {
	rows := collectRows(fs)
	lay := newLayout(rows, DetectWidth(w))
	for _, row := range rows {
		renderRow(w, row, lay)
	}
}

type flagRow struct {
	names string // joined invocation forms, e.g. "-e, --env string"
	help  string // description, including any "(default ...)" suffix
}

func collectRows(fs *flag.FlagSet) []flagRow {
	canonical, aliases := splitAliases(fs)

	names := make([]string, 0, len(canonical))
	for name := range canonical {
		names = append(names, name)
	}
	slices.Sort(names)

	rows := make([]flagRow, 0, len(names))
	for _, name := range names {
		f := canonical[name]
		arg, help := flag.UnquoteUsage(f)
		if _, ok := f.Value.(stringValue); ok {
			arg = "string"
		}
		if !isZeroDefault(f) {
			help = fmt.Sprintf("%s (default %s)", help, f.DefValue)
		}
		rows = append(rows, flagRow{
			names: joinNames(name, sortedAliases(aliases[name]), arg),
			help:  help,
		})
	}
	return rows
}

func splitAliases(fs *flag.FlagSet) (canonical map[string]*flag.Flag, aliases map[string][]string) {
	canonical = map[string]*flag.Flag{}
	aliases = map[string][]string{}
	fs.VisitAll(func(f *flag.Flag) {
		if target, ok := aliasTarget(f.Usage); ok {
			aliases[target] = append(aliases[target], f.Name)
			return
		}
		canonical[f.Name] = f
	})
	return canonical, aliases
}

func aliasTarget(usage string) (string, bool) {
	if !strings.HasPrefix(usage, aliasUsagePrefix) {
		return "", false
	}
	name := strings.TrimSpace(strings.TrimPrefix(usage, aliasUsagePrefix))
	return name, name != ""
}

func sortedAliases(names []string) []string {
	names = slices.Clone(names)
	slices.SortFunc(names, func(a, b string) int {
		if d := cmp.Compare(len(a), len(b)); d != 0 {
			return d
		}
		return cmp.Compare(a, b)
	})
	return names
}

// isZeroDefault reports whether f's default equals its type's zero value, so
// a redundant "(default ...)" can be dropped.
func isZeroDefault(f *flag.Flag) bool {
	typ := reflect.TypeOf(f.Value)
	var zero reflect.Value
	if typ.Kind() == reflect.Pointer {
		zero = reflect.New(typ.Elem())
	} else {
		zero = reflect.Zero(typ)
	}
	return f.DefValue == zero.Interface().(flag.Value).String()
}

func joinNames(long string, shorts []string, arg string) string {
	names := make([]string, 0, len(shorts)+1)
	for _, short := range shorts {
		names = append(names, "-"+short)
	}
	longName := "--" + long
	if arg != "" {
		longName += " " + arg
	}
	return strings.Join(append(names, longName), ", ")
}

type layout struct {
	width   int // full terminal width, in display columns
	nameCol int // width of the flag-name column
	helpCol int // width of the help-text column
}

func newLayout(rows []flagRow, width int) layout {
	avail := max(width-displayWidth(rowIndent)-displayWidth(columnGap), 2)

	widest := 0
	for _, row := range rows {
		widest = max(widest, displayWidth(row.names))
	}

	// Cap the name column so help text keeps at least 1/minHelpDivisor of the
	// row instead of collapsing into a one word column on narrow terminals.
	nameCol := min(widest, avail-max(avail/minHelpDivisor, 1))
	return layout{width: width, nameCol: nameCol, helpCol: avail - nameCol}
}

func renderRow(w io.Writer, row flagRow, lay layout) {
	help := wrapText(row.help, lay.helpCol)
	if len(help) == 0 {
		help = []string{""}
	}

	blank := strings.Repeat(" ", lay.nameCol)
	name := padRight(row.names, lay.nameCol)
	if displayWidth(row.names) > lay.nameCol {
		// Name is wider than its column: give it its own line(s) above the help.
		for _, line := range wrapText(row.names, lay.width-displayWidth(rowIndent)) {
			_, _ = fmt.Fprintf(w, "%s%s\n", rowIndent, line)
		}
		name = blank
	}
	for _, line := range help {
		_, _ = fmt.Fprintf(w, "%s%s%s%s\n", rowIndent, name, columnGap, line)
		name = blank
	}
}

func wrapText(s string, width int) []string {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	width = max(width, 1)

	var lines []string
	for paragraph := range strings.SplitSeq(s, "\n") {
		lines = append(lines, wrapWords(paragraph, width)...)
	}
	return lines
}

func wrapWords(s string, width int) []string {
	var lines []string
	line := ""
	flush := func() {
		if line != "" {
			lines = append(lines, line)
			line = ""
		}
	}
	for word := range strings.FieldsSeq(s) {
		for displayWidth(word) > width {
			flush()
			head, tail := splitAt(word, width)
			lines = append(lines, head)
			word = tail
		}
		switch {
		case word == "": // fully consumed by splitAt
		case line == "":
			line = word
		case displayWidth(line)+1+displayWidth(word) <= width:
			line += " " + word
		default:
			flush()
			line = word
		}
	}
	flush()
	return lines
}

// splitAt returns the longest prefix of s within width columns and the rest,
// always taking at least one rune so wrapping a too-wide word terminates.
func splitAt(s string, width int) (head, tail string) {
	used := 0
	for i, r := range s {
		rw := runewidth.RuneWidth(r)
		if used > 0 && used+rw > width {
			return s[:i], s[i:]
		}
		used += rw
	}
	return s, ""
}

func padRight(s string, width int) string {
	if gap := width - displayWidth(s); gap > 0 {
		return s + strings.Repeat(" ", gap)
	}
	return s
}

func displayWidth(s string) int {
	return runewidth.StringWidth(s)
}

// DetectWidth resolves the terminal width. COLUMNS overrides everything, otherwise
// writer, the standard streams and finally /dev/tty are probed so a redirected
// stderr still yields the real width. defaultWidth is the last resort.
func DetectWidth(w io.Writer) int {
	if cols, ok := envWidth(); ok {
		return cols
	}
	if cols, ok := fdWidth(w); ok {
		return cols
	}
	for _, std := range []io.Writer{os.Stderr, os.Stdout, os.Stdin} {
		if cols, ok := fdWidth(std); ok {
			return cols
		}
	}
	if tty, err := os.Open("/dev/tty"); err == nil {
		cols, ok := fdWidth(tty)
		_ = tty.Close()
		if ok {
			return cols
		}
	}
	return defaultWidth
}

func fdWidth(v io.Writer) (int, bool) {
	f, ok := v.(interface{ Fd() uintptr })
	if !ok {
		return 0, false
	}
	cols, _, err := term.GetSize(int(f.Fd()))
	if err != nil || cols <= 0 {
		return 0, false
	}
	return cols, true
}

func envWidth() (int, bool) {
	cols, err := strconv.Atoi(strings.TrimSpace(os.Getenv("COLUMNS")))
	if err != nil || cols <= 0 {
		return 0, false
	}
	return cols, true
}
