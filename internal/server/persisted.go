package server

import (
	"encoding/json"
	"fmt"
	"os"
	"runtime"
	"strings"
)

type PersistedConnection struct {
	Name     string `json:"name,omitempty"`
	Tenant   string `json:"tenant"`
	ClientID string `json:"clientId"`
}

func normalizeTenant(t string) string {
	t = strings.ToLower(strings.TrimSpace(t))
	for _, suffix := range []string{".sharepoint.com", ".onmicrosoft.com"} {
		t = strings.TrimSuffix(t, suffix)
	}
	return t
}

// ParsePersistedConnections parses a JSON array of {tenant, clientId, name?}, or
// "@path" to read that JSON from a file. Empty input means no entries.
func ParsePersistedConnections(spec string) ([]PersistedConnection, error) {
	spec = strings.TrimSpace(spec)
	if spec == "" {
		return nil, nil
	}
	fromFile := ""
	if path, ok := strings.CutPrefix(spec, "@"); ok {
		fromFile = path
		info, err := os.Stat(path)
		if err != nil {
			return nil, fmt.Errorf("reading persisted connections: %w", err)
		}
		if !info.Mode().IsRegular() {
			return nil, fmt.Errorf("persisted connections file %s is not a regular file", path)
		}
		if runtime.GOOS != "windows" && info.Mode().Perm()&0o022 != 0 {
			return nil, fmt.Errorf("persisted connections file %s is writable by group or others (mode %04o); run chmod go-w on it", path, info.Mode().Perm())
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("reading persisted connections: %w", err)
		}
		spec = string(b)
	}
	var list []PersistedConnection
	if err := json.Unmarshal([]byte(spec), &list); err != nil {
		if fromFile != "" {
			return nil, fmt.Errorf("persisted connections file %s must be a JSON array of {\"tenant\",\"clientId\"}", fromFile)
		}
		return nil, fmt.Errorf("persisted connections must be a JSON array of {\"tenant\",\"clientId\"}: %w", err)
	}
	seen := map[string]bool{}
	names := map[string]bool{}
	for i := range list {
		list[i].Tenant = normalizeTenant(list[i].Tenant)
		list[i].ClientID = strings.TrimSpace(list[i].ClientID)
		list[i].Name = strings.ToLower(strings.TrimSpace(list[i].Name))
		if list[i].Name == "" {
			list[i].Name = list[i].Tenant
		}
		if list[i].Tenant == "" || list[i].ClientID == "" {
			return nil, fmt.Errorf("persisted connection %d needs both tenant and clientId", i+1)
		}
		if strings.ContainsAny(list[i].Tenant, "./:") {
			return nil, fmt.Errorf("persisted connection %d: tenant must be the tenant name (e.g. \"contoso\"), got %q", i+1, list[i].Tenant)
		}
		if seen[list[i].Tenant] {
			return nil, fmt.Errorf("persisted connection tenant %q is listed twice", list[i].Tenant)
		}
		seen[list[i].Tenant] = true
		if names[list[i].Name] {
			return nil, fmt.Errorf("persisted connection name %q is used twice", list[i].Name)
		}
		names[list[i].Name] = true
	}
	return list, nil
}
