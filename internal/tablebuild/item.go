package tablebuild

import (
	"encoding/binary"
	"sort"
	"strconv"
	"strings"

	"github.com/odvcencio/gotreesitter/internal/lexbuild"
)

// This file ports build_tables/item.rs: item keys, parse items, lookahead
// set interning, and item sets. Display code is not ported.

// noReservedWords is grammars.rs: const ProductionStep::NO_RESERVED_WORDS,
// the reserved set of the augmented start production's one step.
const noReservedWords = ^uint16(0)

// startSteps is item.rs: const START_STEPS.
var startSteps = []lexbuild.ProductionStep{{
	Symbol:   lexbuild.NonTerminalSym(0),
	Reserved: noReservedWords,
}}

// startProductionID is item.rs: const START_PRODUCTION_ID.
const startProductionID = ^uint32(0)

// prodRef is grammars.rs: struct ProdRef, a production of the grammar.
type prodRef struct {
	steps             []lexbuild.ProductionStep
	dynamicPrecedence int32
	requiresEOF       bool
}

// startProduction is item.rs: fn start_production.
func startProduction() prodRef { return prodRef{steps: startSteps} }

// production is grammars.rs: fn SyntaxGrammar::production.
func production(sg *lexbuild.SyntaxGrammar, id uint32) prodRef {
	p := &sg.Productions[id]
	return prodRef{steps: p.Steps, dynamicPrecedence: p.DynamicPrecedence, requiresEOF: p.RequiresEOF}
}

// firstSymbol is grammars.rs: fn ProdRef::first_symbol.
func (p prodRef) firstSymbol() (Symbol, bool) {
	if len(p.steps) == 0 {
		return Symbol{}, false
	}
	return p.steps[0].Symbol, true
}

// dotKeys is item.rs: struct DotKeys, the identity keys of one (production,
// dot) pair. cmp is the rank of the pair's content in the order of
// itemContent; eqWithSyms divides cmp by the symbols before the dot.
type dotKeys struct {
	cmp        uint32
	eqWithSyms uint32
}

// ItemKeyMap is item.rs: struct ItemKeyMap: the keys of every (production,
// dot) pair of a grammar, the productions built by inlining and the
// augmented start production included.
type ItemKeyMap struct {
	keys  [][]dotKeys
	start []dotKeys
}

// NewItemKeyMap is item.rs: fn ItemKeyMap::new.
//
// The generator sorts the pairs with sort_unstable_by, so pairs of equal
// content (which get the same cmp rank) may come in any order. That order
// reaches only the numbering of eqWithSyms within one cmp class, and that
// numbering decides nothing: eqWithSyms is compared for equality, and for
// order only between two items of one item set with equal step index,
// variable, cmp rank and inherited-fields flag. All items of one item set
// with the same step index have the same symbols before the dot (they are
// the last symbols of the one path that created the set: an item set holds
// the successors of the items of one state over one symbol, and items at
// step 0 have no symbols before the dot), so two such items have equal
// eqWithSyms and are the same item. The port sorts stably.
func NewItemKeyMap(sg *lexbuild.SyntaxGrammar) *ItemKeyMap {
	prod := func(slot int) prodRef {
		if slot == 0 {
			return startProduction()
		}
		return production(sg, uint32(slot-1))
	}
	slotCount := len(sg.Productions) + 1

	type pair struct{ pi, dot uint32 }
	var contents []pair
	for pi := 0; pi < slotCount; pi++ {
		for dot := 0; dot <= len(prod(pi).steps); dot++ {
			contents = append(contents, pair{uint32(pi), uint32(dot)})
		}
	}
	content := func(p pair) itemContent { return itemContent{production: prod(int(p.pi)), dot: int(p.dot)} }
	sort.SliceStable(contents, func(a, b int) bool { return content(contents[a]).cmp(content(contents[b])) < 0 })

	slotKeys := make([][]dotKeys, slotCount)
	for pi := range slotKeys {
		slotKeys[pi] = make([]dotKeys, len(prod(pi).steps)+1)
	}
	// Dense ids in sorted order: equal content shares an id, distinct content
	// gets the next one up.
	var cmpID uint32
	for k, p := range contents {
		if k > 0 && content(contents[k-1]).cmp(content(p)) != 0 {
			cmpID++
		}
		slotKeys[p.pi][p.dot].cmp = cmpID
	}

	// Refine each cmp class by the symbols before the dot: read only under
	// has_preceding_inherited_fields.
	symClasses := map[string]uint32{}
	var key []byte
	for _, p := range contents {
		keys := &slotKeys[p.pi][p.dot]
		key = binary.LittleEndian.AppendUint32(key[:0], keys.cmp)
		for _, s := range prod(int(p.pi)).steps[:p.dot] {
			key = append(key, byte(s.Symbol.Kind))
			key = binary.LittleEndian.AppendUint32(key, s.Symbol.Index)
		}
		next := uint32(len(symClasses))
		if id, ok := symClasses[string(key)]; ok {
			keys.eqWithSyms = id
		} else {
			symClasses[string(key)] = next
			keys.eqWithSyms = next
		}
	}

	return &ItemKeyMap{keys: slotKeys[1:], start: slotKeys[0]}
}

