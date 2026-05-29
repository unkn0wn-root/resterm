package ui

import (
	"os"
	"os/user"
	"strings"
)

func currentStatusIdentity() (string, string) {
	username := ""
	if current, err := user.Current(); err == nil && current != nil {
		username = cleanStatusUsername(current.Username)
	}
	if username == "" {
		username = cleanStatusUsername(os.Getenv("USER"))
	}
	if username == "" {
		username = cleanStatusUsername(os.Getenv("USERNAME"))
	}

	host := ""
	if name, err := os.Hostname(); err == nil {
		host = cleanStatusHost(name)
	}
	return username, host
}

func cleanStatusUsername(username string) string {
	username = strings.TrimSpace(username)
	if username == "" {
		return ""
	}
	if i := strings.LastIndexAny(username, `\/`); i >= 0 {
		username = username[i+1:]
	}
	return strings.TrimSpace(username)
}

func cleanStatusHost(host string) string {
	host = strings.TrimSpace(host)
	if i := strings.IndexByte(host, '.'); i > 0 {
		host = host[:i]
	}
	return strings.TrimSpace(host)
}
