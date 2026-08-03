package gotreesitter

// OutlineSymbol is one entry of a language-neutral file outline: a definition
// the tags query captured, its normalized kind, its name, its spans, and the
// definitions lexically nested inside it.
//
// The projection is read-only. It never changes the tree and never copies a
// definition body; Name and Owner are the only text it extracts.
type OutlineSymbol struct {
	// Kind is the normalized definition kind. It comes from the
	// "@definition.X" capture suffix through one fixed, language-neutral
	// table (outlineKindTable) plus one fixed node-type refinement table
	// (outlineKindRefinement). It never comes from the language name.
	Kind string
	// Name is the text of the "@name" capture. A "#strip!" directive on the
	// capture applies, because the text comes from QueryCapture.Text.
	Name string
	// NodeType is the grammar node type of the captured definition node.
	NodeType string
	// Range is the full span of the captured definition node.
	Range Range
	// NameRange is the span of the "@name" capture. It is always contained
	// in Range; a candidate that breaks containment is omitted and counted.
	NameRange Range
	// Owner is the non-lexical owner name, for example the receiver type of
	// a method. It is always empty in this change. Owner resolution runs
	// from declarative OutlineOwnerRule rows and lands in a later change;
	// see WithOutlineOwnerRules. Lexical containment never sets Owner --
	// that information lives in Children.
	Owner string
	// Children holds the definitions lexically nested in this one, in source
	// order. Nesting comes from byte containment of Range, never from the
	// language name.
	Children []OutlineSymbol
}

// OutlineReport is the receipt for one OutlineTree call. Every candidate the
// tags query produced is either emitted as a symbol or counted in exactly one
// omission counter, so silent under-coverage cannot look like correctness.
//
// The accounting identity the gates assert is:
//
//	Symbols + OmittedNoName + OmittedDuplicate + OmittedConflict +
//	    OmittedOverlap + OmittedInvalidNameRange == candidates
//
// OwnerRuleMisses is not an omission counter; a miss keeps the symbol and
// leaves Owner empty.
type OutlineReport struct {
	// Symbols counts every emitted symbol, nested symbols included.
	Symbols int
	// OmittedNoName counts definition captures whose name capture is absent
	// or whose name text trims to empty.
	OmittedNoName int
	// OmittedDuplicate counts candidates dropped because an earlier
	// candidate holds the same Range and the same Kind.
	OmittedDuplicate int
	// OmittedConflict counts candidates dropped because two or more
	// candidates hold the same Range with different Kinds. Every member of
	// such a group is dropped; the outline does not guess which one is
	// right.
	OmittedConflict int
	// OmittedOverlap counts candidates dropped because they partially
	// overlap an accepted symbol. Neither range contains the other, so the
	// forest has no well-defined shape; the later start is dropped.
	OmittedOverlap int
	// OmittedInvalidNameRange counts candidates whose NameRange is not byte
	// contained in Range, or whose Range is itself inverted.
	OmittedInvalidNameRange int
	// OwnerRuleMisses counts symbols where an owner rule matched the node
	// type but the field or the normalization did not resolve a single
	// name. It is always zero until owner resolution ships.
	OwnerRuleMisses int
	// QueryEmpty reports that the language has no tags query. The outliner
	// declines: no symbols, and this flag as the receipt.
	QueryEmpty bool
	// Truncated reports that query execution hit the match limit or the
	// match work budget, so the symbol list is partial.
	Truncated bool
	// LanguageMismatch reports that the tree was parsed with a different
	// language than the outliner was built for. The outliner declines,
	// because query symbol identifiers are language specific.
	LanguageMismatch bool
}

// Omitted returns the total number of candidates the outliner dropped.
func (r OutlineReport) Omitted() int {
	return r.OmittedNoName +
		r.OmittedDuplicate +
		r.OmittedConflict +
		r.OmittedOverlap +
		r.OmittedInvalidNameRange
}

// Candidates returns the number of definition candidates the tags query
// produced: the emitted symbols plus every omission.
func (r OutlineReport) Candidates() int {
	return r.Symbols + r.Omitted()
}

// OutlineOwnerRule is a declarative, per-language rule that resolves the
// non-lexical owner of a definition, for example the receiver type of a Go
// method. It reads a named field and unwraps through a fixed list of node
// types until it reaches exactly one node of an accepted terminal type. If
// unwrapping does not end at exactly one such node, the rule fails closed and
// Owner stays empty.
//
// Rules are data. The core holds no rule rows; the grammars package owns the
// per-language table and passes it through WithOutlineOwnerRules.
//
// Owner resolution is not applied in this change. See WithOutlineOwnerRules.
type OutlineOwnerRule struct {
	// NodeType is the definition node type the rule applies to, for example
	// "method_declaration".
	NodeType string
	// OwnerField is the field name read through Node.ChildByFieldName, for
	// example "receiver".
	OwnerField string
	// Unwrap lists node types the rule descends through, for example
	// "parameter_list", "parameter_declaration", "pointer_type".
	Unwrap []string
	// NameTypes lists the accepted terminal node types, for example
	// "type_identifier".
	NameTypes []string
}