// keysFor is item.rs: fn ItemKeyMap::keys_for.
func (m *ItemKeyMap) keysFor(id uint32) []dotKeys {
	if id == startProductionID {
		return m.start
	}
	return m.keys[id]
}

// startKeys is item.rs: fn ItemKeyMap::start_keys.
func (m *ItemKeyMap) startKeys() []dotKeys { return m.start }

// itemContent is item.rs: struct ItemContent: the content that the order of
// ParseItem observes, read from a (production, dot) pair. Names (precedence
// names, alias values, field names) compare as strings, as the generator
// compares the strings its pool resolves.
type itemContent struct {
	production prodRef
	dot        int
}

// prec is item.rs: fn ItemContent::prec.
func (c itemContent) prec() lexbuild.Precedence {
	if c.dot > 0 {
		return c.production.steps[c.dot-1].Precedence
	}
	return lexbuild.Precedence{}
}

// assoc is item.rs: fn ItemContent::assoc. AssocNone stands for None; the
// numeric order None < Left < Right is the order of Option<Associativity>.
func (c itemContent) assoc() lexbuild.Associativity {
	if c.dot > 0 {
		return c.production.steps[c.dot-1].Associativity
	}
	return lexbuild.AssocNone
}

func cmpInt[T ~int | ~int32 | ~uint8 | ~uint16 | ~uint32](a, b T) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	}
	return 0
}

func cmpBool(a, b bool) int {
	switch {
	case a == b:
		return 0
	case !a:
		return -1
	}
	return 1
}

// symbolCmp is the derived Ord of rules.rs: struct Symbol.
func symbolCmp(a, b Symbol) int {
	if c := cmpInt(a.Kind, b.Kind); c != 0 {
		return c
	}
	return cmpInt(a.Index, b.Index)
}

// precCmp is item.rs: fn ItemContent::prec_cmp.
func precCmp(a, b lexbuild.Precedence) int {
	switch {
	case a.Kind == lexbuild.PrecNone && b.Kind == lexbuild.PrecNone:
		return 0
	case a.Kind == lexbuild.PrecInteger && b.Kind == lexbuild.PrecInteger:
		return cmpInt(a.Int, b.Int)
	case a.Kind == lexbuild.PrecName && b.Kind == lexbuild.PrecName:
		return strings.Compare(a.Ident, b.Ident)
	case a.Kind == lexbuild.PrecNone, a.Kind == lexbuild.PrecInteger && b.Kind == lexbuild.PrecName:
		return -1
	}
	return 1
}

// aliasCmp is item.rs: fn ItemContent::alias_cmp: Option<(name, is_named)>.
func aliasCmp(a, b lexbuild.ProductionStep) int {
	if c := cmpBool(a.Alias.Set, b.Alias.Set); c != 0 || !a.Alias.Set {
		return c
	}
	if c := strings.Compare(a.Alias.Value, b.Alias.Value); c != 0 {
		return c
	}
	return cmpBool(a.Alias.Named, b.Alias.Named)
}

// fieldCmp is item.rs: fn ItemContent::field_cmp: Option<name>.
func fieldCmp(a, b lexbuild.ProductionStep) int {
	if c := cmpBool(a.HasField, b.HasField); c != 0 || !a.HasField {
		return c
	}
	return strings.Compare(a.Field, b.Field)
}

// stepCmp is item.rs: fn ItemContent::step_cmp.
func stepCmp(a, b lexbuild.ProductionStep) int {
	if c := symbolCmp(a.Symbol, b.Symbol); c != 0 {
		return c
	}
	if c := precCmp(a.Precedence, b.Precedence); c != 0 {
		return c
	}
	if c := cmpInt(a.Associativity, b.Associativity); c != 0 {
		return c
	}
	if c := aliasCmp(a, b); c != 0 {
		return c
	}
	if c := fieldCmp(a, b); c != 0 {
		return c
	}
	return cmpInt(a.Reserved, b.Reserved)
}

