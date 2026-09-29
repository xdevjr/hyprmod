// Package hyprctl wraps the Hyprland control socket CLI. The config editor uses
// this client for live state (monitors, devices, option values), version gating,
// temporary Lua evaluation and reload/status checks.
package hyprctl

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// Client invokes a hyprctl binary. Bin defaults to "hyprctl".
type Client struct {
	Bin     string
	Timeout time.Duration
}

// New creates a client with the standard command and timeout.
func New() *Client { return &Client{Bin: "hyprctl", Timeout: 4 * time.Second} }

func (c *Client) command(ctx context.Context, args ...string) (string, error) {
	bin := c.Bin
	if bin == "" {
		bin = "hyprctl"
	}
	timeout := c.Timeout
	if timeout <= 0 {
		timeout = 4 * time.Second
	}
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.CommandContext(runCtx, bin, args...)
	out, err := cmd.CombinedOutput()
	text := strings.TrimSpace(string(out))
	if runCtx.Err() == context.DeadlineExceeded {
		return text, fmt.Errorf("hyprctl %s timed out after %s", strings.Join(args, " "), timeout)
	}
	if err != nil {
		return text, fmt.Errorf("hyprctl %s: %w: %s", strings.Join(args, " "), err, text)
	}
	return text, nil
}

// VersionInfo is the JSON output of `hyprctl version -j`.
type VersionInfo struct {
	Branch        string `json:"branch"`
	Commit        string `json:"commit"`
	Version       string `json:"version"`
	Dirty         bool   `json:"dirty"`
	CommitMessage string `json:"commit_message"`
	CommitDate    string `json:"commit_date"`
	Tag           string `json:"tag"`
	Commits       string `json:"commits"`
}

// Version queries the compositor version.
func (c *Client) Version(ctx context.Context) (VersionInfo, error) {
	text, err := c.command(ctx, "version", "-j")
	if err != nil {
		return VersionInfo{}, err
	}
	var v VersionInfo
	if err := json.Unmarshal([]byte(text), &v); err != nil {
		return VersionInfo{}, fmt.Errorf("decode hyprctl version: %w", err)
	}
	if v.Version == "" {
		return VersionInfo{}, errors.New("hyprctl version returned no version string")
	}
	return v, nil
}

// SupportsLua reports whether the version string is at least 0.55. Development
// branches without a parseable numeric version are conservatively accepted if
// they are tagged 0.55+; callers should still surface the raw version.
func (v VersionInfo) SupportsLua() bool {
	major, minor, ok := parseMajorMinor(v.Version)
	if !ok {
		major, minor, ok = parseMajorMinor(v.Tag)
	}
	return ok && (major > 0 || minor >= 55)
}

func parseMajorMinor(s string) (int, int, bool) {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "v")
	parts := strings.SplitN(s, ".", 3)
	if len(parts) < 2 {
		return 0, 0, false
	}
	major, err1 := strconv.Atoi(parts[0])
	minor, err2 := strconv.Atoi(parts[1])
	return major, minor, err1 == nil && err2 == nil
}

// Monitor is a live output as returned by `hyprctl monitors -j`.
type Monitor struct {
	ID              int      `json:"id"`
	Name            string   `json:"name"`
	Description     string   `json:"description"`
	Make            string   `json:"make"`
	Model           string   `json:"model"`
	Serial          string   `json:"serial"`
	Width           int      `json:"width"`
	Height          int      `json:"height"`
	PhysicalWidth   int      `json:"physicalWidth"`
	PhysicalHeight  int      `json:"physicalHeight"`
	RefreshRate     float64  `json:"refreshRate"`
	X               int      `json:"x"`
	Y               int      `json:"y"`
	Scale           float64  `json:"scale"`
	Transform       int      `json:"transform"`
	Focused         bool     `json:"focused"`
	VRR             bool     `json:"vrr"`
	DPMSStatus      bool     `json:"dpmsStatus"`
	Disabled        bool     `json:"disabled"`
	AvailableModes  []string `json:"availableModes"`
	ActiveWorkspace struct {
		ID   int    `json:"id"`
		Name string `json:"name"`
	} `json:"activeWorkspace"`
}

