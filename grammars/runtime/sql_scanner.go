//go:build !grammar_subset || grammar_subset_sql

package grammarruntime

import (
	"crypto/sha256"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/internal/scanmode"
)

// sqlExternalScannerLocalPortSHA256 identifies the marked local scanner port.
// A focused test checks this value against the source region below.
const sqlExternalScannerLocalPortSHA256 = "f2b02624937fb8de4d127c8eb103d4d1948415487b8c9bc69e807e5983ec47d5"

// C26Q_SQL_SCANNER_LOCAL_PORT_BEGIN

// External token indexes for the SQL grammar (DerekStride/tree-sitter-sql).
const (
	sqlTokDollarTagStart = 0 // "_dollar_quoted_string_tag" — opening $tag$
	sqlTokContent        = 1 // "content" — body between tags
	sqlTokDollarTagEnd   = 2 // "_dollar_quoted_string_end_tag" — closing $tag$
)

// C saves the scanner as one "started" byte and the tag, and gives up when the
// tag and that byte would not fit in TREE_SITTER_SERIALIZATION_BUFFER_SIZE (1024)
// bytes (scanner.cc:29). A tag of more than 1022 bytes is still accepted by Scan;
// Serialize returns zero for it, so reuse fails closed at that boundary.
const (
	sqlSerializationBufferSize = 1024
	sqlMaxDollarTagBytes       = sqlSerializationBufferSize - 2
)

const (
	sqlExternalScannerUpstreamRepo    = "https://github.com/m-novikov/tree-sitter-sql"
	sqlExternalScannerUpstreamCommit  = "587f30d184b058450be2a2330878210c5f33b3f9"
	sqlExternalScannerGrammarSHA256   = "42f011860137175a5a0cb820d1a694e5ccca1d17f226729ff6a4e886910cde1c"
	sqlExternalScannerSourceSHA256    = "d437ad9f517d7a1f4248ccd05abe58370b5040c0037c877dab1f0aefeaa04af6"
	sqlExternalScannerIdentityVersion = "gotreesitter/sql-external-scanner/v1"
	sqlExternalScannerSemantics       = "state=started-byte+tag;empty=0;scan=tag-content-end;failure=preserve;max-tag=1022;mode=alpha-tags"
)

var sqlExternalScannerSpec = ExternalScannerSpec{
	Language:       "sql",
	UpstreamRepo:   sqlExternalScannerUpstreamRepo,
	UpstreamCommit: sqlExternalScannerUpstreamCommit,
	SourceFiles: []ExternalScannerSourceFile{
		{Path: "src/grammar.json", SHA256: sqlExternalScannerGrammarSHA256},
		{Path: "src/scanner.cc", SHA256: sqlExternalScannerSourceSHA256},
	},
	Externals: []string{
		"_dollar_quoted_string_tag",
		"_dollar_quoted_string_content",
		"_dollar_quoted_string_end_tag",
	},
}

func init() {
	RegisterExternalScannerSpec(sqlExternalScannerSpec)
	// Checked against the pinned C scanner (cgo_harness scandiff, 200,000 calls) with
	// the C-faithful mode on: C reads tag names of ASCII letters only (scanner.cc:81).
	scanmode.RegisterScanner(SqlExternalScanner{}, (*sqlScannerState)(nil), true, func(v any) (any, []byte, bool) {
		s := v.(SqlExternalScanner)
		key := append([]byte{0}, s.grammarBlobSHA256[:]...)
		if s.grammarIdentityPresent {
			key[0] = 1
		}
		return s, key, true
	}, func(left, right any) bool {
		return left.(SqlExternalScanner) == right.(SqlExternalScanner)
	})
}

// sqlScannerState is the state of the C scanner: whether a dollar-quoted string
// is open (scanner.cc:18) and the last tag read (scanner.cc:16). As in C, the tag
// stays after the closing tag has ended the string (scanner.cc:119); it matters
// only while started is true.
//
// cFaithful is the mode the core turns on (scanmode.CFaithful). C reads a tag
// name of ASCII letters only (scanner.cc:81); the default engine also accepts a
// leading underscore and digits after the first character, as PostgreSQL does.
type sqlScannerState struct {
	started   bool
	tag       string
	cFaithful bool
}

// SetCFaithful implements scanmode.CFaithful.
func (s *sqlScannerState) SetCFaithful(on bool) { s.cFaithful = on }

var _ scanmode.CFaithful = (*sqlScannerState)(nil)

// SqlExternalScanner implements gotreesitter.ExternalScanner for tree-sitter-sql.
//
// This is a Go port of the C external scanner from DerekStride/tree-sitter-sql.
// The scanner handles PostgreSQL dollar-quoted strings: $tag$..content..$tag$.
// It uses state to remember the opening tag so the closing tag can be matched.
type SqlExternalScanner struct {
	grammarBlobSHA256      [32]byte
	grammarIdentityPresent bool
}

