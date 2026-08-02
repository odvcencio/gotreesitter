//go:build cgo && treesitter_c_parity && gts_derivation_set_census

package cgoharness

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	sitter "github.com/tree-sitter/go-tree-sitter"

	gotreesitter "github.com/odvcencio/gotreesitter"
)

// Derivation-set differential instrument, stage D0 of
// spec.derivation-set-equivalence.v1.
//
// The lane's premise (finding.derivation-set-equivalence-prerequisite) is
// that selection cannot repair a wrong candidate set: the compact core's
// live derivation set D at a tied accept is not the reference runtime's live
// version set V. Before any repair, the difference must be MEASURED. This
// file measures it.
//
// Compact side. The root package records D at every accept behind the
// gts_derivation_set_census build tag
// (parsercore_phase0_derivation_set_census.go). Each candidate carries its
// materialized public tree, its summed dynamic precedence, and its fork
// stamp.
//
// C side. The vendored C source stays unpatched. ts_parser_set_logger gives
// the parse event stream, and ts_parser__accept
// (go-tree-sitter@v0.25.0/src/parser.c:1046-1097) folds every popped root
// into finished_tree through ts_parser__select_tree (parser.c:836-879). The
// first root sets finished_tree with no log line; every later root logs
// exactly one select_* line. |V| is therefore
//
//	1 + (number of select_* lines that immediately follow an accept line)
//
// counted over the whole parse. ts_parser__select_tree is also reached from
// ts_parser__select_children inside a reduce (parser.c:901), so only the
// contiguous run of select_* lines directly after an accept line belongs to
// an accept fold; every other select_* line is preceded by at least a
// "reduce" line. recover_eof (parser.c:1346) also calls ts_parser__accept,
// so it counts as an accept event too.
//
// What the logger gives and what it withholds is stated exactly in
// cVersionSetReceipt. The instrument never guesses past that boundary: an
// undecidable axis is reported as undetermined, not as agreement.

// Difference classes of the D0 taxonomy.
const (
	// derivationDiffExtra: compact carries an alternative the reference
	// runtime never builds. |D| > |V|.
	derivationDiffExtra = "EXTRA"
	// derivationDiffMissing: the reference runtime carries an alternative
	// compact lacks. |V| > |D|.
	derivationDiffMissing = "MISSING"
	// derivationDiffDifferent: the reference runtime's published tree is not
	// any compact candidate's tree, so the sets disagree on content at equal
	// or unequal cardinality.
	derivationDiffDifferent = "DIFFERENT"
	// derivationDiffOrder: the sets agree, but the compact fold order lists
	// the reference runtime's published tree at a different position than
	// C's version-index order does (equivalence criterion 4).
	derivationDiffOrder = "ORDER"
)

// Mechanism attribution of one difference, per spec sections 2 and 3.
const (
	// derivationMechanismToken: the differing candidates do not share a leaf
	// sequence, so the two runtimes lexed different tokens. Section 3, the
	// first-leaf token cache; stage D2.
	derivationMechanismToken = "token-class"
	// derivationMechanismCondense: the differing candidates share an
	// identical leaf sequence and differ only in how those leaves are
	// grouped, which is the condense-time tie the compact core defers.
	// Section 2, condense-time tie collapse; stage D1.
	derivationMechanismCondense = "condense-class"
	// derivationMechanismUnattributed: no second candidate was available to
	// compare against, so the mechanism stays unnamed rather than guessed.
	derivationMechanismUnattributed = "unattributed"
)

// dsTree is one materialized tree, rebuilt from either runtime into a single
// shape so the two can be compared directly.
type dsTree struct {
	Field    string
	Type     string
	Start    uint32
	End      uint32
	Named    bool
	Extra    bool
	Missing  bool
	Children []*dsTree
}

