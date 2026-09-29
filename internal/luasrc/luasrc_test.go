package luasrc

import (
	"strings"
	"testing"
)

func TestLexCommentsStringsAndLongBrackets(t *testing.T) {
	src := `-- line comment
local a = "escaped \\\" string" -- tail
local b = [=[long
string]=]
--[==[ block
comment ]==]
local c = -1.25e-2`
	toks := Lex(src)
	var comments, stringsSeen, numbers []string
	for _, tok := range toks {
		switch tok.Kind {
		case KindComment:
			comments = append(comments, tok.Text)
		case KindString:
			stringsSeen = append(stringsSeen, tok.Text)
		case KindNumber:
			numbers = append(numbers, tok.Text)
		}
	}
	if len(comments) != 3 {
		t.Fatalf("got %d comments: %#v", len(comments), comments)
	}
	if len(stringsSeen) != 2 || stringsSeen[1] != "[=[long\nstring]=]" {
		t.Fatalf("unexpected strings: %#v", stringsSeen)
	}
	if len(numbers) != 1 || numbers[0] != "1.25e-2" {
		t.Fatalf("unexpected numbers: %#v", numbers)
	}
}

func TestConfigTableAnalysis(t *testing.T) {
	src := `hl.config({
    general = {
        gaps_in = 5, -- preserve this
        col = { active_border = { colors = { "#abc", "#def" }, angle = 45 } },
    },
    input = { kb_layout = "br,us" },
})`
	d := NewFile(src)
	if got := len(d.ConfigCalls()); got != 1 {
		t.Fatalf("ConfigCalls=%d, want 1", got)
	}
	for _, tc := range []struct {
		path    string
		literal bool
		want    string
	}{
		{"general.gaps_in", true, "5"},
		{"general.col.active_border", true, `{ colors = { "#abc", "#def" }, angle = 45 }`},
		{"input.kb_layout", true, `"br,us"`},
	} {
		f := d.FindConfigValue(tc.path)
		if f == nil {
			t.Fatalf("missing %s", tc.path)
		}
		if f.Literal != tc.literal {
			t.Errorf("%s Literal=%v, want %v", tc.path, f.Literal, tc.literal)
		}
		if strings.TrimSpace(f.ValText) != tc.want {
			t.Errorf("%s ValText=%q, want %q", tc.path, f.ValText, tc.want)
		}
	}
}

func TestDynamicValuesAreNotLiteral(t *testing.T) {
	d := NewFile(`hl.config({ general = { gaps_in = gapSize, border_size = calculateBorder() } })`)
	for _, path := range []string{"general.gaps_in", "general.border_size"} {
		f := d.FindConfigValue(path)
		if f == nil || f.Literal {
			t.Errorf("%s should be present and dynamic: %#v", path, f)
		}
	}
}

func TestSetConfigValueReplacesLiteralAndPreservesComments(t *testing.T) {
	src := `hl.config({ general = { gaps_in = 5, -- keep me
    border_size = 2 } })`
	d := NewFile(src)
	edits, inPlace := d.SetConfigValue("general.gaps_in", "8")
	if !inPlace || len(edits) != 1 {
		t.Fatalf("expected in-place edit, got inPlace=%v edits=%#v", inPlace, edits)
	}
	out := Apply(src, edits...)
	if !strings.Contains(out, "gaps_in = 8") || !strings.Contains(out, "-- keep me") {
		t.Fatalf("unexpected output:\n%s", out)
	}
}

func TestReplacingTableWithInternalCommentUsesOverride(t *testing.T) {
	src := `hl.config({ general = { col = { active_border = {
    colors = { "red", "blue" }, -- keep this note
    angle = 45,
} } } })`
	d := NewFile(src)
	edits, inPlace := d.SetConfigValue("general.col.active_border", `{ colors = { "green" }, angle = 90 }`)
	if inPlace {
		t.Fatal("replacement should not erase comments inside a table value")
	}
	out := Apply(src, edits...)
	if !strings.Contains(out, "-- keep this note") || !strings.Contains(out, Marker+"override general.col.active_border") {
		t.Fatalf("comment-preserving override missing:\n%s", out)
	}
}

func TestSetDynamicOptionAppendsOverride(t *testing.T) {
	src := `local gap = 4
hl.config({ general = { gaps_in = gap } })
return { gap = gap }`
	d := NewFile(src)
	edits, inPlace := d.SetConfigValue("general.gaps_in", "9")
	if inPlace {
		t.Fatal("dynamic value should use an override")
	}
	out := Apply(src, edits...)
	if !strings.Contains(out, Marker+"override general.gaps_in") || !strings.Contains(out, "gaps_in = 9") {
		t.Fatalf("override missing:\n%s", out)
	}
	if strings.Index(out, Marker+"override general.gaps_in") > strings.Index(out, "return {") {
		t.Fatalf("override was appended after a module return:\n%s", out)
	}
	if f := NewFile(out).FindConfigValue("general.gaps_in"); f == nil || f.ValText != "9" {
		t.Fatalf("latest override not discoverable: %#v", f)
	}
}

func TestSetMissingOptionInsertsIntoSection(t *testing.T) {
	src := `hl.config({ general = { gaps_in = 5, } })`
	d := NewFile(src)
	edits, inPlace := d.SetConfigValue("general.border_size", "2")
	if !inPlace {
		t.Fatal("existing section should be editable in place")
	}
	out := Apply(src, edits...)
	if !strings.Contains(out, "border_size = 2") {
		t.Fatalf("missing inserted key:\n%s", out)
	}
	if got := NewFile(out).FindConfigValue("general.border_size"); got == nil || got.ValText != "2" {
		t.Fatalf("inserted key not parseable: %#v\n%s", got, out)
	}
}

func TestSetCallField(t *testing.T) {
	src := `hl.monitor({ output = "eDP-1", mode = "preferred", scale = 1.0 })`
	d := NewFile(src)
	edits, inPlace := d.SetCallField("hl.monitor", "monitor eDP-1", map[string]string{"output": "eDP-1"}, "scale", "1.25", func() string {
		return `hl.monitor({ output = "eDP-1", scale = 1.25 })`
	})
	if !inPlace {
		t.Fatal("expected in-place monitor edit")
	}
	out := Apply(src, edits...)
	if !strings.Contains(out, "scale = 1.25") {
		t.Fatalf("scale update missing: %s", out)
	}
}

func TestRemoveMarkedBlock(t *testing.T) {
	src := `local gap = 5
hl.config({ general = { gaps_in = gap } })`
	d := NewFile(src)
	edits, _ := d.SetConfigValue("general.gaps_in", "9")
	withOverride := Apply(src, edits...)
	d = NewFile(withOverride)
	remove, ok := d.RemoveMarkedBlock("override", "general.gaps_in")
	if !ok {
		t.Fatal("expected marked override")
	}
	out := Apply(withOverride, remove)
	if strings.Contains(out, Marker+"override") || strings.Contains(out, "gaps_in = 9") {
		t.Fatalf("override still present:\n%s", out)
	}
}

func TestRequireTargets(t *testing.T) {
	d := NewFile(`require("config.animations")
require("awesomeconf/keybinds")
require(moduleName)`)
	targets, dynamic := d.RequireTargets()
	if len(targets) != 2 || !dynamic {
		t.Fatalf("targets=%v dynamic=%v", targets, dynamic)
	}
	if targets[0] != "config.animations" || targets[1] != "awesomeconf/keybinds" {
		t.Fatalf("unexpected targets: %v", targets)
	}
}
