package api

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/juniorgs/hyprmod/internal/config"
	"github.com/juniorgs/hyprmod/internal/snapshots"
	"github.com/juniorgs/hyprmod/internal/writer"
)

func TestLoadDocumentResolvesEffectiveValuesAndProvenance(t *testing.T) {
	s, entry, source := serviceFixture(t)
	if err := os.WriteFile(source, []byte(`spacing = 6
hl.config({
  general = {
    gaps_in = spacing,
    border_size = 2,
  },
})
hl.config({ general = { gaps_out = 9 } })
`), 0o600); err != nil {
		t.Fatal(err)
	}
	doc, err := s.LoadDocument(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	values := make(map[string]struct {
		resolved any
		dynamic  bool
		file     string
		line     int
	})
	for _, v := range doc.Values {
		values[v.Path] = struct {
			resolved any
			dynamic  bool
			file     string
			line     int
		}{v.Resolved, v.Dynamic, v.File, v.Line}
	}
	if got := values["general.gaps_in"]; got.resolved != float64(6) || !got.dynamic || got.file != source || got.line != 4 {
		t.Fatalf("dynamic value provenance incorrect: %+v", got)
	}
	if got := values["general.border_size"]; got.resolved != float64(2) || got.dynamic || got.file != source || got.line != 5 {
		t.Fatalf("literal value provenance incorrect: %+v", got)
	}
	if got := values["general.gaps_out"]; got.resolved != float64(9) || got.dynamic || got.file != source {
		t.Fatalf("later config call did not override value: %+v", got)
	}
	if doc.Entry != entry || doc.Root != filepath.Dir(entry) || len(doc.Files) != 2 {
		t.Fatalf("document metadata mismatch: %+v", doc)
	}
}

func TestSetOptionUsesDynamicOverride(t *testing.T) {
	s, _, source := serviceFixture(t)
	if err := os.WriteFile(source, []byte("spacing = 6\nhl.config({ general = { gaps_in = spacing } })\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	s.Writer = writer.New(snapshots.New(filepath.Join(t.TempDir(), "snapshots")))
	result, err := s.SetOption(context.Background(), "general.gaps_in", float64(11))
	if err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(source)
	if !strings.Contains(string(data), "-- hyprmod:override general.gaps_in") || !strings.Contains(string(data), "gaps_in = 11") {
		t.Fatalf("dynamic override not written:\n%s", data)
	}
	if result.Report == nil || len(result.Report.Config) != 2 {
		t.Fatalf("updated effective config missing: %+v", result)
	}
}

func TestAddAndRemoveBindViaRecordedDispatcher(t *testing.T) {
	s, entry, _ := serviceFixture(t)
	s.Writer = writer.New(snapshots.New(filepath.Join(t.TempDir(), "snapshots")))

	added, err := s.AddBind(context.Background(), "SUPER + X", "window.close", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if added.Report == nil || len(added.Report.Binds) != 1 || added.Report.Binds[0].DSP.Name != "window.close" {
		t.Fatalf("added bind not captured: %+v", added.Report)
	}

	removed, err := s.RemoveBind(context.Background(), "SUPER + X")
	if err != nil {
		t.Fatal(err)
	}
	if removed.Report == nil || len(removed.Report.Binds) != 1 || removed.Report.Binds[0].Enabled == nil || *removed.Report.Binds[0].Enabled {
		t.Fatalf("unbind did not disable the recorded bind: %+v", removed.Report)
	}
	data, _ := os.ReadFile(entry)
	if !strings.Contains(string(data), "hl.unbind(\"SUPER + X\")") {
		t.Fatalf("unbind call missing from config: %s", data)
	}
}

func TestSetBindEditsExistingDeclarationInPlace(t *testing.T) {
	s, _, source := serviceFixture(t)
	if err := os.WriteFile(source, []byte(`-- keep me
hl.bind("SUPER + Q", hl.dsp.exec_cmd("kitty"))
hl.bind("SUPER + C", hl.dsp.window.close())
`), 0o600); err != nil {
		t.Fatal(err)
	}
	s.Writer = writer.New(snapshots.New(filepath.Join(t.TempDir(), "snapshots")))
	result, err := s.SetBind(context.Background(), "SUPER + Q", `hl.bind("SUPER + Q", hl.dsp.exec_cmd("alacritty"), { locked = true })`)
	if err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(source)
	if strings.Count(string(data), "-- hyprmod:") != 0 {
		t.Fatalf("existing bind should be edited in place, not appended:\n%s", data)
	}
	if !strings.Contains(string(data), "-- keep me") || !strings.Contains(string(data), "alacritty") {
		t.Fatalf("edit did not preserve the file:\n%s", data)
	}
	if !strings.Contains(string(data), "hl.dsp.window.close()") {
		t.Fatalf("sibling bind was damaged:\n%s", data)
	}
	if result.Report == nil || len(result.Report.Binds) != 2 {
		t.Fatalf("validation report missing: %+v", result)
	}
	if result.Report.Binds[0].DSP.Name != "exec_cmd" || result.Report.Binds[0].DSP.Args[0] != "alacritty" {
		t.Fatalf("effective bind not updated: %+v", result.Report.Binds[0])
	}
}

func TestSetBindAppendsWhenMissing(t *testing.T) {
	s, entry, _ := serviceFixture(t)
	s.Writer = writer.New(snapshots.New(filepath.Join(t.TempDir(), "snapshots")))
	if _, err := s.SetBind(context.Background(), "SUPER + J", `hl.bind("SUPER + J", hl.dsp.workspace.toggle_special())`); err != nil {
		t.Fatal(err)
	}
	// A brand new bind belongs in the entry module, which is the last one
	// evaluated and therefore able to override anything.
	data, _ := os.ReadFile(entry)
	if !strings.Contains(string(data), "-- hyprmod:add bind ") || !strings.Contains(string(data), "toggle_special") {
		t.Fatalf("new bind was not appended to the entry module:\n%s", data)
	}
}

func TestSetBindRejectsNonBindCode(t *testing.T) {
	s, _, _ := serviceFixture(t)
	s.Writer = writer.New(snapshots.New(filepath.Join(t.TempDir(), "snapshots")))
	if _, err := s.SetBind(context.Background(), "SUPER + J", "os.execute('rm -rf /')"); err == nil {
		t.Fatal("expected rejection of non-bind code")
	}
}

func TestRawSourceSaveUsesOptimisticLocking(t *testing.T) {
	s, _, source := serviceFixture(t)
	loaded, err := s.ReadSource(context.Background(), source)
	if err != nil {
		t.Fatal(err)
	}
	updated := strings.Replace(loaded.Content, "gaps_in = 5", "gaps_in = 8", 1)
	if _, err := s.WriteSource(context.Background(), source, updated, loaded.Digest); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(source, []byte("hl.config({ general = { gaps_in = 11 } })\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := s.WriteSource(context.Background(), source, loaded.Content, loaded.Digest); err == nil || !strings.Contains(err.Error(), "changed since it was loaded") {
		t.Fatalf("expected stale-source error, got %v", err)
	}
}

func TestSetMonitorFieldsUsesOneDynamicOverride(t *testing.T) {
	s, _, source := serviceFixture(t)
	if err := os.WriteFile(source, []byte("outputName = \"eDP-1\"\nmodeName = \"preferred\"\nhl.monitor({ output = outputName, mode = modeName, scale = 1 })\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	s.Writer = writer.New(snapshots.New(filepath.Join(t.TempDir(), "snapshots")))
	result, err := s.SetMonitorFields(context.Background(), "eDP-1", map[string]any{
		"mode": "1920x1200@165.00Hz", "position": "0x0", "scale": float64(1.25), "vrr": 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(source)
	if strings.Count(string(data), "-- hyprmod:add monitor") != 1 {
		t.Fatalf("expected one combined monitor override:\n%s", data)
	}
	if result.Report == nil || len(result.Report.Monitors) != 2 {
		t.Fatalf("monitor calls missing from validation report: %+v", result)
	}
	last := result.Report.Monitors[1].Spec
	if last["mode"] != "1920x1200@165.00Hz" || last["position"] != "0x0" || last["scale"] != float64(1.25) || last["vrr"] != float64(1) {
		t.Fatalf("combined spec incomplete: %+v", last)
	}
}

func TestLuaLiteralSerialization(t *testing.T) {
	tests := []struct {
		value any
		want  string
	}{
		{true, "true"},
		{float64(0.95), "0.95"},
		{`a"b\\c\n`, `"a\"b\\\\c\\n"`},
		{[]any{float64(1), float64(2)}, "{ 1, 2 }"},
		{map[string]any{"angle": float64(45), "colors": []any{"red", "blue"}}, `{ angle = 45, colors = { "red", "blue" } }`},
	}
	for _, tc := range tests {
		got, err := toLua(tc.value)
		if err != nil || got != tc.want {
			t.Errorf("toLua(%#v)=%q err=%v, want %q", tc.value, got, err, tc.want)
		}
	}
}

func TestValidateValue(t *testing.T) {
	if err := validateValue("int", float64(5)); err != nil {
		t.Fatal(err)
	}
	if err := validateValue("int", float64(5.5)); err == nil {
		t.Fatal("expected non-integer rejection")
	}
	if err := validateValue("bool", "true"); err == nil {
		t.Fatal("expected bool type rejection")
	}
}

func TestLiveHyprlandSession(t *testing.T) {
	if os.Getenv("HYPRMOD_TEST_LIVE_SESSION") != "1" {
		t.Skip("set HYPRMOD_TEST_LIVE_SESSION=1 to query the active Hyprland session")
	}
	s := NewService()
	dashboard := s.GetDashboard(context.Background())
	if !dashboard.Online || !dashboard.Supported {
		t.Fatalf("live session not supported: %+v", dashboard)
	}
	doc, err := s.LoadDocument(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(doc.Values) == 0 || len(doc.Binds) == 0 || len(doc.Monitors) == 0 {
		t.Fatalf("live config records unexpectedly empty: values=%d binds=%d monitors=%d", len(doc.Values), len(doc.Binds), len(doc.Monitors))
	}
	section, err := s.GetSection(context.Background(), "general")
	if err != nil {
		t.Fatal(err)
	}
	if !section.RuntimeAvailable || len(section.Values) == 0 {
		t.Fatalf("runtime option section unavailable: %+v", section)
	}
	monitors, err := s.GetMonitors(context.Background())
	if err != nil || len(monitors.Active) == 0 {
		t.Fatalf("live monitors=%+v err=%v", monitors, err)
	}
}

// TestRealConfigCopyWriteCycle copies the developer's live configuration into a
// temporary directory and drives a full read/edit/validate cycle against it.
// The real configuration is never modified.
func TestRealConfigCopyWriteCycle(t *testing.T) {
	if os.Getenv("HYPRMOD_TEST_REAL_CONFIG") != "1" {
		t.Skip("set HYPRMOD_TEST_REAL_CONFIG=1 to exercise a copy of the user's live config")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("no home dir")
	}
	source := filepath.Join(home, ".config", "hypr")
	entry := filepath.Join(source, "hyprland.lua")
	if _, err := os.Stat(entry); err != nil {
		t.Skipf("no live config at %s", entry)
	}
	root := t.TempDir()
	if err := copyTree(source, root); err != nil {
		t.Fatalf("copy config: %v", err)
	}
	copiedEntry := filepath.Join(root, "hyprland.lua")

	s := NewService()
	if err := s.SelectConfigPath(copiedEntry); err != nil {
		t.Fatal(err)
	}
	s.Writer = writer.New(snapshots.New(filepath.Join(root, ".snapshots")))

	doc, err := s.LoadDocument(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(doc.Values) == 0 {
		t.Fatal("copied config produced no values")
	}

	// Pick a configured int option and flip it, preferring a dynamic value so
	// the override path is exercised.
	target := ""
	for _, v := range doc.Values {
		if v.Kind == "int" && v.Section != "misc" {
			target = v.Path
			if v.Dynamic {
				break
			}
		}
	}
	if target == "" {
		t.Skip("no integer option available in the live config")
	}
	before := valueFor(t, doc, target)
	after := 42.0
	if before == after {
		after = 43
	}
	var editedFile string
	var dynamic bool
	for _, v := range doc.Values {
		if v.Path == target {
			editedFile, dynamic = v.File, v.Dynamic
		}
	}
	t.Logf("editing %s in %s (dynamic=%v) from %v to %v", target, filepath.Base(editedFile), dynamic, before, after)
	result, err := s.SetOption(context.Background(), target, after)
	if err != nil {
		t.Fatalf("SetOption(%s): %v", target, err)
	}
	t.Logf("snapshot=%s changed=%v", result.SnapshotID, result.Changed)
	if edited, err := os.ReadFile(editedFile); err == nil {
		key := strings.Split(target, ".")[1:]
		needle := key[len(key)-1]
		lines := strings.Split(string(edited), "\n")
		for i, line := range lines {
			if strings.Contains(line, needle) || strings.Contains(line, "-- hyprmod:") {
				t.Logf("%s:%d: %s", filepath.Base(editedFile), i+1, strings.TrimRight(line, " "))
			}
		}
	}

	updated, err := s.LoadDocument(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got := valueFor(t, updated, target); got != after {
		t.Fatalf("effective %s = %v, want %v", target, got, after)
	}
	if target2 := valueFor(t, updated, "general.gaps_in"); target2 == nil {
		t.Fatal("unrelated option disappeared after the edit")
	}
}

func valueFor(t *testing.T, doc config.Document, path string) any {
	t.Helper()
	for _, v := range doc.Values {
		if v.Path == path {
			return v.Resolved
		}
	}
	t.Fatalf("option %s not found in document", path)
	return nil
}

func copyTree(src, dst string) error {
	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dst, 0o700); err != nil {
		return err
	}
	for _, entry := range entries {
		srcPath := filepath.Join(src, entry.Name())
		dstPath := filepath.Join(dst, entry.Name())
		info, err := entry.Info()
		if err != nil {
			return err
		}
		switch {
		case info.Mode()&os.ModeSymlink != 0:
			target, err := os.Readlink(srcPath)
			if err != nil {
				return err
			}
			_ = os.Remove(dstPath)
			if err := os.Symlink(target, dstPath); err != nil {
				return err
			}
		case entry.IsDir():
			if err := copyTree(srcPath, dstPath); err != nil {
				return err
			}
		case strings.HasSuffix(entry.Name(), ".lua"):
			data, err := os.ReadFile(srcPath)
			if err != nil {
				return err
			}
			if err := os.WriteFile(dstPath, data, 0o600); err != nil {
				return err
			}
		}
	}
	return nil
}

func TestLiveBindsExposeCallbacksAndDynamicKeys(t *testing.T) {
	if os.Getenv("HYPRMOD_TEST_REAL_CONFIG") != "1" {
		t.Skip("set HYPRMOD_TEST_REAL_CONFIG=1 to inspect the live binds")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("no home dir")
	}
	entry := filepath.Join(home, ".config", "hypr", "hyprland.lua")
	if _, err := os.Stat(entry); err != nil {
		t.Skipf("no live config at %s", entry)
	}
	s := NewService()
	doc, err := s.LoadDocument(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	var callbacks, dynamic, keycodes int
	for _, bind := range doc.Binds {
		if bind.Dispatcher.Callback != "" {
			callbacks++
			// Either an inline `function ... end` body or a reference to a
			// named function defined elsewhere in the config.
			inline := strings.HasPrefix(bind.Dispatcher.Callback, "function")
			named := regexp.MustCompile(`^[\w.]+$`).MatchString(bind.Dispatcher.Callback)
			if !inline && !named {
				t.Errorf("unexpected callback source: %q", bind.Dispatcher.Callback)
			}
		}
		if bind.KeysDynamic {
			dynamic++
		}
		if strings.Contains(bind.Keys, "code:") {
			keycodes++
		}
	}
	t.Logf("binds=%d callbacks=%d dynamicKeys=%d keycodeBinds=%d", len(doc.Binds), callbacks, dynamic, keycodes)
	if callbacks == 0 {
		t.Error("expected at least one inline function callback to be recovered")
	}
	if dynamic == 0 {
		t.Error("expected at least one bind with expression-built keys")
	}
}

func serviceFixture(t *testing.T) (*Service, string, string) {
	t.Helper()
	root := t.TempDir()
	entry := filepath.Join(root, "hyprland.lua")
	source := filepath.Join(root, "settings.lua")
	if err := os.WriteFile(entry, []byte("require(\"settings\")\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(source, []byte("hl.config({ general = { gaps_in = 5 } })\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	s := NewService()
	s.configPath = entry
	return s, entry, source
}
