package ui

import (
	"fmt"

	xexec "github.com/unkn0wn-root/resterm/internal/exec"
)

func repeatProgressStatus(progress xexec.RepeatProgress) statusMsg {
	cycle := ""
	if progress.Poll > 1 {
		cycle = fmt.Sprintf(" · poll %d", progress.Poll)
	}

	var text string
	switch progress.Phase {
	case xexec.RepeatRetryWait:
		text = fmt.Sprintf(
			"Retrying in %s · attempt %d%s",
			formatDurationShort(progress.Delay),
			progress.Attempt,
			cycle,
		)
	case xexec.RepeatPollWait:
		text = fmt.Sprintf(
			"Polling in %s · attempt %d%s",
			formatDurationShort(progress.Delay),
			progress.Attempt,
			cycle,
		)
	case xexec.RepeatAttempt:
		text = fmt.Sprintf("Request attempt %d%s", progress.Attempt, cycle)
	}
	return statusMsg{text: text, level: statusInfo}
}
