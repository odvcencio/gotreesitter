package corpuscheck

import "fmt"

// Divergence describes the first place two SNode trees stop matching.
type Divergence struct {
	// Path is a human-readable breadcrumb to the mismatching node, e.g.
	// "/source_file/function_declaration[0]/block[2]".
	Path string
	// Category classifies the mismatch for aggregate reporting.
	Category string
	Expected string
	Actual   string
}

func (d *Divergence) String() string {
	if d == nil {
		return "<match>"
	}
	return fmt.Sprintf("%s: %s: expected=%q actual=%q", d.Path, d.Category, d.Expected, d.Actual)
}

// Mismatch categories. Keep these stable -- callers aggregate by string.
const (
	CategoryShape                 = "shape"                       // nil-ness / child count differs
	CategoryType                  = "type"                        // node type name differs
	CategoryField                 = "field"                       // field name differs
	CategoryMissing               = "missing"                     // MISSING-ness differs
	CategoryMissingValue          = "missing_value"               // both MISSING, different token/type
	CategoryUnexpectedUnsupported = "unexpected_node_unsupported" // expected side uses UNEXPECTED; gotreesitter has no equivalent node kind
)

// Compare walks expected and actual in lockstep and returns the first
// point of divergence, or nil if the trees are structurally identical
// (ignoring nothing else -- field names, node types, MISSING-ness, and
// child order/count all must match exactly).
func Compare(expected, actual *SNode) *Divergence {
	return CompareWithOptions(expected, actual, CompareOptions{})
}

// CompareOptions controls how much latitude Compare gives the comparison.
// The zero value is the fully strict comparison Compare uses.
type CompareOptions struct {
	// IgnoreMissingExpectedFields treats "the fixture has no field name
	// but gotreesitter's actual tree does" as a match rather than a
	// CategoryField divergence. It does NOT ignore the reverse (fixture
	// has a field, actual doesn't) or a genuine field-name conflict
	// (both sides have a field, but a different one) -- those remain
	// hard mismatches. This exists because several upstream grammar
	// repos (see corpuscheck's report notes on json/go) added fields to
	// their grammar.js well after their last `tree-sitter test --update`
	// touched the affected fixtures, so their own committed corpus
	// fixtures are stale with respect to their own compiled grammar
	// tables -- confirmed by walking each repo's git history. Use this
	// as a secondary, clearly-labeled lens, never as the sole number.
	IgnoreMissingExpectedFields bool
}

// CompareWithOptions is Compare with configurable leniency; see
// CompareOptions.
func CompareWithOptions(expected, actual *SNode, opts CompareOptions) *Divergence {
	return compareAt(expected, actual, "/"+rootLabel(expected, actual), opts)
}

func rootLabel(expected, actual *SNode) string {
	if expected != nil {
		return expected.Type
	}
	if actual != nil {
		return actual.Type
	}
	return "?"
}

func compareAt(expected, actual *SNode, path string, opts CompareOptions) *Divergence {
	if expected == nil && actual == nil {
		return nil
	}
	if expected != nil && expected.Unexpected {
		// gotreesitter has no equivalent node kind for the C runtime's
		// UNEXPECTED (single unrecognized byte inside error recovery)
		// marker. Name the gap rather than reporting a generic mismatch.
		return &Divergence{Path: path, Category: CategoryUnexpectedUnsupported, Expected: "(UNEXPECTED ...)", Actual: describe(actual)}
	}
	if expected == nil || actual == nil {
		return &Divergence{
			Path:     path,
			Category: CategoryShape,
			Expected: describe(expected),
			Actual:   describe(actual),
		}
	}
	if expected.Field != actual.Field {
		fieldOK := opts.IgnoreMissingExpectedFields && expected.Field == "" && actual.Field != ""
		if !fieldOK {
			return &Divergence{Path: path, Category: CategoryField, Expected: expected.Field, Actual: actual.Field}
		}
	}
	if expected.Missing != actual.Missing {
		return &Divergence{Path: path, Category: CategoryMissing, Expected: describe(expected), Actual: describe(actual)}
	}
	if expected.Missing {
		if expected.Type != actual.Type || expected.MissingIsAnon != actual.MissingIsAnon {
			return &Divergence{Path: path, Category: CategoryMissingValue, Expected: describe(expected), Actual: describe(actual)}
		}
		return nil
	}
	if expected.Type != actual.Type {
		return &Divergence{Path: path, Category: CategoryType, Expected: expected.Type, Actual: actual.Type}
	}
	if len(expected.Children) != len(actual.Children) {
		// If the expected subtree contains an UNEXPECTED node anywhere,
		// prefer that diagnosis over a bare child-count mismatch -- it's
		// the actionable reason, not a coincidence.
		if containsUnexpected(expected) {
			return &Divergence{Path: path, Category: CategoryUnexpectedUnsupported, Expected: describe(expected), Actual: describe(actual)}
		}
		return &Divergence{
			Path:     path,
			Category: CategoryShape,
			Expected: fmt.Sprintf("%d children", len(expected.Children)),
			Actual:   fmt.Sprintf("%d children", len(actual.Children)),
		}
	}
	for i := range expected.Children {
		childPath := fmt.Sprintf("%s/%s[%d]", path, labelFor(expected.Children[i]), i)
		if d := compareAt(expected.Children[i], actual.Children[i], childPath, opts); d != nil {
			return d
		}
	}
	return nil
}

func containsUnexpected(n *SNode) bool {
	if n == nil {
		return false
	}
	if n.Unexpected {
		return true
	}
	for _, c := range n.Children {
		if containsUnexpected(c) {
			return true
		}
	}
	return false
}

func labelFor(n *SNode) string {
	if n == nil {
		return "?"
	}
	if n.Unexpected {
		return "UNEXPECTED"
	}
	if n.Missing {
		return "MISSING:" + n.Type
	}
	if n.Type == "" {
		return "?"
	}
	return n.Type
}

func describe(n *SNode) string {
	if n == nil {
		return "<nil>"
	}
	if n.Unexpected {
		return "(UNEXPECTED)"
	}
	if n.Missing {
		if n.MissingIsAnon {
			return fmt.Sprintf("(MISSING %q)", n.Type)
		}
		return fmt.Sprintf("(MISSING %s)", n.Type)
	}
	return fmt.Sprintf("(%s %d children)", n.Type, len(n.Children))
}
