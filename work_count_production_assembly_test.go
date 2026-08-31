//go:build !gts_workcount

package gotreesitter

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"
)

const (
	finalizeDeferGuardMarker               = "work-count-assembly: finalize-defer guard"
	popPayloadCensusMarker                 = "work-count-assembly: payload-census seam"
	convergenceIterationMarker             = "work-count-assembly: convergence iteration seam"
	resolvedActionCellMarker               = "work-count-assembly: resolved action-cell seam"
	unionFrontierElectionMarker            = "work-count-assembly: union-frontier election seam"
	rawMainLexerInvocationMarker           = "work-count-assembly: raw main-lexer invocation seam"
	convergenceFinalExpandMarker           = "work-count-assembly: convergence final-expand seam"
	convergenceGSSMarker                   = "work-count-assembly: convergence GSS seam"
	gssMutationSetPrimaryMarker            = "work-count-assembly: GSS mutation set-primary seam"
	gssMutationSetExtraMarker              = "work-count-assembly: GSS mutation set-extra seam"
	gssAlternateAppendReuseMarker          = "work-count-assembly: alternate predecessor append-reuse seam"
	gssAlternateAppendGrowMarker           = "work-count-assembly: alternate predecessor append-grow seam"
	gssMutationAppendReuseMarker           = "work-count-assembly: GSS mutation append-reuse seam"
	gssMutationAppendGrowMarker            = "work-count-assembly: GSS mutation append-grow seam"
	topologyActionResultMarker             = "work-count-assembly: topology action-result seam"
	topologyInitialVersionMarker           = "work-count-assembly: topology initial-version seam"
	topologyConflictCopyMarker             = "work-count-assembly: topology conflict-copy seam"
	topologyFrontierShiftCopyMarker        = "work-count-assembly: topology frontier-shift-copy seam"
	topologyChildElectionMarker            = "work-count-assembly: topology child-election seam"
	topologyPopPathMarker                  = "work-count-assembly: topology pop-path seam"
	topologyReduceCopyMarker               = "work-count-assembly: topology reduce-copy seam"
	topologyNodeAllocationMarker           = "work-count-assembly: topology primary-link seam"
	semanticPhaseActionCellMarker          = "semantic-phase-assembly: action-cell seam"
	semanticPhaseActionExecutionMarker     = "semantic-phase-assembly: action-execution seam"
	semanticPhaseExtraShiftExecutionMarker = "semantic-phase-assembly: extra-shift-execution seam"
	semanticPhaseEOFActionCellMarker       = "semantic-phase-assembly: EOF-prefix action-cell seam"
	semanticPhaseEOFActionExecutionMarker  = "semantic-phase-assembly: EOF-prefix action-execution seam"
)

