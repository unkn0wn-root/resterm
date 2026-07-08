package update

import (
	"fmt"
	"runtime"
	"strings"
)

type Platform struct {
	OS    string
	Arch  string
	Asset string // release asset name, e.g. "resterm_Darwin_arm64"
}

const binPrefix = "resterm"

func Detect() (Platform, error) {
	return For(runtime.GOOS, runtime.GOARCH)
}

func For(goos, goarch string) (Platform, error) {
	osName, err := mapOS(goos)
	if err != nil {
		return Platform{}, err
	}
	archName, err := mapArch(goarch)
	if err != nil {
		return Platform{}, err
	}
	asset := fmt.Sprintf("%s_%s_%s", binPrefix, osName, archName)
	if osName == "Windows" {
		asset += ".exe"
	}
	return Platform{
		OS:    goos,
		Arch:  goarch,
		Asset: asset,
	}, nil
}

// mapOS and mapArch translate runtime values to the uname style labels the
// release workflow bakes into asset names (see .github/workflows/ci.yml).
func mapOS(v string) (string, error) {
	switch strings.ToLower(v) {
	case "linux":
		return "Linux", nil
	case "darwin":
		return "Darwin", nil
	case "windows":
		return "Windows", nil
	default:
		return "", fmt.Errorf("unsupported os: %s", v)
	}
}

func mapArch(v string) (string, error) {
	switch v {
	case "amd64":
		return "x86_64", nil
	case "arm64":
		return "arm64", nil
	default:
		return "", fmt.Errorf("unsupported arch: %s", v)
	}
}
