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
	// Name is the text of the "@name" capture, with leading and trailing
	// white space removed. A "#strip!" directive on the capture applies,
	// because the text comes from QueryCapture.Text.
	//
	// Invariant: NameRange is the span of the capture NODE, so Name and the
	// source bytes at NameRange agree only when no directive rewrote the
	// text and the node carries no surrounding white space. Trimming or a
	// directive can make Name shorter than NameRange.
	// TestOutlineNameAgreesWithNameRange pins that agreement on every
	// committed fixture, so a language where the two diverge shows up as a
	// test failure rather than as a silently wrong span.
	Name string
	// NodeType is the grammar node type of the captured definition node.
	NodeType string
	// Range is the full span of the captured definition node.
	Range Range
	// NameRange is the span of the "@name" capture node. It is always
	// contained in Range; a candidate that breaks containment is omitted and
	// counted.
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

// Outline decline reasons. An empty DeclineReason means the outliner ran the
// query; it does not mean the file holds symbols.
const (
	// OutlineDeclineNilOutliner reports a call on a nil *Outliner.
	OutlineDeclineNilOutliner = "nil_outliner"
	// OutlineDeclineQueryEmpty reports that the language has no tags query.
	OutlineDeclineQueryEmpty = "query_empty"
	// OutlineDeclineNilTree reports a nil *Tree argument.
	OutlineDeclineNilTree = "nil_tree"
	// OutlineDeclineNilRootNode reports a tree with no root node.
	OutlineDeclineNilRootNode = "nil_root_node"
	// OutlineDeclineLanguageMismatch reports that the tree was parsed with a
	// different language than the outliner was built for. This is caller
	// misuse, not a property of the source.
	OutlineDeclineLanguageMismatch = "language_mismatch"
)

// OutlineReport is the receipt for one OutlineTree call. Every candidate the
// tags query produced is either emitted as a symbol or counted in exactly one
// omission counter, so a dropped candidate cannot look like an absent one.
//
// The accounting identity the gates assert is:
//
//	Symbols + OmittedNoName + OmittedDuplicate + OmittedNameConflict +
//	    OmittedConflict + OmittedOverlap + OmittedInvalidNameRange +
//	    OmittedMultipleDefinitions == candidates
//
// OwnerRuleMisses is not an omission counter; a miss keeps the symbol and
// leaves Owner empty.
//
// READ THIS BEFORE TREATING Omitted() == 0 AS "COMPLETE". Candidates() counts
// what the tags query produced, not what the file contains. A definition the
// query has no pattern for produces no candidate and no omission, so it is
// invisible to every counter here. Call Outliner.DefinitionKinds to see the
// kinds the compiled query can emit at all; a kind absent from that list can
// never appear in the outline, however many such definitions the file holds.
type OutlineReport struct {
	// Symbols counts every emitted symbol, nested symbols included.
	Symbols int
	// OmittedNoName counts definition captures whose name capture is absent
	// or whose name text trims to empty.
	OmittedNoName int
	// OmittedDuplicate counts candidates dropped because an earlier
	// candidate holds the same Range, the same Kind, the same Name, and the
	// same NameRange. These are true repeats: dropping one changes nothing.
	OmittedDuplicate int
	// OmittedNameConflict counts candidates dropped because two or more
	// candidates hold the same Range and the same Kind but disagree about
	// the name. Every member of such a group is dropped.
	//
	// This is the common shape when shared tags patterns bind "@name" twice
	// on one definition node. In C-family method syntax the two bindings are
	// the return type and the method name, and no language-neutral rule can
	// tell them apart. Choosing by capture order would publish the return
	// type as the method name, so the outline drops the group instead and
	// counts it here. The remedy is a data edit: constrain the pattern with
	// a "name:" field, the way the Go tags override already does.
	OmittedNameConflict int
	// OmittedConflict counts candidates dropped because two or more
	// candidates hold the same Range with different Kinds. Every member of
	// such a group is dropped; the outline does not guess which one is
	// right.
	OmittedConflict int
	// OmittedOverlap counts candidates dropped because they partially
	// overlap an accepted symbol.
	//
	// This rule is defensive. Two nodes of one tree are always nested or
	// disjoint, so a partial overlap cannot arise from a single tree today.
	// The rule keeps the forest well-defined if a future capture source
	// relaxes that, and the unit cases exercise it directly.
	OmittedOverlap int
	// OmittedInvalidNameRange counts candidates whose NameRange is not byte
	// contained in Range, or whose Range is itself inverted. Like
	// OmittedOverlap this is defensive: the captures of a match all sit
	// inside the matched subtree, so a single tree does not produce this
	// today.
	OmittedInvalidNameRange int
	// OmittedMultipleDefinitions counts matches dropped because they carry
	// more than one "@definition.X" capture. Such a match names two
	// definitions at once and the outline does not choose between them.
	// No inferred pattern does this today; the counter exists so that a data
	// edit that introduces the shape is visible instead of silent.
	OmittedMultipleDefinitions int
	// OwnerRuleMisses counts symbols where an owner rule matched the node
	// type but the field or the normalization did not resolve a single
	// name. It is always zero until owner resolution ships.
	OwnerRuleMisses int
	// DeclineReason names why the outliner produced nothing, or is empty
	// when it ran the query. See the OutlineDecline constants. An empty
	// reason with zero Symbols means the query ran and matched nothing,
	// which is a different fact from every declined case.
	DeclineReason string
	// Truncated reports that query execution hit the match limit or the
	// match work budget, so the symbol list is partial.
	Truncated bool
	// TreeHasError reports that the parsed tree holds an ERROR or MISSING
	// node, so the parser did not fully recover the source.
	//
	// Symbols are the query's projection of the RECOVERED tree. When
	// TreeHasError is true, definitions inside or after unrecovered regions
	// may be absent, over-long, or missing entirely, and no omission counter
	// fires for them: the query never produced a candidate to omit. Do not
	// read "every counter is zero" as "the outline is complete" on such a
	// tree. Do not assume the symbols before the first error are
	// trustworthy either; a truncated definition is itself a symbol, and its
	// Range can swallow everything that follows it.
	TreeHasError bool
}

// Declined reports whether the outliner produced nothing because it refused to
// run, rather than because the query matched nothing.
func (r OutlineReport) Declined() bool { return r.DeclineReason != "" }

// Omitted returns the total number of candidates the outliner dropped.
func (r OutlineReport) Omitted() int {
	return r.OmittedNoName +
		r.OmittedDuplicate +
		r.OmittedNameConflict +
		r.OmittedConflict +
		r.OmittedOverlap +
		r.OmittedInvalidNameRange +
		r.OmittedMultipleDefinitions
}

// Candidates returns the number of definition candidates the tags query
// produced: the emitted symbols plus every omission.
//
// This is a count of QUERY OUTPUT, not of source content. A definition shape
// the query has no pattern for never becomes a candidate.
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
