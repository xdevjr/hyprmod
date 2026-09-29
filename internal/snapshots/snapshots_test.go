package snapshots

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCreateListAndRestore(t *testing.T) {
	dir := t.TempDir()
	config := filepath.Join(dir, "hyprland.lua")
	if err := os.WriteFile(config, []byte("original\n"), 0o640); err != nil {
		t.Fatal(err)
	}
	m := New(filepath.Join(dir, "data", "snapshots"))

	snap, err := m.Create([]string{config, config})
	if err != nil {
		t.Fatal(err)
	}
	if len(snap.Files) != 1 {
		t.Fatalf("snapshot should dedupe paths: %v", snap.Files)
	}
	if err := os.WriteFile(config, []byte("changed\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	preRestoreID, err := m.Restore(snap.ID)
	if err != nil {
		t.Fatal(err)
	}
	if preRestoreID == "" || preRestoreID == snap.ID {
		t.Fatalf("expected separate pre-restore safety snapshot, got %q", preRestoreID)
	}
	data, err := os.ReadFile(config)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "original\n" {
		t.Fatalf("restored content=%q", data)
	}
	info, err := os.Stat(config)
	if err != nil || info.Mode().Perm() != 0o640 {
		t.Fatalf("restored mode=%v err=%v", info.Mode().Perm(), err)
	}

	list, err := m.List()
	if err != nil || len(list) != 2 {
		t.Fatalf("List()=%+v err=%v", list, err)
	}
}

func TestSnapshotResolvesSymlinkTarget(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "dotfiles", "hyprland.lua")
	if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte("target\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "config", "hyprland.lua")
	if err := os.MkdirAll(filepath.Dir(link), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	m := New(filepath.Join(dir, "snapshots"))
	snap, err := m.Create([]string{link})
	if err != nil {
		t.Fatal(err)
	}
	if snap.Files[0] != target {
		t.Fatalf("snapshot did not resolve symlink: %v", snap.Files)
	}
	if err := os.WriteFile(target, []byte("changed\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Restore(snap.ID); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(target)
	if string(data) != "target\n" {
		t.Fatalf("symlink target content=%q", data)
	}
	if _, err := os.Lstat(link); err != nil {
		t.Fatalf("restore replaced or removed symlink: %v", err)
	}
}
