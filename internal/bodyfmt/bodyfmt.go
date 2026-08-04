package bodyfmt

import (
	"bytes"
	"context"
	"strings"

	"github.com/unkn0wn-root/resterm/internal/binaryview"
	"github.com/unkn0wn-root/resterm/internal/termcolor"
)

const placeholder = "<empty>"

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

// source is the body we actually render: the view body after charset decoding,
// paired with the analysis and the size of the bytes it came from.
type source struct {
	Payload
	body []byte
	ct   string
}

func Build(ctx context.Context, in BuildInput) BodyViews {
	src := in.source()
	hex, b64 := in.dumps()

	text := ""
	if src.readable() {
		text = FormatRaw(src.body, src.ct)
	}

	pretty := ""
	if src.Meta.Kind == binaryview.KindBinary {
		pretty = src.BinarySummaryText()
	} else {
		pretty = TrimBody(Prettify(ctx, src.body, src.ct, in.pretty()))
	}

	mode := src.defaultMode(hex != "")
	raw := text
	switch mode {
	case RawSummary:
		raw = src.RawSummaryText()
	case RawHex:
		raw = hex
	}

	return BodyViews{
		Pretty:      orPlaceholder(pretty),
		Raw:         orPlaceholder(raw),
		RawText:     text,
		RawHex:      hex,
		RawBase64:   b64,
		Mode:        mode,
		Meta:        src.Meta,
		ContentType: src.ct,
	}
}

// source reconciles the analysis of the raw body with the analysis of the view
// body. A substituted view body (a decoded gRPC message, say) is what the user
// reads, so its text verdict wins, but the raw body keeps its MIME and charset
// whenever the view body has none.
func (in BuildInput) source() source {
	meta := in.baseMeta()
	body := in.viewBody()
	ct := in.viewType()

	if !bytes.Equal(body, in.Body) {
		view := binaryview.Analyze(body, ct)
		if view.Kind == binaryview.KindText {
			meta = view
		}
		if strings.TrimSpace(meta.MIME) == "" {
			meta.MIME = view.MIME
		}
		if strings.TrimSpace(meta.Charset) == "" {
			meta.Charset = view.Charset
		}
	}

	if meta.Kind == binaryview.KindText {
		if text, ok, decodeErr := binaryview.DecodeText(body, meta.Charset); ok {
			body = []byte(text)
		} else if decodeErr != "" {
			meta.DecodeErr = decodeErr
		}
	}

	return source{
		Payload: Payload{Meta: meta, Size: len(in.Body)},
		body:    body,
		ct:      ct,
	}
}

// dumps renders the hex and base64 views of the raw body. Heavy bodies are
// skipped here and loaded on demand once the user asks for that raw mode.
func (in BuildInput) dumps() (hex, b64 string) {
	if RawHeavy(len(in.Body)) {
		return "", ""
	}
	return binaryview.HexDump(in.Body, binaryview.HexDumpBytesPerLine),
		binaryview.Base64Lines(in.Body, RawBase64LineWidth)
}

func (in BuildInput) baseMeta() binaryview.Meta {
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

func (in BuildInput) pretty() PrettyOptions {
	return PrettyOptions{Color: in.Color, Style: in.Style}
}

func orPlaceholder(body string) string {
	if IsEmpty(body) {
		return placeholder
	}
	return body
}

func done(ctx context.Context) bool {
	return ctx != nil && ctx.Err() != nil
}
