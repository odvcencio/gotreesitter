// Package parsercorephase0 contains a diagnostic-only parser-core prototype.
//
// It deliberately is not imported by the production parser. The prototype
// consumes decoded Language parse tables, but it does not own a lexer, an
// external-scanner election, recovery, retries, included ranges, or
// incremental parsing. The next parser-core count seam is authenticated token
// replay bound to each token's exact normalized active election-state set,
// lexical state, and scanner checkpoint; a frontier mismatch must decline.
// Exact scanner/election integration is still required before full-parse
// timing. The next integration point is an external diagnostic harness that
// imports both the production parser and this package; the root gotreesitter
// package must not import this diagnostic package. If the direct-work gate
// passes, proven primitive records can move behind a dependency-neutral
// internal boundary. Callers must treat a decline as a request to use the
// production parser; this package never silently substitutes partial work.
package parsercorephase0

import (
	"errors"
	"fmt"
	"math"

	gts "github.com/odvcencio/gotreesitter"
)

// Decline identifies a feature that phase zero cannot execute faithfully.
type Decline string

const (
	DeclineExternalScanner Decline = "external_scanner"
	DeclineRecovery        Decline = "recovery"
	DeclineRetry           Decline = "retry"
	DeclineIncludedRanges  Decline = "included_ranges"
	DeclineIncremental     Decline = "incremental"
	DeclineFleet           Decline = "fleet"
	DeclineExtras          Decline = "reduction_extras"
	DeclineLexerLoop       Decline = "lexer_and_election_loop"
)

// DeclineError is returned instead of weakening a phase-zero boundary.
type DeclineError struct {
	Feature Decline
	Detail  string
}

func (e *DeclineError) Error() string {
	if e == nil || e.Detail == "" {
		return "parser-core phase zero declined " + string(e.Feature)
	}
	return "parser-core phase zero declined " + string(e.Feature) + ": " + e.Detail
}

// IsDecline reports whether err is a phase-zero fail-closed decline.
func IsDecline(err error, feature Decline) bool {
	var decline *DeclineError
	return errors.As(err, &decline) && decline.Feature == feature
}

// FullParseRequest describes only the unsupported boundaries that must remain
// explicit while the compact graph is a diagnostic slice.
type FullParseRequest struct {
	Recovery       bool
	Retry          bool
	IncludedRanges bool
	Incremental    bool
	Fleet          bool
}

// AdmitFullParse always declines until the lexer/election loop is integrated.
// Feature-specific declines precede that missing seam so a caller never
// mistakes an unsupported parse shape for parser-core-only coverage.
func AdmitFullParse(lang *gts.Language, req FullParseRequest) error {
	switch {
	case lang == nil:
		return &DeclineError{Feature: DeclineLexerLoop, Detail: "nil language"}
	case req.Incremental:
		return &DeclineError{Feature: DeclineIncremental}
	case req.Recovery:
		return &DeclineError{Feature: DeclineRecovery}
	case req.Retry:
		return &DeclineError{Feature: DeclineRetry}
	case req.IncludedRanges:
		return &DeclineError{Feature: DeclineIncludedRanges}
	case req.Fleet:
		return &DeclineError{Feature: DeclineFleet}
	case lang.ExternalTokenCount != 0 || len(lang.ExternalSymbols) != 0 || lang.ExternalScanner != nil:
		return &DeclineError{
			Feature: DeclineExternalScanner,
			Detail:  "requires frontier-bound authenticated token replay for parser-core counts and certified scanner/election semantics before full-parse timing",
		}
	default:
		return &DeclineError{
			Feature: DeclineLexerLoop,
			Detail:  "compact graph consumes decoded actions but does not yet elect or lex tokens",
		}
	}
}

// Limits bound every pointer-free arena and the exact derivation fanout.
type Limits struct {
	MaxNodes            uint32
	MaxLinks            uint32
	MaxSubtrees         uint32
	MaxChildren         uint32
	MaxMetadata         uint32
	MaxPathsPerBoundary uint64
	MaxEnumeration      uint64
}

