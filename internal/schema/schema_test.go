package schema

import (
	"strings"
	"testing"

	"github.com/juniorgs/hyprmod/internal/config"
)

func TestParseEmbeddedStub(t *testing.T) {
	s, err := Parse(embeddedStub)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(s.Sections) < 15 {
		t.Fatalf("got %d top-level sections, want at least 15", len(s.Sections))
	}
	if leaves := len(s.Flatten()); leaves < 300 {
		t.Fatalf("got %d option leaves, want at least 300", leaves)
	}
	if len(s.Dispatchers) < 50 {
		t.Fatalf("got %d dispatchers, want at least 50", len(s.Dispatchers))
	}
	if !contains(s.Events, "hyprland.start") {
		t.Fatal("event list is missing hyprland.start")
	}
	if !contains(s.Dispatchers, "exec_cmd") || !contains(s.Dispatchers, "window.close") {
		t.Fatalf("dispatcher catalog missing common entries: %v", s.Dispatchers)
	}

	tests := []struct {
		path string
		kind config.ValueKind
	}{
		{"general.border_size", config.KindInt},
		{"general.col.active_border", config.KindGradient},
		{"decoration.blur.size", config.KindInt},
		{"decoration.shadow.offset", config.KindVec2},
		{"group.groupbar.font_weight_active", config.KindFontWeight},
		{"input.touchpad.natural_scroll", config.KindBool},
	}
	for _, tc := range tests {
		t.Run(tc.path, func(t *testing.T) {
			o := s.Lookup(tc.path)
			if o == nil {
				t.Fatalf("option %q not found", tc.path)
			}
			if o.Kind != tc.kind {
				t.Fatalf("kind=%q, want %q (raw=%q)", o.Kind, tc.kind, o.Raw)
			}
		})
	}
	if len(s.Specs["MonitorSpec"]) == 0 {
		t.Fatal("MonitorSpec fields were not parsed")
	}
}

// TestEveryOptionHasAKnownKind guards the Variables UI: an option classified as
// unknown cannot be rendered with a typed control and would be rejected by the
// writer's schema validation.
func TestEveryOptionHasAKnownKind(t *testing.T) {
	s, err := Parse(embeddedStub)
	if err != nil {
		t.Fatal(err)
	}
	var unknown []string
	for _, leaf := range s.Flatten() {
		if leaf.Kind == config.KindUnknown {
			unknown = append(unknown, leaf.Path+" ("+leaf.Raw+")")
		}
	}
	if len(unknown) > 0 {
		t.Fatalf("%d options could not be classified, e.g. %v", len(unknown), unknown[:min(len(unknown), 10)])
	}
}

func TestLoadPrefersInstalledStubWhenAvailable(t *testing.T) {
	s := Load()
	if s == nil || len(s.Sections) == 0 {
		t.Fatal("Load returned an empty schema")
	}
	if !strings.Contains(s.Source, "hl.meta.lua") && s.Source != "embedded" {
		t.Fatalf("unexpected source %q", s.Source)
	}
}

func TestClassify(t *testing.T) {
	tests := []struct {
		name, raw string
		want      config.ValueKind
	}{
		{"enabled", "boolean", config.KindBool},
		{"size", "integer|boolean", config.KindInt},
		{"opacity", "number|boolean", config.KindFloat},
		{"active_border", "string|HL.Gradient", config.KindGradient},
		{"offset", "HL.Vec2Like", config.KindVec2},
		{"gaps_in", "integer|HL.CssGap", config.KindCSSGaps},
		{"font_weight_active", "integer|string", config.KindFontWeight},
		{"text_color", "string", config.KindColor},
		{"kb_layout", "string", config.KindString},
	}
	for _, tc := range tests {
		if got := Classify(tc.name, tc.raw); got != tc.want {
			t.Errorf("Classify(%q,%q)=%q, want %q", tc.name, tc.raw, got, tc.want)
		}
	}
}

func contains(items []string, want string) bool {
	for _, s := range items {
		if s == want {
			return true
		}
	}
	return false
}
