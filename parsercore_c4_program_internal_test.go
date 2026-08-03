//go:build gts_parsercorephase0

package gotreesitter

// C4 stage 2 static equivalence obligations (spec.c4-bytecode-isa.v1
// section 4.3 and section 5, S1-S3).

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"testing"

	core "github.com/odvcencio/gotreesitter/internal/parsercorephase0"
)

// corridorAnalyzedGrammars are the blobs the spec names for the stage-2
// table-shape receipt (spec section 2).
var corridorAnalyzedGrammars = []string{
	"grammars/grammar_blobs/go.bin",
	"grammars/grammar_blobs/javascript.bin",
	"grammars/grammar_blobs/json.bin",
}

func loadCorridorGrammarForTest(t *testing.T, path string) *Language {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	lang, err := LoadLanguage(data)
	if err != nil {
		t.Fatalf("decode %s: %v", path, err)
	}
	if lang.Name == "" {
		lang.Name = path
	}
	return lang
}

// TestParserCoreCorridorCellExhaustiveness is the completeness proof
// obligation of spec section 4.3, binding: a compile-time exhaustiveness test
// walks every (state, terminal) cell of every admitted grammar and asserts the
// compiled disposition is one of the enumerated forms. There is no default
// arm: the switch below names every admitted opcode and fails the test on
// anything else, and it separately proves the compiler visited exactly the
// cells the tables populate.
func TestParserCoreCorridorCellExhaustiveness(t *testing.T) {
	for _, path := range corridorAnalyzedGrammars {
		path := path
		t.Run(path, func(t *testing.T) {
			lang := loadCorridorGrammarForTest(t, path)
			tables, err := newCorridorTables(lang)
			if err != nil {
				t.Fatalf("table view: %v", err)
			}
			program, err := CompileParserCoreCorridorProgram(lang)
			if err != nil {
				t.Fatalf("compile: %v", err)
			}

			var walked uint64
			var byOpcode [corridorOpcodeCount]uint64
			for state := 0; state < tables.stateCount; state++ {
				for sym := Symbol(0); uint32(sym) < tables.tokenCount; sym++ {
					row, _, rowErr := tables.row(StateID(state), sym)
					if rowErr != nil {
						t.Fatalf("decode (state=%d symbol=%d): %v", state, sym, rowErr)
					}
					decoded, ok := program.DecodeCell(StateID(state), sym)
					if row.Len() == 0 {
						if ok {
							t.Fatalf("state=%d symbol=%d has no table action but compiled to %s", state, sym, decoded.Opcode)
						}
						continue
					}
					if !ok {
						t.Fatalf("state=%d symbol=%d has %d table actions but no compiled disposition", state, sym, row.Len())
					}
					walked++

					// Enumerated forms only. Every arm below is a row of the
					// spec section 4.2 dispositions table.
					switch decoded.Opcode {
					case corridorOpShift.String():
						requireCorridorKind(t, state, sym, row, core.ActionRowShift)
						if decoded.TargetState != StateID(row.At(0).State) {
							t.Fatalf("state=%d symbol=%d SHIFT target=%d table=%d", state, sym, decoded.TargetState, row.At(0).State)
						}
						byOpcode[corridorOpShift]++
					case corridorOpShiftExtra.String():
						requireCorridorKind(t, state, sym, row, core.ActionRowExtraShift)
						byOpcode[corridorOpShiftExtra]++
					case corridorOpReduce.String():
						requireCorridorKind(t, state, sym, row, core.ActionRowReduce)
						action := row.At(0)
						if decoded.ProductionID != action.ProductionID ||
							decoded.LHS != Symbol(action.Symbol) ||
							decoded.ChildCount != action.ChildCount {
							t.Fatalf("state=%d symbol=%d REDUCE decoded=%+v table=%+v", state, sym, decoded, action)
						}
						byOpcode[corridorOpReduce]++
					case corridorOpAccept.String():
						requireCorridorKind(t, state, sym, row, core.ActionRowAccept)
						byOpcode[corridorOpAccept]++
					case corridorOpFork.String():
						requireCorridorKind(t, state, sym, row, core.ActionRowConflict)
						byOpcode[corridorOpFork]++
					case corridorOpExitGeneric.String():
						if row.Descriptor().Kind() != core.ActionRowUnsupported {
							t.Fatalf("state=%d symbol=%d EXIT_GENERIC on kind=%d", state, sym, row.Descriptor().Kind())
						}
						if !tables.repetitionSelectable(row) {
							t.Fatalf("state=%d symbol=%d EXIT_GENERIC without a static generic-lane selection", state, sym)
						}
						byOpcode[corridorOpExitGeneric]++
					case corridorOpExitUnsupported.String():
						if row.Descriptor().Kind() != core.ActionRowUnsupported {
							t.Fatalf("state=%d symbol=%d EXIT_UNSUPPORTED on kind=%d", state, sym, row.Descriptor().Kind())
						}
						if decoded.Reason == "" {
							t.Fatalf("state=%d symbol=%d EXIT_UNSUPPORTED has no named reason", state, sym)
						}
						byOpcode[corridorOpExitUnsupported]++
					default:
						t.Fatalf("state=%d symbol=%d compiled to unenumerated disposition %q", state, sym, decoded.Opcode)
					}
				}
			}

			census := program.Census()
			if walked != census.PopulatedCells {
				t.Fatalf("walked %d populated cells, compiler census recorded %d", walked, census.PopulatedCells)
			}
			sum := byOpcode[corridorOpShift] + byOpcode[corridorOpShiftExtra] + byOpcode[corridorOpReduce] +
				byOpcode[corridorOpAccept] + byOpcode[corridorOpFork] + byOpcode[corridorOpExitGeneric] +
				byOpcode[corridorOpExitUnsupported]
			if sum != walked {
				t.Fatalf("disposition partition sums to %d over %d cells", sum, walked)
			}
			if byOpcode[corridorOpShift] != census.Shift || byOpcode[corridorOpReduce] != census.Reduce ||
				byOpcode[corridorOpShiftExtra] != census.ShiftExtra || byOpcode[corridorOpAccept] != census.Accept ||
				byOpcode[corridorOpFork] != census.Fork || byOpcode[corridorOpExitGeneric] != census.ExitGeneric ||
				byOpcode[corridorOpExitUnsupported] != census.ExitUnsupported {
				t.Fatalf("walked disposition counts diverge from the compiler census: walked=%v census=%+v", byOpcode, census)
			}
		})
	}
}

