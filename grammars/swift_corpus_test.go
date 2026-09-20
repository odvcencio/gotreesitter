package grammars

import (
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/odvcencio/gotreesitter"
)

// swiftCorpusStatus records the expected parse outcome for one corpus file.
type swiftCorpusStatus int

const (
	// swiftCorpusClean means the file must parse with HasError() == false.
	// A file in this state that starts reporting an error is a regression.
	swiftCorpusClean swiftCorpusStatus = iota
	// swiftCorpusKnownFailing means the file currently reports a parse
	// error from a known, tracked cause. A file in this state that starts
	// parsing cleanly means the underlying bug got fixed — the test forces
	// a ratchet: update the row to swiftCorpusClean.
	swiftCorpusKnownFailing
)

// swiftCorpusExpectation is one row of the corpus ratchet table.
type swiftCorpusExpectation struct {
	status swiftCorpusStatus
	// issue names the open GitHub issue(s) whose construct explains the
	// failure, for example "#558". Empty for clean rows. For a failure
	// that matches no open issue, this holds a "new:" prefixed note
	// describing the construct instead of an issue number.
	issue string
}

// swiftCorpusExpectations is the ratchet table for
// grammars/testdata/swift_corpus. Every *.swift file in that directory must
// have exactly one row here; TestSwiftCorpusManifestComplete enforces that.
//
// Populated by running gotreesitter's Swift grammar against each corpus file
// at the branch point (main @ 96671b92, before PRs #566-#571 land) and, for
// every failure, locating the first ERROR/MISSING node and matching its
// construct against the open issue that describes it.
//
// Ratcheted forward after #566, #567, #568, #569, #570, and #571 merged:
// swift-algorithms_AdjacentPairsTests.swift and swift-algorithms_Windows.swift
// now parse cleanly. swift-algorithms_Chunked.swift,
// swift-algorithms_FlattenCollection.swift, and swift-algorithms_Stride.swift
// still fail, but not for their original #558/#560 causes; each carries a
// residual, uncovered variant of the same trailing-closure-ambiguity family.
// See each row's comment for the isolated minimal repro.
var swiftCorpusExpectations = map[string]swiftCorpusExpectation{
	"stdlib_ASCII.swift": {
		status: swiftCorpusKnownFailing,
		// Tracks #574. Minimal repro: `let x = 1&<<7`. The
		// masking left-shift operator `&<<` (and, presumably, the rest of
		// the overflow-operator family `&+`, `&-`, `&*`, `&>>`) breaks the
		// surrounding infix_expression, leaving an ERROR node in place of
		// the right operand. See stdlib_ASCII.swift's `encode(_:)`:
		// `guard source.value < (1&<<7) else { return nil }`.
		issue: "#574",
	},
	"stdlib_Collection.swift": {
		status: swiftCorpusKnownFailing,
		// Tracks #575. Minimal repro:
		//   protocol P {
		//     associatedtype Iterator
		//     override __consuming func makeIterator() -> Iterator
		//   }
		// The `override` modifier combined with the `__consuming`
		// parameter-passing modifier on a protocol function requirement
		// produces an ERROR node covering `__consuming`. See
		// stdlib_Collection.swift's Sequence protocol requirement
		// `override __consuming func makeIterator() -> Iterator`.
		issue: "#575",
	},
	"stdlib_CollectionAlgorithms.swift": {
		status: swiftCorpusKnownFailing,
		// Same root cause as stdlib_FloatingPointToString.swift: the
		// focused minimal witness now matches locked C, but this full file
		// still reports an error.
		// `unsafe` expression-prefix keyword. Verified by bisection: the
		// file parses clean through the `_halfStablePartition` function
		// (byte 13232) and only starts failing once `partition(by:)`
		// appears, which contains `try unsafe
		// withContiguousMutableStorageIfAvailable { ... }`.
		issue: "#576",
	},
	"stdlib_FloatingPointToString.swift": {
		status: swiftCorpusKnownFailing,
		// Tracks #576. Minimal repro: `let x = unsafe bar()`.
		// The focused 20-byte witness now matches locked C through generic
		// recovery. This corpus file still reports an error.
		// The `unsafe` keyword used as an expression prefix (Swift's
		// strict-memory-safety opt-in, used throughout this file as
		// `unsafe buffer.storeBytes(...)`, `let x = unsafe
		// utf8Buffer.mutableBytes`, etc.) is not recognized; it leaves an
		// ERROR node in place of the wrapped expression.
		issue: "#576",
	},
	"stdlib_Optional.swift": {
		status: swiftCorpusKnownFailing,
		// Tracks #577. Minimal repro:
		//   struct S {
		//     @lifetime(copy value)
		//     public init(_ value: Int) {}
		//   }
		// The `@lifetime(copy value)` / `@lifetime(borrow x)` lifetime-
		// dependency attribute argument is misparsed: HasError() is true
		// with NO ERROR node anywhere in the tree, only a zero-width
		// MISSING `integer_literal` inside the attribute's argument list
		// (same "phantom failure" shape #559 documents for a different
		// construct). See stdlib_Optional.swift's noncopyable-Optional
		// initializer `@lifetime(copy value) public init(_ value:
		// consuming Wrapped)`.
		issue: "#577",
	},
	"stdlib_Repeat.swift": {
		status: swiftCorpusClean,
	},
	"stdlib_Stride.swift": {
		status: swiftCorpusKnownFailing,
		// Tracks #578. Minimal repro:
		//   protocol P {
		//     associatedtype Stride: SignedNumeric, Comparable
		//   }
		// An associated type with more than one comma-separated
		// conformance constraint produces an ERROR node spanning the
		// first constraint and the comma. See stdlib_Stride.swift's
		// `associatedtype Stride: SignedNumeric, Comparable` in the
		// `Strideable` protocol.
		issue: "#578",
	},
	"swift-algorithms_AdjacentPairsTests.swift": {
		status: swiftCorpusClean,
		// Ratcheted after #559 and #561 merged: the triple-nested generic
		// `IndexValidator<AdjacentPairsCollection<Range<Int>>>()` on line 73
		// (issue #559, fixed by #567) and the `for n in 4...100 { }` loops
		// on lines 37 and 66 each followed by another test method inside
		// the class body (issue #561, fixed by #570) both now parse clean.
	},
	"swift-algorithms_Chunked.swift": {
		status: swiftCorpusClean,
		// Ratcheted after grammargen's normalize.go regained the -10000
		// immediate bonus for named string-bodied token.immediate()
		// terminals (commit d78ffcdf2): the sibling-statement-after-a-
		// nested-if-chain shape this row tracked was a downstream symptom
		// of the anonymous `?` literal winning same-span lexical ties over
		// `_immediate_quest`, not a grammar-shape gap. Fresh table builds
		// now resolve that tie correctly and the file parses clean.
	},
	"swift-algorithms_FlattenCollection.swift": {
		status: swiftCorpusClean,
		// Ratcheted after grammargen's normalize.go regained the -10000
		// immediate bonus for named string-bodied token.immediate()
		// terminals (commit d78ffcdf2); see swift-algorithms_Chunked.swift.
	},
	"swift-algorithms_Stride.swift": {
		status: swiftCorpusClean,
		// Ratcheted after grammargen's normalize.go regained the -10000
		// immediate bonus for named string-bodied token.immediate()
		// terminals (commit d78ffcdf2); see swift-algorithms_Chunked.swift.
	},
	"swift-algorithms_Windows.swift": {
		status: swiftCorpusClean,
		// Ratcheted after #560 (fixed by #569): `index(before:)`'s
		// `if index == endIndex { return Index(...) } else { return
		// Index(...) }`, the verbatim repro #560 quoted for this file, now
		// parses clean.
	},
}