// dsTreeFromCensus rebuilds a compact candidate's tree from the pre-order
// node slice the census records.
func dsTreeFromCensus(nodes []gotreesitter.DerivationSetCensusNode) *dsTree {
	if len(nodes) == 0 {
		return nil
	}
	index := 0
	var build func() *dsTree
	build = func() *dsTree {
		if index >= len(nodes) {
			return nil
		}
		entry := nodes[index]
		index++
		node := &dsTree{
			Field: entry.Field, Type: entry.Type,
			Start: entry.StartByte, End: entry.EndByte,
			Named: entry.Named, Extra: entry.Extra, Missing: entry.Missing,
		}
		for child := 0; child < entry.ChildCount; child++ {
			node.Children = append(node.Children, build())
		}
		return node
	}
	return build()
}

// dsTreeFromC rebuilds the reference runtime's published tree in the same
// shape. The compact side normalizes a zero-terminator sentinel type to the
// empty string (compactT3GoNodeType); nothing on the C side produces one.
func dsTreeFromC(node *sitter.Node, field string) *dsTree {
	if node == nil {
		return nil
	}
	out := &dsTree{
		Field: field, Type: node.Kind(),
		Start: uint32(node.StartByte()), End: uint32(node.EndByte()),
		Named: node.IsNamed(), Extra: node.IsExtra(), Missing: node.IsMissing(),
	}
	count := node.ChildCount()
	for index := uint(0); index < count; index++ {
		out.Children = append(out.Children, dsTreeFromC(node.Child(index), node.FieldNameForChild(uint32(index))))
	}
	return out
}

// dsShape renders the canonical identity of a tree. Two trees with equal
// shapes publish the same public tree on every axis the materiality gate
// compares (compactAcceptanceDerivationTreesEqual).
func dsShape(node *dsTree) string {
	var builder strings.Builder
	dsWriteShape(&builder, node)
	return builder.String()
}

func dsWriteShape(builder *strings.Builder, node *dsTree) {
	if node == nil {
		builder.WriteString("(nil)")
		return
	}
	builder.WriteString("(")
	if node.Field != "" {
		builder.WriteString(node.Field)
		builder.WriteString(":")
	}
	builder.WriteString(node.Type)
	builder.WriteString("[")
	builder.WriteString(strconv.FormatUint(uint64(node.Start), 10))
	builder.WriteString("-")
	builder.WriteString(strconv.FormatUint(uint64(node.End), 10))
	builder.WriteString("]")
	if !node.Named {
		builder.WriteString("!anon")
	}
	if node.Extra {
		builder.WriteString("!extra")
	}
	if node.Missing {
		builder.WriteString("!missing")
	}
	for _, child := range node.Children {
		builder.WriteString(" ")
		dsWriteShape(builder, child)
	}
	builder.WriteString(")")
}

// dsLeaves returns the tree's terminal sequence as "type[start-end]" entries.
// The mechanism attribution turns on this sequence: equal leaves with a
// different grouping is a condense-time tie (spec section 2); different
// leaves is a token election difference (spec section 3).
func dsLeaves(node *dsTree, out []string) []string {
	if node == nil {
		return out
	}
	if len(node.Children) == 0 {
		return append(out, fmt.Sprintf("%s[%d-%d]", node.Type, node.Start, node.End))
	}
	for _, child := range node.Children {
		out = dsLeaves(child, out)
	}
	return out
}

// dsFirstDifferencePath returns a readable path to the first node where two
// trees disagree, and the two node descriptions there.
func dsFirstDifferencePath(left, right *dsTree) (path, leftDesc, rightDesc string, found bool) {
	return dsWalkFirstDifference(left, right, "/root")
}

func dsWalkFirstDifference(left, right *dsTree, path string) (string, string, string, bool) {
	if left == nil || right == nil {
		if left == nil && right == nil {
			return "", "", "", false
		}
		return path, dsDescribe(left), dsDescribe(right), true
	}
	if left.Type != right.Type || left.Start != right.Start || left.End != right.End ||
		left.Named != right.Named || left.Extra != right.Extra || left.Missing != right.Missing ||
		left.Field != right.Field || len(left.Children) != len(right.Children) {
		return path, dsDescribe(left), dsDescribe(right), true
	}
	for index := range left.Children {
		childPath := fmt.Sprintf("%s/%s[%d]", path, left.Children[index].Type, index)
		if p, l, r, ok := dsWalkFirstDifference(left.Children[index], right.Children[index], childPath); ok {
			return p, l, r, ok
		}
	}
	return "", "", "", false
}

