package grammars

import (
	"testing"

	"github.com/odvcencio/gotreesitter"
)

func TestPythonComparisonOperatorFieldStaysOnOperatorToken(t *testing.T) {
	lang := PythonLanguage()
	parser := gotreesitter.NewParser(lang)

	src := []byte("if left != right:\n    pass\n")
	tree, err := parser.Parse(src)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	root := tree.RootNode()
	if root == nil {
		t.Fatal("parse returned nil root")
	}
	if root.HasError() {
		t.Fatalf("expected error-free Python parse tree, got %s", root.SExpr(lang))
	}

	var cmp *gotreesitter.Node
	gotreesitter.Walk(root, func(node *gotreesitter.Node, depth int) gotreesitter.WalkAction {
		if node.IsNamed() && node.Type(lang) == "comparison_operator" {
			cmp = node
			return gotreesitter.WalkStop
		}
		return gotreesitter.WalkContinue
	})
	if cmp == nil {
		t.Fatal("expected to find comparison_operator in Python parse tree")
	}

	operator := cmp.ChildByFieldName("operators", lang)
	if operator == nil {
		t.Fatal("comparison_operator missing operators field")
	}
	if got, want := operator.Text(src), "!="; got != want {
		t.Fatalf("operators field text = %q, want %q", got, want)
	}

	for i := 0; i < cmp.ChildCount(); i++ {
		child := cmp.Child(i)
		if child == nil || child == operator {
			continue
		}
		if got := cmp.FieldNameForChild(i, lang); got == "operators" {
			t.Fatalf("child %d (%s %q) unexpectedly has operators field", i, child.Type(lang), child.Text(src))
		}
	}
}

// Regression test for https://github.com/odvcencio/gotreesitter/issues/53
//
// An f-string in a decorator (e.g. @create_span(f"{__file__}.func")) causes
// the scanner's insideInterpolatedString flag to become stale after checkpoint
// restore, blocking all subsequent DEDENT tokens. This prevents the parser
// from forming try_statement nodes — the try body can never be closed.
//
// The test below reproduces the exact real-world pattern: an f-string
// decorator, a multi-line bracketed call inside a try body, a blank line,
// and then an except clause.
func TestPythonFStringDecoratorTryExceptBlankLine(t *testing.T) {
	src := []byte(`from datetime import datetime

AMPLITUDE_ASSET = "controller"
BRAZE_EVENT = "HH_SUBMITTED"

CONST_A = "a"
CONST_B = "b"
CONST_C = "c"


@create_span(f"{__file__}.report_status")
def report_status(
    *,
    virta_id: str,
    completed_on: datetime | None,
    status: str,
) -> None:
    """Report the status of the health history form."""
    amplitude_logger.log(
        virta_id,
        CONST_A if completed_on else CONST_B,
        {"asset": AMPLITUDE_ASSET},
    )

    if completed_on:
        try:
            BrazeClient(request_timeout=5).update_user(
                external_id=virta_id,
                attributes={
                    "is_labs_required": status
                    == "REQUIRED"
                },
                events=[
                    {
                        "name": BRAZE_EVENT,
                        "time": completed_on,
                    }
                ],
            )

        except Exception as e:
            send_to_sentry(e)
`)

	lang := PythonLanguage()
	parser := gotreesitter.NewParser(lang)
	tree, err := parser.Parse(src)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	root := tree.RootNode()
	if root == nil {
		t.Fatal("parse returned nil root")
	}
	if root.HasError() {
		t.Fatalf("expected error-free parse tree, got %s", root.SExpr(lang))
	}

	// Verify try_statement with except_clause.
	var tryStmt *gotreesitter.Node
	gotreesitter.Walk(root, func(node *gotreesitter.Node, depth int) gotreesitter.WalkAction {
		if node.IsNamed() && node.Type(lang) == "try_statement" {
			tryStmt = node
			return gotreesitter.WalkStop
		}
		return gotreesitter.WalkContinue
	})
	if tryStmt == nil {
		t.Fatalf("expected try_statement in tree, got %s", root.SExpr(lang))
	}
	var exceptClause *gotreesitter.Node
	for i := 0; i < tryStmt.NamedChildCount(); i++ {
		child := tryStmt.NamedChild(i)
		if child != nil && child.Type(lang) == "except_clause" {
			exceptClause = child
		}
	}
	if exceptClause == nil {
		t.Fatalf("expected except_clause in try_statement, got %s", tryStmt.SExpr(lang))
	}
}