// TestSwiftCorpusManifestComplete ensures every *.swift file under
// testdata/swift_corpus has exactly one row in swiftCorpusExpectations, and
// that no stale rows point at files that no longer exist.
func TestSwiftCorpusManifestComplete(t *testing.T) {
	dir := "testdata/swift_corpus"
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("reading %s: %v", dir, err)
	}

	var onDisk []string
	for _, e := range entries {
		if filepath.Ext(e.Name()) == ".swift" {
			onDisk = append(onDisk, e.Name())
		}
	}
	sort.Strings(onDisk)

	var expected []string
	for name := range swiftCorpusExpectations {
		expected = append(expected, name)
	}
	sort.Strings(expected)

	onDiskSet := make(map[string]bool, len(onDisk))
	for _, name := range onDisk {
		onDiskSet[name] = true
	}
	for _, name := range expected {
		if !onDiskSet[name] {
			t.Errorf("swiftCorpusExpectations has a row for %q, but that file does not exist under %s", name, dir)
		}
	}

	expectedSet := make(map[string]bool, len(expected))
	for _, name := range expected {
		expectedSet[name] = true
	}
	for _, name := range onDisk {
		if !expectedSet[name] {
			t.Errorf("%s has no row in swiftCorpusExpectations; add one (clean or known_failing)", name)
		}
	}
}