func dsDescribe(node *dsTree) string {
	if node == nil {
		return "<nil>"
	}
	field := ""
	if node.Field != "" {
		field = node.Field + ":"
	}
	return fmt.Sprintf("%s%s[%d-%d] children=%d", field, node.Type, node.Start, node.End, len(node.Children))
}

// dsAttributeMechanism names the mechanism that separates two candidate
// trees. Equal leaf sequences mean both runtimes lexed the same tokens and
// only the grouping differs, which is the same-span precedence tie
// Core.condense defers and stack_node_add_link collapses (spec section 2).
// Different leaf sequences mean the token election itself differed, which is
// the first-leaf token cache (spec section 3).
func dsAttributeMechanism(left, right *dsTree) string {
	if left == nil || right == nil {
		return derivationMechanismUnattributed
	}
	leftLeaves := dsLeaves(left, nil)
	rightLeaves := dsLeaves(right, nil)
	if len(leftLeaves) != len(rightLeaves) {
		return derivationMechanismToken
	}
	for index := range leftLeaves {
		if leftLeaves[index] != rightLeaves[index] {
			return derivationMechanismToken
		}
	}
	return derivationMechanismCondense
}

// cAcceptFold is one ts_parser__select_tree call the logger reported inside
// an accept fold.
type cAcceptFold struct {
	// Rung is the select_* line kind: select_smaller_error,
	// select_higher_precedence, select_earlier, or select_existing.
	Rung string
	// WinnerSymbol and LoserSymbol are the two root symbol names the line
	// carries, winner first. Both are usually the grammar's start symbol at
	// an accept fold, which is why the incumbent cannot always be told from
	// the challenger; see cVersionSetReceipt.WinnerIndexKnown.
	WinnerSymbol string
	LoserSymbol  string
	// WinnerPrecedence and LoserPrecedence are set only for
	// select_higher_precedence, the sole rung that prints numbers.
	WinnerPrecedence int
	LoserPrecedence  int
	HasPrecedence    bool
}

// cAcceptEvent is one accept action (or one recover_eof wrap) and the folds
// it performed.
type cAcceptEvent struct {
	RecoverEOF bool
	Folds      []cAcceptFold
}

// cVersionSetReceipt is the reference runtime's live version set V,
// reconstructed from the logger alone.
//
// What the logger gives:
//   - the number of roots folded at accept, exactly (Roots);
//   - the root symbol name of both sides of every fold;
//   - the dynamic precedence of both sides when the precedence rung fires;
//   - the relative error cost when the smaller-error rung fires;
//   - the live stack version count at every step (VersionCountMax), from the
//     "process version:%u, version_count:%u, ..." line (parser.c:2149).
//
// What the logger withholds:
//   - the span, child count and deep shape of a losing root. A losing root
//     is released inside ts_parser__accept and never reaches the caller, so
//     only the winner (the published tree) is inspectable.
//   - which side of a fold line is the incumbent when both print the same
//     root symbol, which is the normal case because every accept root
//     carries the grammar's start symbol. WinnerIndexKnown reports whether
//     the published root's version index could be established.
type cVersionSetReceipt struct {
	Accepts []cAcceptEvent
	// Roots is |V|: the total number of root trees folded into
	// finished_tree, equal to C's own accept_count.
	Roots int
	// VersionCountMax is the highest live stack version count the parse
	// reported.
	VersionCountMax int
	// WinnerIndex is the version-order index of the published root, valid
	// only when WinnerIndexKnown is true.
	WinnerIndex      int
	WinnerIndexKnown bool
	// Published is the tree the reference runtime published.
	Published *dsTree
	// LogLines is the total parse-log line count, recorded so a silent
	// logger failure cannot masquerade as a one-root parse.
	LogLines int
}

