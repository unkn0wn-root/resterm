package bodyfmt

import (
	"bytes"
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"github.com/alecthomas/chroma/quick"

	"github.com/unkn0wn-root/resterm/internal/binaryview"
	js "github.com/unkn0wn-root/resterm/internal/parser/javascript"
	"github.com/unkn0wn-root/resterm/internal/termcolor"
)

const (
	RawHeavyLimit      = 128 * 1024
	RawBase64LineWidth = 76
)

type PrettyOptions struct {
	Color termcolor.Config
	Style string
}

type HeaderField struct {
	Name  string
	Value string
}

type RawMode int

const (
	RawText RawMode = iota
	RawHex
	RawBase64
	RawSummary
)

func (m RawMode) Label() string {
	switch m {
	case RawHex:
		return "hex"
	case RawBase64:
		return "base64"
	case RawSummary:
		return "summary"
	default:
		return "text"
	}
}

type BuildInput struct {
	Body            []byte
	ContentType     string
	Meta            *binaryview.Meta
	ViewBody        []byte
	ViewContentType string
	Color           termcolor.Config
	Style           string
}

type BodyViews struct {
	Pretty      string
	Raw         string
	RawText     string
	RawHex      string
	RawBase64   string
	Mode        RawMode
	Meta        binaryview.Meta
	ContentType string
}

var ANSISequenceRegex = regexp.MustCompile(
	"\x1b\\[[0-9;?]*[ -/]*[@-~]|\x1b\\][^\x07\x1b]*(?:\x07|\x1b\\\\)",
)

func Prettify(body []byte, contentType string, opt PrettyOptions) string {
	return PrettifyContext(context.Background(), body, contentType, opt)
}

func PrettifyContext(
	ctx context.Context,
	body []byte,
	contentType string,
	opt PrettyOptions,
) string {
	ct := strings.ToLower(contentType)
	source := string(body)
	lexer := ""

	if ctxDone(ctx) {
		return source
	}

	switch {
	case strings.Contains(ct, "json"):
		if formatted, ok := RenderJSONAsJSContext(ctx, body); ok {
			source = formatted
			lexer = "javascript"
		} else {
			if ctxDone(ctx) {
				return source
			}
			var buf bytes.Buffer
			if err := json.Indent(&buf, body, "", "  "); err == nil {
				source = buf.String()
			}
			lexer = "json"
		}
	case strings.Contains(ct, "xml"):
		if formatted, ok := indentXML(body); ok {
			source = formatted
		}
		lexer = "xml"
	case strings.Contains(ct, "html"):
		lexer = "html"
	case strings.Contains(ct, "yaml"):
		lexer = "yaml"
	case strings.Contains(ct, "javascript") || strings.Contains(ct, "ecmascript"):
		lexer = "javascript"
	}

	if !opt.Color.Enabled || lexer == "" || ctxDone(ctx) {
		return source
	}

	if highlighted, ok := highlight(source, lexer, opt.Color, opt.Style); ok {
		return highlighted
	}
	return source
}

func RenderJSONAsJSContext(ctx context.Context, body []byte) (string, bool) {
	if ctxDone(ctx) {
		return "", false
	}
	if formatted, err := js.FormatValue(string(body)); err == nil {
		return formatted, true
	}
	if ctxDone(ctx) {
		return "", false
	}
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.UseNumber()

	var value any
	if err := dec.Decode(&value); err != nil {
		return "", false
	}
	if err := dec.Decode(&struct{}{}); err != io.EOF {
		return "", false
	}
	var buf strings.Builder
	writeJSONValue(&buf, value, 0)
	return buf.String(), true
}

func FormatRaw(body []byte, contentType string) string {
	raw := TrimBody(string(body))
	formatted, ok := indentRaw(body, contentType)
	if !ok {
		return raw
	}
	return TrimBody(formatted)
}

