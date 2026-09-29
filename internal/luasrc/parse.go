package luasrc

import (
	"sort"
	"strings"
)

// File is a parsed Lua source file. It keeps the original text so that every
// span can be sliced back out or rewritten.
type File struct {
	Src    string
	Tokens []Token
	Sig    []Token // significant tokens only (no comments/EOF)
}

// NewFile lexes src and builds the significant-token view.
func NewFile(src string) *File {
	toks := Lex(src)
	f := &File{Src: src, Tokens: toks}
	for _, t := range toks {
		if t.isSig() {
			f.Sig = append(f.Sig, t)
		}
	}
	return f
}

// LineAt returns the 1-based line for a byte offset.
func (f *File) LineAt(off int) int {
	line := 1
	for _, t := range f.Tokens {
		if t.Kind == KindComment {
			continue
		}
		if t.Start > off {
			break
		}
		line = t.Line
	}
	return line
}

// Field is a key/value pair inside a table constructor.
type Field struct {
	Key      string // dotted key name, or "" for a positional entry
	KeyStart int
	KeyEnd   int
	ValStart int // byte offset
	ValEnd   int // exclusive
	ValLine  int // 1-based source line of the value
	ValText  string
	Literal  bool
	Table    *Table // set when the value is a table constructor
}

// Table is a `{ ... }` constructor with its fields.
type Table struct {
	Start, End  int // byte offsets of the braces
	Open, Close int // significant-token indices of `{` and `}`
	Fields      []*Field
}

// Field returns the field with the given key, or nil.
func (t *Table) Field(key string) *Field {
	for _, f := range t.Fields {
		if f.Key == key {
			return f
		}
	}
	return nil
}

// Arg is one positional argument of a call.
type Arg struct {
	Start, End int // significant-token indices, inclusive
	ByteStart  int
	ByteEnd    int
	Text       string
	Table      *Table
}

// Call is a call expression such as `hl.config({...})`.
type Call struct {
	Name        string // dotted name, e.g. "hl.config" or "hl.dsp.window.close"
	NameStart   int
	NameEnd     int
	OpenTok     int // index in Sig of `(`
	CloseTok    int // index in Sig of `)`
	ByteStart   int
	ByteEnd     int
	Args        []Arg
	Line        int
	IsStatement bool
}

// FirstTable returns the first argument's table, if any.
func (c *Call) FirstTable() *Table {
	if len(c.Args) == 0 {
		return nil
	}
	return c.Args[0].Table
}

// Calls returns every call expression in the file, in source order.
func (f *File) Calls() []*Call {
	var calls []*Call
	for i := 0; i < len(f.Sig); i++ {
		if f.Sig[i].Kind != KindName {
			continue
		}
		name, ni, ok := f.dottedName(i)
		if !ok {
			continue
		}
		if ni >= len(f.Sig) || f.Sig[ni].Text != "(" {
			continue
		}
		closeTok := f.matchParen(ni)
		if closeTok < 0 {
			continue
		}
		c := &Call{
			Name:      name,
			NameStart: f.Sig[i].Start,
			NameEnd:   f.Sig[ni-1].End,
			OpenTok:   ni,
			CloseTok:  closeTok,
			ByteStart: f.Sig[i].Start,
			ByteEnd:   f.Sig[closeTok].End,
			Line:      f.Sig[i].Line,
		}
		c.Args = f.splitArgs(ni, closeTok)
		calls = append(calls, c)
		i = closeTok
	}
	return calls
}

// dottedName consumes `a.b.c` starting at sig index i and returns the dotted
// name and the index just past it.
func (f *File) dottedName(i int) (string, int, bool) {
	if i >= len(f.Sig) || f.Sig[i].Kind != KindName {
		return "", i, false
	}
	var parts []string
	parts = append(parts, f.Sig[i].Text)
	j := i + 1
	for j+1 < len(f.Sig) && f.Sig[j].Text == "." && f.Sig[j+1].Kind == KindName {
		parts = append(parts, f.Sig[j+1].Text)
		j += 2
	}
	return strings.Join(parts, "."), j, true
}

