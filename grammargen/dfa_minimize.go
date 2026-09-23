package grammargen

import (
	"context"
	"os"

	"github.com/odvcencio/gotreesitter"
)

// lexMinimizeDisabledEnv is a rollback safety valve for minimizeLexStates.
// Set GTS_GRAMMARGEN_DISABLE_LEX_MINIMIZE=1 to fall back to the raw,
// per-mode-concatenated LexStates table (pre-fix behavior) if a correctness
// question ever comes up in the field. Minimization is behavior-preserving
// for every *consumer that treats LexState indices as opaque*, so this
// should never be needed for the pure-Go runtime; it exists purely as an
// emergency escape hatch.
func lexMinimizeDisabledEnv() bool {
	return os.Getenv("GTS_GRAMMARGEN_DISABLE_LEX_MINIMIZE") == "1"
}

// lexMinimizeCtxKey/withLexMinimizeDisabled/lexMinimizeDisabledInCtx thread a
// per-call (not process-global) opt-out through buildLexDFA's ctx parameter.
//
// Why this is needed: grammargen/codegen_c.go's C emitter (modeStartOf) does
// NOT treat LexState indices as opaque. It relies on each lex mode occupying
// a contiguous, increasing block of state indices (buildLexDFA's un-minimized
// output has this property by construction: modes are simply concatenated)
// so that, for a multi-character skip sequence (CRLF, backslash-newline
// continuation) reached mid-token, it can recover "which mode does this
// state belong to" purely from the state's position, and bake a static
// SKIP(<that mode's start>) into the generated C switch-case. Cross-mode
// state sharing breaks that invariant outright: a shared state can be
// reachable from more than one mode's start, so there is no longer a single
// statically-correct answer, and the merge can also reorder states so even
// *unshared* states in a later mode are no longer positioned after an
// earlier mode's block.
//
// The pure-Go runtime (lexer.go, parser_dfa_token_source.go) has no such
// constraint: it always re-enters lexing at an explicit start-state
// parameter tracked by the caller, never inferred from a state's array
// position, so minimization is fully safe there. GenerateLanguage / Generate
// (the blob/runtime path -- where the 491 MB Swift retained-heap problem
// actually lives) therefore minimize by default; GenerateLanguageForC (used
// by GenerateC / EmitC's real production entry point) opts out via this
// context value so the C emitter keeps receiving the contiguous-block
// LexStates layout it depends on.
type lexMinimizeCtxKey struct{}

func withLexMinimizeDisabled(ctx context.Context) context.Context {
	return context.WithValue(ctx, lexMinimizeCtxKey{}, true)
}

func lexMinimizeDisabledInCtx(ctx context.Context) bool {
	if ctx == nil {
		return false
	}
	v, _ := ctx.Value(lexMinimizeCtxKey{}).(bool)
	return v
}

