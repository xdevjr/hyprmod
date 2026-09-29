// Package api exposes HyprMod's backend operations to the Wails frontend. It
// orchestrates the pure-Go domain, schema, sandbox, hyprctl, writer and snapshot
// packages; none of those core packages depend on Wails.
package api

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/juniorgs/hyprmod/internal/config"
	"github.com/juniorgs/hyprmod/internal/hyprctl"
	"github.com/juniorgs/hyprmod/internal/luasrc"
	"github.com/juniorgs/hyprmod/internal/sandbox"
	"github.com/juniorgs/hyprmod/internal/schema"
	"github.com/juniorgs/hyprmod/internal/snapshots"
	"github.com/juniorgs/hyprmod/internal/writer"
)

// Service is the Wails-bound application service.
type Service struct {
	Hypr      *hyprctl.Client
	Writer    *writer.Writer
	Schema    *schema.Schema
	Snapshots *snapshots.Manager

	pathMu     sync.RWMutex
	configPath string
}

// NewService constructs the app service. It does not require Hyprland to be
// running, so the UI can show a useful disconnected dashboard.
func NewService() *Service {
	mgr, _ := snapshots.Default()
	catalog := schema.Load()
	w := writer.New(mgr)
	w.Schema = catalog
	return &Service{
		Hypr:      hyprctl.New(),
		Writer:    w,
		Schema:    catalog,
		Snapshots: mgr,
	}
}

// Dashboard is the health summary shown on launch.
type Dashboard struct {
	Online         bool                `json:"online"`
	Supported      bool                `json:"supported"`
	Version        hyprctl.VersionInfo `json:"version"`
	ConfigProvider string              `json:"configProvider"`
	Backend        string              `json:"backend"`
	ConfigPath     string              `json:"configPath"`
	ConfigErrors   []string            `json:"configErrors"`
	SnapshotCount  int                 `json:"snapshotCount"`
	Message        string              `json:"message"`
}

// SectionView provides schema metadata and effective runtime values for one
// Variables page section.
type SectionView struct {
	Section          *schema.Option `json:"section"`
	Values           []config.Value `json:"values"`
	RuntimeAvailable bool           `json:"runtimeAvailable"`
}

// MonitorView combines active outputs reported by the compositor with the
// monitor declarations found in Lua config files.
type MonitorView struct {
	Active       []hyprctl.Monitor    `json:"active"`
	Configured   []config.MonitorSpec `json:"configured"`
	RuntimeError string               `json:"runtimeError,omitempty"`
}

// RestoreResult reports the safety snapshot made immediately before restore.
type RestoreResult struct {
	PreRestoreSnapshotID string `json:"preRestoreSnapshotId"`
	Reloaded             bool   `json:"reloaded"`
	ReloadError          string `json:"reloadError,omitempty"`
}

// GetDashboard queries Hyprland's version/status and local snapshot count.
func (s *Service) GetDashboard(ctx context.Context) Dashboard {
	d := Dashboard{ConfigPath: s.configEntry()}
	if s.Schema != nil {
		d.Message = "Schema: " + s.Schema.Source
	}
	if s.Snapshots != nil {
		if list, err := s.Snapshots.List(); err == nil {
			d.SnapshotCount = len(list)
		}
	}
	version, err := s.Hypr.Version(ctx)
	if err != nil {
		d.Message = "Hyprland is not reachable: " + err.Error()
		return d
	}
	d.Online = true
	d.Version = version
	d.Supported = version.SupportsLua()
	status, err := s.Hypr.Status(ctx)
	if err != nil {
		d.Message = "Could not query Hyprland status: " + err.Error()
		return d
	}
	d.ConfigProvider = status.ConfigProvider
	d.Backend = status.Backend
	if !d.Supported {
		d.Message = "HyprMod requires Hyprland 0.55 or later (Lua config provider)."
	} else if status.ConfigProvider != "lua" {
		d.Supported = false
		d.Message = "The active Hyprland config provider is not Lua."
	} else if strings.EqualFold(filepath.Ext(d.ConfigPath), ".conf") {
		d.Supported = false
		d.Message = "hyprlang configs are not supported; select a hyprland.lua file."
	} else if _, statErr := os.Stat(d.ConfigPath); statErr != nil {
		d.Message = "Lua config file was not found; select the active hyprland.lua file."
	} else {
		d.ConfigErrors, err = s.Hypr.ConfigErrors(ctx)
		if err != nil {
			d.Message = "Could not read Hyprland config diagnostics: " + err.Error()
		} else if len(d.ConfigErrors) == 0 {
			d.Message = "Hyprland is running with a Lua configuration."
		} else {
			d.Message = "Hyprland reported configuration errors. A snapshot is available before editing."
		}
	}
	return d
}

