package tablebuild

import (
	"fmt"

	"github.com/odvcencio/gotreesitter/internal/lexbuild"
)

// inlineKey is the key (production id, step index) of the inline map.
type inlineKey struct{ prod, step uint32 }

// InlinedProductionMap is grammars.rs: struct InlinedProductionMap. For a
// production and the index of a step that holds an inlined symbol, it gives
// the productions that replace the production there. The generator keeps a
// hash map and only looks keys up in it, so no order of the map reaches a
// result.
type InlinedProductionMap struct {
	m map[inlineKey][]uint32
}

// InlinedProdIDs is grammars.rs: fn InlinedProductionMap::inlined_prod_ids.
func (m *InlinedProductionMap) InlinedProdIDs(prodID, stepIndex uint32) ([]uint32, bool) {
	ids, ok := m.m[inlineKey{prodID, stepIndex}]
	return ids, ok
}

// Len is the number of (production, step) entries.
func (m *InlinedProductionMap) Len() int { return len(m.m) }

// inlineBuilder is process_inlines.rs: struct InlineBuilder.
type inlineBuilder struct {
	out          *lexbuild.SyntaxGrammar
	firstInlined uint32
	inline       []Symbol
	m            map[inlineKey][]uint32
	// memo is the memo of the generator: a hash map from the hash of a built
	// production to the ids of the productions built by this pass with that
	// hash, searched for the first equal one. Only one production with given
	// contents is ever built, so a map from the contents to that id gives the
	// same answer.
	memo map[string]uint32

	built int // production steps built (bInlineSteps)
	work  int // steps of the work list (bInlineWork)
}

// scratchProd is process_inlines.rs: struct ScratchProd.
type scratchProd struct {
	steps             []lexbuild.ProductionStep
	dynamicPrecedence int32
	requiresEOF       bool
}

// build is process_inlines.rs: fn InlineBuilder::build. It returns the inline
// map and the ids of the productions whose every expansion was dropped.
func (b *inlineBuilder) build() (InlinedProductionMap, map[uint32]bool) {
	dead := map[uint32]bool{}
	type item struct{ prod, step uint32 }
	var worklist []item
	for prodID := uint32(0); prodID < b.firstInlined; prodID++ {
		survived := false
		worklist = append(worklist[:0], item{prodID, 0})
		for len(worklist) > 0 {
			i := 0
			for i < len(worklist) {
				b.work++
				bInlineWork.check(b.work)
				it := worklist[i]
				if step, ok := b.productionStepForID(it.prod, it.step); ok {
					if containsSymbol(b.inline, step.Symbol) {
						ids := b.inlineProductionAtStep(it.prod, it.step)
						// worklist.splice(i..=i, ids): the item is replaced, and the
						// first replacement is examined next.
						repl := make([]item, len(ids))
						for k, id := range ids {
							repl[k] = item{id, it.step}
						}
						rest := append(repl, worklist[i+1:]...)
						worklist = append(worklist[:i], rest...)
					} else {
						worklist[i].step++
						i++
					}
				} else {
					worklist = append(worklist[:i], worklist[i+1:]...)
					survived = true
				}
			}
		}
		if !survived {
			dead[prodID] = true
		}
	}
	return InlinedProductionMap{b.m}, dead
}