func (l Limits) withDefaults() Limits {
	if l.MaxNodes == 0 {
		l.MaxNodes = 4096
	}
	if l.MaxLinks == 0 {
		l.MaxLinks = 8192
	}
	if l.MaxSubtrees == 0 {
		l.MaxSubtrees = 8192
	}
	if l.MaxChildren == 0 {
		l.MaxChildren = 32768
	}
	if l.MaxMetadata == 0 {
		l.MaxMetadata = 32768
	}
	if l.MaxPathsPerBoundary == 0 {
		l.MaxPathsPerBoundary = 6
	}
	if l.MaxEnumeration == 0 {
		l.MaxEnumeration = l.MaxPathsPerBoundary
	}
	return l
}

// NodeID, LinkID, and SubtreeID are one-based indexes into pointer-free arenas.
type NodeID uint32
type LinkID uint32
type SubtreeID uint32

type boundaryKey struct {
	frontier   uint64
	state      gts.StateID
	byteOffset uint32
}

type nodeRecord struct {
	state      gts.StateID
	byteOffset uint32
	firstLink  uint32
	linkCount  uint32
	pathCount  uint64
}

type linkRecord struct {
	scoreDelta int64
	order      uint64
	prev       NodeID
	payload    SubtreeID
	next       LinkID
	flags      uint32
}

const linkFlagHasOrder uint32 = 1 << iota

func (l linkRecord) hasOrder() bool { return l.flags&linkFlagHasOrder != 0 }

type subtreeRecord struct {
	symbol            gts.Symbol
	productionID      uint16
	dynamicPrecedence int16
	startByte         uint32
	endByte           uint32
	firstChild        uint32
	childCount        uint32
	firstField        uint32
	fieldCount        uint32
	firstAlias        uint32
	aliasCount        uint32
	extra             bool
	terminal          bool
}

// PathMeta is stored on a graph link. ScoreDelta includes the contributions
// collapsed into that payload; BranchOrder optionally overrides the current
// path-local order when an authenticated dispatch event created a fork.
type PathMeta struct {
	ScoreDelta  int64
	BranchOrder ForkOrder
}

// ForkOrder is an externally authenticated current-order override produced by
// a real dispatch fork event. Table cardinality and action ordinal are not
// sufficient to infer fork creation, so phase zero never invents one.
type ForkOrder struct {
	Value   uint64
	Present bool
}

// Token describes a parser-core terminal payload. Lexing is intentionally out
// of scope; the symbol and span must already be authenticated by the caller.
type Token struct {
	Symbol    gts.Symbol
	StartByte uint32
	EndByte   uint32
	Extra     bool
}

// Head is a compact parse-version head referencing a persistent graph node.
type Head struct {
	Node NodeID
}

// Derivation is one exact root-to-head path. No score/order selection occurs
// while alternatives are condensed.
type Derivation struct {
	Payloads       []SubtreeID
	Score          int64
	BranchOrder    uint64
	HasBranchOrder bool
}

// SubtreeView exposes immutable reduction identity for tests and diagnostics.
type SubtreeView struct {
	Symbol            gts.Symbol
	ProductionID      uint16
	DynamicPrecedence int16
	StartByte         uint32
	EndByte           uint32
	Children          []SubtreeID
	Fields            []gts.FieldMapEntry
	Aliases           []gts.Symbol
	Extra             bool
	Terminal          bool
}

// Stats reports physical storage separately from semantic path counts. It is
// not a replacement for production work-count emissions.
type Stats struct {
	Nodes             uint32
	Links             uint32
	Subtrees          uint32
	CurrentExactPaths uint64
}

// Core is the compact, persistent diagnostic graph. All records are indexes
// into pointer-free slices; the production parser is unaffected.
type Core struct {
	lang       *gts.Language
	limits     Limits
	nodes      []nodeRecord
	links      []linkRecord
	subtrees   []subtreeRecord
	children   []SubtreeID
	fields     []gts.FieldMapEntry
	aliases    []gts.Symbol
	frontier   uint64
	boundaries map[boundaryKey]NodeID
}

type checkpoint struct {
	nodes, links, subtrees, children, fields, aliases int
	boundaries                                        map[boundaryKey]NodeID
}

func (c *Core) mark() checkpoint {
	// This full map clone makes the scaffold transactionally honest but
	// intentionally invalidates wall-time evidence. Replace it with an
	// append-only mutation journal before any timing claim.
	boundaries := make(map[boundaryKey]NodeID, len(c.boundaries))
	for key, id := range c.boundaries {
		boundaries[key] = id
	}
	return checkpoint{
		nodes: len(c.nodes), links: len(c.links), subtrees: len(c.subtrees),
		children: len(c.children), fields: len(c.fields), aliases: len(c.aliases),
		boundaries: boundaries,
	}
}