func requireCorridorKind(t *testing.T, state int, sym Symbol, row core.ActionRow, want core.ActionRowKind) {
	t.Helper()
	if row.Descriptor().Kind() != want {
		t.Fatalf("state=%d symbol=%d compiled disposition expects kind %d, table decodes kind %d", state, sym, want, row.Descriptor().Kind())
	}
}

// TestParserCoreCorridorDecodeBackPerState is obligation S2: a test decodes
// every state block back into []core.Action rows and requires exact equality —
// including descriptor equality — with buildParserCoreLanguageTables output for
// every (state, symbol).
//
// The corridor's own decode path is the interned key set plus the dispatch
// table, so the test walks those and reconstructs the constituent cell from
// the compiled operands, then compares against the row the shared table
// converter produces for the same language.
func TestParserCoreCorridorDecodeBackPerState(t *testing.T) {
	lang := loadCertifiedGoLanguageForTest(t)
	parser := NewParser(lang)
	reference, err := buildParserCoreLanguageTables(parser)
	if err != nil {
		t.Fatalf("build reference tables: %v", err)
	}
	tables, err := newCorridorTables(lang)
	if err != nil {
		t.Fatalf("table view: %v", err)
	}
	program, err := CompileParserCoreCorridorProgram(lang)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}

	for state := 0; state < tables.stateCount; state++ {
		keys := program.KeySet(StateID(state))
		// The interned key set must be exactly the populated terminal keys.
		var expected []Symbol
		for sym := Symbol(0); uint32(sym) < tables.tokenCount; sym++ {
			// Oracle discipline: index through Parser.lookupActionIndex, the
			// lookup the running scheduler actually uses, not through the
			// compiler's own table walk. Using the compiler's walk on both
			// sides would make this test blind to any divergence between the
			// two — and they are not trivially the same function: the raw
			// small-table walk is first-wins while smallTokenLookup is
			// last-wins, so a duplicated symbol in one small-table group would
			// resolve differently.
			idx := parser.lookupActionIndex(StateID(state), sym)
			if idx == 0 || int(idx) >= len(reference.actionRows) {
				continue
			}
			if reference.actionRows[idx].Len() == 0 {
				continue
			}
			expected = append(expected, sym)
		}
		if len(keys) != len(expected) {
			t.Fatalf("state=%d key set has %d keys, table has %d populated terminals", state, len(keys), len(expected))
		}
		for i := range keys {
			if keys[i] != expected[i] {
				t.Fatalf("state=%d key set entry %d is %d, table has %d", state, i, keys[i], expected[i])
			}
		}

		for _, sym := range keys {
			idx := parser.lookupActionIndex(StateID(state), sym)
			if compiled := tables.actionIndex(StateID(state), sym); compiled != idx {
				t.Fatalf("state=%d symbol=%d compiler action index %d differs from Parser.lookupActionIndex %d",
					state, sym, compiled, idx)
			}
			want := reference.actionRows[idx]
			decoded, ok := program.DecodeCell(StateID(state), sym)
			if !ok {
				t.Fatalf("state=%d symbol=%d is in the key set but has no compiled cell", state, sym)
			}
			// The soundness obligation for Core.ClassifyBoundaryWithRow: every
			// executable body's action-row index must equal the table's own
			// action index for this cell, so the interpreter's direct row read
			// is the row ClassifyBoundary would have resolved.
			switch decoded.Opcode {
			case corridorOpShift.String(), corridorOpShiftExtra.String(),
				corridorOpReduce.String(), corridorOpAccept.String(), corridorOpFork.String():
				if decoded.RowIndex != uint32(idx) {
					t.Fatalf("state=%d symbol=%d %s carries action-row index %d, table index is %d",
						state, sym, decoded.Opcode, decoded.RowIndex, idx)
				}
				if reference.actionRows[decoded.RowIndex].Descriptor() != want.Descriptor() {
					t.Fatalf("state=%d symbol=%d row index %d resolves a different descriptor", state, sym, decoded.RowIndex)
				}
			}
			// Descriptor equality: the compiled opcode must name the same
			// dispatch shape describeActionRow computed for the reference row.
			if got := corridorOpcodeForKind(want.Descriptor().Kind(), tables, want); got != decoded.Opcode {
				t.Fatalf("state=%d symbol=%d decodes to %s, reference descriptor implies %s", state, sym, decoded.Opcode, got)
			}
			// Constituent-cell equality for the executable forms.
			switch decoded.Opcode {
			case corridorOpShift.String():
				if want.Len() != 1 || StateID(want.At(0).State) != decoded.TargetState {
					t.Fatalf("state=%d symbol=%d SHIFT operand does not reconstruct the cell", state, sym)
				}
			case corridorOpReduce.String():
				action := want.At(0)
				if want.Len() != 1 || decoded.ProductionID != action.ProductionID ||
					decoded.LHS != Symbol(action.Symbol) || decoded.ChildCount != action.ChildCount ||
					decoded.GotoMode != "indexed" {
					t.Fatalf("state=%d symbol=%d REDUCE operands do not reconstruct the cell: decoded=%+v action=%+v", state, sym, decoded, action)
				}
			case corridorOpShiftExtra.String():
				action := want.At(0)
				if want.Len() != 1 || !action.Extra {
					t.Fatalf("state=%d symbol=%d SHIFT_EXTRA does not reconstruct an extra shift", state, sym)
				}
				if !decoded.SelfLoop && StateID(action.State) != decoded.TargetState {
					t.Fatalf("state=%d symbol=%d SHIFT_EXTRA target=%d table=%d", state, sym, decoded.TargetState, action.State)
				}
			case corridorOpFork.String():
				if decoded.RowIndex != uint32(idx) {
					t.Fatalf("state=%d symbol=%d FORK carries row index %d, table index is %d", state, sym, decoded.RowIndex, idx)
				}
			}
		}

		// The CHECKPOINT decode form must be present exactly for EXT states.
		obligation := program.DecodeCheckpointObligation(StateID(state))
		wantCheckpoint := tables.externalState(StateID(state))
		if wantCheckpoint != (obligation == corridorOpCheckpoint.String()) {
			t.Fatalf("state=%d checkpoint obligation %q does not match external-row emptiness %v", state, obligation, wantCheckpoint)
		}
	}
}