// Monitors returns the active output list.
func (c *Client) Monitors(ctx context.Context) ([]Monitor, error) {
	text, err := c.command(ctx, "monitors", "-j")
	if err != nil {
		return nil, err
	}
	var monitors []Monitor
	if err := json.Unmarshal([]byte(text), &monitors); err != nil {
		return nil, fmt.Errorf("decode monitor list: %w", err)
	}
	return monitors, nil
}

// Keyboard is one connected keyboard from `hyprctl devices -j`.
type Keyboard struct {
	Address           string `json:"address"`
	Name              string `json:"name"`
	Rules             string `json:"rules"`
	Model             string `json:"model"`
	Layout            string `json:"layout"`
	Variant           string `json:"variant"`
	Options           string `json:"options"`
	ActiveLayoutIndex int    `json:"active_layout_index"`
	ActiveKeymap      string `json:"active_keymap"`
	CapsLock          bool   `json:"capsLock"`
	NumLock           bool   `json:"numLock"`
	Main              bool   `json:"main"`
}

// PointerDevice is a mouse/trackpoint entry.
type PointerDevice struct {
	Address      string  `json:"address"`
	Name         string  `json:"name"`
	DefaultSpeed float64 `json:"defaultSpeed"`
	ScrollFactor float64 `json:"scrollFactor"`
}

// Devices is the JSON shape returned by `hyprctl devices -j`.
type Devices struct {
	Keyboards []Keyboard       `json:"keyboards"`
	Mice      []PointerDevice  `json:"mice"`
	Tablets   []map[string]any `json:"tablets"`
	Touch     []map[string]any `json:"touch"`
	Switches  []map[string]any `json:"switches"`
}

// Devices queries input devices.
func (c *Client) Devices(ctx context.Context) (Devices, error) {
	text, err := c.command(ctx, "devices", "-j")
	if err != nil {
		return Devices{}, err
	}
	var d Devices
	if err := json.Unmarshal([]byte(text), &d); err != nil {
		return Devices{}, fmt.Errorf("decode device list: %w", err)
	}
	return d, nil
}

// StatusInfo is the small subset of `hyprctl status -j` used by HyprMod.
type StatusInfo struct {
	ConfigProvider string `json:"configProvider"`
	Backend        string `json:"backend"`
}

// Status queries the compositor's current provider/backend.
func (c *Client) Status(ctx context.Context) (StatusInfo, error) {
	text, err := c.command(ctx, "status", "-j")
	if err != nil {
		return StatusInfo{}, err
	}
	var s StatusInfo
	if err := json.Unmarshal([]byte(text), &s); err != nil {
		return StatusInfo{}, fmt.Errorf("decode status: %w", err)
	}
	return s, nil
}

// OptionValue is one result from `hyprctl getoption <path> -j`.
type OptionValue struct {
	Option string         `json:"option"`
	Kind   string         `json:"kind"`
	Value  any            `json:"value"`
	Set    bool           `json:"set"`
	Raw    map[string]any `json:"raw"`
}

// GetOption queries an active config option. path may use dots or colons, e.g.
// "decoration.blur.size"; hyprctl expects colons between nested sections.
func (c *Client) GetOption(ctx context.Context, path string) (OptionValue, error) {
	hyprPath := strings.ReplaceAll(path, ".", ":")
	text, err := c.command(ctx, "getoption", hyprPath, "-j")
	if err != nil {
		return OptionValue{}, err
	}
	if strings.Contains(strings.ToLower(text), "no such option") {
		return OptionValue{}, fmt.Errorf("unknown Hyprland option %q", path)
	}
	var raw map[string]any
	if err := json.Unmarshal([]byte(text), &raw); err != nil {
		return OptionValue{}, fmt.Errorf("decode option %s: %w (%s)", path, err, text)
	}
	out, err := decodeOption(path, raw)
	if err != nil {
		return OptionValue{}, err
	}
	return out, nil
}