func (c *Core) restore(mark checkpoint) {
	c.nodes = c.nodes[:mark.nodes]
	c.links = c.links[:mark.links]
	c.subtrees = c.subtrees[:mark.subtrees]
	c.children = c.children[:mark.children]
	c.fields = c.fields[:mark.fields]
	c.aliases = c.aliases[:mark.aliases]
	c.boundaries = mark.boundaries
}

func New(lang *gts.Language, limits Limits) (*Core, error) {
	if lang == nil {
		return nil, errors.New("parser-core phase zero: nil language")
	}
	limits = limits.withDefaults()
	if limits.MaxPathsPerBoundary > limits.MaxEnumeration {
		return nil, fmt.Errorf("parser-core phase zero: path cap %d exceeds enumeration cap %d", limits.MaxPathsPerBoundary, limits.MaxEnumeration)
	}
	return &Core{lang: lang, limits: limits, frontier: 1, boundaries: make(map[boundaryKey]NodeID)}, nil
}

// BeginFrontier starts one authenticated election/dispatch generation.
// Boundary condensation never crosses generations, even when state and byte
// position repeat. Existing heads remain valid persistent predecessors.
func (c *Core) BeginFrontier() error {
	if c.frontier == math.MaxUint64 {
		return errors.New("parser-core phase zero: frontier epoch overflow")
	}
	c.frontier++
	return nil
}

func (c *Core) boundaryKey(state gts.StateID, byteOffset uint32) boundaryKey {
	return boundaryKey{frontier: c.frontier, state: state, byteOffset: byteOffset}
}

// Seed creates one empty derivation at a parser boundary.
func (c *Core) Seed(state gts.StateID, byteOffset uint32) (Head, error) {
	key := c.boundaryKey(state, byteOffset)
	if id := c.boundaries[key]; id != 0 {
		return Head{Node: id}, nil
	}
	id, err := c.appendNode(nodeRecord{state: state, byteOffset: byteOffset, pathCount: 1})
	if err != nil {
		return Head{}, err
	}
	c.boundaries[key] = id
	return Head{Node: id}, nil
}

// Actions returns the authentic decoded action entry for (state, lookahead).
func (c *Core) Actions(state gts.StateID, lookahead gts.Symbol) ([]gts.ParseAction, error) {
	idx, err := lookupActionIndex(c.lang, state, lookahead)
	if err != nil || idx == 0 {
		return nil, err
	}
	if int(idx) >= len(c.lang.ParseActions) {
		return nil, fmt.Errorf("parser-core phase zero: action index %d out of range", idx)
	}
	actions := c.lang.ParseActions[idx].Actions
	return actions, nil
}

// Shift applies one authentic decoded shift action and condenses the resulting
// exact path at its (state, byte) boundary.
func (c *Core) Shift(head Head, lookahead gts.Symbol, actionOrdinal int, token Token, fork ForkOrder) (out Head, err error) {
	mark := c.mark()
	defer func() {
		if err != nil {
			c.restore(mark)
		}
	}()
	act, err := c.action(head, lookahead, actionOrdinal)
	if err != nil {
		return Head{}, err
	}
	if act.Type != gts.ParseActionShift {
		return Head{}, fmt.Errorf("parser-core phase zero: action %d is %v, not shift", actionOrdinal, act.Type)
	}
	if token.Symbol != lookahead {
		return Head{}, fmt.Errorf("parser-core phase zero: token symbol %d != lookahead %d", token.Symbol, lookahead)
	}
	if token.Extra != act.Extra {
		return Head{}, fmt.Errorf("parser-core phase zero: token extra=%t disagrees with decoded action extra=%t", token.Extra, act.Extra)
	}
	current, err := c.node(head.Node)
	if err != nil {
		return Head{}, err
	}
	targetState := act.State
	if act.Extra && targetState == 0 {
		// Match production extraShiftTargetState: an extra shift with target
		// zero leaves the LR state unchanged.
		targetState = current.state
	}
	payload, err := c.appendSubtree(subtreeRecord{
		symbol: token.Symbol, startByte: token.StartByte, endByte: token.EndByte,
		extra: act.Extra, terminal: true,
	}, nil, nil, nil)
	if err != nil {
		return Head{}, err
	}
	return c.condense(c.boundaryKey(targetState, token.EndByte), linkInput{
		prev: head.Node, payload: payload, order: fork,
	})
}

