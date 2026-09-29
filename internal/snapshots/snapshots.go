// Package snapshots stores versioned copies of Hyprland Lua config files and
// restores them atomically. Snapshots are deliberately file-based rather than
// directory archives: a config can source dotfiles outside ~/.config/hypr via
// symlinks, and the real target path must be preserved on restore.
package snapshots

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Manager owns a snapshot directory.
type Manager struct {
	Dir string
}

// File describes one snapshotted file.
type File struct {
	Original string `json:"original"`
	Stored   string `json:"stored"`
	Mode     uint32 `json:"mode"`
	Size     int64  `json:"size"`
}

// Snapshot is a user-facing snapshot summary.
type Snapshot struct {
	ID      string    `json:"id"`
	Created time.Time `json:"created"`
	Files   []string  `json:"files"`
}

type manifest struct {
	ID      string    `json:"id"`
	Created time.Time `json:"created"`
	Files   []File    `json:"files"`
}

// New creates a manager at dir.
func New(dir string) *Manager { return &Manager{Dir: dir} }

// Default creates a manager under $XDG_DATA_HOME/hyprmod/snapshots.
func Default() (*Manager, error) {
	base := os.Getenv("XDG_DATA_HOME")
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, err
		}
		base = filepath.Join(home, ".local", "share")
	}
	return New(filepath.Join(base, "hyprmod", "snapshots")), nil
}

// Create snapshots the given regular files and returns the new ID. Symlinks are
// resolved so future atomic writes/restores target the same file contents.
func (m *Manager) Create(paths []string) (Snapshot, error) {
	if len(paths) == 0 {
		return Snapshot{}, errors.New("cannot create an empty snapshot")
	}
	if err := os.MkdirAll(m.Dir, 0o700); err != nil {
		return Snapshot{}, fmt.Errorf("create snapshot directory: %w", err)
	}
	resolved := make([]string, 0, len(paths))
	seen := map[string]bool{}
	for _, path := range paths {
		if path == "" {
			continue
		}
		abs, err := filepath.Abs(path)
		if err != nil {
			return Snapshot{}, err
		}
		if real, err := filepath.EvalSymlinks(abs); err == nil {
			abs = real
		}
		if seen[abs] {
			continue
		}
		seen[abs] = true
		resolved = append(resolved, abs)
	}
	if len(resolved) == 0 {
		return Snapshot{}, errors.New("no files to snapshot")
	}

	created := time.Now().UTC()
	var nonce [4]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return Snapshot{}, err
	}
	id := "snapshot-" + created.Format("20060102T150405.000000000Z") + "-" + hex.EncodeToString(nonce[:])
	dir := filepath.Join(m.Dir, id)
	if err := os.Mkdir(dir, 0o700); err != nil {
		return Snapshot{}, fmt.Errorf("create snapshot: %w", err)
	}
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.RemoveAll(dir)
		}
	}()
	filesDir := filepath.Join(dir, "files")
	if err := os.Mkdir(filesDir, 0o700); err != nil {
		return Snapshot{}, err
	}

	mf := manifest{ID: id, Created: created}
	for i, original := range resolved {
		info, err := os.Stat(original)
		if err != nil {
			return Snapshot{}, fmt.Errorf("stat %s: %w", original, err)
		}
		if !info.Mode().IsRegular() {
			return Snapshot{}, fmt.Errorf("snapshot source is not a regular file: %s", original)
		}
		stored := filepath.Join("files", fmt.Sprintf("%04d", i))
		if err := copyFile(filepath.Join(dir, stored), original, info.Mode().Perm()); err != nil {
			return Snapshot{}, err
		}
		mf.Files = append(mf.Files, File{
			Original: original,
			Stored:   stored,
			Mode:     uint32(info.Mode().Perm()),
			Size:     info.Size(),
		})
	}
	data, err := json.MarshalIndent(mf, "", "  ")
	if err != nil {
		return Snapshot{}, err
	}
	if err := atomicWrite(filepath.Join(dir, "manifest.json"), data, 0o600); err != nil {
		return Snapshot{}, err
	}
	if err := syncDir(dir); err != nil {
		return Snapshot{}, err
	}
	cleanup = false
	return Snapshot{ID: id, Created: created, Files: resolved}, nil
}

