package grammars

import (
	"embed"
	"regexp"
	"strings"
)

// highlightQueryFiles holds highlight queries that replace the text in the
// generated registry entries. Each file is named <language>.scm. The generated
// entries come from the single queries/highlights.scm that ts2go finds in the
// pinned grammar repository. That file is empty or a stub for some grammars and
// misses the parent layers (html_tags) that editors resolve at run time. A file
// here wins over the generated text, so a ts2go run does not undo it. Every
// file opens with a header comment: one "; Source:" line per upstream file
// (repository, path, commit, license) and a "; Change:" line that says what
// differs from upstream, or "none". Record each file in
// licenses/highlight_queries.json too. `go run ./cmd/gen_license_notices -check`
// fails when a header and that audit disagree, and THIRD_PARTY_NOTICES takes its
// attribution for these files from the audit.
//
//go:embed highlight_queries/*.scm
var highlightQueryFiles embed.FS

// applyHighlightQueryFile swaps in the embedded query for a freshly generated
// entry. Entries that already carry captured raw-query state are copies of
// registered entries (see Register); their query stays as the caller set it.
func applyHighlightQueryFile(entry *LangEntry) {
	if entry.highlightRawCaptured || entry.GrammarSource != GrammarSourceTS2GoBlob {
		return
	}
	if data, err := highlightQueryFiles.ReadFile("highlight_queries/" + entry.Name + ".scm"); err == nil {
		entry.HighlightQuery = string(data)
	}
}

// highlightInheritsDirective matches the nvim-style "; inherits: a,b" line.
// Parentheses mark an optional parent in nvim; here every unknown parent is
// skipped, so they carry no extra meaning.
var highlightInheritsDirective = regexp.MustCompile(`^;+\s*inherits:?\s*([A-Za-z0-9_,()\- ]+?)\s*$`)

// highlightInheritsFromQuery returns the parents named by the "; inherits:"
// directive on the first non-blank line of a query. Like nvim, it ignores the
// directive anywhere else: glsl embeds a copy of the C query and mentions
// "inherits: c" mid-file, and composing C again would only duplicate patterns.
func highlightInheritsFromQuery(query string) []string {
	for _, line := range strings.Split(query, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		m := highlightInheritsDirective.FindStringSubmatch(line)
		if m == nil {
			return nil
		}
		var parents []string
		for _, p := range strings.Split(m[1], ",") {
			if p = strings.Trim(strings.TrimSpace(p), "()"); p != "" {
				parents = append(parents, p)
			}
		}
		return parents
	}
	return nil
}

// highlightParents returns the parents whose queries compose ahead of a
// language's own query: the entry's InheritHighlights, else the "; inherits:"
// directive, else the upstream convention in defaultHighlightInherits.
func highlightParents(name, explicit, raw string) []string {
	if explicit != "" {
		return []string{explicit}
	}
	if parents := highlightInheritsFromQuery(raw); len(parents) > 0 {
		return parents
	}
	if parent := defaultHighlightInherits[name]; parent != "" {
		return []string{parent}
	}
	return nil
}

// dropQueryPatterns removes parent-query content that names node types the
// child's locked grammar lacks, so the child can still inherit the rest of the
// parent. A plain name such as module_name drops every top-level pattern that
// uses it. A quoted name such as "import" drops only the list item that sits
// on a line of its own, so the other items of the keyword list stay. Comments
// and blank lines between patterns stay.
func dropQueryPatterns(query string, nodes []string) string {
	var names []string
	for _, n := range nodes {
		if strings.HasPrefix(n, `"`) {
			query = regexp.MustCompile(`(?m)^[ \t]*`+regexp.QuoteMeta(n)+`[ \t]*\r?\n`).ReplaceAllString(query, "")
		} else {
			names = append(names, regexp.QuoteMeta(n))
		}
	}
	if len(names) == 0 {
		return query
	}
	uses := regexp.MustCompile(`\b(?:` + strings.Join(names, "|") + `)\b`)

	var out strings.Builder
	for i := 0; i < len(query); {
		start := i
		switch c := query[i]; {
		case c == ';':
			for i < len(query) && query[i] != '\n' {
				i++
			}
			out.WriteString(query[start:i])
			continue
		case c == ' ' || c == '\t' || c == '\r' || c == '\n':
			i++
			out.WriteString(query[start:i])
			continue
		}
		if i = endOfQueryPattern(query, i); i == start {
			i++ // unknown byte: keep it and move on
		}
		if text := query[start:i]; !uses.MatchString(text) {
			out.WriteString(text)
		}
	}
	return out.String()
}

// endOfQueryPattern returns the end offset of the top-level pattern that
// starts at query[i]: one group, string or bare token, plus the captures and
// quantifiers that trail it.
func endOfQueryPattern(query string, i int) int {
	i = endOfQueryElement(query, i)
	for {
		j := i
		for j < len(query) && (query[j] == ' ' || query[j] == '\t' || query[j] == '\r' || query[j] == '\n') {
			j++
		}
		if j >= len(query) || (query[j] != '@' && query[j] != '*' && query[j] != '+' && query[j] != '?') {
			return i
		}
		j++
		for j < len(query) && isQueryNameByte(query[j]) {
			j++
		}
		i = j
	}
}

func endOfQueryElement(query string, i int) int {
	depth := 0
	for i < len(query) {
		switch query[i] {
		case '"':
			i++
			for i < len(query) && query[i] != '"' {
				if query[i] == '\\' {
					i++
				}
				i++
			}
			i++
		case ';':
			for i < len(query) && query[i] != '\n' {
				i++
			}
		case '(', '[':
			depth++
			i++
		case ')', ']':
			depth--
			i++
		default:
			if depth == 0 {
				// A bare token such as "_" or a field name ends at whitespace.
				if !isQueryNameByte(query[i]) && query[i] != ':' {
					return i
				}
			}
			i++
			continue
		}
		if depth <= 0 {
			return i
		}
	}
	return len(query)
}

func isQueryNameByte(c byte) bool {
	return c == '_' || c == '.' || c == '-' || c == '$' || (c >= '0' && c <= '9') || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}
