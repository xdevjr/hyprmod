// Package luasrc provides a lossless, position-aware view of Lua configuration
// source and the surgical edit operations HyprMod needs.
//
// HyprMod edits the user's hyprland.lua in place. Doing that faithfully requires
// knowing the exact byte span of every value it wants to rewrite, while leaving
// comments, whitespace and unrelated code untouched. tree-sitter would provide
// this, but a purpose-built lexer is dependency-free, requires no CGo and is
// far easier to test; HyprMod only ever needs to understand the declarative
// subset of Lua (calls to hl.* with literal table arguments) that configuration
// files are made of.
package luasrc

// Kind classifies a lexical token.
type Kind int

const (
	// KindEOF marks the end of input.
	KindEOF Kind = iota
	// KindName is an identifier or keyword.
	KindName
	// KindNumber is a numeric literal.
	KindNumber
	// KindString is a short or long string literal.
	KindString
	// KindComment is a line or block comment.
	KindComment
	// KindSymbol is punctuation or an operator.
	KindSymbol
)

// Token is a lexical token with its exact byte span.
type Token struct {
	Kind  Kind
	Text  string
	Start int
	End   int
	Line  int
}

// isSig reports whether a token participates in structural analysis.
func (t Token) isSig() bool { return t.Kind != KindComment && t.Kind != KindEOF }

// Lua keywords, used only to distinguish `true`/`false`/`nil` literals from
// ordinary identifiers.
var keywords = map[string]bool{
	"and": true, "break": true, "do": true, "else": true, "elseif": true,
	"end": true, "false": true, "for": true, "function": true, "goto": true,
	"if": true, "in": true, "local": true, "nil": true, "not": true,
	"or": true, "repeat": true, "return": true, "then": true, "true": true,
	"until": true, "while": true,
}

// IsBoolOrNil reports whether a name token is a literal `true`, `false` or
// `nil`.
func IsBoolOrNil(s string) bool { return s == "true" || s == "false" || s == "nil" }

// Lex tokenises Lua source. It never fails: unrecognised bytes are emitted as
// single-character symbols so that spans stay aligned with the input.
func Lex(src string) []Token {
	var toks []Token
	line := 1
	i := 0
	n := len(src)

	advance := func(to int) {
		for k := i; k < to && k < n; k++ {
			if src[k] == '\n' {
				line++
			}
		}
		i = to
	}

	for i < n {
		c := src[i]

		// Whitespace.
		if c == ' ' || c == '\t' || c == '\r' || c == '\n' || c == '\v' || c == '\f' {
			advance(i + 1)
			continue
		}

		start := i
		startLine := line

		// Comments: `--` line or `--[[ ... ]]` / `--[==[ ... ]==]` block.
		if c == '-' && i+1 < n && src[i+1] == '-' {
			if lvl, ok := longBracketLevel(src, i+2); ok {
				end := scanLongBracket(src, i+2, lvl)
				advance(end)
				toks = append(toks, Token{Kind: KindComment, Text: src[start:end], Start: start, End: end, Line: startLine})
				continue
			}
			j := i
			for j < n && src[j] != '\n' {
				j++
			}
			advance(j)
			toks = append(toks, Token{Kind: KindComment, Text: src[start:j], Start: start, End: j, Line: startLine})
			continue
		}

		// Long strings: [[ ... ]] / [==[ ... ]==].
		if c == '[' {
			if lvl, ok := longBracketLevel(src, i); ok {
				end := scanLongBracket(src, i, lvl)
				advance(end)
				toks = append(toks, Token{Kind: KindString, Text: src[start:end], Start: start, End: end, Line: startLine})
				continue
			}
		}

		// Short strings.
		if c == '"' || c == '\'' {
			j := i + 1
			for j < n {
				if src[j] == '\\' {
					j += 2
					continue
				}
				if src[j] == c {
					j++
					break
				}
				if src[j] == '\n' {
					break
				}
				j++
			}
			if j > n {
				j = n
			}
			advance(j)
			toks = append(toks, Token{Kind: KindString, Text: src[start:j], Start: start, End: j, Line: startLine})
			continue
		}

		// Numbers: decimal, hex, exponents, and leading-dot floats.
		if isDigit(c) || (c == '.' && i+1 < n && isDigit(src[i+1])) {
			j := scanNumber(src, i)
			advance(j)
			toks = append(toks, Token{Kind: KindNumber, Text: src[start:j], Start: start, End: j, Line: startLine})
			continue
		}

		// Identifiers and keywords.
		if isNameStart(c) {
			j := i + 1
			for j < n && isNameChar(src[j]) {
				j++
			}
			advance(j)
			toks = append(toks, Token{Kind: KindName, Text: src[start:j], Start: start, End: j, Line: startLine})
			continue
		}

		// Multi-character symbols.
		if end, ok := matchSymbol(src, i); ok {
			advance(end)
			toks = append(toks, Token{Kind: KindSymbol, Text: src[start:end], Start: start, End: end, Line: startLine})
			continue
		}

		// Any other byte: emit as a symbol so offsets remain aligned.
		advance(i + 1)
		toks = append(toks, Token{Kind: KindSymbol, Text: src[start:i], Start: start, End: i, Line: startLine})
	}

	toks = append(toks, Token{Kind: KindEOF, Start: n, End: n, Line: line})
	return toks
}

