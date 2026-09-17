//go:build !grammar_subset || grammar_subset_dockerfile

package grammarruntime

import gotreesitter "github.com/odvcencio/gotreesitter"

// External token indexes for the dockerfile grammar.
const (
	dockerfileTokMarker        = 0 // "heredoc_marker"
	dockerfileTokLine          = 1 // "heredoc_line"
	dockerfileTokEnd           = 2 // "heredoc_end"
	dockerfileTokNL            = 3 // "_heredoc_nl"
	dockerfileTokErrorSentinel = 4 // "error_sentinel"
)

// Concrete symbol IDs from the generated dockerfile grammar ExternalSymbols.
const (
	dockerfileSymMarker        gotreesitter.Symbol = 82
	dockerfileSymLine          gotreesitter.Symbol = 83
	dockerfileSymEnd           gotreesitter.Symbol = 84
	dockerfileSymNL            gotreesitter.Symbol = 85
	dockerfileSymErrorSentinel gotreesitter.Symbol = 86
)

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
//   - _heredoc_nl: newlines within heredoc context
//   - error_sentinel: error recovery bail-out
type DockerfileExternalScanner struct{}

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

func (DockerfileExternalScanner) Scan(payload any, lexer *gotreesitter.ExternalLexer, validSymbols []bool) bool {
	s := payload.(*dockerfileScannerState)

	// Error sentinel dispatches based on current state.
	if dockerfileValid(validSymbols, dockerfileTokErrorSentinel) {
		if s.inHeredoc {
			return dockerfileScanContent(s, lexer, validSymbols)
		}
		return dockerfileScanMarker(s, lexer)
	}

	// Heredoc newline.
	if dockerfileValid(validSymbols, dockerfileTokNL) {
		if len(s.heredocs) > 0 && lexer.Lookahead() == '\n' {
			lexer.Advance(false)
			lexer.MarkEnd()
			lexer.SetResultSymbol(dockerfileSymNL)
			return true
		}
	}

	// Heredoc marker.
	if dockerfileValid(validSymbols, dockerfileTokMarker) {
		return dockerfileScanMarker(s, lexer)
	}

	// Heredoc content.
	if dockerfileValid(validSymbols, dockerfileTokLine) || dockerfileValid(validSymbols, dockerfileTokEnd) {
		return dockerfileScanContent(s, lexer, validSymbols)
	}

	return false
}

// dockerfileScanMarker scans a <<[-]DELIM marker.
func dockerfileScanMarker(s *dockerfileScannerState, lexer *gotreesitter.ExternalLexer) bool {
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
	lexer.SetResultSymbol(dockerfileSymMarker)
	return true
}

// dockerfileScanContent scans heredoc body content. Tries to match the
// closing delimiter first (if HEREDOC_END is valid), otherwise consumes
// a content line.
func dockerfileScanContent(s *dockerfileScannerState, lexer *gotreesitter.ExternalLexer, validSymbols []bool) bool {
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
				lexer.SetResultSymbol(dockerfileSymEnd)
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
			lexer.SetResultSymbol(dockerfileSymLine)
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
