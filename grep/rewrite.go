package grep

import (
	"fmt"
	"sort"
	"strings"

	"github.com/odvcencio/gotreesitter"
)

// Edit describes a byte-range replacement in source text.
type Edit struct {
	StartByte   uint32
	EndByte     uint32
	Replacement []byte
}

// Diagnostic reports a non-fatal issue encountered during rewriting.
type Diagnostic struct {
	Message   string
	StartByte uint32
	EndByte   uint32
}

// ReplaceResult holds the computed edits and any diagnostics produced by
// a Replace operation.
type ReplaceResult struct {
	Edits       []Edit
	Diagnostics []Diagnostic
}

// Replace finds all matches of pattern in source and computes replacement
// edits by substituting capture references ($NAME, $$$NAME) in the
// replacement template with matched text.
//
// Overlapping matches are discarded (the earlier match wins) and reported
// as diagnostics. After computing edits, the result is validated by
// re-parsing with tree-sitter; new ERROR nodes are reported as diagnostics.
func Replace(lang *gotreesitter.Language, pattern string, replacement string, source []byte) (*ReplaceResult, error) {
	cp, err := CompilePattern(lang, pattern)
	if err != nil {
		return nil, fmt.Errorf("replace: %w", err)
	}

	// Parse the source.
	tree, err := parseSnippet(lang, source)
	if err != nil {
		return nil, fmt.Errorf("replace: parse source: %w", err)
	}
	bt := gotreesitter.Bind(tree)
	defer bt.Release()

	root := bt.RootNode()
	if root == nil {
		return &ReplaceResult{}, nil
	}

	// Count ERROR nodes in the original source for validation after rewriting.
	origErrors := countErrorNodes(root)

	// Execute the query to find all matches.
	matches := cp.Query.ExecuteNode(root, lang, source)
	if len(matches) == 0 {
		return &ReplaceResult{}, nil
	}

	// Determine whether we have metavars (code-pattern mode) or are in
	// S-expression mode.
	hasMeta := len(cp.MetaVars) > 0

	// Build the capture name mapping (reuses logic from match.go).
	capToMeta := buildCaptureMap(cp.MetaVars)

	// Convert each query match into a candidate edit.
	type candidate struct {
		startByte uint32
		endByte   uint32
		edit      Edit
	}
	var candidates []candidate

	for _, m := range matches {
		if len(m.Captures) == 0 {
			continue
		}

		// Compute the union span of all captures (including internal _lit_ ones)
		// so we can find the enclosing AST node.
		var spanStart, spanEnd uint32
		first := true

		// Also collect user-facing capture texts for template substitution.
		capTexts := make(map[string]string)

		for _, c := range m.Captures {
			if c.Node == nil {
				continue
			}
			sb, eb := c.ByteRange()

			if first {
				spanStart = sb
				spanEnd = eb
				first = false
			} else {
				if sb < spanStart {
					spanStart = sb
				}
				if eb > spanEnd {
					spanEnd = eb
				}
			}

			// Collect user-facing captures for template substitution.
			name := c.Name
			if capToMeta != nil {
				if metaName, ok := capToMeta[name]; ok {
					name = metaName
				}
			}
			if !strings.HasPrefix(name, "_lit_") {
				capTexts[name] = c.Text(source)
			}
		}

		if first {
			// No captures with valid nodes.
			continue
		}

		// Find the smallest AST node that fully encloses all captures.
		// This gives us the true match root — e.g., the function_declaration
		// node rather than just the captured identifier and parameter list.
		matchRoot := findMatchRoot(root, spanStart, spanEnd)
		matchStart := spanStart
		matchEnd := spanEnd
		if matchRoot != nil {
			matchStart = matchRoot.StartByte()
			matchEnd = matchRoot.EndByte()
		}

		// Apply template substitution.
		replaced := substituteTemplate(replacement, capTexts, hasMeta)

		candidates = append(candidates, candidate{
			startByte: matchStart,
			endByte:   matchEnd,
			edit: Edit{
				StartByte:   matchStart,
				EndByte:     matchEnd,
				Replacement: []byte(replaced),
			},
		})
	}

	if len(candidates) == 0 {
		return &ReplaceResult{}, nil
	}

	// Sort candidates by StartByte ascending, preferring outermost (larger
	// span) when starts are equal.
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].startByte == candidates[j].startByte {
			return candidates[i].endByte > candidates[j].endByte
		}
		return candidates[i].startByte < candidates[j].startByte
	})

	// Filter overlapping matches: keep the first, discard overlapping followers.
	var rr ReplaceResult
	rr.Edits = append(rr.Edits, candidates[0].edit)
	lastEnd := candidates[0].endByte

	for i := 1; i < len(candidates); i++ {
		c := candidates[i]
		if c.startByte < lastEnd {
			// Overlapping — discard and add diagnostic.
			rr.Diagnostics = append(rr.Diagnostics, Diagnostic{
				Message:   "overlapping match discarded",
				StartByte: c.startByte,
				EndByte:   c.endByte,
			})
			continue
		}
		rr.Edits = append(rr.Edits, c.edit)
		lastEnd = c.endByte
	}

	// Validate output: apply edits and re-parse to check for new ERROR nodes.
	newSource := ApplyEdits(source, rr.Edits)
	newErrors := countSourceErrors(lang, newSource)
	if newErrors > origErrors {
		rr.Diagnostics = append(rr.Diagnostics, Diagnostic{
			Message: fmt.Sprintf(
				"rewrite introduced %d new parse error(s)",
				newErrors-origErrors,
			),
		})
	}

	return &rr, nil
}

