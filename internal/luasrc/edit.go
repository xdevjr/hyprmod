package luasrc

import (
	"fmt"
	"sort"
	"strings"
)

// Marker is the comment prefix HyprMod writes before blocks it owns, so that
// they can be found and removed later without touching user code.
const Marker = "-- hyprmod:"

// Edit is a single byte-range replacement in a source file.
type Edit struct {
	Start, End int
	NewText    string
}

// Apply applies edits to src. Edits must not overlap; they are applied from the
// end of the file backwards so earlier offsets stay valid.
func Apply(src string, edits ...Edit) string {
	sort.SliceStable(edits, func(i, j int) bool { return edits[i].Start > edits[j].Start })
	out := src
	for _, e := range edits {
		if e.Start < 0 || e.End > len(out) || e.Start > e.End {
			continue
		}
		out = out[:e.Start] + e.NewText + out[e.End:]
	}
	return out
}

// SetConfigValue rewrites `path` to `literal` in place when the existing value
// is a literal. When the value is dynamic, absent, or split across constructs,
// it appends an override block instead and reports inPlace=false.
func (f *File) SetConfigValue(path, literal string) (edits []Edit, inPlace bool) {
	if fld := f.FindConfigValue(path); fld != nil && fld.Literal && !f.hasComment(fld.ValStart, fld.ValEnd) {
		return []Edit{{Start: fld.ValStart, End: fld.ValEnd, NewText: literal}}, true
	}
	if fld := f.FindConfigValue(path); fld != nil && fld.Table != nil && !fld.Literal {
		// A dynamic table: overriding the whole table would destroy user logic.
		return f.appendOverride(path, literal), false
	}
	// Not found anywhere: try to insert into an existing section table, else
	// append a full override block.
	if ins, ok := f.insertConfigValue(path, literal); ok {
		return []Edit{ins}, true
	}
	return f.appendOverride(path, literal), false
}

func (f *File) hasComment(start, end int) bool {
	for _, tok := range f.Tokens {
		if tok.Kind == KindComment && tok.Start >= start && tok.End <= end {
			return true
		}
	}
	return false
}

// insertConfigValue inserts a new key into an existing section table when the
// parent section is a literal table, producing tidier output than an override.
func (f *File) insertConfigValue(path, literal string) (Edit, bool) {
	parts := strings.Split(path, ".")
	key := parts[len(parts)-1]
	parent := parts[:len(parts)-1]
	calls := f.ConfigCalls()
	for i := len(calls) - 1; i >= 0; i-- {
		t := descendTable(calls[i].Table, parent)
		if t == nil {
			continue
		}
		if t.Field(key) != nil {
			continue // exists but dynamic: caller handles override
		}
		return f.insertIntoTable(t, key, literal), true
	}
	return Edit{}, false
}

func descendTable(t *Table, parts []string) *Table {
	for _, p := range parts {
		if t == nil {
			return nil
		}
		fld := t.Field(p)
		if fld == nil {
			return nil
		}
		t = fld.Table
	}
	return t
}

// insertIntoTable produces an edit that adds `key = literal` before the table's
// closing brace, preserving the table's indentation.
func (f *File) insertIntoTable(t *Table, key, literal string) Edit {
	insertAt, multiline, _, childIndent := f.tableInsertPoint(t)
	sep := ""
	if t.Close-1 >= 0 {
		last := f.Sig[t.Close-1].Text
		if last != "{" && last != "," && last != ";" {
			sep = ","
		}
	}
	empty := f.Sig[t.Open+1].Text == "}"
	var text string
	if empty {
		if multiline {
			text = fmt.Sprintf("%s%s = %s,\n", childIndent, key, literal)
		} else {
			text = fmt.Sprintf("%s = %s", key, literal)
		}
	} else if multiline {
		if sep != "" {
			sep += "\n"
		}
		text = fmt.Sprintf("%s%s%s = %s,\n", sep, childIndent, key, literal)
	} else {
		text = fmt.Sprintf("%s %s = %s", sep, key, literal)
	}
	return Edit{Start: insertAt, End: insertAt, NewText: text}
}

// indentFor returns (braceIndent, childIndent) for a table, derived from the
// line on which its closing brace sits.
func (f *File) indentFor(t *Table) (string, string) {
	lineStart := strings.LastIndexByte(f.Src[:t.End], '\n') + 1
	prefix := f.Src[lineStart : t.End-1]
	if strings.TrimSpace(prefix) != "" {
		// Closing brace shares a line with content; fall back to detecting the
		// indentation of the opening brace.
		lineStart = strings.LastIndexByte(f.Src[:t.Start], '\n') + 1
		prefix = f.Src[lineStart:t.Start]
	}
	indent := leadingWhitespace(prefix)
	return indent, indent + "    "
}