func HeaderFields(headers http.Header) []HeaderField {
	if len(headers) == 0 {
		return nil
	}
	keys := make([]string, 0, len(headers))
	for name := range headers {
		keys = append(keys, name)
	}
	sort.Strings(keys)

	out := make([]HeaderField, 0, len(keys))
	for _, name := range keys {
		values := append([]string(nil), headers[name]...)
		sort.Strings(values)
		out = append(out, HeaderField{
			Name:  name,
			Value: strings.Join(values, ", "),
		})
	}
	return out
}

func FormatHeaders(headers http.Header) string {
	fields := HeaderFields(headers)
	if len(fields) == 0 {
		return ""
	}
	var buf strings.Builder
	for _, field := range fields {
		if field.Value == "" {
			fmt.Fprintf(&buf, "%s:\n", field.Name)
			continue
		}
		fmt.Fprintf(&buf, "%s: %s\n", field.Name, field.Value)
	}
	return strings.TrimRight(buf.String(), "\n")
}

func Build(in BuildInput) BodyViews {
	return BuildContext(context.Background(), in)
}

func BuildContext(ctx context.Context, in BuildInput) BodyViews {
	meta := in.meta()
	viewBody := in.viewBody()
	viewType := in.viewType()

	if !bytes.Equal(viewBody, in.Body) {
		viewMeta := binaryview.Analyze(viewBody, viewType)
		if viewMeta.Kind == binaryview.KindText {
			meta = viewMeta
		}
		if strings.TrimSpace(meta.MIME) == "" {
			meta.MIME = viewMeta.MIME
		}
		if strings.TrimSpace(meta.Charset) == "" {
			meta.Charset = viewMeta.Charset
		}
	}

	sz := len(in.Body)
	rawHex := ""
	rawBase64 := ""
	if sz <= RawHeavyLimit {
		rawHex = binaryview.HexDump(in.Body, binaryview.HexDumpBytesPerLine)
		rawBase64 = binaryview.Base64Lines(in.Body, RawBase64LineWidth)
	}

	rawMode := RawText
	rawText := ""
	if meta.Kind != binaryview.KindBinary || meta.Printable {
		rawText = FormatRaw(viewBody, viewType)
	}

	decoded := viewBody
	if meta.Kind == binaryview.KindText {
		if decodedText, ok, errText := binaryview.DecodeText(viewBody, meta.Charset); ok {
			decoded = []byte(decodedText)
			rawText = FormatRaw(decoded, viewType)
		} else if errText != "" {
			meta.DecodeErr = errText
		}
	}

	var prettyBody string
	if meta.Kind == binaryview.KindBinary {
		prettyBody = BinarySummary(meta)
		if RawHeavyBinary(meta, sz) {
			rawMode = RawSummary
		} else {
			rawMode = RawHex
		}
	} else {
		prettyBody = TrimBody(
			PrettifyContext(
				ctx,
				decoded,
				viewType,
				PrettyOptions{Color: in.Color, Style: in.Style},
			),
		)
	}

	rawMode = ClampRawMode(meta, sz, rawMode)
	if rawMode == RawHex && rawHex == "" {
		if RawHeavyBinary(meta, sz) {
			rawMode = RawSummary
		} else {
			rawMode = RawText
		}
	}

	if IsEmpty(prettyBody) {
		prettyBody = "<empty>"
	}

	rawDefault := rawText
	switch rawMode {
	case RawSummary:
		rawDefault = RawSummaryText(meta, sz)
	case RawHex:
		if rawHex != "" {
			rawDefault = rawHex
		}
	}
	if IsEmpty(rawDefault) {
		rawDefault = "<empty>"
	}

	return BodyViews{
		Pretty:      prettyBody,
		Raw:         rawDefault,
		RawText:     rawText,
		RawHex:      rawHex,
		RawBase64:   rawBase64,
		Mode:        rawMode,
		Meta:        meta,
		ContentType: viewType,
	}
}

