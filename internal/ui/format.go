package ui

import (
	"github.com/unkn0wn-root/resterm/internal/bodyfmt"
	"github.com/unkn0wn-root/resterm/internal/termcolor"
)

func prettifyBody(body []byte, contentType string) string {
	return bodyfmt.Prettify(
		body,
		contentType,
		bodyfmt.PrettyOptions{Color: termcolor.TrueColor()},
	)
}
