package update

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
)

var (
	ErrUnknownRepo   = errors.New("update repo not set")
	ErrNoRelease     = errors.New("no release available")
	ErrNoUpdate      = errors.New("already on latest version")
	ErrNoAsset       = errors.New("platform asset not found")
	errNilHTTPClient = errors.New("http client is nil")
)

const (
	apiHost   = "https://api.github.com"
	userAgent = "resterm-update"
)

type Client struct {
	http *http.Client
	repo string
	api  string
}

// NewClient constructs an updater client targeting the supplied repository.
func NewClient(h *http.Client, repo string) (Client, error) {
	if repo == "" {
		return Client{}, ErrUnknownRepo
	}
	if h == nil {
		h = http.DefaultClient
	}
	return Client{http: h, repo: repo, api: apiHost}, nil
}

// WithAPI overrides the GitHub API host, useful for testing.
func (c Client) WithAPI(v string) Client {
	if v == "" {
		c.api = apiHost
		return c
	}
	c.api = strings.TrimRight(v, "/")
	return c
}

// Ready reports whether the client has both a repo and an HTTP client.
func (c Client) Ready() bool {
	return c.repo != "" && c.http != nil
}

// Latest fetches the most recent GitHub release metadata.
func (c Client) Latest(ctx context.Context) (Info, error) {
	if c.repo == "" {
		return Info{}, ErrUnknownRepo
	}
	if c.http == nil {
		return Info{}, errNilHTTPClient
	}

	url := fmt.Sprintf("%s/repos/%s/releases/latest", c.api, c.repo)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return Info{}, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", userAgent)

	res, err := c.http.Do(req)
	if err != nil {
		return Info{}, fmt.Errorf("fetch latest release: %w", err)
	}
	defer func() {
		_ = res.Body.Close()
	}()

	switch res.StatusCode {
	case http.StatusOK:
		info, decErr := decodeInfo(res.Body)
		if decErr != nil {
			return Info{}, decErr
		}
		return info, nil
	case http.StatusNotFound:
		return Info{}, ErrNoRelease
	case http.StatusForbidden:
		if strings.Contains(strings.ToLower(res.Header.Get("X-RateLimit-Remaining")), "0") {
			return Info{}, fmt.Errorf("github api rate limited")
		}
		fallthrough
	default:
		return Info{}, fmt.Errorf("latest release request failed: %s", res.Status)
	}
}

type Result struct {
	Info   Info
	Bin    Asset
	Sum    Asset
	HasSum bool
}

// Check determines if an update is available for the platform and current version.
func (c Client) Check(ctx context.Context, curr string, plat Platform) (Result, error) {
	if curr == "" || curr == "dev" {
		return Result{}, ErrNoUpdate
	}

	info, err := c.Latest(ctx)
	if err != nil {
		return Result{}, err
	}

	need, cmpErr := needsUpdate(curr, info.Version)
	if cmpErr != nil {
		return Result{}, cmpErr
	}
	if !need {
		return Result{}, ErrNoUpdate
	}

	bin, ok := info.Asset(plat.Asset)
	if !ok {
		return Result{}, ErrNoAsset
	}

	res := Result{Info: info, Bin: bin}
	if sum, ok := info.Asset(plat.Sum); ok {
		res.Sum = sum
		res.HasSum = true
	}
	return res, nil
}

// needsUpdate compares the semantic versions and ignores parse failures for current versions.
func needsUpdate(curr, latest string) (bool, error) {
	lv, err := parseSemver(latest)
	if err != nil {
		return false, fmt.Errorf("latest: %w", err)
	}

	cv, err := parseSemver(curr)
	if err != nil {
		return true, nil
	}
	return cv.lt(lv), nil
}