// GetSchema returns the option/dispatcher/event catalogs derived from the
// installed Hyprland Lua stub, falling back to the bundled stub.
func (s *Service) GetSchema() *schema.Schema { return s.Schema }

// DeviceView reports the input hardware Hyprland currently sees, so the input
// page can show the devices that the per-device options in `input` apply to.
type DeviceView struct {
	Keyboards []hyprctl.Keyboard      `json:"keyboards"`
	Mice      []hyprctl.PointerDevice `json:"mice"`
	Tablets   []map[string]any        `json:"tablets"`
	Touch     []map[string]any        `json:"touch"`
	Available bool                    `json:"available"`
	Message   string                  `json:"message,omitempty"`
}

// GetDevices lists the live input devices. It never fails the page: when the
// socket is unavailable it reports that no hardware could be queried.
func (s *Service) GetDevices(ctx context.Context) DeviceView {
	devices, err := s.Hypr.Devices(ctx)
	if err != nil {
		return DeviceView{Message: err.Error()}
	}
	return DeviceView{
		Keyboards: devices.Keyboards,
		Mice:      devices.Mice,
		Tablets:   devices.Tablets,
		Touch:     devices.Touch,
		Available: true,
	}
}

// GetConfigPath returns the current entry file path.
func (s *Service) GetConfigPath() string { return s.configEntry() }

// SelectConfigPath chooses a Lua entry file explicitly. It only changes this
// application's selection; no Hyprland config files are rewritten.
func (s *Service) SelectConfigPath(path string) error {
	path = strings.TrimSpace(path)
	if path == "" {
		return errors.New("config path is empty")
	}
	path = expandHome(path)
	abs, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	if strings.EqualFold(filepath.Ext(abs), ".conf") {
		return errors.New("HyprMod supports Lua configs only; choose a .lua file")
	}
	if !strings.EqualFold(filepath.Ext(abs), ".lua") {
		return errors.New("config file must have a .lua extension")
	}
	info, err := os.Stat(abs)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return errors.New("config path is not a regular file")
	}
	s.pathMu.Lock()
	s.configPath = abs
	s.pathMu.Unlock()
	return nil
}

// LoadDocument evaluates the selected Lua config in the recording harness and
// returns the effective declarative config plus source provenance.
func (s *Service) LoadDocument(ctx context.Context) (config.Document, error) {
	entry := s.configEntry()
	if strings.EqualFold(filepath.Ext(entry), ".conf") {
		return config.Document{}, errors.New("hyprlang configs are not supported; select a hyprland.lua file")
	}
	if !strings.EqualFold(filepath.Ext(entry), ".lua") {
		return config.Document{}, fmt.Errorf("not a Lua config file: %s", entry)
	}
	report, evalErr := sandbox.Evaluate(ctx, sandbox.Options{Entry: entry})
	if report == nil {
		return config.Document{}, evalErr
	}
	doc := s.buildDocument(entry, report)
	if evalErr != nil {
		var harnessErr *sandbox.HarnessError
		if errors.As(evalErr, &harnessErr) {
			doc.Diagnostics = append(doc.Diagnostics, config.Diagnostic{Severity: "error", Message: harnessErr.Message})
			return doc, nil
		}
		return doc, evalErr
	}
	return doc, nil
}