// minimizeLexStates collapses observationally-equivalent LexStates produced
// by buildLexDFA's per-lex-mode NFA->DFA subset construction into shared
// physical states.
//
// Root cause this addresses: buildLexDFA builds a fresh, independent DFA per
// lex mode with no cross-mode sharing. For grammars with many lex modes that
// each admit a large, mostly-overlapping set of terminals (e.g. Swift's ~190
// contextual/soft-keyword-driven modes, most differing from a neighboring
// mode by only one or two valid terminal symbols -- see the investigation in
// grammars/language_memory_ceiling_test.go), the *inputs* to buildLexDFA are
// almost never byte-identical across modes (so memoizing purely on the
// (validSymbols, preferredSymbols, skipWhitespace) input signature turns out
// to be close to a no-op in practice: computeLexModesWithContext already
// deduplicates lex mode *specs* by exactly that signature before they ever
// reach buildLexDFA, so no two entries in the modes slice passed to
// buildLexDFA can share it). What *is* true is that most of the resulting
// per-mode automata share huge behaviorally-identical sub-regions (e.g. the
// entire identifier/number/string/comment/operator "core" beneath the first
// one or two distinguishing characters). This function finds and merges that
// shared structure directly, at the state level, after construction:
//
// Two LexStates are equivalent iff they have the same (AcceptToken,
// AcceptPriority, Skip, AcceptEOF, "has Default", "has EOF") signature AND, for every
// character, transition to (recursively) equivalent states with the same
// per-transition Skip flag. This is standard Moore-style DFA partition
// refinement generalized to a forest of automata with multiple start states
// (one per lex mode) sharing one state array. It is the Myhill-Nerode
// equivalence relation on the multi-root automaton: merged states are
// indistinguishable by ANY sequence of future input, by construction, so
// this can never change observable lexer behavior. It only shrinks the
// physical state count and therefore the serialized LexState/LexTransition
// tables (and the retained heap of a decoded Language). Minimization does
// not change the state schema or the runtime format.
//
// modeOffsets (each mode's start-state index into states) is remapped in
// lockstep so every consumer that already treats mode start indices as
// opaque (assemble.go's lang.LexModes wiring) keeps working unmodified.
func minimizeLexStates(states []gotreesitter.LexState, modeOffsets []int) ([]gotreesitter.LexState, []int) {
	n := len(states)
	if n == 0 || lexMinimizeDisabledEnv() {
		return states, modeOffsets
	}

	color := computeLexStateEquivalenceColors(states)
	numColors := int32(0)
	for _, c := range color {
		if c+1 > numColors {
			numColors = c + 1
		}
	}

	// No merging opportunity at all: every state is already in its own class.
	if int(numColors) == n {
		return states, modeOffsets
	}

	// Pick a deterministic representative per class: the smallest original
	// index. Iterating i ascending and only recording the first occurrence
	// per class achieves this directly.
	rep := make([]int32, numColors)
	for i := range rep {
		rep[i] = -1
	}
	for i := 0; i < n; i++ {
		c := color[i]
		if rep[c] == -1 {
			rep[c] = int32(i)
		}
	}

	out := make([]gotreesitter.LexState, numColors)
	for c := int32(0); c < numColors; c++ {
		s := &states[rep[c]]
		def := -1
		if s.Default >= 0 {
			def = int(color[s.Default])
		}
		eof := -1
		if s.EOF >= 0 {
			eof = int(color[s.EOF])
		}
		out[c] = gotreesitter.LexState{
			AcceptToken:    s.AcceptToken,
			AcceptPriority: s.AcceptPriority,
			Skip:           s.Skip,
			AcceptEOF:      s.AcceptEOF,
			Default:        def,
			EOF:            eof,
			Transitions:    coalesceTransitions(s.Transitions, color),
		}
	}

	newOffsets := make([]int, len(modeOffsets))
	for i, off := range modeOffsets {
		if off >= 0 && off < n {
			newOffsets[i] = int(color[off])
		} else {
			newOffsets[i] = off
		}
	}

	return out, newOffsets
}

