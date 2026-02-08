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
	st := captureutil.StrictEnabled(b.doc.Settings, b.fileSettings, req.Settings)
	for _, c := range req.Metadata.Captures {
		if captureutil.SuspiciousJSONDoubleDot(c.Expression) {
			b.addWarning(
				c.Line,
				fmt.Sprintf(
					"@capture %q expression %q looks suspicious (double dot after json)",
					c.Name,
					c.Expression,
				),
			)
		}
		if st && captureutil.IsLegacyTemplate(c.Expression) {
			b.addWarning(
				c.Line,
				fmt.Sprintf(
					"@capture %q uses legacy template syntax while capture.strict=true",
					c.Name,
				),
			)
		}
	}
}
