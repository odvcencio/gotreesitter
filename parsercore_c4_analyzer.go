//go:build !gts_no_parsercorephase0

package gotreesitter

// C4 stage 2 — the table-shape analyzer.
//
// spec.c4-bytecode-isa.v1 section 2 records that the static table-shape
// numbers the brief reasons about — unary-chain density, sole-action cell
// share, per-state key counts — are not committed anywhere, and marks every
// such number an estimate. This analyzer is the precondition the spec sets:
// stage 2 runs it and pins the outputs before any superinstruction lands, so
// every estimate becomes a measured number.
//
// It reads the same decoded *Language table set the corridor compiler reads
// and nothing else, and it reports the compiled program's own census, so the
// receipt and the compiler can never drift.

import (
	"sort"

	core "github.com/odvcencio/gotreesitter/internal/parsercorephase0"
)

// ParserCoreCorridorTableShape is one grammar's committed table-shape receipt.
// Field names are the analyzer rows the spec cites by name.
type ParserCoreCorridorTableShape struct {
	Grammar string `json:"grammar"`
	// BlobSHA256 pins the exact grammar blob the row was measured against.
	BlobSHA256  string `json:"blob_sha256"`
	States      int    `json:"states"`
	TokenCount  uint32 `json:"token_count"`
	SymbolCount uint32 `json:"symbol_count"`

	// Populated (state, terminal) cell census.
	PopulatedCells   uint64 `json:"populated_cells"`
	SoleActionCells  uint64 `json:"sole_action_cells"`
	MultiActionCells uint64 `json:"multi_action_cells"`
	// SoleActionCellShare is the spec's "sole-action cell share" row.
	SoleActionCellShare float64 `json:"sole_action_cell_share"`

	// Compiled disposition census (spec section 4.2). Every populated cell
	// falls in exactly one bucket; the exhaustiveness test asserts the sum.
	Shift           uint64 `json:"disposition_shift"`
	ShiftExtra      uint64 `json:"disposition_shift_extra"`
	Reduce          uint64 `json:"disposition_reduce"`
	Accept          uint64 `json:"disposition_accept"`
	Fork            uint64 `json:"disposition_fork"`
	ExitGeneric     uint64 `json:"disposition_exit_generic"`
	ExitUnsupported uint64 `json:"disposition_exit_unsupported"`
	// CorridorCellShare is the fraction of populated cells the corridor
	// executes without leaving the lane.
	CorridorCellShare float64 `json:"corridor_cell_share"`

	// "EXPECT key counts" row: populated terminal keys per state.
	KeyCountMin  int     `json:"key_count_min"`
	KeyCountP50  int     `json:"key_count_p50"`
	KeyCountP90  int     `json:"key_count_p90"`
	KeyCountP99  int     `json:"key_count_p99"`
	KeyCountMax  int     `json:"key_count_max"`
	KeyCountMean float64 `json:"key_count_mean"`
	// StatesWithNoKeys counts states whose terminal row is entirely empty.
	StatesWithNoKeys int `json:"states_with_no_keys"`

	// Unary-chain rows. UnaryReduceCells counts sole-reduce cells with
	// ChildCount == 1; UnaryReduceShare is that count over all reduce cells.
	ReduceCells      uint64  `json:"reduce_cells"`
	UnaryReduceCells uint64  `json:"unary_reduce_cells"`
	UnaryReduceShare float64 `json:"unary_reduce_share"`
	// UnaryChainEdgesDepth2 is the spec's "unary-chain edges depth>=2" row: a
	// (predecessor edge, lookahead) pair whose sole unary reduce lands, after
	// goto, on another sole unary reduce for the same lookahead.
	UnaryChainEdges       uint64  `json:"unary_chain_edges"`
	UnaryChainEdgesDepth2 uint64  `json:"unary_chain_edges_depth2"`
	UnaryChainDepth2Share float64 `json:"unary_chain_depth2_share"`
	// EdgeStaticUnaryReduceCells counts unary sole-reduce cells whose goto
	// target is the same for every predecessor edge, which is the
	// edge-uniqueness precondition REDUCE_CHAIN needs.
	EdgeStaticUnaryReduceCells uint64  `json:"edge_static_unary_reduce_cells"`
	EdgeStaticUnaryReduceShare float64 `json:"edge_static_unary_reduce_share"`

	// "uniform-sole-reduce states" row: states where every populated terminal
	// key decodes to the same sole reduce action.
	UniformSoleReduceStates int     `json:"uniform_sole_reduce_states"`
	UniformSoleReduceShare  float64 `json:"uniform_sole_reduce_share"`

	// REDUCE_SHIFT candidate row: a sole-reduce cell whose post-goto row for
	// the same lookahead is a sole shift, for every predecessor edge.
	ReduceShiftCandidateCells uint64  `json:"reduce_shift_candidate_cells"`
	ReduceShiftCandidateShare float64 `json:"reduce_shift_candidate_share"`

	// Footprint rows (spec section 3.6 memory estimate, measured).
	ProgramWords      int     `json:"program_words"`
	ProgramBytes      int     `json:"program_bytes"`
	ProgramBodyWords  uint64  `json:"program_body_words"`
	ProgramBlockWords uint64  `json:"program_block_words"`
	InternedBodies    uint64  `json:"interned_bodies"`
	DenseBlocks       uint64  `json:"dense_blocks"`
	SparseBlocks      uint64  `json:"sparse_blocks"`
	BytesPerState     float64 `json:"bytes_per_state"`

	// EXPECT flag rows.
	ExternalStates   uint64 `json:"expect_ext_states"`
	ReservedWordSets uint64 `json:"expect_kw_states"`
}

