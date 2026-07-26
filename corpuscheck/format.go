// Package corpuscheck parses the upstream tree-sitter "corpus test" format
// (the same format read by `tree-sitter test`) and compares gotreesitter's
// parse output against the expected S-expressions that grammar authors ship
// in their own repositories.
//
// The format, empirically surveyed across ~190 upstream grammar repos
// (see gts-corpora/grammar_parity), is:
//
//	====================
//	test name
//	:attribute1
//	:attribute2
//	====================
//	<source text, verbatim, may be multiple lines>
//	---
//	<expected S-expression, may be multiple lines, may be empty>
//
// Repeated for every test in the file. Variations this parser handles:
//
//   - Header/divider marker length varies per file and even per test
//     (any run of 3+ '=' for headers, 3+ '-' for the result divider).
//   - Marker lines may carry an arbitrary non-whitespace suffix appended
//     directly after the run of '=' or '-' (e.g. "===========|||",
//     "---\\\\\\"). Grammars whose source text legitimately contains bare
//     "---" or "====" lines (YAML documents, TLA+ module footers, Liquid
//     front matter) use this to disambiguate real dividers from body text.
//     The suffix is captured from the opening header marker of each test
//     and required to match on that test's closing header marker and
//     result divider.
//   - Attribute lines (":skip", ":error", ":language(x)", ...) appear
//     between the test name and the closing header marker. They are only
//     recognized in that header region -- a body/expected line that
//     happens to start with ':' (a Ruby/Clojure/Lisp symbol literal such
//     as ":foo") is ordinary content, not an attribute.
//   - CRLF line endings (several grammars ship at least one CRLF-encoded
//     corpus file; \r is stripped before matching marker lines and
//     preserved nowhere since tree-sitter test parsing normalizes it away
//     too).
//   - ":error" tests may omit the expected output entirely -- the author
//     is only asserting that the parse contains an error, not asserting
//     an exact tree shape.
//
// Format variants intentionally NOT supported (see doc comment on
// ParseFile for the up to date list).
package corpuscheck

import (
	"bytes"
	"fmt"
	"strings"
)

// TestCase is one corpus test extracted from a corpus file.
type TestCase struct {
	// Name is the test's title line (may be empty for anonymous tests).
	Name string
	// Attrs holds header attribute lines verbatim, without the leading
	// ':' (e.g. "skip", "error", "language(tsx)").
	Attrs []string
	// Input is the verbatim source text for this test.
	Input []byte
	// Expected is the raw expected-output text (the block after the
	// result divider), not yet parsed into a tree. It may be empty
	// (whitespace-only) for tests -- almost always ":error" tests --
	// that don't assert an exact tree shape.
	Expected string
	// Line is the 1-based line number of the test's opening header
	// marker, for diagnostics.
	Line int
	// ParseError is set when this test's own framing was malformed and
	// could not be recovered from (e.g. no closing header marker found
	// before EOF). Input/Expected are unreliable when this is set.
	ParseError error
}

// HasAttr reports whether the test carries the named attribute. Attributes
// with values (e.g. "language(tsx)") match on their bare name prefix, so
// HasAttr("language") is true for "language(tsx)".
func (tc *TestCase) HasAttr(name string) bool {
	for _, a := range tc.Attrs {
		if a == name {
			return true
		}
		if strings.HasPrefix(a, name+"(") {
			return true
		}
	}
	return false
}

// AttrValue returns the parenthesized value of the named attribute (e.g.
// AttrValue("language") returns "tsx" for the attribute "language(tsx)")
// and whether it was present at all.
func (tc *TestCase) AttrValue(name string) (string, bool) {
	for _, a := range tc.Attrs {
		if a == name {
			return "", true
		}
		if strings.HasPrefix(a, name+"(") && strings.HasSuffix(a, ")") {
			return a[len(name)+1 : len(a)-1], true
		}
	}
	return "", false
}

