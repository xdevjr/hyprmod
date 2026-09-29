package schema

import (
	"strings"

	"github.com/juniorgs/hyprmod/internal/config"
)

// Classify maps a stub type expression plus the option name to a UI value kind.
//
// Stub types are unions such as "integer|boolean" (a Hyprland idiom for
// "int or unset"), "number|boolean", "string|HL.Gradient",
// "integer|HL.CssGap" and "integer|string" (font weights).
func Classify(name, raw string) config.ValueKind {
	t := strings.ToLower(strings.TrimSpace(raw))
	switch {
	case t == "":
		return config.KindUnknown
	case strings.Contains(t, "hl.gradient"):
		return config.KindGradient
	case strings.Contains(t, "hl.cssgap"):
		return config.KindCSSGaps
	case strings.Contains(t, "hl.vec2"):
		return config.KindVec2
	case t == "integer|string" || t == "string|integer":
		if strings.Contains(name, "font_weight") {
			return config.KindFontWeight
		}
		return config.KindString
	case t == "integer|boolean":
		return config.KindInt
	case t == "number|boolean":
		return config.KindFloat
	case t == "boolean":
		return config.KindBool
	case t == "integer":
		return config.KindInt
	case t == "number":
		return config.KindFloat
	case t == "string":
		if looksLikeColor(name) {
			return config.KindColor
		}
		return config.KindString
	case strings.Contains(t, "string"):
		if looksLikeColor(name) {
			return config.KindColor
		}
		return config.KindString
	case strings.Contains(t, "integer"):
		return config.KindInt
	case strings.Contains(t, "number"):
		return config.KindFloat
	case strings.Contains(t, "boolean"):
		return config.KindBool
	}
	return config.KindUnknown
}

// looksLikeColor applies a naming heuristic for the many colour options that
// the stub types merely as `string`.
func looksLikeColor(name string) bool {
	n := strings.ToLower(name)
	switch {
	case strings.HasPrefix(n, "col"):
		return true
	case strings.Contains(n, "color"):
		return true
	case strings.HasSuffix(n, "_border"):
		return true
	case n == "tint" || n == "splash":
		return true
	}
	return false
}

// Lookup returns the option node for a dotted path, or nil.
func (s *Schema) Lookup(path string) *Option {
	for _, sec := range s.Sections {
		if sec.Path == path {
			return sec
		}
		if found := lookupIn(sec, path); found != nil {
			return found
		}
	}
	return nil
}

func lookupIn(o *Option, path string) *Option {
	for _, c := range o.Children {
		if c.Path == path {
			return c
		}
		if found := lookupIn(c, path); found != nil {
			return found
		}
	}
	return nil
}

// Flatten returns every leaf option in the schema.
func (s *Schema) Flatten() []*Option {
	var out []*Option
	var walk func([]*Option)
	walk = func(nodes []*Option) {
		for _, n := range nodes {
			if n.IsSection() {
				walk(n.Children)
				continue
			}
			out = append(out, n)
		}
	}
	walk(s.Sections)
	return out
}

// SectionNames returns the top-level section names in order.
func (s *Schema) SectionNames() []string {
	out := make([]string, 0, len(s.Sections))
	for _, sec := range s.Sections {
		out = append(out, sec.Name)
	}
	return out
}
