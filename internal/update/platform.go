package update

import (
	"fmt"
	"runtime"
	"strings"
)

type Platform struct {
	OS    string
	Arch  string
	Asset string
	Sum   string
}

const binPrefix = "resterm"

// Detect inspects runtime.GOOS and GOARCH to produce a platform descriptor.
func Detect() (Platform, error) {
	return For(runtime.GOOS, runtime.GOARCH)
}

// For builds a Platform for the provided Go os/arch tuple.
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
		Sum:   asset + ".sha256",
	}, nil
}

// mapOS converts Go OS names to release asset suffixes.
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

// mapArch converts Go architecture strings to release asset suffixes.
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
