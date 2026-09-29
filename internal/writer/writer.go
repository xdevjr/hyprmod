// Package writer applies source edits through a validate-before-write pipeline:
// construct a temporary copy of the loaded config module closure, run it through
// the Lua sandbox, snapshot original files, then atomically replace the targets.
package writer

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/juniorgs/hyprmod/internal/config"
	"github.com/juniorgs/hyprmod/internal/luasrc"
	"github.com/juniorgs/hyprmod/internal/sandbox"
	"github.com/juniorgs/hyprmod/internal/schema"
	"github.com/juniorgs/hyprmod/internal/snapshots"
)

// FileChange describes the desired content of one Lua file. Path should be the
// logical path seen by `require` (usually under ~/.config/hypr), not a resolved
// symlink target. ExpectedSHA256 can be set to reject edits if another process
// changed the file since the UI loaded it.
type FileChange struct {
	Path           string
	Content        []byte
	ExpectedSHA256 string
}

// Result summarizes a successful write.
type Result struct {
	SnapshotID string          `json:"snapshotId"`
	Changed    []string        `json:"changed"`
	Report     *sandbox.Report `json:"report,omitempty"`
}

// Writer owns the config mutation pipeline.
type Writer struct {
	Snapshots *snapshots.Manager
	Schema    *schema.Schema
	LuaBin    string
	Timeout   time.Duration
	mu        sync.Mutex
}

// New creates a writer. If snapshots is nil, the XDG default is used on first
// write.
func New(snapshotsManager *snapshots.Manager) *Writer {
	return &Writer{Snapshots: snapshotsManager, Timeout: 15 * time.Second}
}

// SetConfigValue surgically edits a config option in file. Dynamic/missing
// values are handled by luasrc as a marked override block at EOF.
func (w *Writer) SetConfigValue(ctx context.Context, entry, file, path, literal string) (Result, bool, error) {
	data, err := os.ReadFile(file)
	if err != nil {
		return Result{}, false, err
	}
	doc := luasrc.NewFile(string(data))
	edits, inPlace := doc.SetConfigValue(path, literal)
	updated := luasrc.Apply(string(data), edits...)
	res, err := w.Apply(ctx, entry, []FileChange{{
		Path:           file,
		Content:        []byte(updated),
		ExpectedSHA256: digest(data),
	}})
	return res, inPlace, err
}

