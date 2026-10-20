package pwsh

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Install hints shown when a prerequisite is missing.
const (
	InstallPwshHint   = "Install PowerShell 7+: https://learn.microsoft.com/powershell/scripting/install/installing-powershell"
	InstallModuleHint = "Install the module in PowerShell 7+:  Install-Module PnP.PowerShell -Scope CurrentUser"
)

// Diagnosis describes the PowerShell environment and anything that blocks the server.
type Diagnosis struct {
	PSVersion     string   `json:"psVersion,omitempty"`
	ModuleVersion string   `json:"moduleVersion,omitempty"`
	Problems      []string `json:"problems,omitempty"`
}

// OK reports whether everything needed is present.
func (d Diagnosis) OK() bool { return len(d.Problems) == 0 }

// Diagnose checks that pwsh (7+) and the PnP.PowerShell module are available.
func Diagnose(ctx context.Context, r Runner) Diagnosis {
	var d Diagnosis
	res, err := r.Run(ctx, CheckScript, nil, time.Minute)
	switch {
	case errors.Is(err, ErrNotFound):
		d.Problems = append(d.Problems, ErrNotFound.Error()+". "+InstallPwshHint)
		return d
	case err != nil:
		d.Problems = append(d.Problems, "could not run PowerShell: "+err.Error())
		return d
	case res.ExitCode != 0:
		d.Problems = append(d.Problems, "PowerShell check failed: "+strings.TrimSpace(res.Stderr))
		return d
	}
	if err := json.Unmarshal([]byte(res.Stdout), &d); err != nil {
		d.Problems = append(d.Problems, "unexpected output from the PowerShell check")
		return d
	}
	if major, _ := strconv.Atoi(strings.SplitN(d.PSVersion, ".", 2)[0]); major < 7 {
		d.Problems = append(d.Problems, fmt.Sprintf("PowerShell %s is too old; version 7+ is required. %s", d.PSVersion, InstallPwshHint))
	}
	if d.ModuleVersion == "" {
		d.Problems = append(d.Problems, "the PnP.PowerShell module is not installed. "+InstallModuleHint)
	}
	return d
}