func TestPythonScannerSerializationRecomputesInterpolatedStringState(t *testing.T) {
	scanner := PythonExternalScanner{}
	buf := make([]byte, 256)

	formatted := &pythonScannerState{
		indents:                  []uint16{0},
		delimiters:               []pyDelimiter{pyDelimDoubleQuote | pyDelimFormat},
		insideInterpolatedString: false,
	}
	n := scanner.Serialize(formatted, buf)
	var restoredFormatted pythonScannerState
	scanner.Deserialize(&restoredFormatted, buf[:n])
	if !restoredFormatted.insideInterpolatedString {
		t.Fatal("f-string checkpoint restored insideInterpolatedString=false, want true")
	}

	// Reuse the same buffer after a formatted checkpoint. A false flag must
	// overwrite the old byte, or later reuse gates see a stale f-string state.
	plain := &pythonScannerState{
		indents:                  []uint16{0},
		delimiters:               []pyDelimiter{pyDelimDoubleQuote},
		insideInterpolatedString: true,
	}
	n = scanner.Serialize(plain, buf)
	if n == 0 || buf[0] != 0 {
		t.Fatalf("plain checkpoint flag=%d, want 0 after formatted serialization", buf[0])
	}
	var restoredPlain pythonScannerState
	scanner.Deserialize(&restoredPlain, buf[:n])
	if restoredPlain.insideInterpolatedString {
		t.Fatal("plain string checkpoint restored insideInterpolatedString=true, want false")
	}
}

func TestPythonScannerSerializationFailsClosed(t *testing.T) {
	scanner := PythonExternalScanner{}

	state := &pythonScannerState{
		indents:    []uint16{0, 4, 8, 12},
		delimiters: []pyDelimiter{pyDelimSingleQuote | pyDelimFormat},
	}
	full := make([]byte, 256)
	n := scanner.Serialize(state, full)
	if n == 0 {
		t.Fatal("representable scanner state did not serialize")
	}
	if got, want := n, 4+1+3*2; got != want {
		t.Fatalf("serialized checkpoint length=%d, want %d", got, want)
	}

	short := make([]byte, n-1)
	for i := range short {
		short[i] = 0xA5
	}
	if got := scanner.Serialize(state, short); got != 0 {
		t.Fatalf("undersized checkpoint serialized %d bytes, want 0", got)
	}
	for i, b := range short {
		if b != 0xA5 {
			t.Fatalf("undersized checkpoint wrote byte %d before rejecting", i)
		}
	}

	var partial pythonScannerState
	partial.delimiters = []pyDelimiter{pyDelimDoubleQuote}
	partial.indents = []uint16{0, 20}
	scanner.Deserialize(&partial, full[:n-1])
	if len(partial.delimiters) != 0 || len(partial.indents) != 1 || partial.indents[0] != 0 || partial.insideInterpolatedString {
		t.Fatalf("partial checkpoint restored scanner state: %+v", partial)
	}

	malformedFlag := append([]byte(nil), full[:n]...)
	malformedFlag[0] = 0
	var inconsistent pythonScannerState
	scanner.Deserialize(&inconsistent, malformedFlag)
	if len(inconsistent.delimiters) != 0 || len(inconsistent.indents) != 1 || inconsistent.indents[0] != 0 || inconsistent.insideInterpolatedString {
		t.Fatalf("inconsistent checkpoint restored scanner state: %+v", inconsistent)
	}

	malformedTrailing := append(append([]byte(nil), full[:n]...), 0x00)
	var trailing pythonScannerState
	scanner.Deserialize(&trailing, malformedTrailing)
	if len(trailing.delimiters) != 0 || len(trailing.indents) != 1 || trailing.indents[0] != 0 || trailing.insideInterpolatedString {
		t.Fatalf("trailing checkpoint bytes restored scanner state: %+v", trailing)
	}

	// Checkpoint bytes are ephemeral and version-local. Reject the former
	// delimiter-only shape instead of treating it as a complete state.
	legacyShape := []byte{1, 1, byte(pyDelimSingleQuote | pyDelimFormat)}
	var legacy pythonScannerState
	scanner.Deserialize(&legacy, legacyShape)
	if len(legacy.delimiters) != 0 || len(legacy.indents) != 1 || legacy.indents[0] != 0 || legacy.insideInterpolatedString {
		t.Fatalf("legacy checkpoint shape restored scanner state: %+v", legacy)
	}

	tooManyDelimiters := &pythonScannerState{delimiters: make([]pyDelimiter, maxPythonScannerDelimiterCount+1)}
	if got := scanner.Serialize(tooManyDelimiters, make([]byte, 4096)); got != 0 {
		t.Fatalf("unrepresentable delimiter stack serialized %d bytes, want 0", got)
	}

	// A deeply nested indentation stack needs more than the runtime checkpoint
	// buffer. The serializer must reject the complete state rather than publish
	// a prefix that could restore a shallower stack.
	deep := &pythonScannerState{indents: make([]uint16, 3000)}
	if got := scanner.Serialize(deep, make([]byte, 4096)); got != 0 {
		t.Fatalf("deep indentation stack serialized %d bytes, want fail-closed 0", got)
	}
}