// GetSection returns one schema section with effective runtime values. The
// runtime query is batched into a single hyprctl IPC call for the section.
func (s *Service) GetSection(ctx context.Context, path string) (SectionView, error) {
	section := s.Schema.Lookup(path)
	if section == nil {
		return SectionView{}, fmt.Errorf("unknown schema path %q", path)
	}
	leaves := []*schema.Option{section}
	if section.IsSection() {
		leaves = flatten(section.Children)
	}
	leafPaths := make([]string, 0, len(leaves))
	for _, leaf := range leaves {
		if leaf.Kind != config.KindUnknown {
			leafPaths = append(leafPaths, leaf.Path)
		}
	}
	runtime, runtimeErr := s.Hypr.GetOptions(ctx, leafPaths)

	doc, docErr := s.LoadDocument(ctx)
	if docErr != nil && len(doc.Values) == 0 {
		return SectionView{}, docErr
	}
	explicit := make(map[string]config.Value, len(doc.Values))
	for _, v := range doc.Values {
		explicit[v.Path] = v
	}

	values := make([]config.Value, 0, len(leaves))
	for _, leaf := range leaves {
		v := config.Value{
			Path: leaf.Path, Kind: leaf.Kind, Editable: leaf.Kind != config.KindUnknown,
		}
		parts := strings.Split(leaf.Path, ".")
		v.Section = parts[0]
		if len(parts) > 1 {
			v.Key = strings.Join(parts[1:], ".")
		}
		if source, ok := explicit[leaf.Path]; ok {
			v.Origin = source.Origin
			v.Dynamic = source.Dynamic
			v.Literal = source.Literal
			v.Editable = source.Editable || source.Dynamic
			v.Resolved = source.Resolved
		}
		if live, ok := runtime[leaf.Path]; ok {
			v.Resolved = live.Value
		}
		values = append(values, v)
	}
	return SectionView{Section: section, Values: values, RuntimeAvailable: runtimeErr == nil}, nil
}

// GetMonitors returns live monitor state when available and all monitor
// declarations discovered in the selected config.
func (s *Service) GetMonitors(ctx context.Context) (MonitorView, error) {
	doc, err := s.LoadDocument(ctx)
	if err != nil {
		return MonitorView{}, err
	}
	view := MonitorView{Configured: doc.Monitors}
	active, err := s.Hypr.Monitors(ctx)
	if err != nil {
		view.RuntimeError = err.Error()
		return view, nil
	}
	view.Active = active
	return view, nil
}

// SetOption validates a schema-backed value, serializes it as a Lua literal and
// commits it through the staged sandbox/snapshot/atomic-write pipeline.
func (s *Service) SetOption(ctx context.Context, path string, value any) (writer.Result, error) {
	opt := s.Schema.Lookup(path)
	if opt == nil || opt.IsSection() {
		return writer.Result{}, fmt.Errorf("unknown option %q", path)
	}
	if err := validateValue(opt.Kind, value); err != nil {
		return writer.Result{}, fmt.Errorf("%s: %w", path, err)
	}
	literal, err := toLua(value)
	if err != nil {
		return writer.Result{}, err
	}
	doc, err := s.LoadDocument(ctx)
	if err != nil {
		return writer.Result{}, err
	}
	file := s.configEntry()
	for _, existing := range doc.Values {
		if existing.Path == path && existing.File != "" {
			file = existing.File
			break
		}
	}
	result, _, err := s.Writer.SetConfigValue(ctx, s.configEntry(), file, path, literal)
	return result, err
}

// SetMonitorField changes one monitor declaration field. If the declaration
// uses computed Lua values, the writer appends a new literal override call.
func (s *Service) SetMonitorField(ctx context.Context, output, field string, value any) (writer.Result, error) {
	return s.SetMonitorFields(ctx, output, map[string]any{field: value})
}