func BinarySummary(meta binaryview.Meta) string {
	lines := []string{fmt.Sprintf("Binary body (%s)", FormatByteSize(int64(meta.Size)))}
	if mime := strings.TrimSpace(meta.MIME); mime != "" {
		lines = append(lines, "MIME: "+mime)
	}
	if warn := strings.TrimSpace(meta.DecodeErr); warn != "" {
		lines = append(lines, "Decode warning: "+warn)
	}
	if meta.PreviewHex != "" {
		lines = append(lines, "Preview hex: "+meta.PreviewHex)
	}
	if meta.PreviewB64 != "" {
		lines = append(lines, "Preview base64: "+meta.PreviewB64)
	}
	if modes := ModeLabels(meta, meta.Size); len(modes) > 0 {
		lines = append(lines, "Raw view: "+strings.Join(modes, " / "))
	}
	return strings.Join(lines, "\n")
}

func RawHeavy(sz int) bool {
	return sz > RawHeavyLimit
}

func RawHeavyBinary(meta binaryview.Meta, sz int) bool {
	if meta.Kind != binaryview.KindBinary || meta.Printable {
		return false
	}
	if sz <= 0 {
		sz = meta.Size
	}
	return RawHeavy(sz)
}

func RawSummaryText(meta binaryview.Meta, sz int) string {
	if sz <= 0 {
		sz = meta.Size
	}
	sizeText := FormatByteSize(int64(sz))
	mime := strings.TrimSpace(meta.MIME)
	title := fmt.Sprintf("Binary body (%s)", sizeText)
	if mime != "" {
		title = fmt.Sprintf("Binary body (%s, %s)", sizeText, mime)
	}
	return title + "\n<raw dump deferred>\nUse the raw view action to load hex/base64."
}

func ClampRawMode(meta binaryview.Meta, sz int, mode RawMode) RawMode {
	modes := AllowedRawModes(meta, sz)
	if slices.Contains(modes, mode) {
		return mode
	}
	if len(modes) == 0 {
		return RawText
	}
	return modes[0]
}

func AllowedRawModes(meta binaryview.Meta, sz int) []RawMode {
	if meta.Kind == binaryview.KindBinary && !meta.Printable {
		if RawHeavyBinary(meta, sz) {
			return []RawMode{RawSummary, RawHex, RawBase64}
		}
		return []RawMode{RawHex, RawBase64}
	}
	return []RawMode{RawText, RawHex, RawBase64}
}

func ModeLabels(meta binaryview.Meta, sz int) []string {
	modes := AllowedRawModes(meta, sz)
	labels := make([]string, 0, len(modes))
	for _, mode := range modes {
		labels = append(labels, mode.Label())
	}
	return labels
}

func FormatByteQuantity(n int64) string {
	if n == 1 {
		return "1 byte"
	}
	return fmt.Sprintf("%d bytes", n)
}

func FormatByteSize(n int64) string {
	if n < 0 {
		n = 0
	}
	units := []string{"B", "KiB", "MiB", "GiB"}
	f := float64(n)
	i := 0
	for i < len(units)-1 && f >= 1024 {
		f /= 1024
		i++
	}
	if i == 0 {
		return fmt.Sprintf("%d %s", n, units[i])
	}
	s := fmt.Sprintf("%.1f", f)
	s = strings.TrimRight(strings.TrimRight(s, "0"), ".")
	return s + " " + units[i]
}

func JoinSections(sections ...string) string {
	var parts []string
	for _, section := range sections {
		trimmed := TrimSection(section)
		if strings.TrimSpace(trimmed) == "" {
			continue
		}
		parts = append(parts, trimmed)
	}
	return strings.Join(parts, "\n\n")
}

func TrimSection(section string) string {
	if section == "" {
		return ""
	}
	return strings.Trim(section, "\r\n")
}

func TrimBody(body string) string {
	return strings.TrimRight(body, "\n")
}

func IsEmpty(body string) bool {
	return strings.TrimSpace(StripANSI(body)) == ""
}

func StripANSI(s string) string {
	return ANSISequenceRegex.ReplaceAllString(s, "")
}

func (in BuildInput) meta() binaryview.Meta {
	if in.Meta != nil {
		return *in.Meta
	}
	return binaryview.Analyze(in.Body, in.ContentType)
}

func (in BuildInput) viewBody() []byte {
	if len(in.ViewBody) > 0 {
		return in.ViewBody
	}
	return in.Body
}