// ApplyEdits applies non-overlapping edits to source, returning new source.
// Edits are applied back-to-front (by StartByte descending) so that earlier
// byte offsets remain valid as later portions of the source are modified.
func ApplyEdits(source []byte, edits []Edit) []byte {
	if len(edits) == 0 {
		// Return a copy so the caller doesn't alias the original.
		out := make([]byte, len(source))
		copy(out, source)
		return out
	}

	// Sort edits by StartByte descending for back-to-front application.
	sorted := make([]Edit, len(edits))
	copy(sorted, edits)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].StartByte > sorted[j].StartByte
	})

	result := make([]byte, len(source))
	copy(result, source)

	for _, e := range sorted {
		start := int(e.StartByte)
		end := int(e.EndByte)
		if start > len(result) {
			start = len(result)
		}
		if end > len(result) {
			end = len(result)
		}
		if start > end {
			continue
		}

		// Splice: result[:start] + replacement + result[end:]
		newResult := make([]byte, 0, start+len(e.Replacement)+(len(result)-end))
		newResult = append(newResult, result[:start]...)
		newResult = append(newResult, e.Replacement...)
		newResult = append(newResult, result[end:]...)
		result = newResult
	}

	return result
}

// substituteTemplate replaces capture references in a replacement template
// with their captured text.
//
// In code-pattern mode (hasMeta=true):
//   - $$$NAME is replaced first (variadic captures)
//   - $NAME is replaced second (single captures)
//
// In S-expression mode (hasMeta=false):
//   - @name references are replaced
//
// Capture names are sorted by length descending to avoid partial prefix
// matches (e.g., $NAMES replacing before $NAME is fully checked).
func substituteTemplate(template string, captures map[string]string, hasMeta bool) string {
	if len(captures) == 0 {
		return template
	}

	if !hasMeta {
		// S-expression mode: replace @name references.
		return substituteAtNames(template, captures)
	}

	// Code-pattern mode: replace $$$NAME then $NAME.
	// Sort capture names by length descending.
	names := make([]string, 0, len(captures))
	for name := range captures {
		names = append(names, name)
	}
	sort.Slice(names, func(i, j int) bool {
		return len(names[i]) > len(names[j])
	})

	result := template

	// First pass: replace $$$NAME references.
	for _, name := range names {
		result = strings.ReplaceAll(result, "$$$"+name, captures[name])
	}

	// Second pass: replace $NAME references.
	for _, name := range names {
		result = strings.ReplaceAll(result, "$"+name, captures[name])
	}

	return result
}

// substituteAtNames replaces @name references in a template with captured text.
func substituteAtNames(template string, captures map[string]string) string {
	names := make([]string, 0, len(captures))
	for name := range captures {
		names = append(names, name)
	}
	sort.Slice(names, func(i, j int) bool {
		return len(names[i]) > len(names[j])
	})

	result := template
	for _, name := range names {
		result = strings.ReplaceAll(result, "@"+name, captures[name])
	}
	return result
}

// findMatchRoot walks the tree to find the smallest node that fully contains
// the span [start, end). This is used to determine the true match root when
// the query captures only cover part of the matched syntax.
func findMatchRoot(root *gotreesitter.Node, start, end uint32) *gotreesitter.Node {
	if root == nil {
		return nil
	}

	best := root
	var search func(n *gotreesitter.Node)
	search = func(n *gotreesitter.Node) {
		if n == nil {
			return
		}
		if n.StartByte() <= start && n.EndByte() >= end {
			if (n.EndByte() - n.StartByte()) < (best.EndByte() - best.StartByte()) {
				best = n
			}
			for i := 0; i < n.ChildCount(); i++ {
				search(n.Child(i))
			}
		}
	}
	search(root)
	return best
}

// countSourceErrors parses source with the given language and returns the
// number of ERROR nodes in the resulting tree. Returns 0 if parsing fails.
func countSourceErrors(lang *gotreesitter.Language, source []byte) int {
	tree, err := parseSnippet(lang, source)
	if err != nil {
		return 0
	}
	bt := gotreesitter.Bind(tree)
	defer bt.Release()

	root := bt.RootNode()
	if root == nil {
		return 0
	}
	return countErrorNodes(root)
}

// countErrorNodes counts the number of ERROR nodes in the tree.
func countErrorNodes(root *gotreesitter.Node) int {
	count := 0
	gotreesitter.Walk(root, func(n *gotreesitter.Node, depth int) gotreesitter.WalkAction {
		if n.IsError() {
			count++
			return gotreesitter.WalkSkipChildren
		}
		return gotreesitter.WalkContinue
	})
	return count
}
