// Package sandbox evaluates a Hyprland Lua configuration the way the compositor
// would, using a real Lua interpreter and a recording implementation of the
// `hl` API. It yields the effective configuration: resolved option values,
// binds produced by loops, rules assembled from variables, and the exact source
// location of every declaration.
package sandbox

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

//go:embed harness.lua
var harnessLua string

// Options configures one evaluation.
type Options struct {
	// Entry is the absolute path of the file to evaluate (hyprland.lua).
	Entry string
	// Timeout bounds execution. Zero selects a 15s default.
	Timeout time.Duration
	// LuaBin overrides interpreter discovery.
	LuaBin string
}

// ConfigCall is an hl.config invocation.
type ConfigCall struct {
	File  string         `json:"file"`
	Line  int            `json:"line"`
	Table map[string]any `json:"table"`
}

// SpecRecord is a generic hl.* call carrying a single spec table.
type SpecRecord struct {
	File    string         `json:"file"`
	Line    int            `json:"line"`
	Spec    map[string]any `json:"spec"`
	Enabled *bool          `json:"enabled,omitempty"`
}

// BindRecord is an hl.bind invocation.
type BindRecord struct {
	File    string         `json:"file"`
	Line    int            `json:"line"`
	Keys    string         `json:"keys"`
	DSP     DispatcherRec  `json:"dsp"`
	Opts    map[string]any `json:"opts"`
	Enabled *bool          `json:"enabled,omitempty"`
}

// DispatcherRec is a captured hl.dsp.* descriptor.
type DispatcherRec struct {
	Name string `json:"name"`
	Args []any  `json:"args"`
}

// CurveRecord is an hl.curve invocation.
type CurveRecord struct {
	File string         `json:"file"`
	Line int            `json:"line"`
	Name string         `json:"name"`
	Spec map[string]any `json:"spec"`
}

// EnvRecord is an hl.env invocation.
type EnvRecord struct {
	File  string `json:"file"`
	Line  int    `json:"line"`
	Name  string `json:"name"`
	Value string `json:"value"`
}

// PermissionRecord is an hl.permission invocation.
type PermissionRecord struct {
	File   string `json:"file"`
	Line   int    `json:"line"`
	Binary string `json:"binary"`
	Type   string `json:"type"`
	Mode   string `json:"mode"`
}

// EventRecord is an hl.on registration.
type EventRecord struct {
	File        string `json:"file"`
	Line        int    `json:"line"`
	Event       string `json:"event"`
	HasCallback bool   `json:"hasCallback"`
}

// ExecRecord is an hl.exec_cmd/hl.exec_raw invocation made while loading.
type ExecRecord struct {
	File  string         `json:"file"`
	Line  int            `json:"line"`
	Cmd   string         `json:"cmd"`
	Rules map[string]any `json:"rules"`
	Raw   bool           `json:"raw"`
}

// Report is the harness output.
type Report struct {
	Entry       string             `json:"entry"`
	Root        string             `json:"root"`
	Version     string             `json:"version"`
	Config      []ConfigCall       `json:"config"`
	Monitors    []SpecRecord       `json:"monitors"`
	Binds       []BindRecord       `json:"binds"`
	WindowRules []SpecRecord       `json:"window_rules"`
	LayerRules  []SpecRecord       `json:"layer_rules"`
	WorkspaceRules []SpecRecord    `json:"workspace_rules"`
	Curves      []CurveRecord      `json:"curves"`
	Animations  []SpecRecord       `json:"animations"`
	Gestures    []SpecRecord       `json:"gestures"`
	Devices     []SpecRecord       `json:"devices"`
	Envs        []EnvRecord        `json:"envs"`
	Permissions []PermissionRecord `json:"permissions"`
	Events      []EventRecord      `json:"events"`
	Timers      []map[string]any   `json:"timers"`
	Dispatches  []map[string]any   `json:"dispatches"`
	Execs       []ExecRecord       `json:"execs"`
	Errors      []struct {
		Message string `json:"message"`
	} `json:"errors"`
	Logs     []string `json:"logs"`
	Requires []string `json:"requires"`
	Files    []string `json:"files"`
}

// HarnessError describes a configuration failure reported by the harness.
type HarnessError struct {
	Message string
}

func (e *HarnessError) Error() string { return e.Message }

// Interpreter locates a usable Lua binary.
func Interpreter(override string) (string, error) {
	if override != "" {
		return override, nil
	}
	for _, c := range []string{"lua", "lua5.5", "lua5.4", "lua5.3", "luajit"} {
		if p, err := exec.LookPath(c); err == nil {
			return p, nil
		}
	}
	return "", fmt.Errorf("no Lua interpreter found (install lua or luajit)")
}

// Evaluate runs the configuration and returns the recording.
//
// The harness exits 0 even when the configuration errors; failures are reported
// in Report.Errors so callers can surface diagnostics rather than a transport
// error.
func Evaluate(ctx context.Context, opts Options) (*Report, error) {
	if opts.Entry == "" {
		return nil, fmt.Errorf("sandbox: empty entry path")
	}
	entry, err := filepath.Abs(opts.Entry)
	if err != nil {
		return nil, err
	}
	if _, err := os.Stat(entry); err != nil {
		return nil, fmt.Errorf("sandbox: entry not readable: %w", err)
	}
	lua, err := Interpreter(opts.LuaBin)
	if err != nil {
		return nil, err
	}

	dir, err := os.MkdirTemp("", "hyprmod-sandbox-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir)

	harnessPath := filepath.Join(dir, "harness.lua")
	if err := os.WriteFile(harnessPath, []byte(harnessLua), 0o600); err != nil {
		return nil, err
	}
	outPath := filepath.Join(dir, "report.json")

	timeout := opts.Timeout
	if timeout == 0 {
		timeout = 15 * time.Second
	}
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(runCtx, lua, harnessPath)
	cmd.Env = append(os.Environ(),
		"HYPRMOD_ENTRY="+entry,
		"HYPRMOD_OUT="+outPath,
	)
	var stderr strings.Builder
	cmd.Stderr = &stderr
	cmd.Stdout = nil

	runErr := cmd.Run()
	if runCtx.Err() == context.DeadlineExceeded {
		return nil, fmt.Errorf("sandbox: evaluation timed out after %s", timeout)
	}

	data, readErr := os.ReadFile(outPath)
	if readErr != nil {
		if runErr != nil {
			return nil, fmt.Errorf("sandbox: lua exited: %v: %s", runErr, strings.TrimSpace(stderr.String()))
		}
		return nil, fmt.Errorf("sandbox: no report produced: %s", strings.TrimSpace(stderr.String()))
	}

	var rep Report
	if err := json.Unmarshal(data, &rep); err != nil {
		return nil, fmt.Errorf("sandbox: invalid report: %w", err)
	}
	if len(rep.Errors) > 0 {
		return &rep, &HarnessError{Message: rep.Errors[0].Message}
	}
	return &rep, nil
}