// corridorOpcodeForKind names the opcode a descriptor kind must compile to.
// It is the decode-back side of the dispositions table and is deliberately
// written independently of classifyCell so the two must agree.
func corridorOpcodeForKind(kind core.ActionRowKind, tables *corridorTables, row core.ActionRow) string {
	switch kind {
	case core.ActionRowShift:
		return corridorOpShift.String()
	case core.ActionRowExtraShift:
		return corridorOpShiftExtra.String()
	case core.ActionRowReduce:
		return corridorOpReduce.String()
	case core.ActionRowAccept:
		return corridorOpAccept.String()
	case core.ActionRowConflict:
		return corridorOpFork.String()
	case core.ActionRowUnsupported:
		if tables.repetitionSelectable(row) {
			return corridorOpExitGeneric.String()
		}
		return corridorOpExitUnsupported.String()
	default:
		return corridorOpHalt.String()
	}
}

// TestParserCoreCorridorStreamValidation is obligation S3: the compiler must
// never publish a stream whose offsets, set ids, or block shapes are invalid,
// and a corrupted stream must be rejected rather than trusted.
func TestParserCoreCorridorStreamValidation(t *testing.T) {
	lang := loadCertifiedGoLanguageForTest(t)
	program, err := CompileParserCoreCorridorProgram(lang)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	if err := program.validate(); err != nil {
		t.Fatalf("published stream fails its own validator: %v", err)
	}

	corruptions := []struct {
		name    string
		corrupt func(p *ParserCoreCorridorProgram)
	}{
		{"halt guard removed", func(p *ParserCoreCorridorProgram) { p.prog[0] = uint32(corridorOpShift) }},
		{"block header is not EXPECT", func(p *ParserCoreCorridorProgram) {
			p.prog[p.stateBlockOffset[1]] = uint32(corridorOpShift)
		}},
		{"key set id out of range", func(p *ParserCoreCorridorProgram) {
			base := p.stateBlockOffset[1]
			p.prog[base+1] = p.prog[base+1]&^uint32(0xFFFF) | uint32(len(p.keySets)+1)
		}},
		{"dispatch table overruns the stream", func(p *ParserCoreCorridorProgram) {
			base := p.stateBlockOffset[1]
			p.prog[base+1] = p.prog[base+1]&0xFFFF | uint32(len(p.prog))<<16
		}},
		{"SHIFT target points inside a block header", func(p *ParserCoreCorridorProgram) {
			// The set-id word of a block whose set id is congruent to the
			// EXPECT opcode modulo 64 reads as an EXPECT instruction to an
			// opcode-bit sniff. Aim a SHIFT at it and require the validator to
			// reject the target because it is not a compiled block start.
			for state := 1; state < p.states; state++ {
				base := p.stateBlockOffset[state]
				if p.prog[base+1]&corridorOpcodeMask != uint32(corridorOpExpect) {
					continue
				}
				for _, body := range p.forEachShiftBodyForTest() {
					p.prog[body] = uint32(corridorOpShift) | (base+1)<<6
					return
				}
			}
		}},
		{"FORK carries an out-of-range action-row index", func(p *ParserCoreCorridorProgram) {
			for word := uint32(1); int(word) < len(p.prog); word++ {
				if p.prog[word]&corridorOpcodeMask == uint32(corridorOpFork) {
					p.prog[word] = uint32(corridorOpFork) | uint32(p.rowCount+1)<<6
					return
				}
			}
		}},
	}
	for _, tc := range corruptions {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			fresh, err := CompileParserCoreCorridorProgram(loadCertifiedGoLanguageForTest(t))
			if err != nil {
				t.Fatalf("compile: %v", err)
			}
			tc.corrupt(fresh)
			if err := fresh.validate(); err == nil {
				t.Fatal("validator accepted a corrupted stream")
			}
		})
	}
}