// SetMonitorFields updates one output in a single atomic write and snapshot.
// If any target property is computed, a single complete monitor override is
// appended rather than several partial declarations.
func (s *Service) SetMonitorFields(ctx context.Context, output string, values map[string]any) (writer.Result, error) {
	if len(values) == 0 {
		return writer.Result{}, errors.New("no monitor fields to update")
	}
	fields := s.Schema.Specs["MonitorSpec"]
	fieldTypes := make(map[string]string, len(fields))
	for _, field := range fields {
		fieldTypes[field.Name] = field.Type
	}
	keys := make([]string, 0, len(values))
	for field := range values {
		keys = append(keys, field)
	}
	sort.Strings(keys)
	literals := make(map[string]string, len(values))
	for _, field := range keys {
		if field == "output" || field == "" {
			return writer.Result{}, errors.New("monitor field must be an editable property (not output)")
		}
		rawType, ok := fieldTypes[field]
		if !ok {
			return writer.Result{}, fmt.Errorf("unsupported monitor field %q", field)
		}
		if field == "scale" && strings.Contains(rawType, "string|number") {
			if _, isString := values[field].(string); !isString {
				if _, isNumber := numberValue(values[field]); !isNumber {
					return writer.Result{}, errors.New("monitor.scale: expected a number or string")
				}
			}
		} else {
			kind := schema.Classify(field, rawType)
			if kind != config.KindUnknown {
				if err := validateValue(kind, values[field]); err != nil {
					return writer.Result{}, fmt.Errorf("monitor.%s: %w", field, err)
				}
			}
		}
		literal, err := toLua(values[field])
		if err != nil {
			return writer.Result{}, err
		}
		literals[field] = literal
	}
	doc, err := s.LoadDocument(ctx)
	if err != nil {
		return writer.Result{}, err
	}
	file := s.configEntry()
	for _, monitor := range doc.Monitors {
		if monitor.Output == output && monitor.File != "" {
			file = monitor.File
		}
	}
	data, err := os.ReadFile(file)
	if err != nil {
		return writer.Result{}, err
	}
	docLua := luasrc.NewFile(string(data))
	markerID := "monitor " + hex.EncodeToString([]byte(output))
	buildNew := func() string {
		kvs := []luasrc.KV{{Key: "output", Value: luaQuote(output)}}
		for _, field := range keys {
			kvs = append(kvs, luasrc.KV{Key: field, Value: literals[field]})
		}
		return "hl.monitor(" + luasrc.TableText(kvs) + ")"
	}
	edits, _ := docLua.SetCallFields("hl.monitor", markerID, map[string]string{"output": output}, literals, buildNew)
	updated := luasrc.Apply(string(data), edits...)
	return s.Writer.Apply(ctx, s.configEntry(), []writer.FileChange{{
		Path: file, Content: []byte(updated), ExpectedSHA256: sourceDigest(data),
	}})
}

// PreviewOption applies a temporary option via hyprctl eval. It is never
// persisted and can be overwritten by a config reload.
func (s *Service) PreviewOption(ctx context.Context, path string, value any) error {
	opt := s.Schema.Lookup(path)
	if opt == nil || opt.IsSection() {
		return fmt.Errorf("unknown option %q", path)
	}
	if err := validateValue(opt.Kind, value); err != nil {
		return err
	}
	literal, err := toLua(value)
	if err != nil {
		return err
	}
	return s.Writer.PreviewOption(ctx, s.Hypr.Eval, path, literal)
}

// ValidateConfig runs the configuration through the sandbox and returns its
// diagnostics. Syntax/runtime config errors are included in the document.
func (s *Service) ValidateConfig(ctx context.Context) (config.Document, error) {
	return s.LoadDocument(ctx)
}

// ListSnapshots returns snapshots newest-first.
func (s *Service) ListSnapshots() ([]snapshots.Snapshot, error) {
	mgr, err := s.snapshotManager()
	if err != nil {
		return nil, err
	}
	return mgr.List()
}

// CreateSnapshot snapshots every file in the active require graph.
func (s *Service) CreateSnapshot(ctx context.Context) (snapshots.Snapshot, error) {
	doc, err := s.LoadDocument(ctx)
	if err != nil {
		return snapshots.Snapshot{}, err
	}
	paths := make([]string, 0, len(doc.Files)+1)
	for _, file := range doc.Files {
		paths = append(paths, file.Path)
	}
	if len(paths) == 0 {
		paths = append(paths, s.configEntry())
	}
	mgr, err := s.snapshotManager()
	if err != nil {
		return snapshots.Snapshot{}, err
	}
	return mgr.Create(paths)
}

// RestoreSnapshot restores a snapshot and asks a running Hyprland to reload.
// The snapshot manager creates a pre-restore snapshot first.
func (s *Service) RestoreSnapshot(ctx context.Context, id string) (RestoreResult, error) {
	mgr, err := s.snapshotManager()
	if err != nil {
		return RestoreResult{}, err
	}
	preID, err := mgr.Restore(id)
	if err != nil {
		return RestoreResult{PreRestoreSnapshotID: preID}, err
	}
	result := RestoreResult{PreRestoreSnapshotID: preID}
	if err := s.Hypr.Reload(ctx); err != nil {
		result.ReloadError = err.Error()
		return result, nil
	}
	result.Reloaded = true
	return result, nil
}

