package gotreesitter_test

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

func TestCSharpDesignerStyleBlockStaysBounded(t *testing.T) {
	if raceEnabled {
		// The contract here is wall-clock boundedness under a real 500ms
		// budget; the race detector's ~10x instrumentation slowdown makes the
		// forest attempt consume the budget before production can accept, so
		// the assertion measures the detector, not the parser. Non-race runs
		// keep the boundedness contract enforced.
		t.Skip("wall-clock budget contract is not meaningful under -race instrumentation")
	}
	src := csharpSyntheticDesignerSource(300)
	parser := gotreesitter.NewParser(grammars.CSharpLanguage())
	parser.SetTimeoutMicros(500_000)

	tree, err := parser.Parse(src)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	defer tree.Release()

	rt := tree.ParseRuntime()
	if got, want := rt.StopReason, gotreesitter.ParseStopAccepted; got != want {
		t.Fatalf("StopReason = %s, want %s (%s)", got, want, rt.Summary())
	}
	if tree.ParseStoppedEarly() {
		t.Fatalf("ParseStoppedEarly = true (%s)", rt.Summary())
	}
	if got, want := tree.RootNode().EndByte(), uint32(len(src)); got != want {
		t.Fatalf("root end = %d, want %d (%s)", got, want, rt.Summary())
	}
	if tree.RootNode().HasError() {
		t.Fatalf("root has error (%s)", rt.Summary())
	}
	if rt.MaxStacksSeen > 4 {
		t.Fatalf("MaxStacksSeen = %d, want <= 4 (%s)", rt.MaxStacksSeen, rt.Summary())
	}
}

func TestCSharpCJKPartialClassesStayBounded(t *testing.T) {
	if raceEnabled {
		// Same rationale as TestCSharpDesignerStyleBlockStaysBounded above:
		// the 500ms wall-clock budget measures the race detector's ~10x
		// instrumentation slowdown (and CI runner contention), not the
		// parser's boundedness. Non-race runs keep the contract enforced.
		t.Skip("wall-clock budget contract is not meaningful under -race instrumentation")
	}
	src := csharpSyntheticCJKNamespaceSource(120)
	parser := gotreesitter.NewParser(grammars.CSharpLanguage())
	parser.SetTimeoutMicros(500_000)

	tree, err := parser.Parse(src)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	defer tree.Release()

	rt := tree.ParseRuntime()
	if got, want := rt.StopReason, gotreesitter.ParseStopAccepted; got != want {
		t.Fatalf("StopReason = %s, want %s (%s)", got, want, rt.Summary())
	}
	if tree.ParseStoppedEarly() {
		t.Fatalf("ParseStoppedEarly = true (%s)", rt.Summary())
	}
	if got, want := tree.RootNode().EndByte(), uint32(len(src)); got != want {
		t.Fatalf("root end = %d, want %d (%s)", got, want, rt.Summary())
	}
	if tree.RootNode().HasError() {
		t.Fatalf("root has error (%s)", rt.Summary())
	}
}

func TestCSharpNamespaceRecoveryStaysBoundedDottedBitorArg(t *testing.T) {
	src := []byte(`namespace N { class C { void M(string n) { F(n, E.A | E.B); } } }`)
	parser := gotreesitter.NewParser(grammars.CSharpLanguage())
	// The two tests above skip entirely under -race because their wall-clock
	// budget measures the detector rather than the parser. This witness is 65
	// bytes, so it keeps every assertion under -race and scales only the
	// budget. Measured on this witness: the whole parse costs about 0.2ms
	// without the detector and about 3ms with it, against a 500ms budget. The
	// remaining risk is not parser cost but co-tenancy: the root race shards
	// run every test in one process, a shard's grammar heap raises
	// garbage-collection pauses, and the partition assigns shards by test
	// position, so any added test name can reseat this one beside heavier
	// neighbours. A 30s budget under -race keeps the runaway contract while
	// leaving a margin no scheduling pause can close.
	//
	// Measured directly (go test -race -count=1 -run
	// '^TestCSharpNamespaceRecoveryStaysBoundedDottedBitorArg$' .), three runs
	// on the host: 0.68s, 0.68s, 0.69s reported test time (1.699s, 1.703s,
	// 1.708s package wall time, which also pays the one-time race-build and
	// grammar-table cost). That is a greater than 40x margin below the 30s
	// budget, so the budget stays unchanged. This test also now runs in its
	// own isolated root-race lane
	// (.github/scripts/root_race_isolated_targets.txt), its own process, so
	// the co-tenancy risk above no longer applies to it; the budget still
	// keeps the margin in case a later change moves it back into a shard.
	budgetMicros := uint64(500_000)
	if raceEnabled {
		budgetMicros = 30_000_000
	}
	parser.SetTimeoutMicros(budgetMicros)

	tree, err := parser.Parse(src)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	defer tree.Release()

	rt := tree.ParseRuntime()
	if got, want := rt.StopReason, gotreesitter.ParseStopAccepted; got != want {
		t.Fatalf("StopReason = %s, want %s (%s)", got, want, rt.Summary())
	}
	if tree.ParseStoppedEarly() {
		t.Fatalf("ParseStoppedEarly = true (%s)", rt.Summary())
	}
	if got, want := tree.RootNode().EndByte(), uint32(len(src)); got != want {
		t.Fatalf("root end = %d, want %d (%s)", got, want, rt.Summary())
	}
}

func csharpSyntheticDesignerSource(fields int) []byte {
	var b strings.Builder
	b.WriteString("namespace Station.UI {\n")
	b.WriteString("partial class FormHome {\n")
	b.WriteString("private void InitializeComponent() {\n")
	for i := 0; i < fields; i++ {
		fmt.Fprintf(&b, "this.button%d = new System.Windows.Forms.Button();\n", i)
		fmt.Fprintf(&b, "this.button%d.Name = \"button%d\";\n", i, i)
		fmt.Fprintf(&b, "this.button%d.Text = \"按钮%d\";\n", i, i)
		fmt.Fprintf(&b, "this.button%d.Click += new System.EventHandler(this.button%d_Click);\n", i, i)
	}
	b.WriteString("}\n")
	for i := 0; i < fields; i++ {
		fmt.Fprintf(&b, "private System.Windows.Forms.Button button%d;\n", i)
	}
	b.WriteString("}\n}\n")
	return []byte(b.String())
}

func csharpSyntheticCJKNamespaceSource(types int) []byte {
	var b bytes.Buffer
	b.WriteString("namespace Station.逻辑 {\n")
	for i := 0; i < types; i++ {
		fmt.Fprintf(&b, "/// <summary>partial class Logic贴合%d handles ModbusTcp 数据点</summary>\n", i)
		fmt.Fprintf(&b, "public partial class Logic贴合%d {\n", i)
		fmt.Fprintf(&b, "  public int 计数%d { get; set; }\n", i)
		fmt.Fprintf(&b, "  public void Step%d() { var 值 = 读取%d(); if (值 > 0) { 计数%d += 值; } }\n", i, i, i)
		fmt.Fprintf(&b, "  private int 读取%d() => %d;\n", i, i)
		b.WriteString("}\n")
	}
	b.WriteString("}\n")
	return b.Bytes()
}
