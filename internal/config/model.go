// Package config defines HyprMod's language-neutral domain model.
//
// The model is deliberately decoupled from both the Lua source and the
// Hyprland runtime: it is what the sandbox produces when it evaluates a
// configuration, enriched with the provenance (file, line, editability)
// recovered from static analysis of the Lua source.
package config

// ValueKind classifies a configuration value for UI control selection.
type ValueKind string

const (
	KindBool       ValueKind = "bool"
	KindInt        ValueKind = "int"
	KindFloat      ValueKind = "float"
	KindString     ValueKind = "str"
	KindColor      ValueKind = "color"
	KindGradient   ValueKind = "gradient"
	KindVec2       ValueKind = "vec2"
	KindCSSGaps    ValueKind = "css_gaps"
	KindFontWeight ValueKind = "font_weight"
	KindStringList ValueKind = "string_list"
	KindUnknown    ValueKind = "unknown"
)

// Origin identifies where a setting physically lives in the Lua source.
type Origin struct {
	File string `json:"file"`
	Line int    `json:"line"`
}

// Value is a single resolved option with provenance.
type Value struct {
	// Path is the dotted option path without the section prefix duplication,
	// e.g. "general.gaps_in" or "decoration.blur.size".
	Path string `json:"path"`
	// Section is the top-level config section, e.g. "general".
	Section string `json:"section"`
	// Key is the path below the section, e.g. "blur.size" or "gaps_in".
	Key string `json:"key"`
	// Resolved holds the effective value computed by the sandbox.
	Resolved any `json:"resolved"`
	// Literal is the raw source text of the value when it is a literal.
	Literal string `json:"literal,omitempty"`
	// Kind is the UI value kind, derived from the schema.
	Kind ValueKind `json:"kind"`
	// Dynamic is true when the value in source is not a literal expression
	// (variable, call, concatenation, loop) and therefore cannot be edited in
	// place without an override block.
	Dynamic bool `json:"dynamic"`
	// Editable is true when the value can be rewritten surgically in place.
	Editable bool `json:"editable"`
	Origin
}

// MonitorSpec mirrors hl.monitor({...}).
type MonitorSpec struct {
	Output      string `json:"output"`
	Mode        string `json:"mode,omitempty"`
	Position    string `json:"position,omitempty"`
	Scale       any    `json:"scale,omitempty"`
	VRR         any    `json:"vrr,omitempty"`
	Transform   any    `json:"transform,omitempty"`
	Mirror      string `json:"mirror,omitempty"`
	Disabled    any    `json:"disabled,omitempty"`
	Bitdepth    any    `json:"bitdepth,omitempty"`
	ColorPreset string `json:"colorPreset,omitempty"`
	// Definition provenance.
	File string `json:"file"`
	Line int    `json:"line"`
	// Editable reports whether the whole spec is a literal table (so fields can
	// be rewritten in place).
	Editable bool `json:"editable"`
}

// Dispatcher mirrors the hl.dsp.* descriptor captured by the sandbox.
type Dispatcher struct {
	Name string `json:"name"`
	Args []any  `json:"args,omitempty"`
	// Callback holds the Lua source of a dispatcher that is an inline
	// `function() ... end` instead of a hl.dsp.* call.
	Callback string `json:"callback,omitempty"`
}

// Bind mirrors hl.bind(keys, dispatcher, opts?).
type Bind struct {
	Keys       string         `json:"keys"`
	Dispatcher Dispatcher     `json:"dispatcher"`
	Opts       map[string]any `json:"opts,omitempty"`
	Enabled    bool           `json:"enabled"`
	// KeysDynamic is true when the key combination comes from a Lua expression
	// (string concatenation, a loop, a variable) instead of a literal string.
	// Such binds cannot be rewritten in place without losing the expression.
	KeysDynamic bool   `json:"keysDynamic"`
	Source      string `json:"source,omitempty"`
	File        string `json:"file"`
	Line        int    `json:"line"`
	Editable    bool   `json:"editable"`
}

// Rule is a generic window/layer/workspace rule record.
type Rule struct {
	Kind     string         `json:"kind"` // window | layer | workspace
	Name     string         `json:"name,omitempty"`
	Match    map[string]any `json:"match,omitempty"`
	Fields   map[string]any `json:"fields,omitempty"`
	File     string         `json:"file"`
	Line     int            `json:"line"`
	Editable bool           `json:"editable"`
}

// Curve mirrors hl.curve(name, spec).
type Curve struct {
	Name string         `json:"name"`
	Spec map[string]any `json:"spec"`
	File string         `json:"file"`
	Line int            `json:"line"`
}

// Animation mirrors hl.animation({...}).
type Animation struct {
	Leaf string         `json:"leaf"`
	Spec map[string]any `json:"spec"`
	File string         `json:"file"`
	Line int            `json:"line"`
}

// Gesture mirrors hl.gesture({...}).
type Gesture struct {
	Spec map[string]any `json:"spec"`
	File string         `json:"file"`
	Line int            `json:"line"`
}

// Device mirrors hl.device({...}).
type Device struct {
	Spec map[string]any `json:"spec"`
	File string         `json:"file"`
	Line int            `json:"line"`
}

// EnvVar mirrors hl.env(name, value).
type EnvVar struct {
	Name  string `json:"name"`
	Value string `json:"value"`
	File  string `json:"file"`
	Line  int    `json:"line"`
}

// Permission mirrors hl.permission({binary, type, mode}).
type Permission struct {
	Binary string `json:"binary"`
	Type   string `json:"type"`
	Mode   string `json:"mode"`
	File   string `json:"file"`
	Line   int    `json:"line"`
}

// EventSubscription mirrors hl.on(event, cb) registrations (not executed).
type EventSubscription struct {
	Event string `json:"event"`
	File  string `json:"file"`
	Line  int    `json:"line"`
}

// Diagnostic is a problem found while reading or validating the config.
type Diagnostic struct {
	Severity string `json:"severity"` // error | warning
	Message  string `json:"message"`
	File     string `json:"file,omitempty"`
	Line     int    `json:"line,omitempty"`
}

// FileInfo describes one Lua file that participates in the document.
type FileInfo struct {
	Path    string `json:"path"`
	Rel     string `json:"rel"`
	Managed bool   `json:"managed"` // reachable from the entry via require()
	Bytes   int    `json:"bytes"`
}

// Document is the fully resolved, UI-facing snapshot of a configuration.
type Document struct {
	Root        string              `json:"root"`
	Entry       string              `json:"entry"`
	Format      string              `json:"format"` // always "lua"
	Files       []FileInfo          `json:"files"`
	Values      []Value             `json:"values"`
	Monitors    []MonitorSpec       `json:"monitors"`
	Binds       []Bind              `json:"binds"`
	Rules       []Rule              `json:"rules"`
	Curves      []Curve             `json:"curves"`
	Animations  []Animation         `json:"animations"`
	Gestures    []Gesture           `json:"gestures"`
	Devices     []Device            `json:"devices"`
	Envs        []EnvVar            `json:"envs"`
	Permissions []Permission        `json:"permissions"`
	Events      []EventSubscription `json:"events"`
	Diagnostics []Diagnostic        `json:"diagnostics"`
}