// AppendDiagnosticPayload adds an already-authenticated terminal payload. It
// is only a setup seam for exercising real reductions before lexer/election
// integration; it is not a parse action and is deliberately named as such.
func (c *Core) AppendDiagnosticPayload(head Head, state gts.StateID, token Token, meta PathMeta) (out Head, err error) {
	mark := c.mark()
	defer func() {
		if err != nil {
			c.restore(mark)
		}
	}()
	if _, err := c.node(head.Node); err != nil {
		return Head{}, err
	}
	payload, err := c.appendSubtree(subtreeRecord{
		symbol: token.Symbol, startByte: token.StartByte, endByte: token.EndByte,
		extra: token.Extra, terminal: true,
	}, nil, nil, nil)
	if err != nil {
		return Head{}, err
	}
	return c.condense(c.boundaryKey(state, token.EndByte), linkInput{
		prev: head.Node, payload: payload, scoreDelta: meta.ScoreDelta, order: meta.BranchOrder,
	})
}

// Reduce applies one authentic decoded reduction to every exact pop path in
// head. Equivalent boundaries are condensed without discarding derivations.
func (c *Core) Reduce(head Head, lookahead gts.Symbol, actionOrdinal int, fork ForkOrder) (frontier []Head, err error) {
	mark := c.mark()
	defer func() {
		if err != nil {
			c.restore(mark)
		}
	}()
	act, err := c.action(head, lookahead, actionOrdinal)
	if err != nil {
		return nil, err
	}
	if act.Type != gts.ParseActionReduce {
		return nil, fmt.Errorf("parser-core phase zero: action %d is %v, not reduce", actionOrdinal, act.Type)
	}
	n, err := c.node(head.Node)
	if err != nil {
		return nil, err
	}
	paths, err := c.popPaths(head.Node, int(act.ChildCount))
	if err != nil {
		return nil, err
	}
	if len(paths) == 0 {
		return nil, errors.New("parser-core phase zero: reduction has no exact pop path")
	}
	frontierByBoundary := make(map[boundaryKey]Head)
	var boundaryOrder []boundaryKey
	for _, path := range paths {
		prev, err := c.node(path.prev)
		if err != nil {
			return nil, err
		}
		gotoState, err := lookupGoto(c.lang, prev.state, act.Symbol)
		if err != nil {
			return nil, err
		}
		if gotoState == 0 {
			return nil, fmt.Errorf("parser-core phase zero: no goto from state %d for reduced symbol %d", prev.state, act.Symbol)
		}
		fields, err := productionFields(c.lang, act.ProductionID)
		if err != nil {
			return nil, err
		}
		aliases, err := productionAliases(c.lang, act.ProductionID, len(path.children))
		if err != nil {
			return nil, err
		}
		payload, err := c.appendSubtree(subtreeRecord{
			symbol: act.Symbol, productionID: act.ProductionID,
			dynamicPrecedence: act.DynamicPrecedence,
			startByte:         path.startByte, endByte: n.byteOffset,
		}, path.children, fields, aliases)
		if err != nil {
			return nil, err
		}
		order := path.order
		if fork.Present {
			order = fork
		}
		scoreDelta, err := checkedAddScore(path.score, int64(act.DynamicPrecedence))
		if err != nil {
			return nil, err
		}
		key := c.boundaryKey(gotoState, n.byteOffset)
		out, err := c.condense(key, linkInput{
			prev: path.prev, payload: payload,
			scoreDelta: scoreDelta, order: order,
		})
		if err != nil {
			return nil, err
		}
		if _, seen := frontierByBoundary[key]; !seen {
			boundaryOrder = append(boundaryOrder, key)
		}
		frontierByBoundary[key] = out
	}
	frontier = make([]Head, 0, len(boundaryOrder))
	for _, key := range boundaryOrder {
		frontier = append(frontier, frontierByBoundary[key])
	}
	return frontier, nil
}

func (c *Core) action(head Head, lookahead gts.Symbol, ordinal int) (gts.ParseAction, error) {
	n, err := c.node(head.Node)
	if err != nil {
		return gts.ParseAction{}, err
	}
	actions, err := c.Actions(n.state, lookahead)
	if err != nil {
		return gts.ParseAction{}, err
	}
	if ordinal < 0 || ordinal >= len(actions) {
		return gts.ParseAction{}, fmt.Errorf("parser-core phase zero: action ordinal %d out of range", ordinal)
	}
	return actions[ordinal], nil
}