func TestWorkCountProductionAssemblyHasNoDiagnosticScaffolding(t *testing.T) {
	testBinary := buildProductionTestBinary(t)

	nm := runGoTool(t, "nm", testBinary)
	productionWorkCountSymbol := regexp.MustCompile(`(?m)^\s*[0-9a-f]+\s+\S\s+github\.com/odvcencio/gotreesitter\.(?:\(\*[^)]*\)\.)?workCount`)
	if match := productionWorkCountSymbol.Find(nm); match != nil {
		t.Fatalf("untagged binary retains a production work-count symbol: %s", match)
	}
	if bytes.Contains(nm, []byte("github.com/odvcencio/gotreesitter.semanticPhaseTrace")) {
		t.Fatal("untagged binary retains semantic-phase trace symbols")
	}
	if bytes.Contains(nm, []byte("github.com/odvcencio/gotreesitter.DiagnosticTopology")) {
		t.Fatal("untagged binary retains diagnostic topology symbols")
	}
	for _, forbidden := range []string{
		"gssMainCanMergeForParserPhase",
		"gssMainCanMergeWithScratchPhase",
		"tryGSSMainMergeForParserPhase",
		"postReduceForkMergePreflight",
	} {
		if bytes.Contains(nm, []byte("github.com/odvcencio/gotreesitter."+forbidden)) {
			t.Fatalf("untagged binary retains diagnostic merge helper %s", forbidden)
		}
	}

	finalizeLine := sourceMarkerLine(t, "parser.go", finalizeDeferGuardMarker) - 1
	closures := runGoTool(t, "objdump", "-s", `github.com/odvcencio/gotreesitter\.\(\*Parser\)\.parseInternal\.func`, testBinary)
	finalizeAssembly := uniqueAssemblySectionForLine(t, closures, "parser.go", finalizeLine)
	if bytes.Contains(finalizeAssembly, []byte("runtime.deferreturn")) {
		t.Fatalf("untagged finalizeTree retains defer scaffolding:\n%s", finalizeAssembly)
	}
	assertNoDiagnosticAssembly(t, finalizeAssembly)

	popCensusLine := sourceMarkerLine(t, "parser_reduce.go", popPayloadCensusMarker)
	reduceAssembly := runGoTool(t, "objdump", "-s", `github.com/odvcencio/gotreesitter\.\(\*Parser\)\.selectedReduceWindowsFromGSSWithBudget`, testBinary)
	if hasAssemblyForLine(reduceAssembly, "parser_reduce.go", popCensusLine) {
		t.Fatalf("untagged reduction path retains instructions at the payload-census seam:\n%s", reduceAssembly)
	}
	assertNoDiagnosticAssembly(t, reduceAssembly)

	assertNoAssemblyAtMarker(t, closures, "parser.go", convergenceIterationMarker)
	parseInternalAssembly := runGoTool(t, "objdump", "-s", `github.com/odvcencio/gotreesitter\.\(\*Parser\)\.parseInternal$`, testBinary)
	assertNoAssemblyAtMarker(t, parseInternalAssembly, "parser.go", resolvedActionCellMarker)
	assertNoAssemblyAtMarker(t, parseInternalAssembly, "parser.go", topologyInitialVersionMarker)
	assertNoAssemblyAtMarker(t, parseInternalAssembly, "parser.go", topologyConflictCopyMarker)
	assertNoDiagnosticAssembly(t, parseInternalAssembly)
	lexerScanAssembly := runGoTool(t, "objdump", "-s", `github.com/odvcencio/gotreesitter\.\(\*Lexer\)\.scan$`, testBinary)
	assertNoAssemblyAtMarker(t, lexerScanAssembly, "lexer.go", rawMainLexerInvocationMarker)
	assertNoDiagnosticAssembly(t, lexerScanAssembly)
	recoverAcquireAssembly := runGoTool(t, "objdump", "-s", `github.com/odvcencio/gotreesitter\.\(\*Parser\)\.cRecoverAcquireToken$`, testBinary)
	assertNoAssemblyAtMarker(t, recoverAcquireAssembly, "parser_recover_c.go", unionFrontierElectionMarker)
	assertNoDiagnosticAssembly(t, recoverAcquireAssembly)
	assertNoAssemblyAtMarker(t, closures, "parser.go", semanticPhaseActionCellMarker)
	assertNoAssemblyAtMarker(t, closures, "parser.go", semanticPhaseExtraShiftExecutionMarker)
	eofAdvanceAssembly := runGoTool(t, "objdump", "-s", `github.com/odvcencio/gotreesitter\.\(\*Parser\)\.tryAdvanceEOFOnSingleStack`, testBinary)
	assertNoAssemblyAtMarker(t, eofAdvanceAssembly, "parser.go", semanticPhaseEOFActionCellMarker)
	assertNoAssemblyAtMarker(t, eofAdvanceAssembly, "parser.go", semanticPhaseEOFActionExecutionMarker)
	assertNoDiagnosticAssembly(t, eofAdvanceAssembly)
	noteStopAssembly := runGoTool(t, "objdump", "-s", `github.com/odvcencio/gotreesitter\.\(\*Parser\)\.noteStopAction(?:Diagnostic|Result)`, testBinary)
	assertNoAssemblyAtMarker(t, noteStopAssembly, "parser.go", semanticPhaseActionExecutionMarker)
	assertNoAssemblyAtMarker(t, noteStopAssembly, "parser.go", topologyActionResultMarker)
	assertNoDiagnosticAssembly(t, noteStopAssembly)
	resultAssembly := runGoTool(t, "objdump", "-s", `github.com/odvcencio/gotreesitter\.\(\*Parser\)\.buildResultFromGLR`, testBinary)
	assertNoAssemblyAtMarker(t, resultAssembly, "parser_result.go", convergenceFinalExpandMarker)
	assertNoDiagnosticAssembly(t, resultAssembly)
	gssAssembly := runGoTool(t, "objdump", "-s", `github.com/odvcencio/gotreesitter\.tryGSSMainMergeForParser`, testBinary)
	assertNoAssemblyAtMarker(t, gssAssembly, "glr.go", convergenceGSSMarker)
	assertNoDiagnosticAssembly(t, gssAssembly)
	gssMutationAssembly := runGoTool(t, "objdump", "-s", `github.com/odvcencio/gotreesitter\.(?:setGSSMainLink|gssMainAddLinkSeenMutate|gssMainReplaceWorstEquivalentLinkIfBetterMutate|gssMainMergeNodesSeenMutate|gssMainMergeWithScratch|tryGSSMainMergeResult|\(\*gssNode\)\.appendExtraLink)`, testBinary)
	assertNoAssemblyAtMarker(t, gssMutationAssembly, "glr.go", gssMutationSetPrimaryMarker)
	assertNoAssemblyAtMarker(t, gssMutationAssembly, "glr.go", gssMutationSetExtraMarker)
	assertNoAssemblyAtMarker(t, gssMutationAssembly, "glr_gss.go", gssAlternateAppendReuseMarker)
	assertNoAssemblyAtMarker(t, gssMutationAssembly, "glr_gss.go", gssAlternateAppendGrowMarker)
	assertNoAssemblyAtMarker(t, gssMutationAssembly, "glr_gss.go", gssMutationAppendReuseMarker)
	assertNoAssemblyAtMarker(t, gssMutationAssembly, "glr_gss.go", gssMutationAppendGrowMarker)
	assertNoDiagnosticAssembly(t, gssMutationAssembly)
	postReduceAssembly := runGoTool(t, "objdump", "-s", `github.com/odvcencio/gotreesitter\.(?:tryMergePostReduceFork|postReduceForkMergePreflight|\(\*Parser\)\.(?:applyReduceActionForked|applyReduceActionFromGSS))`, testBinary)
	assertNoAssemblyAtMarker(t, postReduceAssembly, "parser_reduce.go", topologyReduceCopyMarker)
	assertNoDiagnosticAssembly(t, postReduceAssembly)
	frontierAssembly := runGoTool(t, "objdump", "-s", `github.com/odvcencio/gotreesitter\.\(\*Parser\)\.completeConflictReduceFrontier`, testBinary)
	assertNoAssemblyAtMarker(t, frontierAssembly, "parser_reduce.go", topologyFrontierShiftCopyMarker)
	assertNoDiagnosticAssembly(t, frontierAssembly)
	assertNoAssemblyAtMarker(t, reduceAssembly, "parser_reduce.go", topologyChildElectionMarker)
	assertNoAssemblyAtMarker(t, reduceAssembly, "parser_reduce.go", topologyPopPathMarker)
	pushEntryAssembly := runGoTool(t, "objdump", "-s", `github.com/odvcencio/gotreesitter\.\(\*gssStack\)\.pushEntry`, testBinary)
	assertNoAssemblyAtMarker(t, pushEntryAssembly, "glr_gss.go", topologyNodeAllocationMarker)
	assertNoDiagnosticAssembly(t, pushEntryAssembly)
	allSymbols := runGoTool(t, "nm", testBinary)
	if bytes.Contains(allSymbols, []byte("uniqueActionOrdinal")) {
		t.Fatalf("untagged binary retains action-ordinal reconstruction symbol:\n%s", allSymbols)
	}
}