// List returns snapshot metadata newest-first. Invalid manifests are skipped.
func (m *Manager) List() ([]Snapshot, error) {
	entries, err := os.ReadDir(m.Dir)
	if errors.Is(err, os.ErrNotExist) {
		return []Snapshot{}, nil
	}
	if err != nil {
		return nil, err
	}
	var out []Snapshot
	for _, ent := range entries {
		if !ent.IsDir() || !strings.HasPrefix(ent.Name(), "snapshot-") {
			continue
		}
		mf, err := m.readManifest(ent.Name())
		if err != nil {
			continue
		}
		s := Snapshot{ID: mf.ID, Created: mf.Created}
		for _, f := range mf.Files {
			s.Files = append(s.Files, f.Original)
		}
		out = append(out, s)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Created.After(out[j].Created) })
	return out, nil
}

// Restore restores one snapshot. Before doing so it snapshots current versions
// of every target file; the returned ID is that pre-restore safety snapshot.
func (m *Manager) Restore(id string) (string, error) {
	mf, err := m.readManifest(id)
	if err != nil {
		return "", err
	}
	paths := make([]string, 0, len(mf.Files))
	for _, f := range mf.Files {
		paths = append(paths, f.Original)
	}
	pre, err := m.Create(paths)
	if err != nil {
		return "", fmt.Errorf("create pre-restore snapshot: %w", err)
	}
	for _, f := range mf.Files {
		src := filepath.Join(m.Dir, id, filepath.Clean(f.Stored))
		data, err := os.ReadFile(src)
		if err != nil {
			return pre.ID, fmt.Errorf("read snapshot file %s: %w", f.Stored, err)
		}
		if !filepath.IsAbs(f.Original) || filepath.Clean(f.Original) != f.Original {
			return pre.ID, fmt.Errorf("refusing unsafe restore target %q", f.Original)
		}
		if err := atomicWrite(f.Original, data, os.FileMode(f.Mode)); err != nil {
			return pre.ID, fmt.Errorf("restore %s: %w", f.Original, err)
		}
	}
	return pre.ID, nil
}

// Delete removes one snapshot directory.
func (m *Manager) Delete(id string) error {
	if err := validateID(id); err != nil {
		return err
	}
	return os.RemoveAll(filepath.Join(m.Dir, id))
}

func (m *Manager) readManifest(id string) (manifest, error) {
	if err := validateID(id); err != nil {
		return manifest{}, err
	}
	data, err := os.ReadFile(filepath.Join(m.Dir, id, "manifest.json"))
	if err != nil {
		return manifest{}, err
	}
	var mf manifest
	if err := json.Unmarshal(data, &mf); err != nil {
		return manifest{}, fmt.Errorf("decode snapshot manifest: %w", err)
	}
	if mf.ID != id {
		return manifest{}, errors.New("snapshot manifest ID mismatch")
	}
	for _, f := range mf.Files {
		if filepath.IsAbs(f.Stored) || strings.HasPrefix(filepath.Clean(f.Stored), "..") {
			return manifest{}, fmt.Errorf("unsafe stored path %q", f.Stored)
		}
	}
	return mf, nil
}

func validateID(id string) error {
	if id == "" || filepath.Base(id) != id || strings.ContainsAny(id, `/\\`) || !strings.HasPrefix(id, "snapshot-") {
		return fmt.Errorf("invalid snapshot ID %q", id)
	}
	return nil
}

func copyFile(dst, src string, mode os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	if err := os.MkdirAll(filepath.Dir(dst), 0o700); err != nil {
		return err
	}
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_EXCL|os.O_WRONLY, mode)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	if err := out.Sync(); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

func atomicWrite(path string, data []byte, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".hyprmod-snapshot-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if err := tmp.Chmod(mode); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpName, path); err != nil {
		return err
	}
	return syncDir(filepath.Dir(path))
}

func syncDir(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return f.Sync()
}
