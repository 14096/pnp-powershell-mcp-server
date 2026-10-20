package catalog

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/14096/pnp-powershell-mcp-server/internal/pwsh"
)

type stubRunner struct{ results map[string]pwsh.Result }

func (s stubRunner) Run(_ context.Context, script string, _ map[string]string, _ time.Duration) (pwsh.Result, error) {
	return s.results[script], nil
}

const sample = `{"version":"3.0.0","helpPath":"","commands":[{"name":"Get-PnPList","parameters":[],"parameterSets":[]}]}`

func TestStoreRebuildAndLoad(t *testing.T) {
	cache := filepath.Join(t.TempDir(), "sub", "catalog.json")
	runner := stubRunner{results: map[string]pwsh.Result{
		pwsh.CatalogScript: {Stdout: sample},
		pwsh.VersionScript: {Stdout: "3.0.0\n"},
	}}
	ctx := context.Background()

	s := NewStore(runner, cache)
	if s.Current() != nil {
		t.Fatal("expected empty store")
	}
	if err := s.Rebuild(ctx); err != nil {
		t.Fatal(err)
	}
	if c := s.Current(); c == nil || c.Version != "3.0.0" || len(c.Commands) != 1 {
		t.Fatalf("unexpected catalog: %+v", c)
	}

	runner.results[pwsh.CatalogScript] = pwsh.Result{ExitCode: 1, Stderr: "must not be called"}
	s2 := NewStore(runner, cache)
	if err := s2.Load(ctx); err != nil || s2.Current().Version != "3.0.0" {
		t.Fatalf("Load from cache: %v", err)
	}
}

func TestStoreRebuildErrors(t *testing.T) {
	runner := stubRunner{results: map[string]pwsh.Result{pwsh.CatalogScript: {ExitCode: 1, Stderr: "boom"}}}
	s := NewStore(runner, filepath.Join(t.TempDir(), "c.json"))
	if err := s.Rebuild(context.Background()); err == nil {
		t.Fatal("expected error when pwsh fails")
	}
}