// Apply validates and writes a set of file changes. entry is the root Lua file
// whose require graph must continue to evaluate successfully.
func (w *Writer) Apply(ctx context.Context, entry string, changes []FileChange) (Result, error) {
	if len(changes) == 0 {
		return Result{}, errors.New("no file changes")
	}
	w.mu.Lock()
	defer w.mu.Unlock()

	entryAbs, err := filepath.Abs(entry)
	if err != nil {
		return Result{}, err
	}
	root := filepath.Dir(entryAbs)
	normalized, err := normalizeChanges(root, changes)
	if err != nil {
		return Result{}, err
	}

	// Baseline evaluation discovers the module closure. Existing config errors
	// are allowed here: the candidate must resolve them before a write commits.
	baseline, baselineErr := sandbox.Evaluate(ctx, sandbox.Options{
		Entry:   entryAbs,
		LuaBin:  w.LuaBin,
		Timeout: w.timeout(),
	})
	if baseline == nil {
		return Result{}, fmt.Errorf("inspect current config: %w", baselineErr)
	}

	stageDir, err := os.MkdirTemp("", "hyprmod-validate-")
	if err != nil {
		return Result{}, err
	}
	defer os.RemoveAll(stageDir)
	stageRoot := filepath.Join(stageDir, "config")
	if err := os.MkdirAll(stageRoot, 0o700); err != nil {
		return Result{}, err
	}
	if err := stageLoadedFiles(root, stageRoot, entryAbs, baseline.Files); err != nil {
		return Result{}, fmt.Errorf("stage config closure: %w", err)
	}
	for _, ch := range normalized {
		stagePath, err := stagedPath(root, stageRoot, ch.logical)
		if err != nil {
			return Result{}, err
		}
		if err := writeFile(stagePath, ch.content, ch.mode); err != nil {
			return Result{}, fmt.Errorf("stage %s: %w", ch.logical, err)
		}
	}
	stageEntry, err := stagedPath(root, stageRoot, entryAbs)
	if err != nil {
		return Result{}, err
	}
	report, validateErr := sandbox.Evaluate(ctx, sandbox.Options{
		Entry:   stageEntry,
		LuaBin:  w.LuaBin,
		Timeout: w.timeout(),
	})
	if validateErr != nil {
		return Result{}, fmt.Errorf("candidate config is invalid: %w", validateErr)
	}
	if report == nil {
		return Result{}, errors.New("candidate config produced no validation report")
	}
	optionSchema := w.Schema
	if optionSchema == nil {
		optionSchema = schema.Load()
	}
	if err := validateOptions(report, optionSchema); err != nil {
		return Result{}, fmt.Errorf("candidate config has invalid option types: %w", err)
	}

	manager, err := w.snapshotManager()
	if err != nil {
		return Result{}, err
	}
	originalPaths := make([]string, 0, len(normalized))
	originalData := make(map[string][]byte, len(normalized))
	for _, ch := range normalized {
		current, err := os.ReadFile(ch.canonical)
		if err != nil {
			return Result{}, fmt.Errorf("read original %s: %w", ch.canonical, err)
		}
		if ch.expected != "" && digest(current) != ch.expected {
			return Result{}, fmt.Errorf("%s changed since it was loaded; refusing to overwrite external edits", ch.logical)
		}
		originalPaths = append(originalPaths, ch.canonical)
		originalData[ch.canonical] = current
	}
	snap, err := manager.Create(originalPaths)
	if err != nil {
		return Result{}, fmt.Errorf("snapshot before write: %w", err)
	}

	written := make([]string, 0, len(normalized))
	for _, ch := range normalized {
		written = append(written, ch.canonical)
		if err := writeFile(ch.canonical, ch.content, ch.mode); err != nil {
			rollbackErr := rollback(originalData, written)
			if rollbackErr != nil {
				return Result{SnapshotID: snap.ID}, fmt.Errorf("write %s failed: %v (rollback also failed: %w; recovery snapshot: %s)", ch.canonical, err, rollbackErr, snap.ID)
			}
			return Result{SnapshotID: snap.ID}, fmt.Errorf("write %s failed; previous files restored: %w", ch.canonical, err)
		}
	}
	actualReport, actualErr := sandbox.Evaluate(ctx, sandbox.Options{
		Entry:   entryAbs,
		LuaBin:  w.LuaBin,
		Timeout: w.timeout(),
	})
	if actualErr != nil {
		rollbackErr := rollback(originalData, written)
		if rollbackErr != nil {
			return Result{SnapshotID: snap.ID}, fmt.Errorf("post-write validation failed: %v (rollback also failed: %w; recovery snapshot: %s)", actualErr, rollbackErr, snap.ID)
		}
		return Result{SnapshotID: snap.ID}, fmt.Errorf("post-write validation failed; previous files restored: %w", actualErr)
	}

	changed := make([]string, 0, len(normalized))
	for _, ch := range normalized {
		changed = append(changed, ch.logical)
	}
	return Result{SnapshotID: snap.ID, Changed: changed, Report: actualReport}, nil
}

// Validate evaluates entry without writing. A Lua harness error is returned
// alongside the report so callers can display detailed diagnostics.
func (w *Writer) Validate(ctx context.Context, entry string) (*sandbox.Report, error) {
	return sandbox.Evaluate(ctx, sandbox.Options{Entry: entry, LuaBin: w.LuaBin, Timeout: w.timeout()})
}