// matchParen returns the significant-token index of the `)` matching the `(` at
// openIdx, or -1.
func (f *File) matchParen(openIdx int) int {
	depth := 0
	for i := openIdx; i < len(f.Sig); i++ {
		switch f.Sig[i].Text {
		case "(", "{", "[":
			depth++
		case ")", "}", "]":
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}

// splitArgs splits the argument list between openIdx and closeIdx into
// positional arguments. Block keywords are tracked so that a comma inside a
// callback body (a for loop, for instance) does not split the argument.
func (f *File) splitArgs(openIdx, closeIdx int) []Arg {
	var args []Arg
	if openIdx+1 > closeIdx-1 {
		return args
	}
	start := openIdx + 1
	depth := 0
	blocks := 0
	flush := func(endExclusive int) {
		if endExclusive <= start {
			return
		}
		a := Arg{Start: start, End: endExclusive - 1}
		a.ByteStart = f.Sig[start].Start
		a.ByteEnd = f.Sig[endExclusive-1].End
		a.Text = f.Src[a.ByteStart:a.ByteEnd]
		if f.Sig[start].Text == "{" {
			if t := f.parseTable(start); t != nil && t.Close < endExclusive {
				a.Table = t
			}
		}
		args = append(args, a)
	}
	for i := start; i < closeIdx; i++ {
		switch f.Sig[i].Text {
		case "(", "{", "[":
			depth++
		case ")", "}", "]":
			depth--
		case "function", "if", "for", "while", "repeat":
			blocks++
		case "end", "until":
			if blocks > 0 {
				blocks--
			}
		case ",":
			if depth == 0 && blocks == 0 {
				flush(i)
				start = i + 1
			}
		}
	}
	flush(closeIdx)
	return args
}

// parseTable parses the table constructor whose `{` is at sig index openIdx.
func (f *File) parseTable(openIdx int) *Table {
	if openIdx >= len(f.Sig) || f.Sig[openIdx].Text != "{" {
		return nil
	}
	closeIdx := f.matchBrace(openIdx)
	if closeIdx < 0 {
		return nil
	}
	t := &Table{
		Start: f.Sig[openIdx].Start,
		End:   f.Sig[closeIdx].End,
		Open:  openIdx,
		Close: closeIdx,
	}
	i := openIdx + 1
	for i < closeIdx {
		if f.Sig[i].Text == "," || f.Sig[i].Text == ";" {
			i++
			continue
		}
		field := &Field{}
		// Keyed entry?
		switch {
		case f.Sig[i].Kind == KindName && i+1 < closeIdx && f.Sig[i+1].Text == "=":
			field.Key = f.Sig[i].Text
			field.KeyStart = f.Sig[i].Start
			field.KeyEnd = f.Sig[i].End
			i += 2
		case f.Sig[i].Text == "[":
			br := f.matchBracket(i)
			if br > 0 && br+1 < closeIdx && f.Sig[br+1].Text == "=" {
				// String-literal keys are normalised to their value so that
				// ["gaps_in"] and gaps_in address the same option.
				keyTok := f.Sig[i+1]
				if keyTok.Kind == KindString && br == i+2 {
					field.Key = unquote(keyTok.Text)
				} else {
					field.Key = ""
				}
				field.KeyStart = f.Sig[i].Start
				field.KeyEnd = f.Sig[br].End
				i = br + 2
			}
		}
		// Value span: from i until a depth-0 `,`, `;` or the closing brace.
		vStart := i
		depth := 0
		j := i
		for j < closeIdx {
			switch f.Sig[j].Text {
			case "(", "{", "[":
				depth++
			case ")", "}", "]":
				depth--
			case ",", ";":
				if depth == 0 {
					goto done
				}
			}
			j++
		}
	done:
		if vStart >= j {
			break
		}
		field.ValStart = f.Sig[vStart].Start
		field.ValEnd = f.Sig[j-1].End
		field.ValLine = f.Sig[vStart].Line
		field.ValText = f.Src[field.ValStart:field.ValEnd]
		if f.Sig[vStart].Text == "{" {
			if nt := f.parseTable(vStart); nt != nil && nt.Close == j-1 {
				field.Table = nt
			}
		}
		lit, _ := f.literal(vStart, j-1)
		field.Literal = lit
		t.Fields = append(t.Fields, field)
		i = j
	}
	return t
}

// matchBrace returns the index of the `}` matching `{` at openIdx.
func (f *File) matchBrace(openIdx int) int {
	depth := 0
	for i := openIdx; i < len(f.Sig); i++ {
		switch f.Sig[i].Text {
		case "{", "(", "[":
			depth++
		case "}", ")", "]":
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}

func (f *File) matchBracket(openIdx int) int {
	depth := 0
	for i := openIdx; i < len(f.Sig); i++ {
		switch f.Sig[i].Text {
		case "{", "(", "[":
			depth++
		case "}", ")", "]":
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}

// literal reports whether the value spanning sig indices [start,end] (inclusive)
// is a literal HyprMod can rewrite in place, and its kind.
func (f *File) literal(start, end int) (bool, string) {
	if start > end || end >= len(f.Sig) {
		return false, ""
	}
	// Unary minus on a number, e.g. -1.
	if f.Sig[start].Text == "-" && start+1 == end && f.Sig[start+1].Kind == KindNumber {
		return true, "number"
	}
	if start != end {
		// Multi-token values are only literal when they are a single table.
		if f.Sig[start].Text == "{" && f.Sig[end].Text == "}" {
			t := f.parseTable(start)
			if t == nil || t.Close != end {
				return false, ""
			}
			return tableLiteral(t), "table"
		}
		return false, ""
	}
	tok := f.Sig[start]
	switch tok.Kind {
	case KindNumber:
		return true, "number"
	case KindString:
		return true, "string"
	case KindName:
		if IsBoolOrNil(tok.Text) {
			return true, tok.Text
		}
	}
	return false, ""
}

func tableLiteral(t *Table) bool {
	for _, fld := range t.Fields {
		if fld.Literal {
			continue
		}
		if fld.Table != nil && tableLiteral(fld.Table) {
			continue
		}
		return false
	}
	return true
}

func unquote(s string) string {
	if len(s) >= 2 {
		if (s[0] == '"' && s[len(s)-1] == '"') || (s[0] == '\'' && s[len(s)-1] == '\'') {
			return s[1 : len(s)-1]
		}
	}
	return s
}

// RequireTargets returns the literal module names passed to require(...), in
// order, and whether any non-literal require exists.
func (f *File) RequireTargets() (targets []string, dynamic bool) {
	for _, c := range f.Calls() {
		if c.Name != "require" || len(c.Args) == 0 {
			continue
		}
		a := c.Args[0]
		if a.Start == a.End && f.Sig[a.Start].Kind == KindString {
			targets = append(targets, unquote(f.Sig[a.Start].Text))
			continue
		}
		dynamic = true
	}
	return targets, dynamic
}

// ConfigCall describes an `hl.config` invocation.
type ConfigCall struct {
	Call  *Call
	Table *Table
}

// ConfigCalls returns all `hl.config` calls whose first argument is a table.
func (f *File) ConfigCalls() []*ConfigCall {
	var out []*ConfigCall
	for _, c := range f.Calls() {
		if c.Name != "hl.config" {
			continue
		}
		t := c.FirstTable()
		if t == nil {
			continue
		}
		out = append(out, &ConfigCall{Call: c, Table: t})
	}
	return out
}

// FindConfigValue locates a dotted option path inside any hl.config call.
// It returns the deepest field that holds the value.
func (f *File) FindConfigValue(path string) *Field {
	parts := strings.Split(path, ".")
	calls := f.ConfigCalls()
	for i := len(calls) - 1; i >= 0; i-- {
		if fld := descend(calls[i].Table, parts); fld != nil {
			return fld
		}
	}
	return nil
}

// FindConfigValueAt locates path in the hl.config call whose call expression
// starts on line callLine. This associates sandbox-resolved values with the
// exact source declaration, even when a file contains multiple hl.config calls.
func (f *File) FindConfigValueAt(path string, callLine int) *Field {
	parts := strings.Split(path, ".")
	for _, cc := range f.ConfigCalls() {
		if cc.Call.Line != callLine {
			continue
		}
		if fld := descend(cc.Table, parts); fld != nil {
			return fld
		}
	}
	return nil
}

func descend(t *Table, parts []string) *Field {
	if t == nil || len(parts) == 0 {
		return nil
	}
	fld := t.Field(parts[0])
	if fld == nil {
		return nil
	}
	if len(parts) == 1 {
		return fld
	}
	if fld.Table == nil {
		return nil
	}
	return descend(fld.Table, parts[1:])
}

// FindCalls returns every call with the given dotted name.
func (f *File) FindCalls(name string) []*Call {
	var out []*Call
	for _, c := range f.Calls() {
		if c.Name == name {
			out = append(out, c)
		}
	}
	return out
}

// FindCallWithFirstArg returns the first call named callName whose first
// argument is a string literal equal to want, ignoring case, spacing and the
// order of the " + " separated parts.
func (f *File) FindCallWithFirstArg(callName, want string) *Call {
	target := NormalizeCombo(want)
	if target == "" {
		return nil
	}
	calls := f.FindCalls(callName)
	for i := len(calls) - 1; i >= 0; i-- {
		call := calls[i]
		if len(call.Args) == 0 {
			continue
		}
		arg := call.Args[0]
		if arg.Start != arg.End {
			continue // only literal single-token arguments are matched
		}
		if NormalizeCombo(unquote(f.Sig[arg.Start].Text)) == target {
			return call
		}
	}
	return nil
}

// NormalizeCombo canonicalises a key combination so equivalent spellings
// compare equal: case, spacing, " + " versus "+" and modifier order.
func NormalizeCombo(combo string) string {
	parts := strings.FieldsFunc(strings.ToUpper(combo), func(r rune) bool { return r == '+' || r == ' ' })
	cleaned := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		switch part {
		case "CONTROL", "CTRL":
			part = "CTRL"
		case "META", "SUPER", "MOD4", "LOGO", "WIN":
			part = "SUPER"
		case "":
			continue
		}
		cleaned = append(cleaned, part)
	}
	sort.Strings(cleaned)
	return strings.Join(cleaned, "+")
}

// FindTableFields returns the tables passed as first argument to calls named
// name, in source order.
func (f *File) FindTableFields(name string) []*Table {
	var out []*Table
	for _, c := range f.FindCalls(name) {
		if t := c.FirstTable(); t != nil {
			out = append(out, t)
		}
	}
	return out
}

// SortedFieldKeys returns the field keys of a table sorted for stable output.
func SortedFieldKeys(t *Table) []string {
	keys := make([]string, 0, len(t.Fields))
	for _, f := range t.Fields {
		keys = append(keys, f.Key)
	}
	sort.Strings(keys)
	return keys
}