func (f *File) tableInsertPoint(t *Table) (insertAt int, multiline bool, braceIndent, childIndent string) {
	closeOffset := t.End - 1
	lineStart := strings.LastIndexByte(f.Src[:closeOffset], '\n') + 1
	prefix := f.Src[lineStart:closeOffset]
	if strings.TrimSpace(prefix) == "" {
		braceIndent = leadingWhitespace(prefix)
		return lineStart, true, braceIndent, braceIndent + "    "
	}
	braceIndent, childIndent = f.indentFor(t)
	return closeOffset, false, braceIndent, childIndent
}

func leadingWhitespace(s string) string {
	i := 0
	for i < len(s) && (s[i] == ' ' || s[i] == '\t') {
		i++
	}
	return s[:i]
}

// appendOverride builds an `hl.config({...})` block carrying a marker comment.
func (f *File) appendOverride(path, literal string) []Edit {
	parts := strings.Split(path, ".")
	var b strings.Builder
	b.WriteString("\n" + Marker + "override " + path + "\n")
	b.WriteString("hl.config({\n")
	for i, p := range parts {
		ind := strings.Repeat("    ", i+1)
		if i == len(parts)-1 {
			b.WriteString(fmt.Sprintf("%s%s = %s,\n", ind, p, literal))
		} else {
			b.WriteString(fmt.Sprintf("%s%s = {\n", ind, p))
		}
	}
	for i := len(parts) - 2; i >= 0; i-- {
		b.WriteString(strings.Repeat("    ", i+1) + "},\n")
	}
	b.WriteString("})\n")
	pos := f.appendPosition()
	return []Edit{{Start: pos, End: pos, NewText: b.String()}}
}

// AppendMarkedCall appends a call block with an ownership marker, used to add
// monitors, binds, rules, curves and similar list entries.
func (f *File) AppendMarkedCall(id, callText string) Edit {
	text := "\n" + Marker + "add " + id + "\n" + strings.TrimRight(callText, "\n") + "\n"
	pos := f.appendPosition()
	return Edit{Start: pos, End: pos, NewText: text}
}

// appendPosition returns EOF unless the file has a top-level return statement.
// Lua requires return to be the final statement in its block, so generated
// declarations must be inserted immediately before it in module files.
func (f *File) appendPosition() int {
	depth := 0
	pendingDo := 0
	for _, tok := range f.Sig {
		switch tok.Text {
		case "function", "if", "repeat":
			depth++
		case "for", "while":
			depth++
			pendingDo++
		case "do":
			if pendingDo > 0 {
				pendingDo--
			} else {
				depth++
			}
		case "end", "until":
			if depth > 0 {
				depth--
			}
		case "return":
			if depth == 0 {
				lineStart := strings.LastIndexByte(f.Src[:tok.Start], '\n') + 1
				if strings.TrimSpace(f.Src[lineStart:tok.Start]) == "" {
					return lineStart
				}
				return tok.Start
			}
		}
	}
	return len(f.Src)
}

// RemoveMarkedBlock deletes a previously appended block identified by
// `<op> <id>` (for example "override general.gaps_in" or "add monitor eDP-1").
// It returns ok=false when no such block exists.
func (f *File) RemoveMarkedBlock(op, id string) (Edit, bool) {
	want := Marker + op + " " + id
	for _, tok := range f.Tokens {
		if tok.Kind != KindComment || strings.TrimSpace(tok.Text) != want {
			continue
		}
		// Extend the deletion to the end of the following statement, which ends
		// at the first `)` closing the call after the marker.
		end := tok.End
		sawCall := false
		for _, c := range f.Calls() {
			if c.ByteStart >= tok.End {
				sawCall = true
				end = c.ByteEnd
				break
			}
		}
		if !sawCall {
			// Marker without a following call: delete just the comment line.
			for end < len(f.Src) && f.Src[end] != '\n' {
				end++
			}
		}
		// Swallow the blank line the block introduced.
		for end < len(f.Src) && f.Src[end] == '\n' {
			end++
		}
		// Swallow one leading newline so the file does not gain blank lines.
		start := tok.Start
		for start > 0 && (f.Src[start-1] == ' ' || f.Src[start-1] == '\t') {
			start--
		}
		if start > 0 && f.Src[start-1] == '\n' {
			start--
		}
		return Edit{Start: start, End: end, NewText: ""}, true
	}
	return Edit{}, false
}