func TestPythonNoTreeBenchmarkSkipsExternalScannerCheckpoints(t *testing.T) {
	src := []byte("def f(x):\n    return f\"{x}\"\n")
	lang := PythonLanguage()

	parser := gotreesitter.NewParser(lang)
	// The checkpoint counters are production-parser allocation telemetry. Compact
	// admission validates the same full-tree structure but does not retain live
	// external-scanner checkpoint records, so measure this contract on production.
	parser.SetAdmissionCandidateRoute(false)
	fullTree, err := parser.Parse(src)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	defer fullTree.Release()
	if fullTree.ParseRuntime().ExternalScannerCheckpointRecords == 0 {
		t.Fatalf("full parse checkpoint records = 0, want > 0; runtime=%s", fullTree.ParseRuntime().Summary())
	}

	noTreeParser := gotreesitter.NewParser(lang)
	noTree, err := noTreeParser.ParseNoTreeBenchmarkOnly(src)
	if err != nil {
		t.Fatalf("no-tree parse failed: %v", err)
	}
	defer noTree.Release()
	if noTree.ParseRuntime().ExternalScannerCheckpointRecords != 0 {
		t.Fatalf("no-tree checkpoint records = %d, want 0; runtime=%s", noTree.ParseRuntime().ExternalScannerCheckpointRecords, noTree.ParseRuntime().Summary())
	}
	if noTree.ParseRuntime().ExternalScannerCheckpointBytesAllocated != 0 {
		t.Fatalf("no-tree checkpoint bytes = %d, want 0; runtime=%s", noTree.ParseRuntime().ExternalScannerCheckpointBytesAllocated, noTree.ParseRuntime().Summary())
	}
	if noTree.ParseRuntime().LeafNodesConstructed == 0 {
		t.Fatalf("no-tree leaf nodes = 0, want > 0 for small corpus; runtime=%s", noTree.ParseRuntime().Summary())
	}
	if noTree.ParseRuntime().NoTreeLeafNodesConstructed != 0 {
		t.Fatalf("no-tree compact leaf nodes = %d, want 0 for small corpus; runtime=%s", noTree.ParseRuntime().NoTreeLeafNodesConstructed, noTree.ParseRuntime().Summary())
	}
}

