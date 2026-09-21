//go:build !grammar_subset || grammar_subset_dockerfile

package grammarruntime

import gotreesitter "github.com/odvcencio/gotreesitter"

// External token indexes for the dockerfile grammar. This is the external
// index (the position of the token in the grammar's `externals: [...]`
// list), which is exactly what tree-sitter's `valid_symbols` array and
// C's result_symbol enum are indexed by. The external index is stable
// across a blob regen as long as the externals list itself does not
// reorder; concrete numeric gotreesitter.Symbol IDs are NOT stable (they
// shift whenever the grammar's total symbol count changes), so this
// scanner never hardcodes them -- see dockerfileDefaultSymTable below.
const (
	dockerfileTokMarker        = 0 // "heredoc_marker"
	dockerfileTokLine          = 1 // "heredoc_line"
	dockerfileTokEnd           = 2 // "heredoc_end"
	dockerfileTokNL            = 3 // "heredoc_nl", displays as "_heredoc_nl"
	dockerfileTokErrorSentinel = 4 // "error_sentinel"
	dockerfileTokenCount       = 5
)

// dockerfileDefaultSymTable records the concrete gotreesitter.Symbol IDs
// the currently shipped dockerfile.bin assigns to each external, in
// dockerfileTok* order. It exists only as a pre-bind fallback (and as an
// independent value to compare a real bind against in tests);
// ExternalScannerForLanguage below overwrites it with values read from
// the actual loaded Language at bind time, which is what the scanner
// must do to survive a future blob regen that renumbers absolute symbol
// IDs without touching the externals list order.
var dockerfileDefaultSymTable = [dockerfileTokenCount]gotreesitter.Symbol{
	82, // heredoc_marker
	83, // heredoc_line
	84, // heredoc_end
	85, // heredoc_nl (displays as "_heredoc_nl")
	86, // error_sentinel
}

// dockerfileExternalScannerSpec records the source contract for this
// hand-written port, so updater tooling can tell a grammar-only upstream
// change apart from one that also touches the external scanner or its
// token list. Its Externals list is also the binding source for
// ExternalScannerForLanguage: index i here is scanner token index i
// (dockerfileTok* order).
var dockerfileExternalScannerSpec = ExternalScannerSpec{
	Language:       "dockerfile",
	UpstreamRepo:   "https://github.com/camdencheek/tree-sitter-dockerfile",
	UpstreamCommit: "971acdd908568b4531b0ba28a445bf0bb720aba5",
	SourceFiles: []ExternalScannerSourceFile{
		{Path: "src/grammar.json", SHA256: "c7248d23cd9f143958eb25829a7447a864bf7751791ca56660354b04513b4ac8"},
		{Path: "src/scanner.c", SHA256: "1080c2eb2ac41f102974e009cb62f644d9d638dd2468d0a700674d07d346fde7"},
	},
	Externals: []string{
		"heredoc_marker",
		"heredoc_line",
		"heredoc_end",
		"heredoc_nl",
		"error_sentinel",
	},
}

func init() {
	RegisterExternalScannerSpec(dockerfileExternalScannerSpec)
}

const dockerfileMaxHeredocs = 10

// dockerfileScannerState manages the heredoc delimiter stack.
type dockerfileScannerState struct {
	inHeredoc bool
	stripping bool     // <<- mode (strip leading tabs)
	heredocs  []string // stack of delimiter strings
}

// DockerfileExternalScanner implements gotreesitter.ExternalScanner for tree-sitter-dockerfile.
//
// This is a Go port of the C external scanner from camdencheek/tree-sitter-dockerfile.
// The scanner manages Dockerfile heredoc syntax (<<MARKER / <<-MARKER) with a stack
// of up to 10 delimiter strings and handles:
//   - heredoc_marker: the <<[-]DELIM opening
//   - heredoc_line: content lines within a heredoc
//   - heredoc_end: the closing delimiter line
//   - heredoc_nl (displays as "_heredoc_nl"): newlines within heredoc context
//   - error_sentinel: error recovery bail-out
//
// symbols holds the concrete gotreesitter.Symbol each external index maps to
// in the Language this instance was bound to (see ExternalScannerForLanguage).
// The scanner never hardcodes an absolute Symbol value: a blob regen can
// renumber the grammar's absolute symbol IDs without touching the externals
// list order, and a scanner that still called SetResultSymbol with a stale
// hardcoded ID would silently emit the wrong (but still structurally valid)
// node type instead of failing loudly.
type DockerfileExternalScanner struct {
	symbols         [dockerfileTokenCount]gotreesitter.Symbol
	externalToToken []int
}