// ExternalScannerForLanguage binds SQL identity to the exact compressed blob.
// A language without that identity keeps the scanner functional but fails
// closed at the identity-aware incremental reuse boundary.
func (SqlExternalScanner) ExternalScannerForLanguage(lang *gotreesitter.Language) gotreesitter.ExternalScanner {
	scanner := SqlExternalScanner{}
	if lang != nil {
		if sum, ok := lang.GrammarBlobSHA256(); ok {
			scanner.grammarBlobSHA256 = sum
			scanner.grammarIdentityPresent = true
		}
	}
	return scanner
}

// CheckpointIdentity returns the stable SQL scanner identity and its bound
// grammar blob identity. Both values are raw 32-byte SHA-256 values.
func (s SqlExternalScanner) CheckpointIdentity() (gotreesitter.ExternalScannerCheckpointIdentity, bool) {
	if !s.grammarIdentityPresent {
		return gotreesitter.ExternalScannerCheckpointIdentity{}, false
	}
	return gotreesitter.ExternalScannerCheckpointIdentity{
		Scanner: append([]byte(nil), sqlExternalScannerIdentity[:]...),
		Grammar: append([]byte(nil), s.grammarBlobSHA256[:]...),
	}, true
}

func (SqlExternalScanner) Create() any {
	return &sqlScannerState{}
}

func (SqlExternalScanner) Destroy(payload any) {}

func (SqlExternalScanner) Serialize(payload any, buf []byte) int {
	s := payload.(*sqlScannerState)
	n := len(s.tag) + 1
	// scanner.cc:29
	if n >= sqlSerializationBufferSize || n > len(buf) {
		return 0
	}
	// scanner.cc:30-32: the started byte, then the tag with no terminator.
	buf[0] = 0
	if s.started {
		buf[0] = 1
	}
	copy(buf[1:], s.tag)
	return n
}

func (SqlExternalScanner) Deserialize(payload any, buf []byte) {
	s := payload.(*sqlScannerState)
	// scanner.cc:36-38
	if len(buf) == 0 {
		s.started = false
		s.tag = ""
		return
	}
	// scanner.cc:40-41: any non-zero first byte is started; the rest is the tag,
	// whatever bytes it holds.
	s.started = buf[0] != 0
	s.tag = string(buf[1:])
}

// SupportsIncrementalReuse certifies the SQL scanner for subtree reuse. Its
// only state is the active dollar-quote tag. Serialize/Deserialize encode every
// representable state bijectively (including an explicit empty marker); an
// oversized tag returns no checkpoint and therefore fails closed for reuse
// while remaining valid during full parsing. Failed scans preserve state, and
// the runtime restores recorded start/end checkpoints around reused nodes.
func (SqlExternalScanner) SupportsIncrementalReuse() bool { return true }

func (SqlExternalScanner) UsesExternalScannerCheckpoints() bool { return true }

func (SqlExternalScanner) PreservesStateOnScanFailure() bool { return true }

func (SqlExternalScanner) Scan(payload any, lexer *gotreesitter.ExternalLexer, validSymbols []bool) bool {
	s := payload.(*sqlScannerState)
	lang := SqlLanguage()
	tagStartSym := lang.ExternalSymbols[sqlTokDollarTagStart]
	contentSym := lang.ExternalSymbols[sqlTokContent]
	tagEndSym := lang.ExternalSymbols[sqlTokDollarTagEnd]

	// scanner.cc:109: start tag. C writes the tag it read into its state even when
	// the scan fails (scanner.cc:73), but then started stays false and nothing reads
	// the tag, and the parser restores the saved state before the next scan; the port
	// leaves the state alone on failure, which no scan can tell apart.
	if sqlValid(validSymbols, sqlTokDollarTagStart) && !s.started {
		tag, ok := scanSqlDollarTag(lexer, true, s.cFaithful)
		if !ok {
			return false
		}
		s.tag = tag
		s.started = true
		lexer.MarkEnd()
		lexer.SetResultSymbol(tagStartSym)
		return true
	}

	// scanner.cc:113: content, up to the full matching tag.
	if sqlValid(validSymbols, sqlTokContent) && s.started {
		return scanSqlDollarContent(lexer, s.tag, contentSym)
	}

	// scanner.cc:117: end tag.
	if sqlValid(validSymbols, sqlTokDollarTagEnd) && s.started {
		if !scanSqlDollarEndTag(lexer, s.tag) {
			return false
		}
		// scanner.cc:119-120: started ends, the tag stays.
		s.started = false
		lexer.MarkEnd()
		lexer.SetResultSymbol(tagEndSym)
		return true
	}

	return false
}

