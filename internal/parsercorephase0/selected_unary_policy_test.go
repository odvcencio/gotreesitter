package parsercorephase0

import (
	"reflect"
	"strings"
	"testing"
	"unsafe"
)

func programPolicyForTest(t *testing.T, dense SelectedStorePolicy) SelectedStorePolicy {
	t.Helper()
	width := len(dense.Symbols)
	rows := make([]SelectedUnaryRow, width)
	var pairs []SelectedUnaryOverride
	for parent := range rows {
		rows[parent].Self = dense.Unary[parent*width+parent]
		for child := 0; child < width; child++ {
			if parent != child && dense.Unary[parent*width+child] != SelectedUnaryKeep {
				pairs = append(pairs, SelectedUnaryOverride{Parent: Symbol(parent), Child: Symbol(child), Rule: dense.Unary[parent*width+child]})
			}
		}
	}
	program, err := NewSelectedUnaryProgram(rows, pairs)
	if err != nil {
		t.Fatal(err)
	}
	out, err := NewSelectedStoreProgramPolicy(dense.Symbols, program, dense.ExpectedRoot)
	if err != nil {
		t.Fatal(err)
	}
	symbols := out.Symbols
	out = dense
	out.Symbols, out.Unary, out.unaryProgram = symbols, nil, program
	return out
}

func TestSelectedUnaryPolicyOwnershipValidationAndFootprint(t *testing.T) {
	dense := selectedStoreTestPolicy(t, 4, 2, 1)
	dense.Unary[2*len(dense.Symbols)+1] = SelectedUnaryRenameLeaf
	policy := programPolicyForTest(t, dense)
	dense.Symbols[1].Visible = false
	dense.Unary[2*len(dense.Symbols)+1] = SelectedUnaryPass
	if !policy.Symbols[1].Visible || policy.unary(2, 1) != SelectedUnaryRenameLeaf {
		t.Fatal("program policy borrowed mutable caller metadata")
	}
	if policy.unary(65535, 1) != SelectedUnaryKeep || policy.unary(1, 65535) != SelectedUnaryKeep {
		t.Fatal("unknown-symbol lookup changed")
	}
	if err := policy.validate(); err != nil {
		t.Fatal(err)
	}
	expected := uint64(unsafe.Sizeof(policy)) + uint64(cap(policy.Symbols))*uint64(unsafe.Sizeof(SelectedSymbolPolicy{})) + policy.unaryProgram.backingBytes()
	if got := policy.RetainedBytes(); got != expected {
		t.Fatalf("retained=%d want=%d", got, expected)
	}
	c := newTinyCore(t, 8)
	before := c.FootprintBytes()
	c.selectedPolicy = &policy
	if got := c.FootprintBytes() - before; got != expected {
		t.Fatalf("core omitted program footprint: %d want=%d", got, expected)
	}
	mixed := policy
	mixed.Unary = make([]SelectedUnaryRule, len(policy.Symbols)*len(policy.Symbols))
	if err := mixed.validate(); err == nil {
		t.Fatal("mixed dense/program policy accepted")
	}
	short := policy
	short.Symbols = short.Symbols[:len(short.Symbols)-1]
	if err := short.validate(); err == nil {
		t.Fatal("program width drift accepted")
	}
	if _, err := NewSelectedStoreProgramPolicy(policy.Symbols, SelectedUnaryProgram{}, 4); err == nil {
		t.Fatal("nil program accepted")
	}
}

