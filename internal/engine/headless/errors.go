package headless

import "errors"

var (
	errNilRequest           = errors.New("request is nil")
	errInteractiveWebSocket = errors.New(
		"interactive websocket requests are not supported in headless mode",
	)
	errCompareWithForEach   = errors.New("@compare cannot run alongside @for-each")
	errProfileWithForEach   = errors.New("@profile cannot run alongside @for-each")
	errProfileDuringCompare = errors.New("@profile cannot run during compare")
)
