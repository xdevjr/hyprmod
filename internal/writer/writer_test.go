package writer

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/juniorgs/hyprmod/internal/sandbox"
	"github.com/juniorgs/hyprmod/internal/schema"
	"github.com/juniorgs/hyprmod/internal/snapshots"
)

func TestSetConfigValueValidatesSnapshotsAndWrites(t *testing.T) {
	root, entry, source := configFixture(t, "hl.config({ general = { gaps_in = 5 } })\n")
	m := snapshots.New(filepath.Join(t.TempDir(), "snapshots"))
	w := New(m)
	w.Timeout = 3 * time.Second

	result, inPlace, err := w.SetConfigValue(context.Background(), entry, source, "general.gaps_in", "9")
	if err != nil {
		t.Fatalf("SetConfigValue: %v", err)
	}
	if !inPlace {
		t.Fatal("literal should be edited in place")
	}
	data, err := os.ReadFile(source)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "gaps_in = 9") {
		t.Fatalf("file not updated: %s", data)
	}
	if result.SnapshotID == "" || result.Report == nil || len(result.Report.Config) != 1 {
		t.Fatalf("incomplete result: %+v", result)
	}
	if got := result.Report.Config[0].Table["general"].(map[string]any)["gaps_in"]; got != float64(9) {
		t.Fatalf("effective option=%v, want 9", got)
	}
	if result.Report.Config[0].File != source {
		t.Fatalf("report provenance=%q, want original file %q", result.Report.Config[0].File, source)
	}
	list, err := m.List()
	if err != nil || len(list) != 1 || len(list[0].Files) != 1 {
		t.Fatalf("snapshot list=%+v err=%v", list, err)
	}
	if filepath.Dir(entry) != root {
		t.Fatalf("fixture root mismatch: %s %s", root, entry)
	}
}

func TestInvalidCandidateDoesNotWriteOrSnapshot(t *testing.T) {
	_, entry, source := configFixture(t, "hl.config({ general = { gaps_in = 5 } })\n")
	m := snapshots.New(filepath.Join(t.TempDir(), "snapshots"))
	w := New(m)
	if _, _, err := w.SetConfigValue(context.Background(), entry, source, "general.gaps_in", "("); err == nil {
		t.Fatal("expected candidate validation failure")
	}
	data, err := os.ReadFile(source)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "gaps_in = 5") {
		t.Fatalf("invalid candidate changed source: %s", data)
	}
	list, err := m.List()
	if err != nil || len(list) != 0 {
		t.Fatalf("invalid candidate created snapshots: %+v err=%v", list, err)
	}
}

func TestRawEditRejectsSchemaTypeMismatch(t *testing.T) {
	_, entry, source := configFixture(t, "hl.config({ general = { gaps_in = 5 } })\n")
	original, _ := os.ReadFile(source)
	updated := strings.Replace(string(original), "gaps_in = 5", `gaps_in = "not-an-integer"`, 1)
	m := snapshots.New(filepath.Join(t.TempDir(), "snapshots"))
	w := New(m)
	_, err := w.Apply(context.Background(), entry, []FileChange{{
		Path: source, Content: []byte(updated), ExpectedSHA256: digest(original),
	}})
	if err == nil || !strings.Contains(err.Error(), "expects css_gaps") {
		t.Fatalf("expected schema type error, got %v", err)
	}
	current, _ := os.ReadFile(source)
	if string(current) != string(original) {
		t.Fatalf("invalid type changed source: %s", current)
	}
	list, _ := m.List()
	if len(list) != 0 {
		t.Fatalf("invalid type created a snapshot: %+v", list)
	}
}

func TestLiveConfigMatchesInstalledSchema(t *testing.T) {
	if os.Getenv("HYPRMOD_TEST_REAL_CONFIG") != "1" {
		t.Skip("set HYPRMOD_TEST_REAL_CONFIG=1 to validate the user's live config types")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("no home dir")
	}
	entry := filepath.Join(home, ".config", "hypr", "hyprland.lua")
	report, err := sandbox.Evaluate(context.Background(), sandbox.Options{Entry: entry})
	if err != nil {
		t.Fatal(err)
	}
	if err := validateOptions(report, schema.Load()); err != nil {
		t.Fatal(err)
	}
}

