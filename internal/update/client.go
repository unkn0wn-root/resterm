package update

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"golang.org/x/mod/semver"
)

var (
	ErrUnknownRepo = errors.New("update repo not set")
	ErrNoRelease   = errors.New("no release available")
	ErrDevBuild    = errors.New("update disabled for dev build")
	ErrNoAsset     = errors.New("platform asset not found")
	ErrNoDigest    = errors.New("release asset has no sha256 digest")
	ErrRateLimited = errors.New("github api rate limited")
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

type Result struct {
	Info   Info
	Bin    Asset
	Digest [sha256.Size]byte
}

func NewClient(h *http.Client, repo string) (Client, error) {
	if repo == "" {
		return Client{}, ErrUnknownRepo
	}
	if h == nil {
		h = http.DefaultClient
	}
	return Client{http: h, repo: repo, api: apiHost}, nil
}

func (c Client) Ready() bool {
	return c.repo != "" && c.http != nil
}

func (c Client) Latest(ctx context.Context) (Info, error) {
	url := fmt.Sprintf("%s/repos/%s/releases/latest", c.api, c.repo)
	res, err := c.do(ctx, url, "latest release", "application/vnd.github+json")
	if err != nil {
		return Info{}, err
	}
	defer func() {
		_ = res.Body.Close()
	}()

	switch res.StatusCode {
	case http.StatusOK:
		return decodeInfo(res.Body)
	case http.StatusNotFound:
		return Info{}, ErrNoRelease
	case http.StatusTooManyRequests:
		return Info{}, ErrRateLimited
	case http.StatusForbidden:
		if res.Header.Get("X-RateLimit-Remaining") == "0" {
			return Info{}, ErrRateLimited
		}
		fallthrough
	default:
		return Info{}, fmt.Errorf("latest release request failed: %s", res.Status)
	}
}

func (c Client) Check(ctx context.Context, curr string, plat Platform) (Result, bool, error) {
	if DevBuild(curr) {
		return Result{}, false, ErrDevBuild
	}

	info, err := c.Latest(ctx)
	if err != nil {
		return Result{}, false, err
	}

	need, err := needsUpdate(curr, info.Version)
	if err != nil || !need {
		return Result{}, false, err
	}

	bin, ok := info.Asset(plat.Asset)
	if !ok {
		return Result{}, false, ErrNoAsset
	}

	want, err := parseDigest(bin.Digest)
	if err != nil {
		return Result{}, false, err
	}
	return Result{Info: info, Bin: bin, Digest: want}, true, nil
}

func (c Client) do(ctx context.Context, url, what, accept string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("build %s request: %w", what, err)
	}
	req.Header.Set("User-Agent", userAgent)
	if accept != "" {
		req.Header.Set("Accept", accept)
	}

	res, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch %s: %w", what, err)
	}
	return res, nil
}

func (c Client) get(ctx context.Context, url, what string) (*http.Response, error) {
	res, err := c.do(ctx, url, what, "")
	if err != nil {
		return nil, err
	}
	if res.StatusCode != http.StatusOK {
		_ = res.Body.Close()
		return nil, fmt.Errorf("download %s failed: %s", what, res.Status)
	}
	return res, nil
}

func DevBuild(ver string) bool {
	return ver == "" || ver == "dev"
}

func needsUpdate(curr, latest string) (bool, error) {
	lv := canon(latest)
	if !semver.IsValid(lv) {
		return false, fmt.Errorf("invalid latest version %q", latest)
	}

	// snapshot builds carry commit-ish versions; treat them as outdated
	cv := canon(curr)
	if !semver.IsValid(cv) {
		return true, nil
	}
	return semver.Compare(cv, lv) < 0, nil
}

func canon(v string) string {
	if strings.HasPrefix(v, "v") {
		return v
	}
	return "v" + v
}