func TestSelectedUnaryPolicySelectedTreeParityAndCaps(t *testing.T) {
	for _, name := range []string{"keep", "pass", "rename", "extra", "missing", "alias", "supertype"} {
		t.Run(name, func(t *testing.T) {
			dense := selectedStoreTestPolicy(t, 4, 2, 1, 5)
			if name == "pass" || name == "extra" || name == "missing" {
				dense.Unary[2*len(dense.Symbols)+1] = SelectedUnaryPass
			}
			if name == "rename" || name == "alias" {
				dense.Unary[2*len(dense.Symbols)+1] = SelectedUnaryRenameLeaf
			}
			if name == "alias" {
				dense.SetRetainedAliasChildren([]SelectedAliasChildPair{{Alias: 5, Child: 2}})
			}
			if name == "supertype" {
				masks := make([]uint32, len(dense.Symbols))
				masks[3] = 1
				if err := dense.SetSupertypeMasks(masks); err != nil {
					t.Fatal(err)
				}
			}
			program := programPolicyForTest(t, dense)
			build := func(policy SelectedStorePolicy, limits Limits) (*SelectedStore, error) {
				c, err := New(&fakeTable{}, limits)
				if err != nil {
					return nil, err
				}
				end := uint32(1)
				if name == "missing" {
					end = 0
				}
				leaf, err := c.appendSubtree(subtreeRecord{symbol: 1, productionID: 8, dynamicPrecedence: 2, endByte: end, terminal: true, extra: name == "extra", missing: name == "missing"}, nil, nil, nil)
				if err != nil {
					return nil, err
				}
				wrapper, err := c.appendSubtree(subtreeRecord{symbol: 2, productionID: 9, dynamicPrecedence: 3, endByte: end}, []SubtreeID{leaf}, nil, nil)
				if err != nil {
					return nil, err
				}
				child := wrapper
				if name == "supertype" {
					child, err = c.appendSubtree(subtreeRecord{symbol: 3, endByte: end}, []SubtreeID{wrapper}, nil, nil)
					if err != nil {
						return nil, err
					}
				}
				var aliases []Symbol
				if name == "alias" {
					aliases = []Symbol{5, 0}
				}
				root, err := c.appendSubtree(subtreeRecord{symbol: 4, endByte: 2}, []SubtreeID{child, child}, []FieldMapEntry{{FieldID: 3, ChildIndex: 0}, {FieldID: 4, ChildIndex: 1}}, aliases)
				if err != nil {
					return nil, err
				}
				return c.BuildSelectedStore([]SubtreeID{root}, policy, []byte("x "), nil)
			}
			left, err := build(dense, Limits{})
			if err != nil {
				t.Fatal(err)
			}
			defer left.Release()
			right, err := build(program, Limits{})
			if err != nil {
				t.Fatal(err)
			}
			defer right.Release()
			if left.root != right.root || !reflect.DeepEqual(left.records, right.records) || !reflect.DeepEqual(left.children, right.children) || !reflect.DeepEqual(left.payloadSources, right.payloadSources) {
				t.Fatal("selected records, fields, aliases, occurrences, or payload provenance changed")
			}
			if left.RetainedBytes() != right.RetainedBytes() {
				t.Fatal("selected result footprint changed")
			}
			for _, policy := range []SelectedStorePolicy{dense, program} {
				store, err := build(policy, Limits{MaxSelectedBytes: 1})
				if store != nil || err == nil || !strings.Contains(err.Error(), "retained-byte cap") {
					t.Fatalf("byte cap bypassed: store=%v err=%v", store, err)
				}
				store, err = build(policy, Limits{MaxSelectedOccurrences: 1, MaxSelectedBytes: 1 << 20})
				if store != nil || err == nil || !strings.Contains(err.Error(), "occurrence cap") {
					t.Fatalf("occurrence cap bypassed: store=%v err=%v", store, err)
				}
			}
		})
	}
}

func TestSelectedUnaryPolicyNativeReplayParity(t *testing.T) {
	for _, rule := range []SelectedUnaryRule{SelectedUnaryKeep, SelectedUnaryPass, SelectedUnaryRenameLeaf} {
		build := func(program bool) NativeSelectedSnapshot {
			c, root, replay, scratch := nativeSelectedFixture(t)
			table := c.tables.(*nativeSelectedIdentityTable)
			table.policy.Unary[2*len(table.policy.Symbols)+1] = rule
			if rule == SelectedUnaryPass {
				table.policy.ExpectedRoot = 1
			}
			if program {
				table.policy = programPolicyForTest(t, table.policy)
			}
			snapshot, err := c.BuildNativeSelectedSnapshot([]SubtreeID{root}, []byte("x"), replay, scratch, nil)
			if err != nil {
				t.Fatal(err)
			}
			if err = c.Reset(); err != nil {
				t.Fatal(err)
			}
			return snapshot
		}
		dense, program := build(false), build(true)
		if dense.store.root != program.store.root || !reflect.DeepEqual(dense.store.records, program.store.records) || !reflect.DeepEqual(dense.store.children, program.store.children) || !reflect.DeepEqual(dense.store.payloadSources, program.store.payloadSources) || !reflect.DeepEqual(dense.replay, program.replay) {
			t.Fatalf("native syntax or private replay changed for unary rule%d", rule)
		}
		dense.Release()
		program.Release()
	}
}

