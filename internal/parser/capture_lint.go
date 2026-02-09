package parser

import (
	"fmt"

	"github.com/unkn0wn-root/resterm/internal/captureutil"
	"github.com/unkn0wn-root/resterm/internal/restfile"
)

func (b *documentBuilder) lintRequestCaptures(req *restfile.Request) {
	if b == nil || req == nil || len(req.Metadata.Captures) == 0 {
		return
	}
	for _, c := range req.Metadata.Captures {
		if captureutil.HasJSONPathDoubleDot(c.Expression) {
			b.addWarning(
				c.Line,
				fmt.Sprintf(
					"@capture %q expression %q has double dot after json (use response.json.<field>)",
					c.Name,
					c.Expression,
				),
			)
		}
		if c.Mode == restfile.CaptureExprModeTemplate &&
			captureutil.MixedTemplateRTSCall(c.Expression) {
			b.addWarning(
				c.Line,
				fmt.Sprintf(
					"@capture %q mixes template markers with RTS call syntax; use pure RTS or {{= ... }}",
					c.Name,
				),
			)
		}
	}
}