// TestParserCoreCorridorCompilerIsDeterministic backs obligation S1: one
// compiler, one output. Two compiles of the same decoded table set must
// produce byte-identical streams, which is what makes a future
// grammargen-baked stream a pure cache.
func TestParserCoreCorridorCompilerIsDeterministic(t *testing.T) {
	for _, path := range corridorAnalyzedGrammars {
		path := path
		t.Run(path, func(t *testing.T) {
			first, err := CompileParserCoreCorridorProgram(loadCorridorGrammarForTest(t, path))
			if err != nil {
				t.Fatalf("compile: %v", err)
			}
			second, err := CompileParserCoreCorridorProgram(loadCorridorGrammarForTest(t, path))
			if err != nil {
				t.Fatalf("recompile: %v", err)
			}
			if len(first.prog) != len(second.prog) {
				t.Fatalf("stream length differs: %d vs %d", len(first.prog), len(second.prog))
			}
			for i := range first.prog {
				if first.prog[i] != second.prog[i] {
					t.Fatalf("stream word %d differs: %#x vs %#x", i, first.prog[i], second.prog[i])
				}
			}
		})
	}
}

// TestParserCoreCorridorTableShapeReceipt pins the analyzer output
// (spec section 2: stage 2 runs the analyzer and pins the outputs). It fails
// when the shipped tables no longer produce the committed numbers, so every
// figure the stage-2 model rests on stays a measured number.
func TestParserCoreCorridorTableShapeReceipt(t *testing.T) {
	const receiptPath = "testdata/c4_table_shape.json"
	committed, err := os.ReadFile(receiptPath)
	if err != nil {
		t.Fatalf("read committed receipt: %v", err)
	}

	type receipt struct {
		Schema   string                         `json:"schema"`
		Spec     string                         `json:"spec"`
		Grammars []ParserCoreCorridorTableShape `json:"grammars"`
	}
	measured := receipt{Schema: "gts-c4-table-shape/v1", Spec: "spec.c4-bytecode-isa.v1"}
	for _, path := range corridorAnalyzedGrammars {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		lang, err := LoadLanguage(data)
		if err != nil {
			t.Fatalf("decode %s: %v", path, err)
		}
		shape, err := AnalyzeParserCoreCorridorTables(lang)
		if err != nil {
			t.Fatalf("analyze %s: %v", path, err)
		}
		shape.BlobSHA256 = fmt.Sprintf("%x", sha256.Sum256(data))
		measured.Grammars = append(measured.Grammars, shape)
	}
	encoded, err := json.MarshalIndent(measured, "", "  ")
	if err != nil {
		t.Fatalf("encode measured receipt: %v", err)
	}
	encoded = append(encoded, '\n')
	if !bytes.Equal(committed, encoded) {
		t.Fatalf("table-shape receipt %s is stale; regenerate with:\n"+
			"  go run ./cmd/c4tablestats -out %s %s %s %s",
			receiptPath, receiptPath,
			corridorAnalyzedGrammars[0], corridorAnalyzedGrammars[1], corridorAnalyzedGrammars[2])
	}
}

// forEachShiftBodyForTest returns the stream offsets of every SHIFT body, so a
// corruption case can aim one at a chosen word.
func (p *ParserCoreCorridorProgram) forEachShiftBodyForTest() []uint32 {
	var found []uint32
	for state := 0; state < p.states; state++ {
		for _, sym := range p.KeySet(StateID(state)) {
			body := corridorDispatch(p.prog, p.stateBlockOffset[state], sym)
			if body != 0 && p.prog[body]&corridorOpcodeMask == uint32(corridorOpShift) {
				found = append(found, body)
			}
		}
	}
	return found
}