// PreviewOption applies a temporary value through hyprctl eval. The change is
// not persisted and may be overwritten by a config reload.
func (w *Writer) PreviewOption(ctx context.Context, hyprEval func(context.Context, string) error, path, literal string) error {
	parts := strings.Split(path, ".")
	if len(parts) < 2 {
		return fmt.Errorf("invalid option path %q", path)
	}
	// The Lua table path is emitted recursively, so nested options retain their
	// exact section hierarchy.
	var b strings.Builder
	b.WriteString("hl.config({ ")
	for i, part := range parts {
		b.WriteString(part)
		if i == len(parts)-1 {
			b.WriteString(" = " + literal)
		} else {
			b.WriteString(" = { ")
		}
	}
	for range len(parts) - 1 {
		b.WriteString(" }")
	}
	b.WriteString(" })")
	return hyprEval(ctx, b.String())
}

func (w *Writer) timeout() time.Duration {
	if w.Timeout <= 0 {
		return 15 * time.Second
	}
	return w.Timeout
}

func (w *Writer) snapshotManager() (*snapshots.Manager, error) {
	if w.Snapshots != nil {
		return w.Snapshots, nil
	}
	return snapshots.Default()
}

type normalizedChange struct {
	logical   string
	canonical string
	content   []byte
	mode      os.FileMode
	expected  string
}

func normalizeChanges(root string, changes []FileChange) ([]normalizedChange, error) {
	seen := map[string]bool{}
	out := make([]normalizedChange, 0, len(changes))
	for _, ch := range changes {
		if ch.Path == "" {
			return nil, errors.New("change has an empty path")
		}
		logical, err := filepath.Abs(ch.Path)
		if err != nil {
			return nil, err
		}
		logical = filepath.Clean(logical)
		if err := requireWithinRoot(root, logical); err != nil {
			return nil, err
		}
		canonical := logical
		if real, err := filepath.EvalSymlinks(logical); err == nil {
			canonical = real
		}
		if seen[canonical] {
			return nil, fmt.Errorf("multiple changes target the same file: %s", canonical)
		}
		seen[canonical] = true
		info, err := os.Stat(canonical)
		if err != nil {
			return nil, fmt.Errorf("stat %s: %w", canonical, err)
		}
		if !info.Mode().IsRegular() {
			return nil, fmt.Errorf("refusing to edit non-regular file %s", canonical)
		}
		out = append(out, normalizedChange{
			logical: logical, canonical: canonical, content: append([]byte(nil), ch.Content...),
			mode: info.Mode().Perm(), expected: ch.ExpectedSHA256,
		})
	}
	return out, nil
}

func stageLoadedFiles(root, stageRoot, entry string, files []string) error {
	seen := map[string]bool{}
	files = append(files, entry)
	for _, file := range files {
		abs, err := filepath.Abs(file)
		if err != nil {
			continue
		}
		if seen[abs] {
			continue
		}
		seen[abs] = true
		stage, err := stagedPath(root, stageRoot, abs)
		if err != nil {
			// Absolute external modules remain available read-only to the staged
			// config; mutations themselves are restricted to the config tree.
			continue
		}
		data, err := os.ReadFile(abs)
		if err != nil {
			continue // may be an optional/missing require guarded by pcall
		}
		info, err := os.Stat(abs)
		if err != nil {
			continue
		}
		if err := writeFile(stage, data, info.Mode().Perm()); err != nil {
			return err
		}
	}
	return nil
}

func stagedPath(root, stageRoot, path string) (string, error) {
	if err := requireWithinRoot(root, path); err != nil {
		return "", err
	}
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return "", err
	}
	return filepath.Join(stageRoot, rel), nil
}

// requireWithinRoot rejects any path that is not contained in the config
// directory, so an edit can never reach an unrelated file on the system.
func requireWithinRoot(root, path string) error {
	rel, err := filepath.Rel(root, path)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return fmt.Errorf("refusing to edit a file outside the config directory %s: %s", root, path)
	}
	return nil
}