func TestPythonNoTreeBenchmarkCanKeepExternalScannerCheckpoints(t *testing.T) {
	src := []byte("def f(x):\n    return f\"{x}\"\n")
	parser := gotreesitter.NewParser(PythonLanguage())

	tree, err := parser.ParseNoTreeWithExternalCheckpointsBenchmarkOnly(src)
	if err != nil {
		t.Fatalf("no-tree checkpoint parse failed: %v", err)
	}
	defer tree.Release()
	rt := tree.ParseRuntime()
	if rt.NoTreeReduceNodesConstructed == 0 {
		t.Fatalf("no-tree reduce nodes = 0, want > 0; runtime=%s", rt.Summary())
	}
	if rt.LeafNodesConstructed == 0 {
		t.Fatalf("checkpoint leaf nodes = 0, want > 0; runtime=%s", rt.Summary())
	}
	if rt.NoTreeLeafNodesConstructed != 0 {
		t.Fatalf("checkpoint compact leaf nodes = %d, want 0; runtime=%s", rt.NoTreeLeafNodesConstructed, rt.Summary())
	}
	if rt.ExternalScannerCheckpointRecords == 0 {
		t.Fatalf("checkpoint records = 0, want > 0; runtime=%s", rt.Summary())
	}
	if rt.ExternalScannerCheckpointBytesAllocated == 0 {
		t.Fatalf("checkpoint bytes = 0, want > 0; runtime=%s", rt.Summary())
	}
}

func TestPythonUnderscoreAssignmentDoesNotTerminateModule(t *testing.T) {
	src := []byte(`import os
import sys

_ = os
_ = sys


def main():
    import boto3

    _ = boto3
`)

	lang := PythonLanguage()
	parser := gotreesitter.NewParser(lang)
	tree, err := parser.Parse(src)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	root := tree.RootNode()
	if root == nil {
		t.Fatal("parse returned nil root")
	}
	if got, want := root.EndByte(), uint32(len(src)); got != want {
		t.Fatalf("root end byte = %d, want %d: %s", got, want, root.SExpr(lang))
	}
	if tree.ParseRuntime().Truncated {
		t.Fatalf("parse runtime reports truncation: %s", tree.ParseRuntime().Summary())
	}
	if root.HasError() {
		t.Fatalf("expected error-free Python parse tree, got %s", root.SExpr(lang))
	}

	var foundFunction, foundBoto3 bool
	gotreesitter.Walk(root, func(node *gotreesitter.Node, depth int) gotreesitter.WalkAction {
		if node.IsNamed() && node.Type(lang) == "function_definition" {
			foundFunction = true
		}
		if node.IsNamed() && node.Type(lang) == "dotted_name" && node.Text(src) == "boto3" {
			foundBoto3 = true
		}
		if foundFunction && foundBoto3 {
			return gotreesitter.WalkStop
		}
		return gotreesitter.WalkContinue
	})
	if !foundFunction {
		t.Fatalf("expected function_definition in tree, got %s", root.SExpr(lang))
	}
	if !foundBoto3 {
		t.Fatalf("expected nested boto3 import in tree, got %s", root.SExpr(lang))
	}
}

func TestParseFilePythonNestedMethodDedentsReturnToModule(t *testing.T) {
	src := []byte("import unittest\n\nclass GrammarTests(unittest.TestCase):\n    def test_case(self):\n        keywords = (1,)\n        cases = (2,)\n        for keyword in (1,):\n            for case in (2,):\n                pass\n\nif __name__ == '__main__':\n    unittest.main()\n")

	bt, err := ParseFile("script.py", src)
	if err != nil {
		t.Fatalf("ParseFile error: %v", err)
	}
	defer bt.Release()

	lang := PythonLanguage()
	root := bt.RootNode()
	if root == nil {
		t.Fatal("ParseFile returned nil root for Python nested-loop source")
	}
	if root.HasError() {
		t.Fatalf("expected error-free Python parse tree, got %s", root.SExpr(lang))
	}

	if got, want := root.NamedChildCount(), 3; got != want {
		t.Fatalf("root named child count = %d, want %d: %s", got, want, root.SExpr(lang))
	}
	if got, want := root.NamedChild(0).Type(lang), "import_statement"; got != want {
		t.Fatalf("root named child 0 type = %q, want %q", got, want)
	}
	if got, want := root.NamedChild(1).Type(lang), "class_definition"; got != want {
		t.Fatalf("root named child 1 type = %q, want %q", got, want)
	}
	if got, want := root.NamedChild(2).Type(lang), "if_statement"; got != want {
		t.Fatalf("root named child 2 type = %q, want %q", got, want)
	}
}
