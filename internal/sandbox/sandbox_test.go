package sandbox

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestEvaluateRealConfig evaluates the developer's live Hyprland configuration
// when present. It is skipped in environments without one.
func TestEvaluateRealConfig(t *testing.T) {
	if os.Getenv("HYPRMOD_TEST_REAL_CONFIG") != "1" {
		t.Skip("set HYPRMOD_TEST_REAL_CONFIG=1 to evaluate the user's live config")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("no home dir")
	}
	entry := filepath.Join(home, ".config", "hypr", "hyprland.lua")
	if _, err := os.Stat(entry); err != nil {
		t.Skipf("no live config at %s", entry)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	rep, err := Evaluate(ctx, Options{Entry: entry})
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	t.Logf("lua=%s root=%s", rep.Version, rep.Root)
	t.Logf("config calls=%d binds=%d monitors=%d rules=%d curves=%d anims=%d gestures=%d devices=%d envs=%d events=%d requires=%d files=%d",
		len(rep.Config), len(rep.Binds), len(rep.Monitors), len(rep.WindowRules),
		len(rep.Curves), len(rep.Animations), len(rep.Gestures), len(rep.Devices),
		len(rep.Envs), len(rep.Events), len(rep.Requires), len(rep.Files))

	if len(rep.Requires) == 0 {
		t.Errorf("expected require() resolution to find modules")
	}
	if len(rep.Binds) == 0 {
		t.Errorf("expected binds to be recorded")
	}
	if len(rep.Monitors) == 0 {
		t.Errorf("expected monitors to be recorded")
	}

	// Every config call must carry provenance.
	for _, c := range rep.Config {
		if c.File == "" || c.Line == 0 {
			t.Errorf("config call without provenance: %+v", c)
		}
		if !filepath.IsAbs(c.File) {
			t.Errorf("source path should be absolute for editing: %q", c.File)
		}
		break
	}

	for _, b := range rep.Binds[:min(3, len(rep.Binds))] {
		t.Logf("bind %s -> %s (file=%s:%d)", b.Keys, b.DSP.Name, filepath.Base(b.File), b.Line)
	}
}

func TestEvaluateFixtureResolvesModulesAndLoops(t *testing.T) {
	root := t.TempDir()
	entry := filepath.Join(root, "hyprland.lua")
	moduleDir := filepath.Join(root, "settings")
	if err := os.MkdirAll(moduleDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(entry, []byte(`spacing = 7
require("settings.core")
`), 0o600); err != nil {
		t.Fatal(err)
	}
	module := filepath.Join(moduleDir, "core.lua")
	if err := os.WriteFile(module, []byte(`hl.config({ general = { gaps_in = spacing } })
for i = 1, 3 do
  hl.bind("SUPER + " .. i, hl.dsp.focus({ workspace = i }))
end
hl.on("hyprland.start", function() end)
`), 0o600); err != nil {
		t.Fatal(err)
	}

	rep, err := Evaluate(context.Background(), Options{Entry: entry})
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if len(rep.Config) != 1 || len(rep.Binds) != 3 || len(rep.Events) != 1 {
		t.Fatalf("unexpected records: config=%d binds=%d events=%d", len(rep.Config), len(rep.Binds), len(rep.Events))
	}
	if got := rep.Config[0].Table["general"].(map[string]any)["gaps_in"]; got != float64(7) {
		t.Fatalf("resolved gaps_in=%v, want 7", got)
	}
	if rep.Binds[0].Keys != "SUPER + 1" || rep.Binds[2].Keys != "SUPER + 3" {
		t.Fatalf("loop binds were not captured: %+v", rep.Binds)
	}
	if rep.Config[0].File != module || rep.Config[0].Line != 1 {
		t.Fatalf("bad config provenance: %+v", rep.Config[0])
	}
}

func TestEvaluateReportsLuaErrors(t *testing.T) {
	root := t.TempDir()
	entry := filepath.Join(root, "hyprland.lua")
	if err := os.WriteFile(entry, []byte("hl.config({ general = { gaps_in = } })\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	rep, err := Evaluate(context.Background(), Options{Entry: entry})
	if err == nil || rep == nil || len(rep.Errors) == 0 {
		t.Fatalf("expected reported Lua syntax error, report=%#v err=%v", rep, err)
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
