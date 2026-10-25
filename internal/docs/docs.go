package docs

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

const (
	DefaultBaseURL = "https://raw.githubusercontent.com/pnp/powershell/dev/documentation/"
	SiteURL        = "https://pnp.github.io/powershell/cmdlets/"
	DefaultTTL     = 24 * time.Hour
	maxBytes       = 2 << 20
)

type Client struct {
	BaseURL  string
	HTTP     *http.Client
	CacheDir string
	TTL      time.Duration

	now func() time.Time
}

func NewClient(cacheDir string) *Client {
	return &Client{BaseURL: DefaultBaseURL, HTTP: &http.Client{Timeout: 15 * time.Second}, CacheDir: cacheDir, TTL: DefaultTTL}
}

func DefaultCacheDir() string {
	dir, err := os.UserCacheDir()
	if err != nil {
		dir = os.TempDir()
	}
	return filepath.Join(dir, "pnp-powershell-mcp-server", "docs")
}

func (c *Client) Fetch(ctx context.Context, cmdlet, version string) (string, error) {
	path := c.cachePath(cmdlet, version)
	var cached string
	var cachedAt time.Time
	if path != "" {
		if b, err := os.ReadFile(path); err == nil {
			if fi, err := os.Stat(path); err == nil {
				cached, cachedAt = string(b), fi.ModTime()
			}
		}
		if cached != "" && c.clock().Sub(cachedAt) < c.TTL {
			return cached, nil
		}
	}

	md, err := c.download(ctx, cmdlet)
	if err != nil {
		if cached != "" {
			return cached, nil
		}
		return "", err
	}
	if path != "" && os.MkdirAll(filepath.Dir(path), 0o700) == nil {
		_ = os.WriteFile(path, []byte(md), 0o600)
	}
	return md, nil
}

func (c *Client) clock() time.Time {
	if c.now != nil {
		return c.now()
	}
	return time.Now()
}

func (c *Client) cachePath(cmdlet, version string) string {
	if c.CacheDir == "" {
		return ""
	}
	if version == "" {
		version = "unknown"
	}
	return filepath.Join(c.CacheDir, version, cmdlet+".md")
}

func (c *Client) download(ctx context.Context, cmdlet string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+cmdlet+".md", nil)
	if err != nil {
		return "", err
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, maxBytes))
	return string(b), err
}