type linkInput struct {
	prev       NodeID
	payload    SubtreeID
	scoreDelta int64
	order      ForkOrder
}

func (c *Core) condense(key boundaryKey, in linkInput) (Head, error) {
	if key.frontier != c.frontier {
		return Head{}, errors.New("parser-core phase zero: boundary frontier mismatch")
	}
	prev, err := c.node(in.prev)
	if err != nil {
		return Head{}, err
	}
	if _, err := c.subtree(in.payload); err != nil {
		return Head{}, err
	}
	var oldID NodeID
	var old nodeRecord
	var oldLinks []linkRecord
	oldID = c.boundaries[key]
	if oldID != 0 {
		oldRecord, err := c.node(oldID)
		if err != nil {
			return Head{}, err
		}
		old = *oldRecord
		oldLinks, err = c.nodeLinks(old)
		if err != nil {
			return Head{}, err
		}
		for _, link := range oldLinks {
			equal, err := c.linkEqualInput(link, in)
			if err != nil {
				return Head{}, err
			}
			if equal {
				return Head{Node: oldID}, nil
			}
		}
	}
	newPathCount := prev.pathCount
	if oldID != 0 {
		if math.MaxUint64-newPathCount < old.pathCount {
			return Head{}, errors.New("parser-core phase zero: exact path count overflow")
		}
		newPathCount += old.pathCount
	}
	if newPathCount > c.limits.MaxPathsPerBoundary {
		return Head{}, fmt.Errorf("parser-core phase zero: shared (%d,%d) exact-path cap exceeded: %d > %d", key.state, key.byteOffset, newPathCount, c.limits.MaxPathsPerBoundary)
	}
	if uint64(len(c.links))+1 > uint64(c.limits.MaxLinks) || len(c.links) >= math.MaxUint32 {
		return Head{}, errors.New("parser-core phase zero: link arena cap")
	}
	if uint64(len(c.nodes))+1 > uint64(c.limits.MaxNodes) || len(c.nodes) >= math.MaxUint32 {
		return Head{}, errors.New("parser-core phase zero: node arena cap")
	}
	linkCount := uint32(1)
	if oldID != 0 {
		if old.linkCount == math.MaxUint32 {
			return Head{}, errors.New("parser-core phase zero: boundary link count overflow")
		}
		linkCount += old.linkCount
	}
	linkID := LinkID(len(c.links) + 1)
	flags := uint32(0)
	if in.order.Present {
		flags |= linkFlagHasOrder
	}
	c.links = append(c.links, linkRecord{
		prev: in.prev, payload: in.payload, scoreDelta: in.scoreDelta,
		order: in.order.Value, flags: flags, next: LinkID(old.firstLink),
	})
	id, err := c.appendNode(nodeRecord{
		state: key.state, byteOffset: key.byteOffset,
		firstLink: uint32(linkID), linkCount: linkCount, pathCount: newPathCount,
	})
	if err != nil {
		return Head{}, err
	}
	c.boundaries[key] = id
	return Head{Node: id}, nil
}

func (c *Core) linkEqualInput(link linkRecord, in linkInput) (bool, error) {
	return link.prev == in.prev && link.payload == in.payload && link.scoreDelta == in.scoreDelta &&
		link.hasOrder() == in.order.Present && (!link.hasOrder() || link.order == in.order.Value), nil
}

type popPath struct {
	prev      NodeID
	children  []SubtreeID
	score     int64
	order     ForkOrder
	startByte uint32
}