// cReconstructVersionSet parses source with the locked C oracle and rebuilds
// V from the parse log. It never patches or re-reads the vendored C source.
func cReconstructVersionSet(cLang *sitter.Language, source []byte) (cVersionSetReceipt, error) {
	parser := sitter.NewParser()
	defer parser.Close()
	if err := parser.SetLanguage(cLang); err != nil {
		return cVersionSetReceipt{}, fmt.Errorf("set C oracle language: %w", err)
	}
	var lines []string
	parser.SetLogger(func(kind sitter.LogType, message string) {
		if kind == sitter.LogTypeParse {
			lines = append(lines, message)
		}
	})
	tree := parser.Parse(source, nil)
	if tree == nil || tree.RootNode() == nil {
		return cVersionSetReceipt{}, fmt.Errorf("C oracle parse returned no root")
	}
	defer tree.Close()

	receipt := cVersionSetReceipt{LogLines: len(lines), Published: dsTreeFromC(tree.RootNode(), "")}
	for index := 0; index < len(lines); index++ {
		line := lines[index]
		if count, ok := cParseVersionCount(line); ok && count > receipt.VersionCountMax {
			receipt.VersionCountMax = count
		}
		if line != "accept" && line != "recover_eof" {
			continue
		}
		event := cAcceptEvent{RecoverEOF: line == "recover_eof"}
		for index+1 < len(lines) && strings.HasPrefix(lines[index+1], "select_") {
			index++
			event.Folds = append(event.Folds, cParseSelectLine(lines[index]))
		}
		receipt.Accepts = append(receipt.Accepts, event)
	}
	if len(receipt.Accepts) == 0 {
		return receipt, nil
	}
	receipt.Roots = 1
	for _, event := range receipt.Accepts {
		receipt.Roots += len(event.Folds)
	}
	receipt.WinnerIndex, receipt.WinnerIndexKnown = cWinnerVersionIndex(receipt.Accepts)
	return receipt, nil
}

// cWinnerVersionIndex derives the version-order index of the published root
// when the log determines it. select_existing always keeps the incumbent, so
// a fold sequence made only of select_existing lines proves the winner is
// version 0. Every other rung prints the winner first, and both sides carry
// the same start symbol at an accept fold, so the incumbent cannot be told
// from the challenger and the index stays unknown. The instrument reports
// unknown rather than guessing.
func cWinnerVersionIndex(events []cAcceptEvent) (int, bool) {
	for _, event := range events {
		for _, fold := range event.Folds {
			if fold.Rung != "select_existing" {
				return 0, false
			}
		}
	}
	return 0, true
}

var cVersionCountPrefix = "process version:"

func cParseVersionCount(line string) (int, bool) {
	if !strings.HasPrefix(line, cVersionCountPrefix) {
		return 0, false
	}
	for _, field := range strings.Split(line, ", ") {
		if !strings.HasPrefix(field, "version_count:") {
			continue
		}
		value, err := strconv.Atoi(strings.TrimPrefix(field, "version_count:"))
		if err != nil {
			return 0, false
		}
		return value, true
	}
	return 0, false
}

// cParseSelectLine decodes one select_* log line. The four line shapes are
// fixed by parser.c:841-877.
func cParseSelectLine(line string) cAcceptFold {
	rung := line
	if space := strings.IndexByte(line, ' '); space > 0 {
		rung = line[:space]
	}
	fold := cAcceptFold{Rung: rung}
	for _, field := range strings.Split(line, ", ") {
		switch {
		case strings.Contains(field, "symbol:") && !strings.Contains(field, "over_symbol:") && !strings.Contains(field, "other_"):
			if at := strings.Index(field, "symbol:"); at >= 0 {
				fold.WinnerSymbol = field[at+len("symbol:"):]
			}
		case strings.HasPrefix(field, "over_symbol:"):
			fold.LoserSymbol = strings.TrimPrefix(field, "over_symbol:")
		case strings.HasPrefix(field, "prec:"):
			if value, err := strconv.Atoi(strings.TrimPrefix(field, "prec:")); err == nil {
				fold.WinnerPrecedence, fold.HasPrecedence = value, true
			}
		case strings.HasPrefix(field, "other_prec:"):
			if value, err := strconv.Atoi(strings.TrimPrefix(field, "other_prec:")); err == nil {
				fold.LoserPrecedence = value
			}
		}
	}
	return fold
}