// cmp is item.rs: impl Ord for ItemContent.
func (c itemContent) cmp(o itemContent) int {
	if r := cmpInt(c.production.dynamicPrecedence, o.production.dynamicPrecedence); r != 0 {
		return r
	}
	if r := cmpBool(c.production.requiresEOF, o.production.requiresEOF); r != 0 {
		return r
	}
	if r := cmpInt(len(c.production.steps), len(o.production.steps)); r != 0 {
		return r
	}
	if r := precCmp(c.prec(), o.prec()); r != 0 {
		return r
	}
	if r := cmpInt(c.assoc(), o.assoc()); r != 0 {
		return r
	}
	if r := cmpInt(c.dot, o.dot); r != 0 {
		return r
	}
	n := min(len(c.production.steps), len(o.production.steps))
	for i := 0; i < n; i++ {
		sa, sb := c.production.steps[i], o.production.steps[i]
		var r int
		if i < c.dot {
			if r = aliasCmp(sa, sb); r == 0 {
				r = fieldCmp(sa, sb)
			}
		} else {
			r = stepCmp(sa, sb)
		}
		if r != 0 {
			return r
		}
	}
	return 0
}

// ParseItem is item.rs: struct ParseItem: an in-progress match of one
// production. Two items are equal (fn eq) and ordered (fn compare) by their
// variable, step index, inherited-fields flag and keys, not by ProdID: of two
// equal items an item set keeps the first one inserted.
type ParseItem struct {
	// VariableIndex is the index of the parent rule (startProductionID for
	// the augmented start item).
	VariableIndex uint32
	// StepIndex is the number of symbols already matched.
	StepIndex uint32
	// ProdID is the id of the production being matched.
	ProdID uint32
	// keys are the production's identity keys, indexed by StepIndex.
	keys []dotKeys
	// HasPrecedingInheritedFields tells whether an already-matched child was
	// a hidden node with fields: then the symbols before the dot take part in
	// the item's identity (its inherited fields may depend on them).
	HasPrecedingInheritedFields bool
}

// lookaheadSetID is item.rs: struct LookaheadSetId, an index into a
// lookaheadSetPool. Ids are canonical: equal ids hold equal sets.
type lookaheadSetID uint32

// emptyLookaheads is item.rs: const LookaheadSetPool::EMPTY.
const emptyLookaheads lookaheadSetID = 0

type insertMemoKey struct {
	id  lookaheadSetID
	sym Symbol
}

// lookaheadSetPool is item.rs: struct LookaheadSetPool, the interner of
// lookahead token sets. Ids are handed out in the order sets are first
// interned. The generator compares ids only for equality and uses them as
// hash keys, so the order in which sets are interned reaches no result.
type lookaheadSetPool struct {
	sets       []lexbuild.TokenSet
	index      map[string]lookaheadSetID
	unionMemo  map[[2]lookaheadSetID]lookaheadSetID
	insertMemo map[insertMemoKey]lookaheadSetID
}

// newLookaheadSetPool is item.rs: fn LookaheadSetPool::new.
func newLookaheadSetPool() *lookaheadSetPool {
	p := &lookaheadSetPool{
		index:      map[string]lookaheadSetID{},
		unionMemo:  map[[2]lookaheadSetID]lookaheadSetID{},
		insertMemo: map[insertMemoKey]lookaheadSetID{},
	}
	p.intern(lexbuild.TokenSet{})
	return p
}

// get is item.rs: fn LookaheadSetPool::get.
func (p *lookaheadSetPool) get(id lookaheadSetID) *lexbuild.TokenSet { return &p.sets[id] }

// intern is item.rs: fn LookaheadSetPool::intern. The pool keeps the set.
func (p *lookaheadSetPool) intern(set lexbuild.TokenSet) lookaheadSetID {
	k := set.Key()
	if id, ok := p.index[k]; ok {
		return id
	}
	bLookaheadSets.check(len(p.sets) + 1)
	id := lookaheadSetID(len(p.sets))
	p.sets = append(p.sets, set)
	p.index[k] = id
	return id
}

// internRef is item.rs: fn LookaheadSetPool::intern_ref.
func (p *lookaheadSetPool) internRef(set *lexbuild.TokenSet) lookaheadSetID {
	if id, ok := p.index[set.Key()]; ok {
		return id
	}
	return p.intern(set.Clone())
}

