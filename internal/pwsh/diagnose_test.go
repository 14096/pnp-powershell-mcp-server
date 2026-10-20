package pwsh

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"
)

type fakeRunner struct {
	res Result
	err error
}

func (f fakeRunner) Run(context.Context, string, map[string]string, time.Duration) (Result, error) {
	return f.res, f.err
}

func TestDiagnose(t *testing.T) {
	tests := []struct {
		name    string
		runner  fakeRunner
		problem string // substring expected in Problems; empty = healthy
	}{
		{"healthy", fakeRunner{res: Result{Stdout: `{"psVersion":"7.6.4","moduleVersion":"3.4.1"}`}}, ""},
		{"pwsh missing", fakeRunner{err: fmt.Errorf("%w: x", ErrNotFound)}, "not found in PATH"},
		{"pwsh too old", fakeRunner{res: Result{Stdout: `{"psVersion":"5.1.1","moduleVersion":"3.4.1"}`}}, "too old"},
		{"module missing", fakeRunner{res: Result{Stdout: `{"psVersion":"7.6.4","moduleVersion":""}`}}, "Install-Module"},
		{"script failure", fakeRunner{res: Result{ExitCode: 1, Stderr: "boom"}}, "boom"},
		{"garbage output", fakeRunner{res: Result{Stdout: "nope"}}, "unexpected output"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := Diagnose(context.Background(), tt.runner)
			if tt.problem == "" {
				if !d.OK() || d.ModuleVersion != "3.4.1" {
					t.Errorf("expected healthy: %+v", d)
				}
				return
			}
			if d.OK() || !strings.Contains(strings.Join(d.Problems, "\n"), tt.problem) {
				t.Errorf("expected problem %q, got %+v", tt.problem, d)
			}
		})
	}
}