// ParseFile parses one corpus file's contents into its test cases.
//
// Known format variants this parser does NOT support (found during the
// survey but rare enough, or upstream-tool-specific enough, that
// implementing them was not worth the complexity for a comparison
// harness):
//
//   - Nothing observed forced an outright parse failure across the
//     surveyed languages; the recovery strategy below (re-synchronize on
//     the next header marker) is believed sufficient for any remaining
//     oddities. If a file produces zero test cases where it plainly has
//     content, that is reported as a parse error by the caller, not
//     silently swallowed.
func ParseFile(data []byte) ([]TestCase, error) {
	text := normalizeLineEndings(data)
	lines := strings.Split(text, "\n")

	var cases []TestCase
	i := 0
	n := len(lines)

	// Skip leading blank lines.
	for i < n && strings.TrimSpace(lines[i]) == "" {
		i++
	}

	for i < n {
		lineNo := i + 1
		openSuffix, ok := matchMarker(lines[i], '=')
		if !ok {
			// Not a marker where we expected one: try to resynchronize by
			// scanning forward for the next plausible header marker so one
			// malformed test doesn't sink the rest of the file.
			j := i + 1
			for j < n {
				if _, ok := matchMarker(lines[j], '='); ok {
					break
				}
				j++
			}
			if j >= n {
				if strings.TrimSpace(strings.Join(lines[i:], "\n")) == "" {
					break
				}
				return cases, fmt.Errorf("line %d: expected header marker, found %q (no further marker in file)", lineNo, lines[i])
			}
			cases = append(cases, TestCase{
				Line:       lineNo,
				ParseError: fmt.Errorf("line %d: expected header marker, found %q", lineNo, lines[i]),
			})
			i = j
			continue
		}
		i++

		// Header region: first non-attribute line is the name; subsequent
		// lines starting with ':' are attributes, up until the matching
		// closing marker (same suffix as the opener).
		var nameLines []string
		var attrs []string
		closed := false
		for i < n {
			if suf, ok := matchMarker(lines[i], '='); ok && suf == openSuffix {
				closed = true
				i++
				break
			}
			line := lines[i]
			if strings.HasPrefix(line, ":") {
				attrs = append(attrs, strings.TrimSpace(line[1:]))
			} else if strings.TrimSpace(line) != "" {
				nameLines = append(nameLines, line)
			}
			i++
		}
		if !closed {
			cases = append(cases, TestCase{
				Line:       lineNo,
				ParseError: fmt.Errorf("line %d: no closing header marker (suffix %q) before EOF", lineNo, openSuffix),
			})
			break
		}
		name := strings.TrimSpace(strings.Join(nameLines, " "))

		// Skip the single blank line conventionally separating the header
		// from the source, but don't require it.
		for i < n && strings.TrimSpace(lines[i]) == "" {
			i++
		}

		// Body: read source lines until we find the result divider. The
		// divider must carry the SAME suffix as this test's header marker
		// so that body text containing bare "---" (front matter, YAML
		// documents, etc.) isn't mistaken for the divider.
		bodyStart := i
		dividerIdx := -1
		for k := i; k < n; k++ {
			if suf, ok := matchMarker(lines[k], '-'); ok && suf == openSuffix {
				dividerIdx = k
				break
			}
		}
		if dividerIdx == -1 {
			cases = append(cases, TestCase{
				Name:       name,
				Attrs:      attrs,
				Line:       lineNo,
				ParseError: fmt.Errorf("line %d: no result divider (suffix %q) before EOF", lineNo, openSuffix),
			})
			break
		}
		bodyLines := lines[bodyStart:dividerIdx]
		// Trim exactly one trailing blank line (the conventional blank
		// line before the divider); preserve interior/trailing blank
		// lines that are part of the source itself.
		if len(bodyLines) > 0 && strings.TrimSpace(bodyLines[len(bodyLines)-1]) == "" {
			bodyLines = bodyLines[:len(bodyLines)-1]
		}
		input := strings.Join(bodyLines, "\n")
		i = dividerIdx + 1

		// Expected: everything from here until the next header marker
		// (any suffix -- expected S-expressions never look like a bare
		// run of '=' characters) or EOF.
		expStart := i
		expEnd := n
		for k := i; k < n; k++ {
			if _, ok := matchMarker(lines[k], '='); ok {
				expEnd = k
				break
			}
		}
		expected := strings.TrimSpace(strings.Join(lines[expStart:expEnd], "\n"))
		i = expEnd

		cases = append(cases, TestCase{
			Name:     name,
			Attrs:    attrs,
			Input:    []byte(input),
			Expected: expected,
			Line:     lineNo,
		})

		for i < n && strings.TrimSpace(lines[i]) == "" {
			i++
		}
	}

	return cases, nil
}

// matchMarker reports whether line is a divider/header marker line: a run
// of 3 or more of the given rune, optionally followed by a non-whitespace,
// rune-free suffix used to disambiguate the marker from body content that
// itself contains bare "---" or "====" lines. It returns the suffix (which
// is "" for the common case) and whether the line matched at all.
func matchMarker(line string, r byte) (string, bool) {
	i := 0
	for i < len(line) && line[i] == r {
		i++
	}
	if i < 3 {
		return "", false
	}
	suffix := line[i:]
	if strings.ContainsAny(suffix, " \t") || strings.ContainsRune(suffix, rune(r)) {
		return "", false
	}
	return suffix, true
}

// normalizeLineEndings converts CRLF and lone CR line endings to LF.
func normalizeLineEndings(data []byte) string {
	if !bytes.ContainsRune(data, '\r') {
		return string(data)
	}
	data = bytes.ReplaceAll(data, []byte("\r\n"), []byte("\n"))
	data = bytes.ReplaceAll(data, []byte("\r"), []byte("\n"))
	return string(data)
}