// TestSwiftCorpus parses every file in grammars/testdata/swift_corpus and
// checks the outcome against swiftCorpusExpectations. A "clean" file that
// starts erroring is a regression. A "known_failing" file that starts
// parsing cleanly means the underlying bug got fixed, so the test fails on
// purpose to force the expectations table to ratchet forward.
func TestSwiftCorpus(t *testing.T) {
	dir := "testdata/swift_corpus"
	lang := SwiftLanguage()

	for name, exp := range swiftCorpusExpectations {
		name, exp := name, exp
		t.Run(name, func(t *testing.T) {
			if raceEnabled && exp.status == swiftCorpusKnownFailing {
				t.Skip("known-failing file drives full error recovery; its cost under the race detector exceeds the CI shard budget; the ratchet runs in non-race lanes")
			}
			src, err := os.ReadFile(filepath.Join(dir, name))
			if err != nil {
				t.Fatalf("reading corpus file: %v", err)
			}

			parser := gotreesitter.NewParser(lang)
			tree, err := parser.Parse(src)
			if err != nil {
				t.Fatalf("parsing corpus file: %v", err)
			}
			defer tree.Release()

			root := tree.RootNode()
			hasErr := root.HasError()

			switch exp.status {
			case swiftCorpusClean:
				if hasErr {
					t.Fatalf("regression: %s now reports a parse error but the expectations table marks it clean; "+
						"first bad node: %s", name, describeFirstSwiftCorpusFailure(root, lang))
				}
			case swiftCorpusKnownFailing:
				if !hasErr {
					t.Fatalf("ratchet needed: %s now parses cleanly, but the expectations table marks it "+
						"known_failing (%s); update its row in swiftCorpusExpectations to swiftCorpusClean", name, exp.issue)
				}
			default:
				t.Fatalf("%s has an unrecognized status", name)
			}
		})
	}
}

// describeFirstSwiftCorpusFailure narrows down from the root to the
// smallest descendant whose subtree still reports an error, then returns a
// short description of that node for failure messages.
func describeFirstSwiftCorpusFailure(root *gotreesitter.Node, lang *gotreesitter.Language) string {
	n := root
	for {
		if n.IsError() || n.IsMissing() {
			break
		}
		var next *gotreesitter.Node
		for i := 0; i < n.ChildCount(); i++ {
			c := n.Child(i)
			if c != nil && c.HasError() {
				next = c
				break
			}
		}
		if next == nil {
			break
		}
		n = next
	}
	return n.Type(lang)
}