func isDigit(c byte) bool { return c >= '0' && c <= '9' }

func isNameStart(c byte) bool {
	return c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || c >= 0x80
}

func isNameChar(c byte) bool { return isNameStart(c) || isDigit(c) }

func isHex(c byte) bool {
	return isDigit(c) || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')
}

func scanNumber(src string, i int) int {
	n := len(src)
	j := i
	if src[j] == '0' && j+1 < n && (src[j+1] == 'x' || src[j+1] == 'X') {
		j += 2
		for j < n && (isHex(src[j]) || src[j] == '.') {
			j++
		}
		if j < n && (src[j] == 'p' || src[j] == 'P') {
			j++
			if j < n && (src[j] == '+' || src[j] == '-') {
				j++
			}
			for j < n && isDigit(src[j]) {
				j++
			}
		}
		return j
	}
	for j < n && (isDigit(src[j]) || src[j] == '_') {
		j++
	}
	if j < n && src[j] == '.' {
		j++
		for j < n && (isDigit(src[j]) || src[j] == '_') {
			j++
		}
	}
	if j < n && (src[j] == 'e' || src[j] == 'E') {
		k := j + 1
		if k < n && (src[k] == '+' || src[k] == '-') {
			k++
		}
		if k < n && isDigit(src[k]) {
			j = k
			for j < n && isDigit(src[j]) {
				j++
			}
		}
	}
	return j
}

// longBracketLevel checks for `[`, `[=`, `[==` ... followed by `[` and returns
// the number of `=` plus 2 (the total bracket length), or ok=false.
func longBracketLevel(src string, i int) (int, bool) {
	if i >= len(src) || src[i] != '[' {
		return 0, false
	}
	j := i + 1
	for j < len(src) && src[j] == '=' {
		j++
	}
	if j < len(src) && src[j] == '[' {
		return j - i + 1, true
	}
	return 0, false
}

// scanLongBracket returns the end offset (exclusive) of the long bracket whose
// opening starts at i and whose total bracket width is width.
func scanLongBracket(src string, i, width int) int {
	level := width - 2 // number of '='
	// Closing delimiter: ']' + level '=' + ']'.
	closeSeq := make([]byte, 0, level+2)
	closeSeq = append(closeSeq, ']')
	for k := 0; k < level; k++ {
		closeSeq = append(closeSeq, '=')
	}
	closeSeq = append(closeSeq, ']')
	body := i + width
	for j := body; j < len(src); j++ {
		if src[j] == ']' && j+len(closeSeq) <= len(src) && src[j:j+len(closeSeq)] == string(closeSeq) {
			return j + len(closeSeq)
		}
	}
	return len(src)
}

// multiSymbols must be matched longest-first.
var multiSymbols = []string{"...", "..", "==", "~=", "<=", ">=", "//", "<<", ">>", "::"}

func matchSymbol(src string, i int) (int, bool) {
	for _, s := range multiSymbols {
		if i+len(s) <= len(src) && src[i:i+len(s)] == s {
			return i + len(s), true
		}
	}
	return 0, false
}