// inlineProductionAtStep is process_inlines.rs: fn
// InlineBuilder::inline_production_at_step. It expands the inlined symbols at
// one step of one production until the step holds no inlined symbol, stores
// each distinct result once, and records the ids in the map.
func (b *inlineBuilder) inlineProductionAtStep(prodID, stepIndex uint32) []uint32 {
	if ids, ok := b.m[inlineKey{prodID, stepIndex}]; ok {
		return ids
	}

	si := int(stepIndex)
	src := &b.out.Productions[prodID]
	scratch := []scratchProd{{
		steps:             append([]lexbuild.ProductionStep(nil), src.Steps...),
		dynamicPrecedence: src.DynamicPrecedence,
		requiresEOF:       src.RequiresEOF,
	}}
	held := len(src.Steps) // steps held in scratch (bInlineScratch)
	bInlineScratch.check(held)
	i := 0
	for i < len(scratch) {
		if si >= len(scratch[i].steps) || !containsSymbol(b.inline, scratch[i].steps[si].Symbol) {
			i++
			continue
		}
		symbol := scratch[i].steps[si].Symbol
		removedProd := scratch[i]
		removedStep := removedProd.steps[si]
		held -= len(removedProd.steps)
		vp := b.out.VarProds[symbol.Index]
		var replacements []scratchProd
		for pIdx := vp[0]; pIdx < vp[1]; pIdx++ {
			p := &b.out.Productions[pIdx]
			if p.RequiresEOF && si+1 < len(removedProd.steps) {
				continue
			}
			n := len(removedProd.steps) - 1 + len(p.Steps)
			held += n
			bInlineScratch.check(held)
			steps := make([]lexbuild.ProductionStep, 0, n)
			steps = append(steps, removedProd.steps[:si]...)
			steps = append(steps, p.Steps...)
			steps = append(steps, removedProd.steps[si+1:]...)
			inserted := steps[si : si+len(p.Steps)]
			if removedStep.Alias.Set {
				for k := range inserted {
					inserted[k].Alias = removedStep.Alias
				}
			}
			if removedStep.HasField {
				for k := range inserted {
					inserted[k].Field, inserted[k].HasField = removedStep.Field, true
				}
			}
			if len(inserted) > 0 {
				last := &inserted[len(inserted)-1]
				if last.Precedence.Kind == lexbuild.PrecNone {
					last.Precedence = removedStep.Precedence
				}
				if last.Associativity == lexbuild.AssocNone {
					last.Associativity = removedStep.Associativity
				}
			}
			production := scratchProd{steps: steps, dynamicPrecedence: removedProd.dynamicPrecedence, requiresEOF: removedProd.requiresEOF}
			if abs32(p.DynamicPrecedence) > abs32(production.dynamicPrecedence) {
				production.dynamicPrecedence = p.DynamicPrecedence
			}
			production.requiresEOF = production.requiresEOF || p.RequiresEOF
			replacements = append(replacements, production)
		}
		// scratch.splice(i..=i, replacements): the first replacement is examined next.
		rest := append(replacements, scratch[i+1:]...)
		scratch = append(scratch[:i], rest...)
	}

	result := make([]uint32, 0, len(scratch))
	for _, sp := range scratch {
		key := productionKey(&sp)
		if id, ok := b.memo[key]; ok {
			result = append(result, id)
			continue
		}
		b.built += len(sp.steps)
		bInlineSteps.check(b.built)
		b.out.Productions = append(b.out.Productions, lexbuild.Production{
			Steps:             sp.steps,
			DynamicPrecedence: sp.dynamicPrecedence,
			RequiresEOF:       sp.requiresEOF,
		})
		id := uint32(len(b.out.Productions) - 1)
		b.memo[key] = id
		result = append(result, id)
	}

	b.m[inlineKey{prodID, stepIndex}] = result
	return result
}

// productionStepForID is process_inlines.rs: fn InlineBuilder::production_step_for_id.
func (b *inlineBuilder) productionStepForID(prodID, step uint32) (lexbuild.ProductionStep, bool) {
	p := &b.out.Productions[prodID]
	if int(step) < len(p.Steps) {
		return p.Steps[step], true
	}
	return lexbuild.ProductionStep{}, false
}

// ProcessInlinesError is process_inlines.rs: enum ProcessInlinesError.
type ProcessInlinesError struct {
	Kind ProcessInlinesErrorKind
	Name string
}

// ProcessInlinesErrorKind names the variant of ProcessInlinesError.
type ProcessInlinesErrorKind uint8

const (
	InlineExternalToken ProcessInlinesErrorKind = iota
	InlineToken
	InlineFirstRule
	InlineNoReachableProductions
)

func (e *ProcessInlinesError) Error() string {
	switch e.Kind {
	case InlineExternalToken:
		return fmt.Sprintf("External token `%s` cannot be inlined", e.Name)
	case InlineToken:
		return fmt.Sprintf("Token `%s` cannot be inlined", e.Name)
	case InlineFirstRule:
		return fmt.Sprintf("Rule `%s` cannot be inlined because it is the first rule", e.Name)
	}
	return fmt.Sprintf("Rule `%s` has no reachable productions after inlining", e.Name)
}