func assertNoAssemblyAtMarker(t *testing.T, assembly []byte, file, marker string) {
	t.Helper()
	line := sourceMarkerLine(t, file, marker)
	if hasAssemblyForLine(assembly, file, line) {
		t.Fatalf("untagged build retains instructions at %s:%d (%s):\n%s", file, line, marker, assembly)
	}
}

func assertNoDiagnosticAssembly(t *testing.T, assembly []byte) {
	t.Helper()
	for _, forbidden := range [][]byte{
		[]byte("workCount"),
		[]byte("workCountConvergence"),
		[]byte("work_count_convergence.go:"),
		[]byte("work_count_hooks.go:"),
		[]byte("semanticPhaseTrace"),
		[]byte("work_count_semantic_phase_trace.go:"),
		[]byte("DiagnosticTopology"),
		[]byte("work_count_topology.go:"),
	} {
		if bytes.Contains(assembly, forbidden) {
			t.Fatalf("untagged assembly retains diagnostic code %q:\n%s", forbidden, assembly)
		}
	}
}

func sourceMarkerLine(t *testing.T, name, marker string) int {
	t.Helper()
	source, err := os.ReadFile(filepath.Join(".", name))
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	lines := bytes.Split(source, []byte("\n"))
	if got := bytes.Count(source, []byte(marker)); got != 1 {
		t.Fatalf("want one %q marker in %s, got %d", marker, name, got)
	}
	for index, line := range lines {
		if bytes.Contains(line, []byte(marker)) {
			return index + 1
		}
	}
	t.Fatalf("marker %q not found in %s", marker, name)
	return 0
}

