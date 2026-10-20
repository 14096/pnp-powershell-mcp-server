package docs

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestFetch(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/Get-PnPList.md" {
			_, _ = w.Write([]byte("# Get-PnPList"))
			return
		}
		http.NotFound(w, r)
	}))
	defer ts.Close()
	c := &Client{BaseURL: ts.URL + "/", HTTP: ts.Client()}

	got, err := c.Fetch(context.Background(), "Get-PnPList")
	if err != nil || got != "# Get-PnPList" {
		t.Fatalf("Fetch = %q, %v", got, err)
	}
	if _, err := c.Fetch(context.Background(), "Get-PnPMissing"); err == nil {
		t.Fatal("expected error on 404")
	}
}