// GetOptions reads multiple runtime options with one synchronous IPC call. It
// uses hyprctl --batch, which is significantly lighter than launching one
// process per UI field. Unknown options in an older compositor are skipped.
func (c *Client) GetOptions(ctx context.Context, paths []string) (map[string]OptionValue, error) {
	if len(paths) == 0 {
		return map[string]OptionValue{}, nil
	}
	commands := make([]string, 0, len(paths))
	pathByHyprName := make(map[string]string, len(paths))
	for _, path := range paths {
		hyprPath := strings.ReplaceAll(path, ".", ":")
		commands = append(commands, "getoption "+hyprPath)
		pathByHyprName[hyprPath] = path
	}
	text, err := c.command(ctx, "--batch", strings.Join(commands, " ; "), "-j")
	if err != nil {
		return nil, err
	}
	out := make(map[string]OptionValue, len(paths))
	for _, chunk := range strings.Split(text, "\n\n") {
		chunk = strings.TrimSpace(chunk)
		if chunk == "" || strings.Contains(strings.ToLower(chunk), "no such option") {
			continue
		}
		var raw map[string]any
		if err := json.Unmarshal([]byte(chunk), &raw); err != nil {
			// Hyprland can include a diagnostic for an unknown option in the
			// batch response. Keep valid results instead of failing the section.
			continue
		}
		name, _ := raw["option"].(string)
		path := pathByHyprName[name]
		if path == "" {
			path = strings.ReplaceAll(name, ":", ".")
		}
		value, err := decodeOption(path, raw)
		if err != nil {
			continue
		}
		out[path] = value
	}
	return out, nil
}

func decodeOption(path string, raw map[string]any) (OptionValue, error) {
	out := OptionValue{Option: path, Set: boolValue(raw["set"]), Raw: raw}
	for _, kind := range []string{"bool", "int", "float", "str", "gradient", "css", "vec2"} {
		if v, ok := raw[kind]; ok {
			out.Kind = kind
			out.Value = v
			break
		}
	}
	if out.Kind == "" {
		return OptionValue{}, fmt.Errorf("getoption %q returned no typed value", path)
	}
	return out, nil
}

func boolValue(v any) bool {
	b, _ := v.(bool)
	return b
}

// ConfigErrors returns non-empty lines from `hyprctl configerrors -j`.
func (c *Client) ConfigErrors(ctx context.Context) ([]string, error) {
	text, err := c.command(ctx, "configerrors", "-j")
	if err != nil {
		return nil, err
	}
	var lines []string
	if err := json.Unmarshal([]byte(text), &lines); err != nil {
		return nil, fmt.Errorf("decode config errors: %w", err)
	}
	var out []string
	for _, line := range lines {
		if strings.TrimSpace(line) != "" {
			out = append(out, line)
		}
	}
	return out, nil
}

// Eval executes Lua code with the compositor's config provider. This is
// intentionally non-persistent: a config reload can overwrite the preview.
func (c *Client) Eval(ctx context.Context, code string) error {
	text, err := c.command(ctx, "eval", code)
	if err != nil {
		return err
	}
	if text != "ok" {
		return fmt.Errorf("hyprctl eval: %s", text)
	}
	return nil
}

// Reload asks Hyprland to reload the active configuration.
func (c *Client) Reload(ctx context.Context) error {
	text, err := c.command(ctx, "reload")
	if err != nil {
		return err
	}
	if text != "" && text != "ok" {
		return fmt.Errorf("hyprctl reload: %s", text)
	}
	return nil
}

// Dispatch invokes a Lua-style dispatcher expression, e.g.
// `hl.dsp.window.close()`.
func (c *Client) Dispatch(ctx context.Context, expression string) error {
	text, err := c.command(ctx, "dispatch", expression)
	if err != nil {
		return err
	}
	if text != "ok" {
		return fmt.Errorf("hyprctl dispatch: %s", text)
	}
	return nil
}