// singleton is item.rs: fn LookaheadSetPool::singleton.
func (p *lookaheadSetPool) singleton(sym Symbol) lookaheadSetID {
	return p.insert(emptyLookaheads, sym)
}

// insert is item.rs: fn LookaheadSetPool::insert.
func (p *lookaheadSetPool) insert(id lookaheadSetID, sym Symbol) lookaheadSetID {
	if p.get(id).Contains(sym) {
		return id
	}
	if r, ok := p.insertMemo[insertMemoKey{id, sym}]; ok {
		return r
	}
	set := p.get(id).Clone()
	set.Insert(sym)
	r := p.intern(set)
	p.insertMemo[insertMemoKey{id, sym}] = r
	return r
}

// union is item.rs: fn LookaheadSetPool::union.
func (p *lookaheadSetPool) union(left, right lookaheadSetID) lookaheadSetID {
	if left == right || right == emptyLookaheads {
		return left
	}
	if left == emptyLookaheads {
		return right
	}
	key := [2]lookaheadSetID{min(left, right), max(left, right)}
	if r, ok := p.unionMemo[key]; ok {
		return r
	}
	set := p.get(key[0]).Clone()
	set.InsertAll(p.get(key[1]))
	r := p.intern(set)
	p.unionMemo[key] = r
	return r
}

// ParseItemSet is item.rs: struct ParseItemSet: items with their lookaheads,
// sorted by ParseItem order, one entry per distinct item.
type ParseItemSet struct {
	Entries []ParseItemSetEntry
}

// ParseItemSetEntry is item.rs: struct ParseItemSetEntry.
type ParseItemSetEntry struct {
	Item       ParseItem
	lookaheads lookaheadSetID
	// FollowingReservedWordSet is the reserved word set id that may follow.
	FollowingReservedWordSet uint32
}

// ParseItemSetCore is item.rs: struct ParseItemSetCore: an item set without
// lookaheads.
type ParseItemSetCore struct {
	Entries []ParseItem
}

// startItem is item.rs: fn ParseItem::start.
func startItem(m *ItemKeyMap) ParseItem {
	return ParseItem{VariableIndex: startProductionID, ProdID: startProductionID, keys: m.startKeys()}
}

// production is item.rs: fn ParseItem::production.
func (it *ParseItem) production(sg *lexbuild.SyntaxGrammar) prodRef {
	if it.ProdID == startProductionID {
		return startProduction()
	}
	return production(sg, it.ProdID)
}

// step is item.rs: fn ParseItem::step.
func (it *ParseItem) step(sg *lexbuild.SyntaxGrammar) (lexbuild.ProductionStep, bool) {
	steps := it.production(sg).steps
	if int(it.StepIndex) < len(steps) {
		return steps[it.StepIndex], true
	}
	return lexbuild.ProductionStep{}, false
}

// symbol is item.rs: fn ParseItem::symbol.
func (it *ParseItem) symbol(sg *lexbuild.SyntaxGrammar) (Symbol, bool) {
	s, ok := it.step(sg)
	return s.Symbol, ok
}

// associativity is item.rs: fn ParseItem::associativity (AssocNone for None).
func (it *ParseItem) associativity(sg *lexbuild.SyntaxGrammar) lexbuild.Associativity {
	if s, ok := it.prevStep(sg); ok {
		return s.Associativity
	}
	return lexbuild.AssocNone
}

// precedence is item.rs: fn ParseItem::precedence.
func (it *ParseItem) precedence(sg *lexbuild.SyntaxGrammar) lexbuild.Precedence {
	if s, ok := it.prevStep(sg); ok {
		return s.Precedence
	}
	return lexbuild.Precedence{}
}

// prevStep is item.rs: fn ParseItem::prev_step.
func (it *ParseItem) prevStep(sg *lexbuild.SyntaxGrammar) (lexbuild.ProductionStep, bool) {
	if it.StepIndex > 0 {
		return it.production(sg).steps[it.StepIndex-1], true
	}
	return lexbuild.ProductionStep{}, false
}

// isDone is item.rs: fn ParseItem::is_done.
func (it *ParseItem) isDone() bool { return int(it.StepIndex)+1 == len(it.keys) }

// isAugmented is item.rs: fn ParseItem::is_augmented.
func (it *ParseItem) isAugmented() bool { return it.VariableIndex == startProductionID }

// successor is item.rs: fn ParseItem::successor.
func (it ParseItem) successor() ParseItem {
	it.StepIndex++
	return it
}