func TestSetDynamicOptionUsesOverride(t *testing.T) {
	_, entry, source := configFixture(t, "spacing = 5\nhl.config({ general = { gaps_in = spacing } })\nreturn { spacing = spacing }\n")
	w := New(snapshots.New(filepath.Join(t.TempDir(), "snapshots")))
	result, inPlace, err := w.SetConfigValue(context.Background(), entry, source, "general.gaps_in", "11")
	if err != nil {
		t.Fatal(err)
	}
	if inPlace {
		t.Fatal("dynamic expression should be overridden")
	}
	data, _ := os.ReadFile(source)
	if !strings.Contains(string(data), "-- hyprmod:override general.gaps_in") {
		t.Fatalf("override marker missing: %s", data)
	}
	if strings.Index(string(data), "-- hyprmod:override general.gaps_in") > strings.Index(string(data), "return {") {
		t.Fatalf("override was appended after module return: %s", data)
	}
	if result.Report.Config[1].Table["general"].(map[string]any)["gaps_in"] != float64(11) {
		t.Fatalf("override did not take effect: %+v", result.Report.Config)
	}
}

func TestRejectsExternalConcurrentEdit(t *testing.T) {
	_, entry, source := configFixture(t, "hl.config({ general = { gaps_in = 5 } })\n")
	original, _ := os.ReadFile(source)
	hash := sha256.Sum256(original)
	updated := strings.Replace(string(original), "gaps_in = 5", "gaps_in = 8", 1)
	if err := os.WriteFile(source, []byte("hl.config({ general = { gaps_in = 6 } })\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	w := New(snapshots.New(filepath.Join(t.TempDir(), "snapshots")))
	_, err := w.Apply(context.Background(), entry, []FileChange{{
		Path: source, Content: []byte(updated), ExpectedSHA256: hex.EncodeToString(hash[:]),
	}})
	if err == nil || !strings.Contains(err.Error(), "changed since it was loaded") {
		t.Fatalf("expected concurrent-edit error, got %v", err)
	}
	data, _ := os.ReadFile(source)
	if !strings.Contains(string(data), "gaps_in = 6") {
		t.Fatalf("concurrent edit was overwritten: %s", data)
	}
}

func TestWritesThroughConfigSymlink(t *testing.T) {
	base := t.TempDir()
	root := filepath.Join(base, "config")
	if err := os.MkdirAll(root, 0o700); err != nil {
		t.Fatal(err)
	}
	entry := filepath.Join(root, "hyprland.lua")
	if err := os.WriteFile(entry, []byte("require(\"settings\")\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	dotfiles := filepath.Join(base, "dotfiles", "settings.lua")
	if err := os.MkdirAll(filepath.Dir(dotfiles), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dotfiles, []byte("hl.config({ general = { gaps_in = 5 } })\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "settings.lua")
	if err := os.Symlink(dotfiles, link); err != nil {
		t.Fatal(err)
	}
	w := New(snapshots.New(filepath.Join(base, "snapshots")))
	if _, _, err := w.SetConfigValue(context.Background(), entry, link, "general.gaps_in", "10"); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(dotfiles)
	if !strings.Contains(string(data), "gaps_in = 10") {
		t.Fatalf("symlink target not updated: %s", data)
	}
	if _, err := os.Lstat(link); err != nil {
		t.Fatalf("symlink was replaced: %v", err)
	}
}

func configFixture(t *testing.T, sourceText string) (root, entry, source string) {
	t.Helper()
	root = t.TempDir()
	entry = filepath.Join(root, "hyprland.lua")
	source = filepath.Join(root, "settings.lua")
	if err := os.WriteFile(entry, []byte("require(\"settings\")\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(source, []byte(sourceText), 0o600); err != nil {
		t.Fatal(err)
	}
	return root, entry, source
}