func (in BuildInput) viewType() string {
	if strings.TrimSpace(in.ViewContentType) != "" {
		return in.ViewContentType
	}
	return in.ContentType
}

func indentRaw(body []byte, contentType string) (string, bool) {
	ct := strings.ToLower(contentType)
	switch {
	case strings.Contains(ct, "json"):
		var buf bytes.Buffer
		if err := json.Indent(&buf, body, "", "  "); err == nil {
			return buf.String(), true
		}
	case strings.Contains(ct, "xml"):
		return indentXML(body)
	}
	return "", false
}

func indentXML(body []byte) (string, bool) {
	dec := xml.NewDecoder(bytes.NewReader(body))
	var buf bytes.Buffer
	enc := xml.NewEncoder(&buf)
	enc.Indent("", "  ")
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", false
		}
		if err := enc.EncodeToken(tok); err != nil {
			return "", false
		}
	}
	if err := enc.Flush(); err != nil {
		return "", false
	}
	return buf.String(), true
}

func writeJSONValue(buf *strings.Builder, value any, indent int) {
	switch v := value.(type) {
	case map[string]any:
		writeJSONObject(buf, v, indent)
	case []any:
		writeJSONArray(buf, v, indent)
	case json.Number:
		buf.WriteString(v.String())
	case string:
		if formatted, ok := js.FormatInlineValue(v, indent); ok {
			buf.WriteString(formatted)
			return
		}
		buf.WriteString(strconv.Quote(v))
	case bool:
		if v {
			buf.WriteString("true")
		} else {
			buf.WriteString("false")
		}
	case nil:
		buf.WriteString("null")
	default:
		encoded, err := json.Marshal(v)
		if err != nil {
			buf.WriteString(strconv.Quote(fmt.Sprintf("%v", v)))
		} else {
			buf.Write(encoded)
		}
	}
}

func writeJSONObject(buf *strings.Builder, obj map[string]any, indent int) {
	keys := make([]string, 0, len(obj))
	for key := range obj {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	if len(keys) == 0 {
		buf.WriteString("{}")
		return
	}

	buf.WriteString("{\n")
	for i, key := range keys {
		buf.WriteString(strings.Repeat("  ", indent+1))
		buf.WriteString(formatJSProperty(key))
		buf.WriteString(": ")
		writeJSONValue(buf, obj[key], indent+1)
		if i < len(keys)-1 {
			buf.WriteString(",")
		}
		buf.WriteByte('\n')
	}
	buf.WriteString(strings.Repeat("  ", indent))
	buf.WriteString("}")
}

func writeJSONArray(buf *strings.Builder, arr []any, indent int) {
	if len(arr) == 0 {
		buf.WriteString("[]")
		return
	}

	buf.WriteString("[\n")
	for i, item := range arr {
		buf.WriteString(strings.Repeat("  ", indent+1))
		writeJSONValue(buf, item, indent+1)
		if i < len(arr)-1 {
			buf.WriteString(",")
		}
		buf.WriteByte('\n')
	}
	buf.WriteString(strings.Repeat("  ", indent))
	buf.WriteString("]")
}

func formatJSProperty(name string) string {
	if isJSIdentifier(name) {
		return name
	}
	return strconv.Quote(name)
}

func isJSIdentifier(name string) bool {
	if name == "" {
		return false
	}
	for i, r := range name {
		if i == 0 {
			if !unicode.IsLetter(r) && r != '_' && r != '$' {
				return false
			}
			continue
		}
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '_' && r != '$' {
			return false
		}
	}
	return true
}

func highlight(content, lexer string, color termcolor.Config, style string) (string, bool) {
	fmtter := color.Formatter()
	if fmtter == "" {
		return "", false
	}
	style = strings.TrimSpace(style)
	if style == "" {
		style = "monokai"
	}
	var buf bytes.Buffer
	if err := quick.Highlight(&buf, content, lexer, fmtter, style); err != nil {
		return "", false
	}
	return buf.String(), true
}

func ctxDone(ctx context.Context) bool {
	return ctx != nil && ctx.Err() != nil
}