// substituteProduction is item.rs: fn ParseItem::substitute_production.
func (it ParseItem) substituteProduction(prodID uint32, keys []dotKeys) ParseItem {
	it.ProdID = prodID
	it.keys = keys
	return it
}

// dotKeys is item.rs: fn ParseItem::dot_keys.
func (it *ParseItem) dotKeys() dotKeys { return it.keys[it.StepIndex] }

// insert is item.rs: fn ParseItemSet::insert. It returns the entry of the
// item, new (with empty lookaheads) or existing. The pointer is valid until
// the next insertion.
func (s *ParseItemSet) insert(item ParseItem) *ParseItemSetEntry {
	i := sort.Search(len(s.Entries), func(k int) bool { return s.Entries[k].Item.compare(&item) >= 0 })
	if i < len(s.Entries) && s.Entries[i].Item.compare(&item) == 0 {
		return &s.Entries[i]
	}
	s.Entries = append(s.Entries, ParseItemSetEntry{})
	copy(s.Entries[i+1:], s.Entries[i:])
	s.Entries[i] = ParseItemSetEntry{Item: item}
	return &s.Entries[i]
}

// core is item.rs: fn ParseItemSet::core.
func (s *ParseItemSet) core() ParseItemSetCore {
	c := ParseItemSetCore{Entries: make([]ParseItem, len(s.Entries))}
	for i := range s.Entries {
		c.Entries[i] = s.Entries[i].Item
	}
	return c
}

// precDisplay is item.rs: fn prec_display.
func precDisplay(p lexbuild.Precedence) string {
	switch p.Kind {
	case lexbuild.PrecInteger:
		return strconv.Itoa(int(p.Int))
	case lexbuild.PrecName:
		return "'" + p.Ident + "'"
	}
	return "none"
}

// appendItemKey writes the fields that item.rs: impl Hash for ParseItem
// hashes. Two items have the same key exactly when fn eq holds.
func appendItemKey(b []byte, it *ParseItem) []byte {
	b = binary.LittleEndian.AppendUint32(b, it.VariableIndex)
	b = binary.LittleEndian.AppendUint32(b, it.StepIndex)
	k := it.dotKeys()
	if it.HasPrecedingInheritedFields {
		b = append(b, 1)
		return binary.LittleEndian.AppendUint32(b, k.eqWithSyms)
	}
	b = append(b, 0)
	return binary.LittleEndian.AppendUint32(b, k.cmp)
}

// eq is item.rs: impl PartialEq for ParseItem.
func (it *ParseItem) eq(o *ParseItem) bool {
	if it.VariableIndex != o.VariableIndex || it.StepIndex != o.StepIndex || it.HasPrecedingInheritedFields != o.HasPrecedingInheritedFields {
		return false
	}
	if it.HasPrecedingInheritedFields {
		return it.dotKeys().eqWithSyms == o.dotKeys().eqWithSyms
	}
	return it.dotKeys().cmp == o.dotKeys().cmp
}

// compare is item.rs: impl Ord for ParseItem.
func (it *ParseItem) compare(o *ParseItem) int {
	if c := cmpInt(it.StepIndex, o.StepIndex); c != 0 {
		return c
	}
	if c := cmpInt(it.VariableIndex, o.VariableIndex); c != 0 {
		return c
	}
	if c := cmpInt(it.dotKeys().cmp, o.dotKeys().cmp); c != 0 {
		return c
	}
	if c := cmpBool(it.HasPrecedingInheritedFields, o.HasPrecedingInheritedFields); c != 0 {
		return c
	}
	if it.HasPrecedingInheritedFields {
		return cmpInt(it.dotKeys().eqWithSyms, o.dotKeys().eqWithSyms)
	}
	return 0
}

// key is item.rs: impl Hash for ParseItemSet, with the derived equality: two
// item sets have the same key exactly when they are equal.
func (s *ParseItemSet) key() string {
	b := make([]byte, 0, len(s.Entries)*21)
	for i := range s.Entries {
		e := &s.Entries[i]
		b = appendItemKey(b, &e.Item)
		b = binary.LittleEndian.AppendUint32(b, uint32(e.lookaheads))
		b = binary.LittleEndian.AppendUint32(b, e.FollowingReservedWordSet)
	}
	return string(b)
}

// key is item.rs: impl Hash for ParseItemSetCore, with the derived equality.
func (c *ParseItemSetCore) key() string {
	b := make([]byte, 0, len(c.Entries)*13)
	for i := range c.Entries {
		b = appendItemKey(b, &c.Entries[i])
	}
	return string(b)
}
