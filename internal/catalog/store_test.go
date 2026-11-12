package catalog

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"sync"
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

type gatedRunner struct {
	stubRunner
	entered chan struct{}
	release chan struct{}
}

func (g gatedRunner) Run(ctx context.Context, script string, env map[string]string, d time.Duration) (pwsh.Result, error) {
	if script == pwsh.CatalogScript {
		g.entered <- struct{}{}
		<-g.release
	}
	return g.stubRunner.Run(ctx, script, env, d)
}

func TestStoreSerializesRebuilds(t *testing.T) {
	runner := gatedRunner{
		stubRunner: stubRunner{results: map[string]pwsh.Result{pwsh.CatalogScript: {Stdout: sample}}},
		entered:    make(chan struct{}, 2),
		release:    make(chan struct{}),
	}
	s := NewStore(runner, filepath.Join(t.TempDir(), "catalog.json"))

	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(runner.release) }) }
	defer release()

	results := make(chan error, 2)
	go func() { results <- s.Rebuild(context.Background()) }()
	select {
	case <-runner.entered:
	case <-time.After(time.Second):
		t.Fatal("first rebuild did not start")
	}
	go func() { results <- s.Rebuild(context.Background()) }()

	select {
	case <-runner.entered:
		release()
		<-results
		<-results
		t.Fatal("second rebuild started while the first was still running")
	case <-time.After(30 * time.Millisecond):
	}
	release()
	for range 2 {
		if err := <-results; err != nil {
			t.Fatalf("Rebuild() error = %v", err)
		}
	}
}

func TestWriteCacheAtomicallyReplacesCache(t *testing.T) {
	cache := filepath.Join(t.TempDir(), "nested", "catalog.json")
	s := NewStore(stubRunner{}, cache)
	s.writeCache(&Catalog{Version: "1.0.0", Commands: []Cmdlet{{Name: "Get-PnPList"}}})
	s.writeCache(&Catalog{Version: "2.0.0", Commands: []Cmdlet{{Name: "Get-PnPWeb"}}})

	c, err := s.readCache()
	if err != nil {
		t.Fatalf("readCache() error = %v", err)
	}
	if c.Version != "2.0.0" || c.Commands[0].Name != "Get-PnPWeb" {
		t.Fatalf("unexpected cached catalog: %+v", c)
	}
	info, err := os.Stat(cache)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
		t.Errorf("cache permissions = %o, want 600", info.Mode().Perm())
	}
	entries, err := os.ReadDir(filepath.Dir(cache))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != "catalog.json" {
		t.Errorf("temporary cache files were not cleaned up: %v", entries)
	}
}

type slowRunner struct {
	stubRunner
	release chan struct{}
}

func (s slowRunner) Run(ctx context.Context, script string, env map[string]string, d time.Duration) (pwsh.Result, error) {
	if script == pwsh.CatalogScript {
		<-s.release
	}
	return s.stubRunner.Run(ctx, script, env, d)
}

func TestStartDoesNotBlockAndWaitBlocksUntilBuilt(t *testing.T) {
	release := make(chan struct{})
	runner := slowRunner{
		stubRunner{results: map[string]pwsh.Result{pwsh.CatalogScript: {Stdout: sample}, pwsh.VersionScript: {Stdout: "3.0.0"}}},
		release,
	}
	s := NewStore(runner, filepath.Join(t.TempDir(), "c.json"))
	t.Cleanup(func() { <-s.ready })

	done := make(chan struct{})
	go func() { s.Start(context.Background()); close(done) }()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Start must not wait for the build")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if _, err := s.Wait(ctx); err == nil {
		t.Fatal("Wait should give up while the build is still running")
	}

	close(release)
	c, err := s.Wait(context.Background())
	if err != nil || c.Version != "3.0.0" {
		t.Fatalf("Wait = %v, %v", c, err)
	}
}

func TestStartServesCacheImmediately(t *testing.T) {
	cache := filepath.Join(t.TempDir(), "c.json")
	ok := stubRunner{results: map[string]pwsh.Result{pwsh.CatalogScript: {Stdout: sample}, pwsh.VersionScript: {Stdout: "3.0.0"}}}
	if err := NewStore(ok, cache).Rebuild(context.Background()); err != nil {
		t.Fatal(err)
	}

	release := make(chan struct{})
	s := NewStore(slowRunner{stubRunner{results: map[string]pwsh.Result{pwsh.VersionScript: {Stdout: "9.9.9"}, pwsh.CatalogScript: {Stdout: sample}}}, release}, cache)
	s.Start(context.Background())
	t.Cleanup(func() { <-s.ready })
	defer close(release)
	if c, err := s.Wait(context.Background()); err != nil || c.Version != "3.0.0" {
		t.Fatalf("cached catalog should be served immediately: %v %v", c, err)
	}
}
