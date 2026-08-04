package bodyfmt

import (
	"fmt"
	"strings"
)

var byteUnits = [...]string{"B", "KiB", "MiB", "GiB"}

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

	f := float64(n)
	unit := 0
	for unit < len(byteUnits)-1 && f >= 1024 {
		f /= 1024
		unit++
	}
	if unit == 0 {
		return fmt.Sprintf("%d %s", n, byteUnits[unit])
	}

	s := strings.TrimRight(strings.TrimRight(fmt.Sprintf("%.1f", f), "0"), ".")
	return s + " " + byteUnits[unit]
}