// computeLexStateEquivalenceColors runs Moore-style partition refinement to
// a fixed point and returns each state's final equivalence-class id. Two
// states end up in the same class iff they are provably behaviorally
// indistinguishable (same accept/priority/skip/default/eof shape and,
// recursively, equivalent successors on every character).
func computeLexStateEquivalenceColors(states []gotreesitter.LexState) []int32 {
	n := len(states)
	color := make([]int32, n)

	// Initial partition: states with different observable "shape" (accept
	// token+priority+skip, whether Default/EOF are present) can never be
	// equivalent, so they always start in different classes.
	type colorKey struct {
		accept    gotreesitter.Symbol
		prio      int16
		skip      bool
		acceptEOF bool
		hasDef    bool
		hasEOF    bool
	}
	initMap := make(map[colorKey]int32, 64)
	var nextColor int32
	for i := range states {
		s := &states[i]
		k := colorKey{s.AcceptToken, s.AcceptPriority, s.Skip, s.AcceptEOF, s.Default >= 0, s.EOF >= 0}
		c, ok := initMap[k]
		if !ok {
			c = nextColor
			nextColor++
			initMap[k] = c
		}
		color[i] = c
	}

	newColor := make([]int32, n)
	var sigBuf []byte

	for {
		numColors := int(nextColor)
		groups := make([][]int32, numColors)
		for i, c := range color {
			groups[c] = append(groups[c], int32(i))
		}

		var assignedColor int32
		changed := false
		sigs := make([]string, 0, 8)

		for _, members := range groups {
			if len(members) == 0 {
				continue
			}
			if len(members) == 1 {
				newColor[members[0]] = assignedColor
				assignedColor++
				continue
			}
			sigs = sigs[:0]
			for _, idx := range members {
				sigBuf = lexStateSignature(sigBuf[:0], &states[idx], color)
				sigs = append(sigs, string(sigBuf))
			}
			sigMap := make(map[string]int32, len(members))
			for _, sig := range sigs {
				if _, ok := sigMap[sig]; !ok {
					sigMap[sig] = int32(len(sigMap))
				}
			}
			if len(sigMap) > 1 {
				changed = true
			}
			base := assignedColor
			for mi, idx := range members {
				newColor[idx] = base + sigMap[sigs[mi]]
			}
			assignedColor += int32(len(sigMap))
		}

		color, newColor = newColor, color
		nextColor = assignedColor

		if !changed {
			break
		}
	}

	return color
}

// lexStateSignature appends a canonical, self-delimiting encoding of state
// s's observable transition behavior (given the current color assignment) to
// buf and returns the extended slice. Adjacent transitions that target the
// same color with the same Skip flag are coalesced first so that boundary
// granularity differences between two otherwise-equivalent states (which can
// arise because each mode's DFA is built from a different, independently
// range-partitioned NFA) never cause a spurious split.
func lexStateSignature(buf []byte, s *gotreesitter.LexState, color []int32) []byte {
	if s.Default >= 0 {
		buf = append(buf, 1)
		buf = appendColor(buf, color[s.Default])
	} else {
		buf = append(buf, 0)
	}
	if s.EOF >= 0 {
		buf = append(buf, 1)
		buf = appendColor(buf, color[s.EOF])
	} else {
		buf = append(buf, 0)
	}

	trs := s.Transitions
	for i := 0; i < len(trs); {
		lo := trs[i].Lo
		hi := trs[i].Hi
		tc := color[trs[i].NextState]
		sk := trs[i].Skip
		j := i + 1
		for j < len(trs) && trs[j].Lo == hi+1 && color[trs[j].NextState] == tc && trs[j].Skip == sk {
			hi = trs[j].Hi
			j++
		}
		buf = appendRune(buf, lo)
		buf = appendRune(buf, hi)
		buf = appendColor(buf, tc)
		if sk {
			buf = append(buf, 1)
		} else {
			buf = append(buf, 0)
		}
		i = j
	}
	return buf
}

// coalesceTransitions builds the final, maximally-merged transition list for
// a class representative using the FINAL (fixed-point) color assignment.
// This can merge more aggressively than any single round's signature did,
// because two ranges whose targets only converged to the same class in a
// later round could not have been merged earlier.
func coalesceTransitions(trs []gotreesitter.LexTransition, color []int32) []gotreesitter.LexTransition {
	if len(trs) == 0 {
		return nil
	}
	out := make([]gotreesitter.LexTransition, 0, len(trs))
	for i := 0; i < len(trs); {
		lo := trs[i].Lo
		hi := trs[i].Hi
		tc := color[trs[i].NextState]
		sk := trs[i].Skip
		j := i + 1
		for j < len(trs) && trs[j].Lo == hi+1 && color[trs[j].NextState] == tc && trs[j].Skip == sk {
			hi = trs[j].Hi
			j++
		}
		out = append(out, gotreesitter.LexTransition{Lo: lo, Hi: hi, NextState: int(tc), Skip: sk})
		i = j
	}
	return out
}

func appendColor(buf []byte, c int32) []byte {
	return append(buf, byte(c>>24), byte(c>>16), byte(c>>8), byte(c))
}

func appendRune(buf []byte, r rune) []byte {
	return appendColor(buf, int32(r))
}