// derivationSetDifference is one classified set-level difference at one
// source.
type derivationSetDifference struct {
	Class     string
	Mechanism string
	Detail    string
}

// derivationSetReport is the full differential receipt for one source.
type derivationSetReport struct {
	Language string
	Name     string
	// CompactAccepts is the number of accept events the compact scheduler
	// reached. Zero means the compact route never reached an accept, so
	// there is nothing to compare.
	CompactAccepts int
	// CompactSetSize is |D|, summed over the recorded accepts.
	CompactSetSize int
	// CompactTruncated reports a capped derivation enumeration, which leaves
	// D unknown.
	CompactTruncated bool
	// CompactDeclined reports that the R1 materiality gate declined.
	CompactDeclined bool
	// CompactSelectedFoldIndex is the fold-order position of the derivation
	// the route selected, or -1 when it selected none.
	CompactSelectedFoldIndex int
	// CompactDeclineReason is the route's own fallback reason, recorded only
	// when the compact route reached no accept at all.
	CompactDeclineReason string
	// CVersionSetSize is |V|.
	CVersionSetSize int
	// CVersionCountMax is C's highest live stack version count.
	CVersionCountMax int
	// CWinnerIndexKnown reports whether the published root's version index
	// could be established from the log.
	CWinnerIndexKnown bool
	// CPublishedInCompactSet reports whether the reference runtime's
	// published tree is one of the compact candidates.
	CPublishedInCompactSet bool
	// CPublishedFoldIndex is the compact fold-order position of the
	// candidate that matches the published tree, or -1.
	CPublishedFoldIndex int
	// CandidateShapes and PublishedShape are the compared identities, kept so
	// a receipt can print exactly what the instrument matched.
	CandidateShapes []string
	PublishedShape  string
	// Differences is the classified set-level difference list. An empty list
	// means the two sets agreed on every axis this instrument can decide.
	Differences []derivationSetDifference
	// Skipped records why no comparison ran.
	Skipped string
}