func uniqueAssemblySectionForLine(t *testing.T, assembly []byte, file string, line int) []byte {
	t.Helper()
	var matches [][]byte
	for _, section := range splitAssemblySections(assembly) {
		if hasAssemblyForLine(section, file, line) {
			matches = append(matches, section)
		}
	}
	if len(matches) != 1 {
		t.Fatalf("want one assembly section for %s:%d, got %d", file, line, len(matches))
	}
	return matches[0]
}

func splitAssemblySections(assembly []byte) [][]byte {
	const header = "TEXT "
	text := string(assembly)
	starts := regexp.MustCompile(`(?m)^TEXT `).FindAllStringIndex(text, -1)
	sections := make([][]byte, 0, len(starts))
	for index, start := range starts {
		end := len(text)
		if index+1 < len(starts) {
			end = starts[index+1][0]
		}
		section := text[start[0]:end]
		if strings.HasPrefix(section, header) {
			sections = append(sections, []byte(section))
		}
	}
	return sections
}

func hasAssemblyForLine(assembly []byte, file string, line int) bool {
	needle := []byte(filepath.Base(file) + ":" + strconv.Itoa(line))
	for _, assemblyLine := range bytes.Split(assembly, []byte("\n")) {
		if bytes.Contains(assemblyLine, needle) {
			return true
		}
	}
	return false
}

func runGoTool(t *testing.T, arguments ...string) []byte {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, "go", append([]string{"tool"}, arguments...)...)
	output, err := command.CombinedOutput()
	if ctx.Err() != nil {
		t.Fatalf("go tool %s: %v", strings.Join(arguments, " "), ctx.Err())
	}
	if err != nil {
		t.Fatalf("go tool %s: %v\n%s", strings.Join(arguments, " "), err, output)
	}
	return output
}

func buildProductionTestBinary(t *testing.T) string {
	t.Helper()
	binary := filepath.Join(t.TempDir(), "gotreesitter.test")
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	command := exec.CommandContext(ctx, "go", "test", "-c", "-o", binary, ".")
	output, err := command.CombinedOutput()
	if ctx.Err() != nil {
		t.Fatalf("build untagged test binary: %v", ctx.Err())
	}
	if err != nil {
		t.Fatalf("build untagged test binary: %v\n%s", err, output)
	}
	return binary
}