// scanSqlDollarTag scans a $identifier$ or $$ tag and returns the full tag
// string (including the $ delimiters). Returns ("", false) if the current
// position doesn't start a valid dollar tag (scanner.cc:70-94).
func scanSqlDollarTag(lexer *gotreesitter.ExternalLexer, skipWhitespace, cFaithful bool) (string, bool) {
	if skipWhitespace {
		// scanner.cc:71
		for sqlIsScannerWhitespace(scanmode.CLookahead(lexer)) {
			lexer.Advance(true)
		}
	}
	if scanmode.CLookahead(lexer) != '$' {
		return "", false
	}
	lexer.Advance(false)

	tag := []byte{'$'}
	// scanner.cc:81: letters only in C; the default engine also takes '_' and,
	// after the first character, digits.
	for first := true; ; first = false {
		ch := scanmode.CLookahead(lexer)
		if cFaithful {
			if !isSqlASCIILetter(ch) {
				break
			}
		} else if first && !isSqlTagStart(ch) || !first && !isSqlTagChar(ch) {
			break
		}
		tag = append(tag, byte(ch))
		lexer.Advance(false)
	}

	// scanner.cc:86
	if scanmode.CLookahead(lexer) != '$' {
		return "", false
	}
	lexer.Advance(false)
	tag = append(tag, '$')
	return string(tag), true
}

// scanSqlDollarContent scans the body of a dollar-quoted string (scanner.cc:45-68):
// content ends only before the full matching tag. A NUL byte and the end of input
// both end it without a token (scanner.cc:51). The tag is compared as C does,
// against its chars (signed bytes): an empty tag reads as NUL and never matches
// (scanner.cc:53), so the scan runs to the end.
func scanSqlDollarContent(lexer *gotreesitter.ExternalLexer, tag string, contentSym gotreesitter.Symbol) bool {
	pos := 0
	lexer.SetResultSymbol(contentSym)
	lexer.MarkEnd()
	for {
		ch := scanmode.CLookahead(lexer)
		if ch == 0 {
			return false
		}
		if pos < len(tag) && ch == int32(int8(tag[pos])) {
			if pos == len(tag)-1 {
				return true
			}
			if pos == 0 {
				lexer.SetResultSymbol(contentSym)
				lexer.MarkEnd()
			}
			pos++
			lexer.Advance(false)
			continue
		}
		if pos != 0 {
			pos = 0
			continue
		}
		lexer.Advance(false)
	}
}

// scanSqlDollarEndTag reads as many characters as the tag has bytes, stopping at
// NUL or the end of input, and reports whether they are the tag (scanner.cc:96-106).
// C appends each lookahead to a string of chars, which keeps its low eight bits,
// and counts characters, not bytes. An empty tag matches with nothing read.
func scanSqlDollarEndTag(lexer *gotreesitter.ExternalLexer, tag string) bool {
	word := make([]byte, 0, len(tag))
	for {
		ch := scanmode.CLookahead(lexer)
		if ch == 0 || len(word) >= len(tag) {
			break
		}
		word = append(word, byte(ch))
		lexer.Advance(false)
	}
	return string(word) == tag
}

func isSqlASCIILetter(ch int32) bool {
	return (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z')
}

func isSqlTagStart(ch int32) bool {
	return (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || ch == '_'
}

func isSqlTagChar(ch int32) bool {
	return isSqlTagStart(ch) || (ch >= '0' && ch <= '9')
}

// sqlIsScannerWhitespace is iswspace in the C locale (scanner.cc:71): ASCII space,
// tab, line feed, vertical tab, form feed and carriage return.
func sqlIsScannerWhitespace(ch int32) bool {
	return ch == ' ' || (ch >= '\t' && ch <= '\r')
}

func sqlValid(validSymbols []bool, idx int) bool {
	return idx >= 0 && idx < len(validSymbols) && validSymbols[idx]
}

// C26Q_SQL_SCANNER_LOCAL_PORT_END

var sqlExternalScannerIdentity = sha256.Sum256([]byte(
	sqlExternalScannerIdentityVersion + "\x00" +
		"local-port=" + sqlExternalScannerLocalPortSHA256 + "\x00" +
		sqlExternalScannerUpstreamRepo + "\x00" +
		sqlExternalScannerUpstreamCommit + "\x00" +
		"src/grammar.json=" + sqlExternalScannerGrammarSHA256 + "\x00" +
		"src/scanner.cc=" + sqlExternalScannerSourceSHA256 + "\x00" +
		"externals=_dollar_quoted_string_tag,_dollar_quoted_string_content,_dollar_quoted_string_end_tag\x00" +
		sqlExternalScannerSemantics,
))