// DeleteSnapshot removes one saved snapshot.
func (s *Service) DeleteSnapshot(id string) error {
	mgr, err := s.snapshotManager()
	if err != nil {
		return err
	}
	return mgr.Delete(id)
}

func (s *Service) configEntry() string {
	s.pathMu.RLock()
	selected := s.configPath
	s.pathMu.RUnlock()
	if selected != "" {
		return selected
	}
	if configured := os.Getenv("HYPRLAND_CONFIG"); configured != "" {
		return expandHome(configured)
	}
	xdg := os.Getenv("XDG_CONFIG_HOME")
	if xdg == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return filepath.Join(".config", "hypr", "hyprland.lua")
		}
		xdg = filepath.Join(home, ".config")
	}
	base := filepath.Join(xdg, "hypr")
	lua := filepath.Join(base, "hyprland.lua")
	if _, err := os.Stat(lua); err == nil {
		return lua
	}
	legacy := filepath.Join(base, "hyprland.conf")
	if _, err := os.Stat(legacy); err == nil {
		return legacy
	}
	return lua
}

func (s *Service) snapshotManager() (*snapshots.Manager, error) {
	if s.Snapshots != nil {
		return s.Snapshots, nil
	}
	return snapshots.Default()
}

func (s *Service) buildDocument(entry string, report *sandbox.Report) config.Document {
	root, _ := filepath.Abs(filepath.Dir(entry))
	doc := config.Document{Root: root, Entry: entry, Format: "lua"}
	fileSeen := map[string]bool{}
	for _, path := range report.Files {
		abs, err := filepath.Abs(path)
		if err != nil || fileSeen[abs] {
			continue
		}
		fileSeen[abs] = true
		info := config.FileInfo{Path: abs, Rel: filepath.Base(abs), Managed: true}
		if rel, err := filepath.Rel(root, abs); err == nil && !strings.HasPrefix(rel, "..") {
			info.Rel = rel
		}
		if stat, err := os.Stat(abs); err == nil {
			info.Bytes = int(stat.Size())
		}
		doc.Files = append(doc.Files, info)
	}
	if len(report.Errors) > 0 {
		for _, e := range report.Errors {
			doc.Diagnostics = append(doc.Diagnostics, config.Diagnostic{Severity: "error", Message: e.Message})
		}
	}

	values := map[string]config.Value{}
	staticFiles := map[string]*luasrc.File{}
	for _, call := range report.Config {
		walkConfigTable(call.Table, "", call, s.Schema, staticFiles, values)
	}
	paths := make([]string, 0, len(values))
	for path := range values {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	for _, path := range paths {
		doc.Values = append(doc.Values, values[path])
	}

	for _, rec := range report.Monitors {
		spec := config.MonitorSpec{File: rec.File, Line: rec.Line}
		if v, ok := rec.Spec["output"].(string); ok {
			spec.Output = v
		}
		if v, ok := rec.Spec["mode"].(string); ok {
			spec.Mode = v
		}
		if v, ok := rec.Spec["position"].(string); ok {
			spec.Position = v
		}
		spec.Scale = rec.Spec["scale"]
		spec.VRR = rec.Spec["vrr"]
		spec.Transform = rec.Spec["transform"]
		if v, ok := rec.Spec["mirror"].(string); ok {
			spec.Mirror = v
		}
		spec.Disabled = rec.Spec["disabled"]
		spec.Bitdepth = rec.Spec["bitdepth"]
		spec.ColorPreset, _ = rec.Spec["cm"].(string)
		spec.Editable = s.callEditable(rec.File, "hl.monitor", rec.Line)
		doc.Monitors = append(doc.Monitors, spec)
	}
	for _, b := range report.Binds {
		enabled := b.Enabled == nil || *b.Enabled
		bind := config.Bind{
			Keys: b.Keys, Dispatcher: config.Dispatcher{Name: b.DSP.Name, Args: b.DSP.Args},
			Opts: b.Opts, Enabled: enabled, File: b.File, Line: b.Line,
			Editable: s.callEditable(b.File, "hl.bind", b.Line),
		}
		// Inline Lua callbacks and expressions-built key combos only exist in the
		// source, so they are recovered from the call itself.
		callback, keysDynamic := describeBindSource(b.File, b.Line)
		bind.KeysDynamic = keysDynamic
		if bind.Dispatcher.Name == functionDispatcherName {
			bind.Dispatcher.Callback = callback
		}
		doc.Binds = append(doc.Binds, bind)
	}
	for _, rec := range report.WindowRules {
		doc.Rules = append(doc.Rules, ruleFromRecord("window", rec))
	}
	for _, rec := range report.LayerRules {
		doc.Rules = append(doc.Rules, ruleFromRecord("layer", rec))
	}
	for _, rec := range report.WorkspaceRules {
		doc.Rules = append(doc.Rules, ruleFromRecord("workspace", rec))
	}
	for _, rec := range report.Curves {
		doc.Curves = append(doc.Curves, config.Curve{Name: rec.Name, Spec: rec.Spec, File: rec.File, Line: rec.Line})
	}
	for _, rec := range report.Animations {
		doc.Animations = append(doc.Animations, config.Animation{Leaf: stringField(rec.Spec, "leaf"), Spec: rec.Spec, File: rec.File, Line: rec.Line})
	}
	for _, rec := range report.Gestures {
		doc.Gestures = append(doc.Gestures, config.Gesture{Spec: rec.Spec, File: rec.File, Line: rec.Line})
	}
	for _, rec := range report.Devices {
		doc.Devices = append(doc.Devices, config.Device{Spec: rec.Spec, File: rec.File, Line: rec.Line})
	}
	for _, rec := range report.Envs {
		doc.Envs = append(doc.Envs, config.EnvVar{Name: rec.Name, Value: rec.Value, File: rec.File, Line: rec.Line})
	}
	for _, rec := range report.Permissions {
		doc.Permissions = append(doc.Permissions, config.Permission{Binary: rec.Binary, Type: rec.Type, Mode: rec.Mode, File: rec.File, Line: rec.Line})
	}
	for _, rec := range report.Events {
		doc.Events = append(doc.Events, config.EventSubscription{Event: rec.Event, File: rec.File, Line: rec.Line})
	}
	return doc
}

func walkConfigTable(table map[string]any, prefix string, call sandbox.ConfigCall, sch *schema.Schema, staticFiles map[string]*luasrc.File, out map[string]config.Value) {
	keys := make([]string, 0, len(table))
	for k := range table {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, key := range keys {
		path := key
		if prefix != "" {
			path = prefix + "." + key
		}
		value := table[key]
		opt := sch.Lookup(path)
		if child, ok := value.(map[string]any); ok && (opt == nil || opt.IsSection()) {
			walkConfigTable(child, path, call, sch, staticFiles, out)
			continue
		}
		v := config.Value{
			Path: path, Resolved: value, Origin: config.Origin{File: call.File, Line: call.Line}, Dynamic: true,
		}
		parts := strings.Split(path, ".")
		v.Section = parts[0]
		if len(parts) > 1 {
			v.Key = strings.Join(parts[1:], ".")
		}
		if opt != nil {
			v.Kind = opt.Kind
			v.Editable = true // dynamic values can be overridden without destruction
		}
		if src, ok := staticFiles[call.File]; ok {
			if fld := src.FindConfigValueAt(path, call.Line); fld != nil {
				v.File = call.File
				v.Line = fld.ValLine
				v.Literal = fld.ValText
				v.Dynamic = !fld.Literal
				v.Editable = true
			}
		} else if data, err := os.ReadFile(call.File); err == nil {
			src := luasrc.NewFile(string(data))
			staticFiles[call.File] = src
			if fld := src.FindConfigValueAt(path, call.Line); fld != nil {
				v.Line = fld.ValLine
				v.Literal = fld.ValText
				v.Dynamic = !fld.Literal
				v.Editable = true
			}
		}
		out[path] = v
	}
}

// functionDispatcherName is how the sandbox labels a dispatcher that is an
// inline Lua function rather than an hl.dsp.* call.
const functionDispatcherName = "<function>"

// describeBindSource inspects the hl.bind call on the given line and returns
// the raw source of its dispatcher argument plus whether the key combination is
// built by a Lua expression instead of a literal string.
func describeBindSource(file string, line int) (dispatcher string, keysDynamic bool) {
	data, err := os.ReadFile(file)
	if err != nil {
		return "", false
	}
	parsed := luasrc.NewFile(string(data))
	calls := parsed.FindCalls("hl.bind")
	for i := len(calls) - 1; i >= 0; i-- {
		call := calls[i]
		if call.Line != line {
			continue
		}
		if len(call.Args) == 0 {
			return "", false
		}
		// A single literal token is a static key combo; anything else (a
		// concatenation, a loop variable) is computed at load time.
		keysDynamic = call.Args[0].Start != call.Args[0].End
		if len(call.Args) >= 2 {
			dispatcher = call.Args[1].Text
		}
		return dispatcher, keysDynamic
	}
	return "", false
}

func (s *Service) callEditable(file, name string, line int) bool {
	data, err := os.ReadFile(file)
	if err != nil {
		return false
	}
	parsed := luasrc.NewFile(string(data))
	for _, c := range parsed.FindCalls(name) {
		if c.Line == line {
			return true
		}
	}
	return false
}

func ruleFromRecord(kind string, rec sandbox.SpecRecord) config.Rule {
	fields := make(map[string]any, len(rec.Spec))
	var match map[string]any
	for k, v := range rec.Spec {
		if k == "match" {
			match = asMap(v)
			continue
		}
		if k == "name" {
			continue
		}
		fields[k] = v
	}
	return config.Rule{Kind: kind, Name: stringField(rec.Spec, "name"), Match: match, Fields: fields, File: rec.File, Line: rec.Line, Editable: true}
}

func flatten(nodes []*schema.Option) []*schema.Option {
	var out []*schema.Option
	for _, node := range nodes {
		if node.IsSection() {
			out = append(out, flatten(node.Children)...)
		} else {
			out = append(out, node)
		}
	}
	return out
}

func validateValue(kind config.ValueKind, value any) error {
	switch kind {
	case config.KindBool:
		if _, ok := value.(bool); !ok {
			return errors.New("expected a boolean")
		}
	case config.KindInt:
		if _, isBool := value.(bool); !isBool {
			if _, ok := integerValue(value); ok {
				break
			}
			return errors.New("expected an integer")
		}
	case config.KindFloat:
		if _, isBool := value.(bool); !isBool {
			if _, ok := numberValue(value); ok {
				break
			}
			return errors.New("expected a finite number")
		}
	case config.KindString, config.KindColor:
		if _, ok := value.(string); !ok {
			return errors.New("expected a string")
		}
	case config.KindGradient:
		if _, ok := value.(string); !ok {
			if asMap(value) == nil {
				return errors.New("expected a color string or gradient object")
			}
		}
	case config.KindVec2:
		if _, isString := value.(string); !isString {
			if _, ok := vec2(value); ok {
				break
			}
			return errors.New("expected a 2D vector")
		}
	case config.KindCSSGaps:
		if _, ok := integerValue(value); !ok {
			m := asMap(value)
			if m == nil {
				return errors.New("expected an integer or gap object")
			}
			for key, item := range m {
				switch key {
				case "top", "right", "bottom", "left":
					if _, ok := integerValue(item); !ok {
						return fmt.Errorf("gap %s must be an integer", key)
					}
				default:
					return fmt.Errorf("unknown gap edge %q", key)
				}
			}
		}
	case config.KindFontWeight:
		if _, ok := value.(string); !ok {
			if _, ok := integerValue(value); !ok {
				return errors.New("expected a font weight string or integer")
			}
		}
	case config.KindStringList:
		if _, ok := value.([]any); !ok {
			return errors.New("expected a list")
		}
	default:
		return errors.New("unsupported schema value type")
	}
	return nil
}

func toLua(value any) (string, error) {
	switch v := value.(type) {
	case nil:
		return "nil", nil
	case bool:
		if v {
			return "true", nil
		}
		return "false", nil
	case string:
		return luaQuote(v), nil
	case int:
		return strconv.Itoa(v), nil
	case int8:
		return strconv.FormatInt(int64(v), 10), nil
	case int16:
		return strconv.FormatInt(int64(v), 10), nil
	case int32:
		return strconv.FormatInt(int64(v), 10), nil
	case int64:
		return strconv.FormatInt(v, 10), nil
	case uint, uint8, uint16, uint32, uint64:
		return fmt.Sprint(v), nil
	case float32:
		return numberLua(float64(v))
	case float64:
		return numberLua(v)
	case []any:
		parts := make([]string, 0, len(v))
		for _, item := range v {
			text, err := toLua(item)
			if err != nil {
				return "", err
			}
			parts = append(parts, text)
		}
		return "{ " + strings.Join(parts, ", ") + " }", nil
	case map[string]any:
		keys := make([]string, 0, len(v))
		for key := range v {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		parts := make([]string, 0, len(keys))
		for _, key := range keys {
			keyExpr := key
			if !luaIdentifier(key) {
				keyExpr = "[" + luaQuote(key) + "]"
			}
			text, err := toLua(v[key])
			if err != nil {
				return "", err
			}
			parts = append(parts, keyExpr+" = "+text)
		}
		return "{ " + strings.Join(parts, ", ") + " }", nil
	default:
		return "", fmt.Errorf("cannot serialize %T as a Lua literal", value)
	}
}

func numberLua(v float64) (string, error) {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return "", errors.New("non-finite numbers are not valid Hyprland values")
	}
	return strconv.FormatFloat(v, 'g', -1, 64), nil
}

func luaQuote(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range s {
		switch r {
		case '\\', '"':
			b.WriteByte('\\')
			b.WriteRune(r)
		case '\n':
			b.WriteString("\\n")
		case '\r':
			b.WriteString("\\r")
		case '\t':
			b.WriteString("\\t")
		case '\b':
			b.WriteString("\\b")
		case '\f':
			b.WriteString("\\f")
		default:
			if r < 0x20 || r == 0x7f {
				b.WriteString(fmt.Sprintf("\\%03d", r))
			} else {
				b.WriteRune(r)
			}
		}
	}
	b.WriteByte('"')
	return b.String()
}

func luaIdentifier(s string) bool {
	if s == "" || !(s[0] == '_' || s[0] >= 'a' && s[0] <= 'z' || s[0] >= 'A' && s[0] <= 'Z') {
		return false
	}
	if luaKeywords[s] {
		return false
	}
	for i := 1; i < len(s); i++ {
		c := s[i]
		if !(c == '_' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9') {
			return false
		}
	}
	return true
}

var luaKeywords = map[string]bool{
	"and": true, "break": true, "do": true, "else": true, "elseif": true,
	"end": true, "false": true, "for": true, "function": true, "goto": true,
	"if": true, "in": true, "local": true, "nil": true, "not": true,
	"or": true, "repeat": true, "return": true, "then": true, "true": true,
	"until": true, "while": true,
}

func numberValue(v any) (float64, bool) {
	switch n := v.(type) {
	case int:
		return float64(n), true
	case int8:
		return float64(n), true
	case int16:
		return float64(n), true
	case int32:
		return float64(n), true
	case int64:
		return float64(n), true
	case uint:
		return float64(n), true
	case uint8:
		return float64(n), true
	case uint16:
		return float64(n), true
	case uint32:
		return float64(n), true
	case uint64:
		return float64(n), true
	case float32:
		f := float64(n)
		return f, !math.IsNaN(f) && !math.IsInf(f, 0)
	case float64:
		return n, !math.IsNaN(n) && !math.IsInf(n, 0)
	}
	return 0, false
}

func integerValue(v any) (int64, bool) {
	f, ok := numberValue(v)
	if !ok || math.Trunc(f) != f || f < math.MinInt64 || f > math.MaxInt64 {
		return 0, false
	}
	return int64(f), true
}

func vec2(v any) ([]any, bool) {
	if arr, ok := v.([]any); ok && len(arr) == 2 {
		if _, good := numberValue(arr[0]); good {
			if _, good := numberValue(arr[1]); good {
				return arr, true
			}
		}
	}
	if m := asMap(v); m != nil {
		x, xok := numberValue(m["x"])
		y, yok := numberValue(m["y"])
		if xok && yok {
			return []any{x, y}, true
		}
	}
	return nil, false
}

func asMap(v any) map[string]any {
	m, _ := v.(map[string]any)
	return m
}

func stringField(m map[string]any, key string) string {
	v, _ := m[key].(string)
	return v
}

func expandHome(path string) string {
	if path == "~" || strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err == nil {
			return filepath.Join(home, strings.TrimPrefix(strings.TrimPrefix(path, "~"), string(filepath.Separator)))
		}
	}
	return path
}

func sourceDigest(data []byte) string {
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}