// ExternalScannerForLanguage binds the scanner's token slots to the loaded
// Language's ExternalSymbols positionally. A hardcoded absolute
// gotreesitter.Symbol constant here would emit the wrong token whenever a
// grammar bump renumbers dockerfile's external symbols.
func (DockerfileExternalScanner) ExternalScannerForLanguage(lang *gotreesitter.Language) gotreesitter.ExternalScanner {
	s := DockerfileExternalScanner{symbols: dockerfileDefaultSymTable}
	s.externalToToken = bindExternalScannerSpec(lang, dockerfileExternalScannerSpec, func(tokenIdx int, sym gotreesitter.Symbol) {
		s.symbols[tokenIdx] = sym
	})
	return s
}

func (s DockerfileExternalScanner) symbolTable() *[dockerfileTokenCount]gotreesitter.Symbol {
	if s.symbols == ([dockerfileTokenCount]gotreesitter.Symbol{}) {
		return &dockerfileDefaultSymTable
	}
	return &s.symbols
}

func (DockerfileExternalScanner) Create() any {
	return &dockerfileScannerState{}
}

func (DockerfileExternalScanner) Destroy(payload any) {}

func (DockerfileExternalScanner) Serialize(payload any, buf []byte) int {
	s := payload.(*dockerfileScannerState)
	if len(buf) < 2 {
		return 0
	}
	pos := 0
	if s.inHeredoc {
		buf[pos] = 1
	} else {
		buf[pos] = 0
	}
	pos++
	if s.stripping {
		buf[pos] = 1
	} else {
		buf[pos] = 0
	}
	pos++

	// Write heredoc delimiters as null-terminated strings.
	for _, delim := range s.heredocs {
		dlen := len(delim) + 1 // include null terminator
		if pos+dlen+1 > len(buf) {
			break
		}
		copy(buf[pos:], delim)
		pos += len(delim)
		buf[pos] = 0
		pos++
	}

	// Double-null terminator to mark end of list.
	if pos < len(buf) {
		buf[pos] = 0
		pos++
	}
	return pos
}

func (DockerfileExternalScanner) Deserialize(payload any, buf []byte) {
	s := payload.(*dockerfileScannerState)
	s.inHeredoc = false
	s.stripping = false
	s.heredocs = s.heredocs[:0]

	if len(buf) == 0 {
		return
	}
	if len(buf) < 2 {
		return
	}

	pos := 0
	s.inHeredoc = buf[pos] != 0
	pos++
	s.stripping = buf[pos] != 0
	pos++

	// Read null-terminated delimiter strings until double-null.
	for pos < len(buf) && len(s.heredocs) < dockerfileMaxHeredocs {
		// Find end of this string.
		start := pos
		for pos < len(buf) && buf[pos] != 0 {
			pos++
		}
		if pos == start {
			break // empty string = end of list
		}
		s.heredocs = append(s.heredocs, string(buf[start:pos]))
		if pos < len(buf) {
			pos++ // skip null terminator
		}
	}
}

func (sc DockerfileExternalScanner) Scan(payload any, lexer *gotreesitter.ExternalLexer, validSymbols []bool) bool {
	s := payload.(*dockerfileScannerState)

	if len(sc.externalToToken) > 0 {
		var semanticValid [dockerfileTokenCount]bool
		for externalIdx, valid := range validSymbols {
			if !valid || externalIdx >= len(sc.externalToToken) {
				continue
			}
			tokenIdx := sc.externalToToken[externalIdx]
			if tokenIdx >= 0 && tokenIdx < dockerfileTokenCount {
				semanticValid[tokenIdx] = true
			}
		}
		validSymbols = semanticValid[:]
	}
	syms := sc.symbolTable()

	// Error sentinel dispatches based on current state.
	if dockerfileValid(validSymbols, dockerfileTokErrorSentinel) {
		if s.inHeredoc {
			return dockerfileScanContent(s, lexer, validSymbols, syms)
		}
		return dockerfileScanMarker(s, lexer, syms)
	}

	// Heredoc newline.
	if dockerfileValid(validSymbols, dockerfileTokNL) {
		if len(s.heredocs) > 0 && lexer.Lookahead() == '\n' {
			lexer.Advance(false)
			lexer.MarkEnd()
			lexer.SetResultSymbol(syms[dockerfileTokNL])
			return true
		}
	}

	// Heredoc marker.
	if dockerfileValid(validSymbols, dockerfileTokMarker) {
		return dockerfileScanMarker(s, lexer, syms)
	}

	// Heredoc content.
	if dockerfileValid(validSymbols, dockerfileTokLine) || dockerfileValid(validSymbols, dockerfileTokEnd) {
		return dockerfileScanContent(s, lexer, validSymbols, syms)
	}

	return false
}

