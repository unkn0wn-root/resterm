package mock

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type ClientOptions struct {
	Timeout            time.Duration
	InsecureSkipVerify bool
}

type Client struct {
	baseURL *url.URL
	http    *http.Client
}

type controlError struct {
	Status int
	Detail string
}

func (e *controlError) Error() string {
	if e.Detail == "" {
		return fmt.Sprintf("mock control request failed with HTTP %d", e.Status)
	}
	return e.Detail
}

func NewClient(rawURL string, opts ClientOptions) (*Client, error) {
	baseURL, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return nil, fmt.Errorf("parse mock URL: %w", err)
	}
	if baseURL.Scheme != "http" && baseURL.Scheme != "https" {
		return nil, errors.New("mock URL must use http or https")
	}
	if baseURL.Host == "" || baseURL.User != nil || baseURL.RawQuery != "" || baseURL.Fragment != "" {
		return nil, errors.New("mock URL must contain only a scheme and host")
	}
	if baseURL.Path != "" && baseURL.Path != "/" {
		return nil, errors.New("mock URL must not contain a path")
	}
	baseURL.Path = ""
	baseURL.RawPath = ""

	transport := http.DefaultTransport.(*http.Transport).Clone()
	if opts.InsecureSkipVerify {
		transport.TLSClientConfig = &tls.Config{
			MinVersion:         tls.VersionTLS12,
			InsecureSkipVerify: true, //nolint:gosec // explicitly requested for a local mock server
		}
	}
	timeout := opts.Timeout
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	return &Client{
		baseURL: baseURL,
		http: &http.Client{
			Transport: transport,
			Timeout:   timeout,
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
	}, nil
}

func (c *Client) Count(ctx context.Context, pattern RequestPattern) (uint64, error) {
	var response countResponse
	if err := c.post(ctx, controlCountPath, pattern, &response); err != nil {
		return 0, err
	}
	return response.Count, nil
}

func (c *Client) ResetSequences(ctx context.Context, name string) (int, error) {
	var response resetResponse
	if err := c.post(ctx, controlResetPath, resetRequest{Name: name}, &response); err != nil {
		return 0, err
	}
	return response.Reset, nil
}

func (c *Client) Clear(ctx context.Context) error {
	return c.post(ctx, controlClearPath, struct{}{}, nil)
}

func (c *Client) post(ctx context.Context, path string, request, response any) error {
	body, err := json.Marshal(request)
	if err != nil {
		return fmt.Errorf("encode mock control request: %w", err)
	}
	endpoint := *c.baseURL
	endpoint.Path = path
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint.String(), bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create mock control request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set(controlHeader, "1")

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("call mock control API: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	limited := io.LimitReader(resp.Body, controlBodyLimit+1)
	data, err := io.ReadAll(limited)
	if err != nil {
		return fmt.Errorf("read mock control response: %w", err)
	}
	if len(data) > controlBodyLimit {
		return errors.New("mock control response is too large")
	}
	if resp.StatusCode != http.StatusOK {
		var problem struct {
			Detail string `json:"detail"`
		}
		_ = json.Unmarshal(data, &problem)
		return &controlError{Status: resp.StatusCode, Detail: problem.Detail}
	}
	if response == nil {
		return nil
	}
	if err := json.Unmarshal(data, response); err != nil {
		return fmt.Errorf("decode mock control response: %w", err)
	}
	return nil
}