func (c *Core) popPaths(head NodeID, childCount int) ([]popPath, error) {
	if childCount == 0 {
		n, err := c.node(head)
		if err != nil {
			return nil, err
		}
		links, err := c.nodeLinks(*n)
		if err != nil {
			return nil, err
		}
		for _, link := range links {
			payload, err := c.subtree(link.payload)
			if err != nil {
				return nil, err
			}
			if payload.extra {
				return nil, &DeclineError{Feature: DeclineExtras, Detail: "zero-child reduction below trailing extras is not implemented"}
			}
		}
		return []popPath{{prev: head, startByte: n.byteOffset}}, nil
	}
	var out []popPath
	visiting := make(map[NodeID]bool)
	var rev []SubtreeID
	var revScores []int64
	var revOrders []ForkOrder
	var walk func(NodeID, int) error
	walk = func(id NodeID, remaining int) error {
		if visiting[id] {
			return errors.New("parser-core phase zero: graph cycle")
		}
		n, err := c.node(id)
		if err != nil {
			return err
		}
		visiting[id] = true
		defer delete(visiting, id)
		links, err := c.nodeLinks(*n)
		if err != nil {
			return err
		}
		for _, link := range links {
			payload, err := c.subtree(link.payload)
			if err != nil {
				return err
			}
			if payload.extra {
				return &DeclineError{Feature: DeclineExtras, Detail: "exact trailing-extra re-push is not implemented"}
			}
			rev = append(rev, link.payload)
			revScores = append(revScores, link.scoreDelta)
			revOrders = append(revOrders, ForkOrder{Value: link.order, Present: link.hasOrder()})
			next := remaining
			next--
			if next == 0 {
				if uint64(len(out)) >= c.limits.MaxEnumeration {
					return errors.New("parser-core phase zero: pop enumeration cap")
				}
				prev, err := c.node(link.prev)
				if err != nil {
					return err
				}
				path := popPath{prev: link.prev, startByte: prev.byteOffset}
				for i := len(rev) - 1; i >= 0; i-- {
					path.children = append(path.children, rev[i])
					path.score, err = checkedAddScore(path.score, revScores[i])
					if err != nil {
						return err
					}
					if revOrders[i].Present {
						path.order = revOrders[i]
					}
				}
				out = append(out, path)
			} else if err := walk(link.prev, next); err != nil {
				return err
			}
			rev = rev[:len(rev)-1]
			revScores = revScores[:len(revScores)-1]
			revOrders = revOrders[:len(revOrders)-1]
		}
		return nil
	}
	if err := walk(head, childCount); err != nil {
		return nil, err
	}
	return out, nil
}

// Derivations enumerates the exact alternatives represented by head.
func (c *Core) Derivations(head Head) ([]Derivation, error) {
	var out []Derivation
	visiting := make(map[NodeID]bool)
	var walk func(NodeID) ([]Derivation, error)
	walk = func(id NodeID) ([]Derivation, error) {
		if visiting[id] {
			return nil, errors.New("parser-core phase zero: graph cycle")
		}
		n, err := c.node(id)
		if err != nil {
			return nil, err
		}
		if n.linkCount == 0 {
			if n.pathCount != 1 {
				return nil, errors.New("parser-core phase zero: malformed seed path count")
			}
			return []Derivation{{}}, nil
		}
		visiting[id] = true
		defer delete(visiting, id)
		var paths []Derivation
		links, err := c.nodeLinks(*n)
		if err != nil {
			return nil, err
		}
		for _, link := range links {
			prefixes, err := walk(link.prev)
			if err != nil {
				return nil, err
			}
			for _, prefix := range prefixes {
				if uint64(len(paths)) >= c.limits.MaxEnumeration {
					return nil, errors.New("parser-core phase zero: derivation enumeration cap")
				}
				score, err := checkedAddScore(prefix.Score, link.scoreDelta)
				if err != nil {
					return nil, err
				}
				path := Derivation{Score: score}
				path.Payloads = append(path.Payloads, prefix.Payloads...)
				path.Payloads = append(path.Payloads, link.payload)
				path.BranchOrder = prefix.BranchOrder
				path.HasBranchOrder = prefix.HasBranchOrder
				if link.hasOrder() {
					path.BranchOrder = link.order
					path.HasBranchOrder = true
				}
				paths = append(paths, path)
			}
		}
		if uint64(len(paths)) != n.pathCount {
			return nil, fmt.Errorf("parser-core phase zero: path-count mismatch: enumerated %d, recorded %d", len(paths), n.pathCount)
		}
		return paths, nil
	}
	paths, err := walk(head.Node)
	if err != nil {
		return nil, err
	}
	out = append(out, paths...)
	return out, nil
}

func (c *Core) Subtree(id SubtreeID) (SubtreeView, error) {
	r, err := c.subtree(id)
	if err != nil {
		return SubtreeView{}, err
	}
	view := SubtreeView{
		Symbol: r.symbol, ProductionID: r.productionID, DynamicPrecedence: r.dynamicPrecedence,
		StartByte: r.startByte, EndByte: r.endByte, Extra: r.extra, Terminal: r.terminal,
	}
	view.Children = append(view.Children, c.children[r.firstChild:r.firstChild+r.childCount]...)
	view.Fields = append(view.Fields, c.fields[r.firstField:r.firstField+r.fieldCount]...)
	view.Aliases = append(view.Aliases, c.aliases[r.firstAlias:r.firstAlias+r.aliasCount]...)
	return view, nil
}