// dockerfileScanMarker scans a <<[-]DELIM marker.
func dockerfileScanMarker(s *dockerfileScannerState, lexer *gotreesitter.ExternalLexer, syms *[dockerfileTokenCount]gotreesitter.Symbol) bool {
	if lexer.Lookahead() != '<' {
		return false
	}
	lexer.Advance(false)
	if lexer.Lookahead() != '<' {
		return false
	}
	lexer.Advance(false)

	// Check for strip mode: <<-
	stripping := false
	if lexer.Lookahead() == '-' {
		stripping = true
		lexer.Advance(false)
	}

	// Delimiter may be quoted or unquoted.
	var delim []byte
	ch := lexer.Lookahead()
	if ch == '"' || ch == '\'' {
		// Quoted delimiter: consume until matching quote.
		quote := ch
		lexer.Advance(false)
		for {
			ch = lexer.Lookahead()
			if ch == 0 || ch == '\n' {
				return false
			}
			if ch == '\\' {
				lexer.Advance(false)
				ch = lexer.Lookahead()
				if ch == 0 || ch == '\n' {
					return false
				}
				delim = append(delim, byte(ch))
				lexer.Advance(false)
				continue
			}
			if ch == quote {
				lexer.Advance(false)
				break
			}
			delim = append(delim, byte(ch))
			lexer.Advance(false)
		}
	} else {
		// Unquoted delimiter: [a-zA-Z_][a-zA-Z0-9_]*
		if !isDockerfileDelimStart(ch) {
			return false
		}
		for isDockerfileDelimChar(lexer.Lookahead()) {
			delim = append(delim, byte(lexer.Lookahead()))
			lexer.Advance(false)
		}
	}

	if len(delim) == 0 {
		return false
	}

	if len(s.heredocs) >= dockerfileMaxHeredocs {
		return false
	}

	s.heredocs = append(s.heredocs, string(delim))
	s.stripping = stripping
	s.inHeredoc = true

	lexer.MarkEnd()
	lexer.SetResultSymbol(syms[dockerfileTokMarker])
	return true
}

// dockerfileScanContent scans heredoc body content. Tries to match the
// closing delimiter first (if HEREDOC_END is valid), otherwise consumes
// a content line.
func dockerfileScanContent(s *dockerfileScannerState, lexer *gotreesitter.ExternalLexer, validSymbols []bool, syms *[dockerfileTokenCount]gotreesitter.Symbol) bool {
	if len(s.heredocs) == 0 {
		return false
	}

	delim := s.heredocs[len(s.heredocs)-1]

	// Try matching the closing delimiter.
	if dockerfileValid(validSymbols, dockerfileTokEnd) {
		// Optionally strip leading tabs in <<- mode.
		if s.stripping {
			for lexer.Lookahead() == '\t' {
				lexer.Advance(false)
			}
		}

		// Try to match the delimiter character by character.
		matched := true
		for i := 0; i < len(delim); i++ {
			if lexer.Lookahead() != rune(delim[i]) {
				matched = false
				break
			}
			lexer.Advance(false)
		}

		if matched {
			// Delimiter must be followed by newline or EOF.
			next := lexer.Lookahead()
			if next == '\n' || next == 0 {
				lexer.MarkEnd()
				lexer.SetResultSymbol(syms[dockerfileTokEnd])
				s.heredocs = s.heredocs[:len(s.heredocs)-1]
				if len(s.heredocs) == 0 {
					s.inHeredoc = false
				}
				return true
			}
		}

		// No match — fall through and try scanning as a content line.
		// We need to reset, but since the lexer doesn't support rewinding,
		// we can only scan content if the parser also accepts HEREDOC_LINE.
	}

	// Scan a content line: consume everything until newline.
	if dockerfileValid(validSymbols, dockerfileTokLine) {
		hasContent := false
		for {
			ch := lexer.Lookahead()
			if ch == '\n' || ch == 0 {
				break
			}
			hasContent = true
			lexer.Advance(false)
		}
		if hasContent {
			lexer.MarkEnd()
			lexer.SetResultSymbol(syms[dockerfileTokLine])
			return true
		}
	}

	return false
}

func isDockerfileDelimStart(ch rune) bool {
	return (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || ch == '_'
}

func isDockerfileDelimChar(ch rune) bool {
	return isDockerfileDelimStart(ch) || (ch >= '0' && ch <= '9')
}

func dockerfileValid(validSymbols []bool, idx int) bool {
	return idx >= 0 && idx < len(validSymbols) && validSymbols[idx]
}
