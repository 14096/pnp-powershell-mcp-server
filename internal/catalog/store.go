package catalog

import (
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/14096/pnp-powershell-mcp-server/internal/pwsh"
)

type Store struct {
	runner    pwsh.Runner
	cacheFile string

	mu  sync.RWMutex
	cat *Catalog
}

func NewStore(runner pwsh.Runner, cacheFile string) *Store {
	return &Store{runner: runner, cacheFile: cacheFile}
}

func DefaultCacheFile() string {
	dir, err := os.UserCacheDir()
	if err != nil {
		dir = os.TempDir()
	}
	return filepath.Join(dir, "pnp-powershell-mcp-server", "catalog.json")
}

func (s *Store) Current() *Catalog {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.cat
}

func (s *Store) set(c *Catalog) {
	s.mu.Lock()
	s.cat = c
	s.mu.Unlock()
}

func (s *Store) Load(ctx context.Context) error {
	if c, err := s.readCache(); err == nil {
		s.set(c)
		if v, err := s.installedVersion(ctx); err == nil && v == c.Version {
			return nil
		}
	}
	return s.Rebuild(ctx)
}

func (s *Store) readCache() (*Catalog, error) {
	b, err := os.ReadFile(s.cacheFile)
	if err != nil {
		return nil, err
	}
	var c Catalog
	if err := json.Unmarshal(b, &c); err != nil {
		return nil, err
	}
	if len(c.Commands) == 0 {
		return nil, fmt.Errorf("empty catalog cache")
	}
	return &c, nil
}

func (s *Store) installedVersion(ctx context.Context) (string, error) {
	r, err := s.runner.Run(ctx, pwsh.VersionScript, nil, time.Minute)
	if err != nil {
		return "", err
	}
	if r.ExitCode != 0 {
		return "", fmt.Errorf("PnP.PowerShell not found: %s", strings.TrimSpace(r.Stderr))
	}
	return strings.TrimSpace(r.Stdout), nil
}

func (s *Store) Rebuild(ctx context.Context) error {
	r, err := s.runner.Run(ctx, pwsh.CatalogScript, nil, 3*time.Minute)
	if err != nil {
		return err
	}
	if r.ExitCode != 0 {
		return fmt.Errorf("reading PnP.PowerShell commands failed (is the module installed? Install-Module PnP.PowerShell -Scope CurrentUser): %s", strings.TrimSpace(r.Stderr))
	}
	var c Catalog
	if err := json.Unmarshal([]byte(r.Stdout), &c); err != nil {
		return fmt.Errorf("parsing catalog: %w", err)
	}
	synopses := loadSynopses(c.HelpPath)
	for i := range c.Commands {
		c.Commands[i].Synopsis = synopses[strings.ToLower(c.Commands[i].Name)]
	}
	slices.SortFunc(c.Commands, func(a, b Cmdlet) int { return cmp.Compare(a.Name, b.Name) })
	c.UpdatedAt = time.Now()
	s.set(&c)
	s.writeCache(&c)
	return nil
}

func (s *Store) writeCache(c *Catalog) {
	b, err := json.Marshal(c)
	if err != nil || os.MkdirAll(filepath.Dir(s.cacheFile), 0o755) != nil {
		return
	}
	_ = os.WriteFile(s.cacheFile, b, 0o644)
}
