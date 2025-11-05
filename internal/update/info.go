package update

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"time"
)

var (
	errEmptyPayload = errors.New("empty release payload")
	errNoTag        = errors.New("missing release tag")
)

type Info struct {
	Version   string
	Notes     string
	Published time.Time
	Assets    []Asset
}

type Asset struct {
	Name string
	URL  string
	Size int64
}

// decodeInfo reads GitHub style release payloads into Info structures.
func decodeInfo(r io.Reader) (Info, error) {
	if r == nil {
		return Info{}, errEmptyPayload
	}

	var raw struct {
		Tag    string    `json:"tag_name"`
		Body   string    `json:"body"`
		Pub    time.Time `json:"published_at"`
		Assets []struct {
			Name string `json:"name"`
			URL  string `json:"browser_download_url"`
			Size int64  `json:"size"`
		} `json:"assets"`
	}

	dec := json.NewDecoder(r)
	if err := dec.Decode(&raw); err != nil {
		return Info{}, fmt.Errorf("decode release: %w", err)
	}

	if raw.Tag == "" {
		return Info{}, errNoTag
	}

	info := Info{
		Version:   raw.Tag,
		Notes:     raw.Body,
		Published: raw.Pub.UTC(),
	}

	if len(raw.Assets) > 0 {
		info.Assets = make([]Asset, 0, len(raw.Assets))
		for _, a := range raw.Assets {
			if a.Name == "" || a.URL == "" {
				continue
			}
			info.Assets = append(info.Assets, Asset{
				Name: a.Name,
				URL:  a.URL,
				Size: a.Size,
			})
		}
	}

	return info, nil
}

// Asset looks up an asset by name.
func (i Info) Asset(name string) (Asset, bool) {
	for _, a := range i.Assets {
		if a.Name == name {
			return a, true
		}
	}
	return Asset{}, false
}
