package server

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestParsePersistedConnectionsFile(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("permission bits not meaningful on Windows")
	}
	p := filepath.Join(t.TempDir(), "p.json")
	if err := os.WriteFile(p, []byte(`[{"tenant":"contoso","clientId":"abc"}]`), 0o600); err != nil {
		t.Fatal(err)
	}
	if got, err := ParsePersistedConnections("@" + p); err != nil || len(got) != 1 {
		t.Fatalf("0600: %+v, %v", got, err)
	}
	if err := os.Chmod(p, 0o666); err != nil {
		t.Fatal(err)
	}
	if _, err := ParsePersistedConnections("@" + p); err == nil {
		t.Error("expected error for group/world-writable file")
	}
}

func TestParsePersistedConnections(t *testing.T) {
	got, err := ParsePersistedConnections(`[{"tenant":"Contoso.sharepoint.com","clientId":" abc "}]`)
	if err != nil || len(got) != 1 || got[0].Tenant != "contoso" || got[0].ClientID != "abc" {
		t.Fatalf("got %+v, err %v", got, err)
	}
	if got, err := ParsePersistedConnections(""); err != nil || got != nil {
		t.Fatalf("empty: %+v, %v", got, err)
	}
	for _, bad := range []string{
		`not json`,
		`[{"tenant":"contoso"}]`,
		`[{"tenant":"a.b","clientId":"x"}]`,
		`[{"tenant":"a","clientId":"x"},{"tenant":"A","clientId":"y"}]`,
	} {
		if _, err := ParsePersistedConnections(bad); err == nil {
			t.Errorf("%s: expected error", bad)
		}
	}
}

func TestParsePersistedConnectionNames(t *testing.T) {
	got, err := ParsePersistedConnections(`[{"tenant":"a","clientId":"1"},{"tenant":"b","clientId":"2","name":" Dev "}]`)
	if err != nil || got[0].Name != "a" || got[1].Name != "dev" {
		t.Fatalf("got %+v, %v", got, err)
	}
	if _, err := ParsePersistedConnections(`[{"tenant":"a","clientId":"1"},{"tenant":"b","clientId":"2","name":"a"}]`); err == nil {
		t.Fatal("duplicate name accepted")
	}
}