func (c *Core) Stats(head Head) (Stats, error) {
	n, err := c.node(head.Node)
	if err != nil {
		return Stats{}, err
	}
	return Stats{
		Nodes: uint32(len(c.nodes)), Links: uint32(len(c.links)), Subtrees: uint32(len(c.subtrees)),
		CurrentExactPaths: n.pathCount,
	}, nil
}

func (c *Core) appendNode(r nodeRecord) (NodeID, error) {
	if uint64(len(c.nodes))+1 > uint64(c.limits.MaxNodes) || len(c.nodes) >= math.MaxUint32 {
		return 0, errors.New("parser-core phase zero: node arena cap")
	}
	c.nodes = append(c.nodes, r)
	return NodeID(len(c.nodes)), nil
}

func (c *Core) appendSubtree(r subtreeRecord, children []SubtreeID, fields []gts.FieldMapEntry, aliases []gts.Symbol) (SubtreeID, error) {
	if uint64(len(c.subtrees))+1 > uint64(c.limits.MaxSubtrees) || len(c.subtrees) >= math.MaxUint32 {
		return 0, errors.New("parser-core phase zero: subtree arena cap")
	}
	if uint64(len(c.children))+uint64(len(children)) > uint64(c.limits.MaxChildren) {
		return 0, errors.New("parser-core phase zero: child arena cap")
	}
	if uint64(len(c.fields))+uint64(len(fields)) > uint64(c.limits.MaxMetadata) || uint64(len(c.aliases))+uint64(len(aliases)) > uint64(c.limits.MaxMetadata) {
		return 0, errors.New("parser-core phase zero: metadata arena cap")
	}
	r.firstChild, r.childCount = uint32(len(c.children)), uint32(len(children))
	r.firstField, r.fieldCount = uint32(len(c.fields)), uint32(len(fields))
	r.firstAlias, r.aliasCount = uint32(len(c.aliases)), uint32(len(aliases))
	c.children = append(c.children, children...)
	c.fields = append(c.fields, fields...)
	c.aliases = append(c.aliases, aliases...)
	c.subtrees = append(c.subtrees, r)
	return SubtreeID(len(c.subtrees)), nil
}

func (c *Core) node(id NodeID) (*nodeRecord, error) {
	if id == 0 || uint64(id) > uint64(len(c.nodes)) {
		return nil, fmt.Errorf("parser-core phase zero: invalid node id %d", id)
	}
	return &c.nodes[id-1], nil
}

func (c *Core) subtree(id SubtreeID) (*subtreeRecord, error) {
	if id == 0 || uint64(id) > uint64(len(c.subtrees)) {
		return nil, fmt.Errorf("parser-core phase zero: invalid subtree id %d", id)
	}
	return &c.subtrees[id-1], nil
}

func (c *Core) nodeLinks(n nodeRecord) ([]linkRecord, error) {
	links := make([]linkRecord, 0, n.linkCount)
	id := LinkID(n.firstLink)
	seen := make(map[LinkID]bool, n.linkCount)
	for id != 0 {
		if seen[id] {
			return nil, errors.New("parser-core phase zero: adjacency cycle")
		}
		seen[id] = true
		if uint64(id) > uint64(len(c.links)) {
			return nil, errors.New("parser-core phase zero: link adjacency out of range")
		}
		link := c.links[id-1]
		links = append(links, link)
		if uint64(len(links)) > uint64(n.linkCount) {
			return nil, errors.New("parser-core phase zero: adjacency exceeds recorded link count")
		}
		id = link.next
	}
	if uint32(len(links)) != n.linkCount {
		return nil, errors.New("parser-core phase zero: adjacency shorter than recorded link count")
	}
	// The chain prepends in O(1); callers observe stable insertion order.
	for i, j := 0, len(links)-1; i < j; i, j = i+1, j-1 {
		links[i], links[j] = links[j], links[i]
	}
	return links, nil
}

func checkedAddScore(a, b int64) (int64, error) {
	if (b > 0 && a > math.MaxInt64-b) || (b < 0 && a < math.MinInt64-b) {
		return 0, errors.New("parser-core phase zero: score overflow")
	}
	return a + b, nil
}