// AnalyzeParserCoreCorridorTables measures one grammar's static table shape
// and compiles its corridor program, returning the committed receipt row.
func AnalyzeParserCoreCorridorTables(lang *Language) (ParserCoreCorridorTableShape, error) {
	tables, err := newCorridorTables(lang)
	if err != nil {
		return ParserCoreCorridorTableShape{}, err
	}
	program, err := CompileParserCoreCorridorProgram(lang)
	if err != nil {
		return ParserCoreCorridorTableShape{}, err
	}
	census := program.Census()

	shape := ParserCoreCorridorTableShape{
		Grammar:     lang.Name,
		States:      tables.stateCount,
		TokenCount:  lang.TokenCount,
		SymbolCount: lang.SymbolCount,

		PopulatedCells:  census.PopulatedCells,
		Shift:           census.Shift,
		ShiftExtra:      census.ShiftExtra,
		Reduce:          census.Reduce,
		Accept:          census.Accept,
		Fork:            census.Fork,
		ExitGeneric:     census.ExitGeneric,
		ExitUnsupported: census.ExitUnsupported,

		ProgramWords:      program.Words(),
		ProgramBytes:      program.Bytes(),
		ProgramBodyWords:  census.BodyWords,
		ProgramBlockWords: census.BlockWords,
		InternedBodies:    census.InternedBodies,
		DenseBlocks:       census.DenseBlocks,
		SparseBlocks:      census.SparseBlocks,
		ExternalStates:    census.ExternalStates,
		ReservedWordSets:  census.ReservedWordSets,
	}

	// Build the predecessor-edge map once. An edge P -> S exists when state P
	// has a shift action to S on some terminal, or a goto to S on some
	// nonterminal. Both are the ways a graph-stack node in state S can have a
	// predecessor in state P, which is exactly the relation a k=1 reduction
	// walks (spec section 3.3: chains are emitted per predecessor edge).
	predecessors := corridorPredecessorEdges(tables)

	keyCounts := make([]int, 0, tables.stateCount)
	var totalKeys int
	for state := 0; state < tables.stateCount; state++ {
		keys := program.KeySet(StateID(state))
		keyCounts = append(keyCounts, len(keys))
		totalKeys += len(keys)
		if len(keys) == 0 {
			shape.StatesWithNoKeys++
			continue
		}

		uniform := true
		var uniformAction core.Action
		uniformSet := false

		for _, sym := range keys {
			row, _, rowErr := tables.row(StateID(state), sym)
			if rowErr != nil {
				return ParserCoreCorridorTableShape{}, rowErr
			}
			if row.Len() == 1 {
				shape.SoleActionCells++
			} else {
				shape.MultiActionCells++
			}
			descriptor := row.Descriptor()
			if descriptor.Kind() != core.ActionRowReduce {
				uniform = false
				continue
			}
			action := row.At(0)
			shape.ReduceCells++
			if !uniformSet {
				uniformAction, uniformSet = action, true
			} else if action != uniformAction {
				uniform = false
			}
			if action.ChildCount != 1 {
				continue
			}
			shape.UnaryReduceCells++

			// Walk the predecessor edges: resolve goto(P, lhs) for every
			// predecessor P of this state, then read the landing row for the
			// same lookahead.
			edges := predecessors[state]
			edgeStatic := len(edges) > 0
			var firstTarget StateID
			reduceShiftCandidate := len(edges) > 0
			for edgeIndex, pred := range edges {
				target := tables.gotoTarget(pred, Symbol(action.Symbol))
				if edgeIndex == 0 {
					firstTarget = target
				} else if target != firstTarget {
					edgeStatic = false
				}
				if target == 0 {
					reduceShiftCandidate = false
					continue
				}
				landing, _, landErr := tables.row(target, sym)
				if landErr != nil {
					return ParserCoreCorridorTableShape{}, landErr
				}
				shape.UnaryChainEdges++
				switch landing.Descriptor().Kind() {
				case core.ActionRowReduce:
					reduceShiftCandidate = false
					if landing.At(0).ChildCount == 1 {
						shape.UnaryChainEdgesDepth2++
					}
				case core.ActionRowShift:
				default:
					reduceShiftCandidate = false
				}
			}
			if edgeStatic {
				shape.EdgeStaticUnaryReduceCells++
			}
			if reduceShiftCandidate {
				shape.ReduceShiftCandidateCells++
			}
		}
		if uniform && uniformSet {
			shape.UniformSoleReduceStates++
		}
	}

	sort.Ints(keyCounts)
	populated := keyCounts[shape.StatesWithNoKeys:]
	if len(populated) != 0 {
		shape.KeyCountMin = populated[0]
		shape.KeyCountMax = populated[len(populated)-1]
		shape.KeyCountP50 = corridorPercentile(populated, 0.50)
		shape.KeyCountP90 = corridorPercentile(populated, 0.90)
		shape.KeyCountP99 = corridorPercentile(populated, 0.99)
		shape.KeyCountMean = float64(totalKeys) / float64(len(populated))
	}

	if shape.PopulatedCells != 0 {
		total := float64(shape.PopulatedCells)
		shape.SoleActionCellShare = float64(shape.SoleActionCells) / total
		shape.CorridorCellShare = float64(census.Shift+census.ShiftExtra+census.Reduce+census.Accept) / total
	}
	if shape.ReduceCells != 0 {
		shape.UnaryReduceShare = float64(shape.UnaryReduceCells) / float64(shape.ReduceCells)
	}
	if shape.UnaryChainEdges != 0 {
		shape.UnaryChainDepth2Share = float64(shape.UnaryChainEdgesDepth2) / float64(shape.UnaryChainEdges)
	}
	if shape.UnaryReduceCells != 0 {
		shape.EdgeStaticUnaryReduceShare = float64(shape.EdgeStaticUnaryReduceCells) / float64(shape.UnaryReduceCells)
		shape.ReduceShiftCandidateShare = float64(shape.ReduceShiftCandidateCells) / float64(shape.UnaryReduceCells)
	}
	if shape.States != 0 {
		shape.UniformSoleReduceShare = float64(shape.UniformSoleReduceStates) / float64(shape.States)
		shape.BytesPerState = float64(shape.ProgramBytes) / float64(shape.States)
	}
	return shape, nil
}

