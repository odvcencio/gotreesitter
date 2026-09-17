//go:build !grammar_subset || grammar_subset_sql

package grammarruntime

import (
	"crypto/sha256"

	gotreesitter "github.com/odvcencio/gotreesitter"
)

// sqlExternalScannerLocalPortSHA256 identifies the marked local scanner port.
// A focused test checks this value against the source region below.
const sqlExternalScannerLocalPortSHA256 = "588328cd27eea49e88b704b9bd8e46958046564187a5db1a70f6622308a7fff8"

// C26Q_SQL_SCANNER_LOCAL_PORT_BEGIN

// External token indexes for the SQL grammar (DerekStride/tree-sitter-sql).
const (
	sqlTokDollarTagStart = 0 // "_dollar_quoted_string_tag" — opening $tag$
	sqlTokContent        = 1 // "content" — body between tags
	sqlTokDollarTagEnd   = 2 // "_dollar_quoted_string_end_tag" — closing $tag$
)

// The runtime currently snapshots scanners into a 4096-byte buffer. One byte
// is reserved for the trailing NUL, so tags longer than this cannot participate
// in checkpoint-authenticated incremental reuse. They remain valid SQL and are
// still accepted by Scan; Serialize returns zero for those states so reuse fails
// closed at the affected boundary without changing full-parse semantics.
const sqlMaxDollarTagBytes = 4095

const (
	sqlExternalScannerUpstreamRepo    = "https://github.com/m-novikov/tree-sitter-sql"
	sqlExternalScannerUpstreamCommit  = "587f30d184b058450be2a2330878210c5f33b3f9"
	sqlExternalScannerGrammarSHA256   = "42f011860137175a5a0cb820d1a694e5ccca1d17f226729ff6a4e886910cde1c"
	sqlExternalScannerSourceSHA256    = "d437ad9f517d7a1f4248ccd05abe58370b5040c0037c877dab1f0aefeaa04af6"
	sqlExternalScannerIdentityVersion = "gotreesitter/sql-external-scanner/v1"
	sqlExternalScannerSemantics       = "state=tag;empty=0;nonempty=tag-nul;scan=tag-content-end;failure=preserve;max-tag=4095"
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
}

// sqlScannerState stores the dollar-quote tag for matching the closing delimiter.
type sqlScannerState struct {
	tag string // empty when not inside a dollar-quoted string
}

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
	if s.tag == "" {
		if len(buf) == 0 {
			return 0
		}
		// A one-byte empty-state marker keeps "empty checkpoint" distinct from
		// the checkpoint store's zero-byte "checkpoint absent" sentinel.
		buf[0] = 0
		return 1
	}
	// Store tag + null terminator.
	tagLen := len(s.tag) + 1
	if tagLen > len(buf) {
		return 0
	}
	copy(buf, s.tag)
	buf[len(s.tag)] = 0
	return tagLen
}

func (SqlExternalScanner) Deserialize(payload any, buf []byte) {
	s := payload.(*sqlScannerState)
	s.tag = ""
	if len(buf) > 1 {
		// Find the null terminator.
		for i, b := range buf {
			if b == 0 {
				s.tag = string(buf[:i])
				return
			}
		}
		// No null terminator found — use the whole buffer.
		s.tag = string(buf)
	}
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

	// Start tag: scan a $..$ tag and store it.
	if sqlValid(validSymbols, sqlTokDollarTagStart) && s.tag == "" {
		tag, ok := scanSqlDollarTag(lexer, true)
		if !ok {
			return false
		}
		s.tag = tag
		lexer.MarkEnd()
		lexer.SetResultSymbol(tagStartSym)
		return true
	}

	// Content: scan the body between dollar-quote tags, stopping immediately
	// before the matching closing tag. Non-matching '$' sequences are content.
	if sqlValid(validSymbols, sqlTokContent) && s.tag != "" {
		return scanSqlDollarContent(lexer, s.tag, contentSym)
	}

	// End tag: scan for a matching $..$ tag.
	if sqlValid(validSymbols, sqlTokDollarTagEnd) && s.tag != "" {
		tag, ok := scanSqlDollarTag(lexer, false)
		if !ok || tag != s.tag {
			return false
		}
		s.tag = ""
		lexer.MarkEnd()
		lexer.SetResultSymbol(tagEndSym)
		return true
	}

	return false
}

// scanSqlDollarTag scans a $identifier$ or $$ tag and returns the full tag
// string (including the $ delimiters). Returns ("", false) if the current
// position doesn't start a valid dollar tag.
func scanSqlDollarTag(lexer *gotreesitter.ExternalLexer, skipWhitespace bool) (string, bool) {
	if skipWhitespace {
		for sqlIsScannerWhitespace(lexer.Lookahead()) {
			lexer.Advance(true)
		}
	}
	if lexer.Lookahead() != '$' {
		return "", false
	}
	lexer.Advance(false)

	var tag []byte
	tag = append(tag, '$')

	// Tag identifier: [a-zA-Z_][a-zA-Z0-9_]* or empty (for $$).
	ch := lexer.Lookahead()
	if ch == '$' {
		// Empty tag: $$
		lexer.Advance(false)
		tag = append(tag, '$')
		return string(tag), true
	}

	if !isSqlTagStart(ch) {
		return "", false
	}

	for isSqlTagChar(lexer.Lookahead()) {
		tag = append(tag, byte(lexer.Lookahead()))
		lexer.Advance(false)
	}

	if lexer.Lookahead() != '$' {
		return "", false
	}
	lexer.Advance(false)
	tag = append(tag, '$')

	return string(tag), true
}

// scanSqlDollarContent scans the body of a dollar-quoted string. It mirrors
// upstream tree-sitter-sql: content ends only before the full matching tag.
func scanSqlDollarContent(lexer *gotreesitter.ExternalLexer, tag string, contentSym gotreesitter.Symbol) bool {
	if tag == "" {
		return false
	}
	pos := 0
	lexer.SetResultSymbol(contentSym)
	lexer.MarkEnd()
	for {
		ch := lexer.Lookahead()
		if ch == 0 {
			return false
		}
		if pos < len(tag) && ch == rune(tag[pos]) {
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

func isSqlTagStart(ch rune) bool {
	return (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || ch == '_'
}

func isSqlTagChar(ch rune) bool {
	return isSqlTagStart(ch) || (ch >= '0' && ch <= '9')
}

func sqlIsScannerWhitespace(ch rune) bool {
	return ch == ' ' || ch == '\t' || ch == '\n' || ch == '\r' || ch == '\f'
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