// MatchSpec matches a call's table against expected literal field values.
// Only literal field values are compared; string literals are unquoted.
func MatchSpec(t *Table, want map[string]string) bool {
	for k, v := range want {
		fld := t.Field(k)
		if fld == nil || !fld.Literal {
			return false
		}
		if unquote(fld.ValText) != v && fld.ValText != v {
			return false
		}
	}
	return true
}

// SetCallField rewrites field `field` to `literal` in the table passed to a call
// named callName that matches `want`. When no matching call exists it appends a
// new marked call built by buildNew.
func (f *File) SetCallField(callName, id string, want map[string]string, field, literal string, buildNew func() string) (edits []Edit, inPlace bool) {
	for _, t := range f.FindTableFields(callName) {
		if !MatchSpec(t, want) {
			continue
		}
		if fld := t.Field(field); fld != nil && fld.Literal {
			return []Edit{{Start: fld.ValStart, End: fld.ValEnd, NewText: literal}}, true
		}
		if t.Field(field) == nil {
			return []Edit{f.insertIntoTable(t, field, literal)}, true
		}
		// Present but dynamic: fall through to append a whole new call that
		// overrides it (last definition wins in Hyprland).
		edits = []Edit{f.AppendMarkedCall(id, buildNew())}
		return edits, false
	}
	return []Edit{f.AppendMarkedCall(id, buildNew())}, false
}

// SetCallFields updates several fields on one matching table argument in a
// single edit batch. If any requested field is computed, or no matching call
// exists, one marked call containing every new field is appended.
func (f *File) SetCallFields(callName, id string, want map[string]string, fields map[string]string, buildNew func() string) (edits []Edit, inPlace bool) {
	calls := f.FindCalls(callName)
	for i := len(calls) - 1; i >= 0; i-- {
		t := calls[i].FirstTable()
		if t == nil || !MatchSpec(t, want) {
			continue
		}
		var replacements []Edit
		var missing []KV
		for key, literal := range fields {
			if fld := t.Field(key); fld != nil {
				if !fld.Literal {
					return []Edit{f.AppendMarkedCall(id, buildNew())}, false
				}
				replacements = append(replacements, Edit{Start: fld.ValStart, End: fld.ValEnd, NewText: literal})
			} else {
				missing = append(missing, KV{Key: key, Value: literal})
			}
		}
		if len(missing) > 0 {
			sort.Slice(missing, func(i, j int) bool { return missing[i].Key < missing[j].Key })
			replacements = append(replacements, f.insertManyIntoTable(t, missing))
		}
		return replacements, true
	}
	return []Edit{f.AppendMarkedCall(id, buildNew())}, false
}

func (f *File) insertManyIntoTable(t *Table, fields []KV) Edit {
	insertAt, multiline, _, childIndent := f.tableInsertPoint(t)
	last := f.Sig[t.Close-1].Text
	sep := ""
	if last != "{" && last != "," && last != ";" {
		sep = ","
	}
	var b strings.Builder
	if len(t.Fields) == 0 && !multiline {
		for i, field := range fields {
			if i > 0 {
				b.WriteString(", ")
			}
			b.WriteString(field.Key + " = " + field.Value)
		}
	} else if multiline {
		if sep != "" {
			b.WriteString(sep)
			b.WriteByte('\n')
		}
		for _, field := range fields {
			b.WriteString(childIndent + field.Key + " = " + field.Value + ",\n")
		}
	} else {
		b.WriteString(sep)
		for i, field := range fields {
			if i > 0 {
				b.WriteString(", ")
			} else {
				b.WriteByte(' ')
			}
			b.WriteString(field.Key + " = " + field.Value)
		}
	}
	return Edit{Start: insertAt, End: insertAt, NewText: b.String()}
}

// TableText renders a flat map of key -> literal into a Lua table body.
func TableText(fields []KV) string {
	var b strings.Builder
	b.WriteString("{\n")
	for _, kv := range fields {
		b.WriteString("    " + kv.Key + " = " + kv.Value + ",\n")
	}
	b.WriteString("}")
	return b.String()
}

// KV is an ordered key/value pair used when generating Lua tables.
type KV struct {
	Key   string
	Value string
}
