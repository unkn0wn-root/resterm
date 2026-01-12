package curl

import (
	"fmt"
	"sort"
	"strings"
)

const (
	warnFlagFormat    = "unsupported flag %s (ignored)"
	warnSettingFormat = "unsupported setting %s (ignored)"
)

type WarningCollector struct {
	seen map[string]struct{}
}

func newWarningCollector() *WarningCollector {
	return &WarningCollector{}
}

func (c *WarningCollector) Add(msg string) {
	if c == nil {
		return
	}
	msg = strings.TrimSpace(msg)
	if msg == "" {
		return
	}
	if c.seen == nil {
		c.seen = make(map[string]struct{})
	}
	c.seen[msg] = struct{}{}
}

func (c *WarningCollector) Flag(flag string) {
	flag = strings.TrimSpace(flag)
	if flag == "" {
		return
	}
	c.Add(fmt.Sprintf(warnFlagFormat, flag))
}

func (c *WarningCollector) Setting(key string) {
	key = strings.TrimSpace(key)
	if key == "" {
		return
	}
	c.Add(fmt.Sprintf(warnSettingFormat, key))
}

func (c *WarningCollector) UnknownFlags(flags []string) {
	if c == nil || len(flags) == 0 {
		return
	}
	for _, flag := range flags {
		c.Flag(flag)
	}
}

func (c *WarningCollector) List() []string {
	if c == nil || len(c.seen) == 0 {
		return nil
	}
	out := make([]string, 0, len(c.seen))
	for msg := range c.seen {
		out = append(out, msg)
	}
	sort.Strings(out)
	return out
}