// gotoTarget resolves goto(state, nonterminal) with the same rule
// Parser.lookupGoto uses, over language data only.
func (t *corridorTables) gotoTarget(state StateID, sym Symbol) StateID {
	lang := t.lang
	if lang.TokenCount > 0 && uint32(sym) >= lang.TokenCount {
		if target := lang.lookupLargeStateGoto(state, sym); target != 0 {
			return target
		}
	}
	raw := t.actionIndex(state, sym)
	if raw == 0 {
		return 0
	}
	if lang.TokenCount > 0 && uint32(sym) >= lang.TokenCount &&
		lang.StateCount > 0 && lang.InitialState > 0 {
		return StateID(raw)
	}
	if int(raw) < len(lang.ParseActions) {
		entry := &lang.ParseActions[raw]
		if len(entry.Actions) > 0 && entry.Actions[0].Type == ParseActionShift {
			return entry.Actions[0].State
		}
	}
	return 0
}

// corridorPredecessorEdges builds, for every state, the sorted deduplicated
// list of states that can precede it on the graph stack.
func corridorPredecessorEdges(t *corridorTables) [][]StateID {
	edges := make([][]StateID, t.stateCount)
	seen := make(map[uint64]struct{})
	for state := 0; state < t.stateCount; state++ {
		t.forEachKey(StateID(state), func(sym Symbol, idx uint16) {
			var target StateID
			if uint32(sym) >= t.tokenCount {
				target = t.gotoTarget(StateID(state), sym)
			} else if int(idx) < len(t.lang.ParseActions) {
				for _, action := range t.lang.ParseActions[idx].Actions {
					if action.Type == ParseActionShift {
						target = action.State
						break
					}
				}
			}
			if target == 0 || int(target) >= t.stateCount {
				return
			}
			key := uint64(target)<<32 | uint64(state)
			if _, ok := seen[key]; ok {
				return
			}
			seen[key] = struct{}{}
			edges[target] = append(edges[target], StateID(state))
		})
	}
	return edges
}

// corridorPercentile returns the nearest-rank percentile of a sorted slice.
func corridorPercentile(sorted []int, q float64) int {
	if len(sorted) == 0 {
		return 0
	}
	rank := int(q*float64(len(sorted))+0.5) - 1
	if rank < 0 {
		rank = 0
	}
	if rank >= len(sorted) {
		rank = len(sorted) - 1
	}
	return sorted[rank]
}
