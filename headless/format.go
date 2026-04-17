package headless

import (
	"fmt"

	str "github.com/unkn0wn-root/resterm/internal/util"
)

type Format int

const (
	JSON Format = iota
	JUnit
	Text
)

func (f Format) String() string {
	switch f {
	case JSON:
		return "json"
	case JUnit:
		return "junit"
	case Text:
		return "text"
	default:
		return fmt.Sprintf("format(%d)", int(f))
	}
}

func ParseFormat(s string) (Format, error) {
	switch str.LowerTrim(s) {
	case JSON.String():
		return JSON, nil
	case JUnit.String():
		return JUnit, nil
	case Text.String():
		return Text, nil
	default:
		return 0, fmt.Errorf("headless: unknown format %q", s)
	}
}