func writeFile(path string, data []byte, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".hyprmod-write-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
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
	if err := os.Rename(tmpPath, path); err != nil {
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

func rollback(originals map[string][]byte, written []string) error {
	var errs []error
	for i := len(written) - 1; i >= 0; i-- {
		path := written[i]
		info, err := os.Stat(path)
		mode := fs.FileMode(0o600)
		if err == nil {
			mode = info.Mode().Perm()
		}
		if err := writeFile(path, originals[path], mode); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func digest(data []byte) string {
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}

func validateOptions(report *sandbox.Report, sch *schema.Schema) error {
	for _, call := range report.Config {
		if err := validateOptionTable(call.Table, "", sch); err != nil {
			return fmt.Errorf("%s:%d: %w", call.File, call.Line, err)
		}
	}
	monitorTypes := make(map[string]string, len(sch.Specs["MonitorSpec"]))
	for _, field := range sch.Specs["MonitorSpec"] {
		monitorTypes[field.Name] = field.Type
	}
	for _, monitor := range report.Monitors {
		for field, value := range monitor.Spec {
			rawType, ok := monitorTypes[field]
			if !ok {
				continue
			}
			if field == "scale" && rawType == "string|number" {
				if _, isString := value.(string); isString {
					continue
				}
				if finiteNumber(value) {
					continue
				}
				return fmt.Errorf("%s:%d: monitor.scale expects string|number, got %T", monitor.File, monitor.Line, value)
			}
			kind := schema.Classify(field, rawType)
			if kind != config.KindUnknown && !validSchemaValue(kind, value) {
				return fmt.Errorf("%s:%d: monitor.%s expects %s, got %T", monitor.File, monitor.Line, field, kind, value)
			}
		}
	}
	return nil
}

func validateOptionTable(values map[string]any, prefix string, sch *schema.Schema) error {
	for key, value := range values {
		path := key
		if prefix != "" {
			path = prefix + "." + key
		}
		option := sch.Lookup(path)
		if children, ok := value.(map[string]any); ok && (option == nil || option.IsSection()) {
			if err := validateOptionTable(children, path, sch); err != nil {
				return err
			}
			continue
		}
		if option == nil || option.Kind == config.KindUnknown {
			continue
		}
		if !validSchemaValue(option.Kind, value) {
			return fmt.Errorf("%s expects %s, got %T", path, option.Kind, value)
		}
	}
	return nil
}

func validSchemaValue(kind config.ValueKind, value any) bool {
	switch kind {
	case config.KindBool:
		_, ok := value.(bool)
		return ok
	case config.KindInt:
		if _, ok := value.(bool); ok {
			return true
		}
		f, ok := value.(float64)
		return ok && math.Trunc(f) == f
	case config.KindFloat:
		if _, ok := value.(bool); ok {
			return true
		}
		f, ok := value.(float64)
		return ok && !math.IsNaN(f) && !math.IsInf(f, 0)
	case config.KindString, config.KindColor:
		_, ok := value.(string)
		return ok
	case config.KindGradient:
		if _, ok := value.(string); ok {
			return true
		}
		_, ok := value.(map[string]any)
		return ok
	case config.KindVec2:
		if _, ok := value.(string); ok {
			return true
		}
		if pair, ok := value.([]any); ok {
			return len(pair) == 2 && finiteNumber(pair[0]) && finiteNumber(pair[1])
		}
		m, ok := value.(map[string]any)
		return ok && finiteNumber(m["x"]) && finiteNumber(m["y"])
	case config.KindCSSGaps:
		if f, ok := value.(float64); ok {
			return math.Trunc(f) == f
		}
		_, ok := value.(map[string]any)
		return ok
	case config.KindFontWeight:
		if _, ok := value.(string); ok {
			return true
		}
		f, ok := value.(float64)
		return ok && math.Trunc(f) == f
	case config.KindStringList:
		_, ok := value.([]any)
		return ok
	default:
		return true
	}
}

func finiteNumber(v any) bool {
	f, ok := v.(float64)
	return ok && !math.IsNaN(f) && !math.IsInf(f, 0)
}