// runDerivationSetDifferential measures one source on both runtimes and
// classifies the set-level difference.
//
// The five legitimate representation differences the spec names are excluded
// by construction, never by a filter:
//
//   - Link cap. The compact core fails closed with LiveLinkCapacityError; C
//     silently drops a ninth link. Neither is compared here: the instrument
//     compares published candidate sets, not link arrays.
//   - Dynamic precedence carrier. The compact core sums per-link ScoreDelta
//     per derivation; C keeps a node-level running maximum. The instrument
//     compares the maximum Score over D against the precedence C's fold
//     lines report, never per-link deltas.
//   - External scanner state. The compact core interns CheckpointIDs; C
//     compares serialized bytes. Neither appears in a published tree, so
//     neither is compared.
//   - Head-merge key. boundaryKey against C's (state, position, error cost,
//     external state). Not compared: the instrument reads the accept, not
//     the merge.
//   - Order carrier. Fork stamps against version indices. Compared only as
//     positions, through the fold order, never as raw values.
func runDerivationSetDifferential(
	language, cLangName string, lang *gotreesitter.Language, cLang *sitter.Language,
	name string, source []byte,
) (derivationSetReport, error) {
	report := derivationSetReport{Language: language, Name: name, CPublishedFoldIndex: -1, CompactSelectedFoldIndex: -1}

	gotreesitter.DerivationSetCensusReset()
	parser := gotreesitter.NewParser(lang)
	parser.SetAdmissionCandidateRoute(true)
	tree, err := parser.Parse(source)
	if err != nil {
		return report, fmt.Errorf("%s/%s: compact-route parse: %w", language, name, err)
	}
	tree.Release()
	accepts := gotreesitter.DerivationSetCensusSnapshot()

	report.CompactAccepts = len(accepts)
	if len(accepts) == 0 {
		report.Skipped = "compact route reached no accept"
		report.CompactDeclineReason = gotreesitter.AdmissionCandidateLastFallbackReason()
		return report, nil
	}

	// D is the derivation set at THE accept, not the union over a parse. The
	// shipped route reaches an authenticated EOF accept with exactly one
	// surviving header, so one parse normally records one accept. A parse
	// that records more (a re-run seed, for example) would otherwise make an
	// n-candidate set look like an n-accept run, so the last recorded accept
	// -- the one that terminated the run -- is the set under comparison and
	// CompactAccepts keeps the count visible.
	accept := accepts[len(accepts)-1]
	report.CompactTruncated = accept.EnumerationTruncated
	report.CompactDeclined = accept.DeclinedMaterial
	report.CompactSelectedFoldIndex = accept.SelectedFoldIndex

	var candidates []*dsTree
	var candidateShapes []string
	for _, candidate := range accept.Candidates {
		report.CompactSetSize++
		if candidate.MaterializeError != "" {
			candidates = append(candidates, nil)
			candidateShapes = append(candidateShapes, "<materialize-error: "+candidate.MaterializeError+">")
			continue
		}
		built := dsTreeFromCensus(candidate.Nodes)
		candidates = append(candidates, built)
		candidateShapes = append(candidateShapes, dsShape(built))
	}
	if report.CompactTruncated {
		report.Skipped = "compact derivation enumeration capped; D unknown"
		return report, nil
	}

	receipt, err := cReconstructVersionSet(cLang, source)
	if err != nil {
		return report, fmt.Errorf("%s/%s: C version-set reconstruction: %w", language, name, err)
	}
	report.CVersionSetSize = receipt.Roots
	report.CVersionCountMax = receipt.VersionCountMax
	report.CWinnerIndexKnown = receipt.WinnerIndexKnown
	if receipt.Roots == 0 {
		report.Skipped = "C oracle log reported no accept event"
		return report, nil
	}

	publishedShape := dsShape(receipt.Published)
	report.CandidateShapes = candidateShapes
	report.PublishedShape = publishedShape
	for index, shape := range candidateShapes {
		if shape == publishedShape {
			report.CPublishedInCompactSet = true
			report.CPublishedFoldIndex = index
			break
		}
	}

	// The candidate that best represents "what compact would have to give up
	// or gain" is the one that does not match C. Pick the first non-matching
	// candidate for the mechanism attribution, compared against the matching
	// one when it exists and against C's published tree otherwise.
	var oddOne *dsTree
	for index, shape := range candidateShapes {
		if shape != publishedShape {
			oddOne = candidates[index]
			break
		}
	}
	reference := receipt.Published
	if report.CPublishedInCompactSet && report.CPublishedFoldIndex >= 0 {
		reference = candidates[report.CPublishedFoldIndex]
	}

	switch {
	case report.CompactSetSize > receipt.Roots:
		mechanism := derivationMechanismUnattributed
		detail := fmt.Sprintf("|D|=%d |V|=%d", report.CompactSetSize, receipt.Roots)
		if oddOne != nil {
			mechanism = dsAttributeMechanism(oddOne, reference)
			path, left, right, ok := dsFirstDifferencePath(oddOne, reference)
			if ok {
				detail = fmt.Sprintf("|D|=%d |V|=%d; extra candidate diverges at %s: compact=%s reference=%s", report.CompactSetSize, receipt.Roots, path, left, right)
			}
		}
		report.Differences = append(report.Differences, derivationSetDifference{
			Class: derivationDiffExtra, Mechanism: mechanism, Detail: detail,
		})
	case report.CompactSetSize < receipt.Roots:
		mechanism := derivationMechanismUnattributed
		if oddOne != nil {
			mechanism = dsAttributeMechanism(oddOne, reference)
		}
		report.Differences = append(report.Differences, derivationSetDifference{
			Class: derivationDiffMissing, Mechanism: mechanism,
			Detail: fmt.Sprintf("|D|=%d |V|=%d", report.CompactSetSize, receipt.Roots),
		})
	}

	if !report.CPublishedInCompactSet {
		mechanism := derivationMechanismUnattributed
		detail := fmt.Sprintf("|D|=%d |V|=%d; no compact candidate equals the published tree", report.CompactSetSize, receipt.Roots)
		if len(candidates) > 0 && candidates[0] != nil {
			mechanism = dsAttributeMechanism(candidates[0], receipt.Published)
			if path, left, right, ok := dsFirstDifferencePath(candidates[0], receipt.Published); ok {
				detail = fmt.Sprintf("|D|=%d |V|=%d; candidate 0 diverges at %s: compact=%s reference=%s", report.CompactSetSize, receipt.Roots, path, left, right)
			}
		}
		report.Differences = append(report.Differences, derivationSetDifference{
			Class: derivationDiffDifferent, Mechanism: mechanism, Detail: detail,
		})
	}

	// ORDER, equivalence criterion 4. Only decidable when the log
	// established the published root's version index. Every other case is
	// reported as undetermined by the census, not as agreement.
	if report.CPublishedInCompactSet && receipt.WinnerIndexKnown && report.CompactSetSize == receipt.Roots &&
		report.CPublishedFoldIndex != receipt.WinnerIndex {
		report.Differences = append(report.Differences, derivationSetDifference{
			Class: derivationDiffOrder, Mechanism: derivationMechanismUnattributed,
			Detail: fmt.Sprintf("published tree sits at compact fold index %d but C version index %d", report.CPublishedFoldIndex, receipt.WinnerIndex),
		})
	}

	return report, nil
}

