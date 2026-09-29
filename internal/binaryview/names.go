package binaryview

import (
	"mime"
	"net/http"
	"net/url"
	"path"
	"slices"
	"strings"
	"unicode"
)

type Body struct {
	Data        []byte
	ContentType string
	Disposition string
	URL         string
}

// Name tries to figure out a good filename for saving a response.
// Order:
// 1) Content-Disposition header
// 2) URL path
// 3) MIME type extension
// 4) Fallback to "response.bin"
func (b Body) Name() string {
	name := b.serverName()
	if name == "" {
		name = "response"
	}
	if path.Ext(name) != "" {
		return name
	}

	mt, _ := parseContentType(b.ContentType)
	if ft := lookupFileType(mt); ft.ext != "" {
		return name + ft.ext
	}
	exts, _ := mime.ExtensionsByType(mt)
	if len(exts) == 0 {
		return name + ".bin"
	}
	// The system list is sorted, so first try the extension named after the subtype.
	_, sub, _ := strings.Cut(mt, "/")
	base, _, _ := strings.Cut(sub, "+")
	for _, ext := range []string{"." + base, "." + sub} {
		if slices.Contains(exts, ext) {
			return name + ext
		}
	}
	return name + exts[0]
}

// ViewerName returns a file name that is safe to open with the default app.
// The extension decides which app runs, so it always comes from fileTypes and
// never from the server. We look at the Content-Type first, then the file
// name the server sent, then the body bytes.
func (b Body) ViewerName() (string, bool) {
	const maxName = 64
	server := b.serverName()
	ext := path.Ext(server)
	name := strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || strings.ContainsRune(" ._-", r) {
			return r
		}
		return '_'
	}, strings.TrimSuffix(server, ext))
	if runes := []rune(name); len(runes) > maxName {
		name = string(runes[:maxName])
	}
	name = avoidDevice(strings.Trim(name, " ."))
	if name == "" {
		name = "response"
	}

	for _, ct := range []string{b.ContentType, mime.TypeByExtension(ext), http.DetectContentType(b.Data)} {
		mt, _ := parseContentType(ct)
		switch ft := lookupFileType(mt); ft.open {
		case openFile:
			return name + ft.ext, true
		case openText:
			return name + ".txt", true
		}
	}
	return "", false
}

func (b Body) serverName() string {
	// ParseMediaType decodes filename* into params["filename"].
	if _, params, err := mime.ParseMediaType(b.Disposition); err == nil {
		if name := sanitizeFilename(params["filename"]); name != "" {
			return name
		}
	}
	u, err := url.Parse(b.URL)
	if err != nil {
		return ""
	}
	// A bare "/" segment sanitizes to "_", which is not a real name.
	if name := sanitizeFilename(path.Base(u.Path)); name != "_" {
		return name
	}
	return ""
}

type openMode int

const (
	openNever openMode = iota
	openFile
	openText
)

type fileType struct {
	ext  string
	open openMode
}

// The system MIME tables differ per OS and sort extensions alphabetically,
// so audio/mpeg would get .m2a. This table is checked first.
// HTML, SVG, XML and Markdown can carry scripts, so they open as plain text.
var fileTypes = map[string]fileType{
	"application/gzip":              {".gz", openFile},
	"application/json":              {".json", openFile},
	"application/msword":            {".doc", openNever},
	"application/pdf":               {".pdf", openFile},
	"application/vnd.ms-excel":      {".xls", openNever},
	"application/vnd.ms-powerpoint": {".ppt", openNever},
	"application/x-gzip":            {".gz", openFile},
	"application/xml":               {".xml", openText},
	"application/yaml":              {".yaml", openFile},
	"application/zip":               {".zip", openFile},
	"audio/aac":                     {".aac", openFile},
	"audio/flac":                    {".flac", openFile},
	"audio/mp4":                     {".m4a", openFile},
	"audio/mpeg":                    {".mp3", openFile},
	"audio/ogg":                     {".ogg", openFile},
	"audio/wav":                     {".wav", openFile},
	"audio/wave":                    {".wav", openFile},
	"image/avif":                    {".avif", openFile},
	"image/bmp":                     {".bmp", openFile},
	"image/gif":                     {".gif", openFile},
	"image/heic":                    {".heic", openFile},
	"image/jpeg":                    {".jpg", openFile},
	"image/png":                     {".png", openFile},
	"image/svg+xml":                 {".svg", openText},
	"image/tiff":                    {".tiff", openFile},
	"image/vnd.microsoft.icon":      {".ico", openFile},
	"image/webp":                    {".webp", openFile},
	"image/x-icon":                  {".ico", openFile},
	"text/csv":                      {".csv", openFile},
	"text/html":                     {".html", openText},
	"text/markdown":                 {".md", openText},
	"text/plain":                    {".txt", openFile},
	"text/xml":                      {".xml", openText},
	"video/mp4":                     {".mp4", openFile},
	"video/quicktime":               {".mov", openFile},
	"video/webm":                    {".webm", openFile},

	"application/vnd.openxmlformats-officedocument.presentationml.presentation": {".pptx", openFile},
	"application/vnd.openxmlformats-officedocument.spreadsheetml.sheet":         {".xlsx", openFile},
	"application/vnd.openxmlformats-officedocument.wordprocessingml.document":   {".docx", openFile},
}

var structuredSuffixes = map[string]string{
	"+json": "application/json",
	"+xml":  "application/xml",
	"+yaml": "application/yaml",
}

func lookupFileType(mimeType string) fileType {
	if ft, ok := fileTypes[mimeType]; ok {
		return ft
	}
	if i := strings.LastIndexByte(mimeType, '+'); i >= 0 {
		return fileTypes[structuredSuffixes[mimeType[i:]]]
	}
	return fileType{}
}

func sanitizeFilename(name string) string {
	clean := strings.TrimSpace(name)
	clean = strings.ReplaceAll(clean, "\\", "_")
	clean = strings.ReplaceAll(clean, "/", "_")
	return avoidDevice(strings.Trim(clean, "."))
}

// Windows 10 and older open a device for these names, even with an extension.
func avoidDevice(name string) string {
	base := name
	if i := strings.IndexAny(base, ".:"); i >= 0 {
		base = base[:i]
	}
	if windowsDevices[strings.ToUpper(strings.TrimRight(base, " "))] {
		return "_" + name
	}
	return name
}

var windowsDevices = func() map[string]bool {
	names := map[string]bool{"CON": true, "PRN": true, "AUX": true, "NUL": true, "CONIN$": true, "CONOUT$": true}
	for _, n := range []string{"0", "1", "2", "3", "4", "5", "6", "7", "8", "9", "¹", "²", "³"} {
		names["COM"+n] = true
		names["LPT"+n] = true
	}
	return names
}()