func TestSelectedUnaryPolicyCopiesShareOnlySealedBacking(t *testing.T) {
	rows := []SelectedUnaryRow{{Self: SelectedUnaryPass}, {Self: SelectedUnaryPass}, {Self: SelectedUnaryPass}, {Self: SelectedUnaryPass}}
	pairs := []SelectedUnaryOverride{{Parent: 2, Child: 1, Rule: SelectedUnaryRenameLeaf}}
	program, err := NewSelectedUnaryProgram(rows, pairs)
	if err != nil {
		t.Fatal(err)
	}
	symbols := make([]SelectedSymbolPolicy, 4)
	for i := range symbols {
		symbols[i] = SelectedSymbolPolicy{Visible: true, Named: true}
	}
	first, err := NewSelectedStoreProgramPolicy(symbols, program, 2)
	if err != nil {
		t.Fatal(err)
	}
	second, err := NewSelectedStoreProgramPolicy(symbols, program, 2)
	if err != nil {
		t.Fatal(err)
	}
	rows[2].Self = SelectedUnaryKeep
	pairs[0].Rule = SelectedUnaryPass
	symbols[1].Visible = false
	if first.unary(2, 1) != SelectedUnaryRenameLeaf || second.unary(2, 1) != SelectedUnaryRenameLeaf || !first.Symbols[1].Visible || !second.Symbols[1].Visible {
		t.Fatal("caller mutation reached sealed program or copied symbols")
	}
	if &first.unaryProgram.rows[0] != &second.unaryProgram.rows[0] || &first.unaryProgram.pairs[0] != &second.unaryProgram.pairs[0] {
		t.Fatal("policy construction recopied a sealed program")
	}
	aliases := []SelectedAliasChildPair{{Alias: 2, Child: 1}}
	masks := []uint32{0, 0, 1, 0}
	containers := []bool{false, false, true, false}
	cases := []bool{false, true, false, false}
	lists := []bool{true, false, false, false}
	for _, policy := range []*SelectedStorePolicy{&first, &second} {
		policy.SetRetainedAliasChildren(aliases)
		if err := policy.SetSupertypeMasks(masks); err != nil {
			t.Fatal(err)
		}
		if err := policy.SetGoCompatibility(1, 3, containers, cases, lists); err != nil {
			t.Fatal(err)
		}
	}
	second.Symbols[1].Visible = false
	second.RetainedAliasChildren[0].Child = 3
	second.SupertypeMasks[2] = 2
	second.SemicolonContainers[2] = false
	second.Cases[1] = false
	second.StatementLists[0] = false
	if !first.Symbols[1].Visible || first.RetainedAliasChildren[0].Child != 1 || first.SupertypeMasks[2] != 1 || !first.SemicolonContainers[2] || !first.Cases[1] || !first.StatementLists[0] {
		t.Fatal("mutable policy fields crossed independently constructed policies")
	}
	c := newTinyCore(t, 8)
	before := c.FootprintBytes()
	c.selectedPolicy = &first
	retained := c.FootprintBytes() - before
	view := *c.selectedPolicy
	view.transientProjection = true
	view.projectionCapacityHint = 17
	if &view.unaryProgram.rows[0] != &first.unaryProgram.rows[0] || c.FootprintBytes()-before != retained || first.transientProjection || first.projectionCapacityHint != 0 {
		t.Fatal("transient policy copy duplicated backing, charged it twice, or changed cached policy")
	}
	expected := uint64(unsafe.Sizeof(first)) + uint64(cap(first.Symbols))*uint64(unsafe.Sizeof(SelectedSymbolPolicy{})) + first.unaryProgram.backingBytes() + uint64(cap(first.RetainedAliasChildren))*uint64(unsafe.Sizeof(SelectedAliasChildPair{})) + uint64(cap(first.SupertypeMasks))*4 + uint64(cap(first.SemicolonContainers)+cap(first.Cases)+cap(first.StatementLists))
	if retained != expected {
		t.Fatalf("shared backing accounting=%d want=%d", retained, expected)
	}
}
