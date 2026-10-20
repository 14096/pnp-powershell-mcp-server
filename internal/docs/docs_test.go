package docs

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

func newTestClient(t *testing.T, handler http.HandlerFunc) (*Client, *atomic.Int32, *httptest.Server) {
	t.Helper()
	var hits atomic.Int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		handler(w, r)
	}))
	t.Cleanup(ts.Close)
	return &Client{BaseURL: ts.URL + "/", HTTP: ts.Client(), CacheDir: t.TempDir(), TTL: time.Hour}, &hits, ts
}

func ok(body string) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(body)) }
}

func TestFetch(t *testing.T) {
	c, _, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/Get-PnPList.md" {
			_, _ = w.Write([]byte("# Get-PnPList"))
			return
		}
		http.NotFound(w, r)
	})
	got, err := c.Fetch(context.Background(), "Get-PnPList", "1.0")
	if err != nil || got != "# Get-PnPList" {
		t.Fatalf("Fetch = %q, %v", got, err)
	}
	if _, err := c.Fetch(context.Background(), "Get-PnPMissing", "1.0"); err == nil {
		t.Fatal("expected error on 404")
	}
}

func TestFetchUsesCache(t *testing.T) {
	c, hits, _ := newTestClient(t, ok("DOC"))
	ctx := context.Background()
	for range 3 {
		if got, err := c.Fetch(ctx, "Get-PnPList", "1.0"); err != nil || got != "DOC" {
			t.Fatalf("Fetch = %q, %v", got, err)
		}
	}
	if hits.Load() != 1 {
		t.Errorf("expected 1 network hit, got %d", hits.Load())
	}
	if _, err := c.Fetch(ctx, "Get-PnPList", "2.0"); err != nil || hits.Load() != 2 {
		t.Errorf("version change should refetch: hits=%d err=%v", hits.Load(), err)
	}
}

func TestFetchRefreshesAfterTTL(t *testing.T) {
	c, hits, _ := newTestClient(t, ok("DOC"))
	ctx := context.Background()
	_, _ = c.Fetch(ctx, "Get-PnPList", "1.0")
	c.now = func() time.Time { return time.Now().Add(2 * time.Hour) }
	_, _ = c.Fetch(ctx, "Get-PnPList", "1.0")
	if hits.Load() != 2 {
		t.Errorf("expected refetch after TTL, hits=%d", hits.Load())
	}
}

func TestFetchServesStaleWhenOffline(t *testing.T) {
	c, _, ts := newTestClient(t, ok("DOC"))
	ctx := context.Background()
	if _, err := c.Fetch(ctx, "Get-PnPList", "1.0"); err != nil {
		t.Fatal(err)
	}
	ts.Close()
	c.now = func() time.Time { return time.Now().Add(48 * time.Hour) }
	got, err := c.Fetch(ctx, "Get-PnPList", "1.0")
	if err != nil || got != "DOC" {
		t.Errorf("stale fallback = %q, %v", got, err)
	}
	if _, err := c.Fetch(ctx, "Get-PnPOther", "1.0"); err == nil {
		t.Error("no cache and offline should error")
	}
}

func TestFetchWithoutCacheDir(t *testing.T) {
	c, hits, _ := newTestClient(t, ok("DOC"))
	c.CacheDir = ""
	_, _ = c.Fetch(context.Background(), "Get-PnPList", "1.0")
	_, _ = c.Fetch(context.Background(), "Get-PnPList", "1.0")
	if hits.Load() != 2 {
		t.Errorf("hits=%d", hits.Load())
	}
	if entries, _ := os.ReadDir(filepath.Join(c.CacheDir)); len(entries) != 0 {
		t.Error("nothing should be written")
	}
}