// processInlines is process_inlines.rs: fn process_inlines. It appends the
// productions that inlining builds to sg.Productions (each variable's range in
// sg.VarProds is unchanged) and returns the map from (production, step) to the
// ids of the replacing productions. lex gives the names of the tokens for the
// errors.
func processInlines(sg *lexbuild.SyntaxGrammar, lex *lexbuild.LexicalGrammar) (InlinedProductionMap, error) {
	if len(sg.VariablesToInline) == 0 {
		return InlinedProductionMap{}, nil
	}
	for _, s := range sg.VariablesToInline {
		switch {
		case s.Kind == lexbuild.External:
			return InlinedProductionMap{}, &ProcessInlinesError{InlineExternalToken, sg.ExternalTokens[s.Index].Ident}
		case s.Kind == lexbuild.Terminal:
			return InlinedProductionMap{}, &ProcessInlinesError{InlineToken, lex.Variables[s.Index].Ident}
		case s.Kind == lexbuild.NonTerminal && s.Index == 0:
			return InlinedProductionMap{}, &ProcessInlinesError{InlineFirstRule, sg.Variables[0].Ident}
		}
	}

	b := &inlineBuilder{
		out:          sg,
		firstInlined: uint32(len(sg.Productions)),
		inline:       sg.VariablesToInline,
		m:            map[inlineKey][]uint32{},
		memo:         map[string]uint32{},
	}
	var m InlinedProductionMap
	var dead map[uint32]bool
	if err := guardSize(func() { m, dead = b.build() }); err != nil {
		return InlinedProductionMap{}, err
	}

	if len(dead) > 0 {
		for i, vp := range sg.VarProds {
			if vp[0] == vp[1] {
				continue
			}
			allDead := true
			for p := vp[0]; p < vp[1]; p++ {
				if !dead[uint32(p)] {
					allDead = false
					break
				}
			}
			if !allDead {
				continue
			}
			symbol := lexbuild.NonTerminalSym(i)
			used := i == 0
			// out.steps holds the steps of every production, the built ones too.
			for p := 0; p < len(sg.Productions) && !used; p++ {
				for _, st := range sg.Productions[p].Steps {
					if st.Symbol == symbol {
						used = true
						break
					}
				}
			}
			if used {
				return InlinedProductionMap{}, &ProcessInlinesError{InlineNoReachableProductions, sg.Variables[i].Ident}
			}
		}
	}

	return m, nil
}

func abs32(x int32) int32 {
	if x < 0 {
		return -x
	}
	return x
}

// productionKey encodes the fields that the memo of inline_production_at_step
// compares (dynamic precedence, the end-of-input flag, and every field of every
// step), so that two productions have the same key exactly when they are equal.
// Strings are written with their length first, so no two encodings run into
// each other.
func productionKey(p *scratchProd) string {
	buf := make([]byte, 0, 8+len(p.steps)*40)
	u32 := func(v uint32) { buf = append(buf, byte(v), byte(v>>8), byte(v>>16), byte(v>>24)) }
	str := func(s string) { u32(uint32(len(s))); buf = append(buf, s...) }
	flag := func(b bool) {
		if b {
			buf = append(buf, 1)
		} else {
			buf = append(buf, 0)
		}
	}
	u32(uint32(p.dynamicPrecedence))
	flag(p.requiresEOF)
	for _, st := range p.steps {
		buf = append(buf, byte(st.Symbol.Kind))
		u32(st.Symbol.Index)
		buf = append(buf, byte(st.Precedence.Kind))
		switch st.Precedence.Kind {
		case lexbuild.PrecInteger:
			u32(uint32(st.Precedence.Int))
		case lexbuild.PrecName:
			str(st.Precedence.Ident)
		}
		buf = append(buf, byte(st.Associativity))
		flag(st.Alias.Set)
		if st.Alias.Set {
			str(st.Alias.Value)
			flag(st.Alias.Named)
		}
		flag(st.HasField)
		if st.HasField {
			str(st.Field)
		}
		buf = append(buf, byte(st.Reserved), byte(st.Reserved>>8))
	}
	return string(buf)
}