// derivationSetCensusTotals aggregates classified differences.
type derivationSetCensusTotals struct {
	Sources           int
	Compared          int
	SkippedNoAccept   int
	SkippedOther      int
	Declined          int
	ByClass           map[string]int
	ByMechanism       map[string]int
	OrderUndetermined int
	Differences       int
}

func newDerivationSetCensusTotals() *derivationSetCensusTotals {
	return &derivationSetCensusTotals{ByClass: map[string]int{}, ByMechanism: map[string]int{}}
}

func (t *derivationSetCensusTotals) add(report derivationSetReport) {
	t.Sources++
	switch {
	case report.Skipped == "":
		t.Compared++
	case strings.Contains(report.Skipped, "no accept"):
		t.SkippedNoAccept++
	default:
		t.SkippedOther++
	}
	if report.CompactDeclined {
		t.Declined++
	}
	if report.Skipped == "" && !report.CWinnerIndexKnown {
		t.OrderUndetermined++
	}
	for _, difference := range report.Differences {
		t.Differences++
		t.ByClass[difference.Class]++
		t.ByMechanism[difference.Mechanism]++
	}
}

func (t *derivationSetCensusTotals) classLine() string {
	classes := []string{derivationDiffExtra, derivationDiffMissing, derivationDiffDifferent, derivationDiffOrder}
	parts := make([]string, 0, len(classes))
	for _, class := range classes {
		parts = append(parts, fmt.Sprintf("%s=%d", class, t.ByClass[class]))
	}
	return strings.Join(parts, " ")
}

func (t *derivationSetCensusTotals) mechanismLine() string {
	keys := make([]string, 0, len(t.ByMechanism))
	for key := range t.ByMechanism {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, fmt.Sprintf("%s=%d", key, t.ByMechanism[key]))
	}
	if len(parts) == 0 {
		return "none"
	}
	return strings.Join(parts, " ")
}