func lookupActionIndex(lang *gts.Language, state gts.StateID, sym gts.Symbol) (uint16, error) {
	if lang == nil {
		return 0, errors.New("parser-core phase zero: nil language")
	}
	denseLimit := int(lang.LargeStateCount)
	if denseLimit == 0 {
		denseLimit = len(lang.ParseTable)
	}
	if int(state) < denseLimit {
		if int(state) >= len(lang.ParseTable) || int(sym) >= len(lang.ParseTable[state]) {
			return 0, nil
		}
		return lang.ParseTable[state][sym], nil
	}
	// Match Parser's split exactly: denseLimit controls dispatch, while
	// smallBase is the generated LargeStateCount (which can differ for
	// hand-built tables).
	smallIdx := int(state) - int(lang.LargeStateCount)
	if smallIdx < 0 || smallIdx >= len(lang.SmallParseTableMap) {
		return 0, nil
	}
	offset := uint64(lang.SmallParseTableMap[smallIdx])
	if offset >= uint64(len(lang.SmallParseTable)) {
		return 0, errors.New("parser-core phase zero: sparse table offset out of range")
	}
	table := lang.SmallParseTable
	groups := table[offset]
	pos := offset + 1
	for i := uint16(0); i < groups; i++ {
		if pos+1 >= uint64(len(table)) {
			return 0, errors.New("parser-core phase zero: truncated sparse table group")
		}
		value, count := table[pos], table[pos+1]
		pos += 2
		for j := uint16(0); j < count; j++ {
			if pos >= uint64(len(table)) {
				return 0, errors.New("parser-core phase zero: truncated sparse table symbols")
			}
			if table[pos] == uint16(sym) {
				return value, nil
			}
			pos++
		}
	}
	return 0, nil
}

func lookupGoto(lang *gts.Language, state gts.StateID, sym gts.Symbol) (gts.StateID, error) {
	if lang.TokenCount > 0 && uint32(sym) >= lang.TokenCount {
		if target := lang.LargeStateGotos[uint64(state)<<32|uint64(sym)]; target != 0 {
			return target, nil
		}
	}
	raw, err := lookupActionIndex(lang, state, sym)
	if err != nil || raw == 0 {
		return 0, err
	}
	if lang.TokenCount > 0 && uint32(sym) >= lang.TokenCount && lang.StateCount > 0 && lang.InitialState > 0 {
		return gts.StateID(raw), nil
	}
	if int(raw) >= len(lang.ParseActions) || len(lang.ParseActions[raw].Actions) == 0 {
		return 0, errors.New("parser-core phase zero: goto action out of range")
	}
	act := lang.ParseActions[raw].Actions[0]
	if act.Type != gts.ParseActionShift {
		return 0, errors.New("parser-core phase zero: hand-built goto is not shift")
	}
	return act.State, nil
}

func productionFields(lang *gts.Language, productionID uint16) ([]gts.FieldMapEntry, error) {
	if lang == nil {
		return nil, errors.New("parser-core phase zero: nil language metadata")
	}
	pid := int(productionID)
	if pid >= len(lang.FieldMapSlices) {
		return nil, nil
	}
	span := lang.FieldMapSlices[pid]
	start, end := int(span[0]), int(span[0])+int(span[1])
	if start < 0 || end > len(lang.FieldMapEntries) || start > end {
		return nil, errors.New("parser-core phase zero: field metadata span out of range")
	}
	return lang.FieldMapEntries[start:end], nil
}

func productionAliases(lang *gts.Language, productionID uint16, childCount int) ([]gts.Symbol, error) {
	if lang == nil {
		return nil, errors.New("parser-core phase zero: nil language metadata")
	}
	pid := int(productionID)
	if pid >= len(lang.AliasSequences) || childCount <= 0 {
		return nil, nil
	}
	seq := lang.AliasSequences[pid]
	if len(seq) == 0 {
		return nil, nil
	}
	// Generated alias rows omit an all-zero tail. A missing entry therefore
	// means "no alias" for that child; it is not malformed metadata. Retain a
	// child-aligned row only when at least one relevant child is aliased.
	aliases := make([]gts.Symbol, childCount)
	copy(aliases, seq)
	for _, alias := range aliases {
		if alias != 0 {
			return aliases, nil
		}
	}
	return nil, nil
}
