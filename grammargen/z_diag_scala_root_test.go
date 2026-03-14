package grammargen

import (
	"os"
	"strings"
	"testing"

	"github.com/odvcencio/gotreesitter"
)

func TestDiagScalaRootRuntime(t *testing.T) {
	if testing.Short() {
		t.Skip("diagnostic test")
	}
	if getenvOr("DIAG_SCALA_ROOT", "") != "1" {
		t.Skip("set DIAG_SCALA_ROOT=1 to run Scala root/runtime diagnostics")
	}

	var pg importParityGrammar
	for _, g := range importParityGrammars {
		if g.name == "scala" {
			pg = g
			break
		}
	}
	if pg.name == "" {
		t.Fatal("scala grammar not found")
	}

	samplePath := getenvOr("DIAG_SCALA_SAMPLE", "/tmp/grammar_parity/scala/examples/RefChecks.scala")
	src, err := os.ReadFile(samplePath)
	if err != nil {
		t.Fatalf("read sample %q: %v", samplePath, err)
	}

	gram, err := importParityGrammarSource(pg)
	if err != nil {
		t.Fatalf("import scala grammar: %v", err)
	}
	ng, err := Normalize(gram)
	if err != nil {
		t.Fatalf("normalize scala grammar: %v", err)
	}
	report, err := GenerateWithReport(gram)
	if err != nil {
		t.Fatalf("GenerateWithReport: %v", err)
	}
	tables, ctx, err := buildLRTablesWithProvenance(ng)
	if err != nil {
		t.Fatalf("build scala lr tables: %v", err)
	}
	addNonterminalExtraChains(tables, ng, ctx)

	genLang := report.Language
	refLang := pg.blobFunc()
	adaptExternalScanner(refLang, genLang)

	genParser := gotreesitter.NewParser(genLang)
	if getenvOr("DIAG_SCALA_GLR_TRACE", "") == "1" {
		genParser.SetGLRTrace(true)
	}
	var genParseLogs []string
	var genLexLogs []string
	genParser.SetLogger(func(kind gotreesitter.ParserLogType, message string) {
		switch kind {
		case gotreesitter.ParserLogParse:
			if len(genParseLogs) < 8 {
				genParseLogs = append(genParseLogs, message)
			}
		case gotreesitter.ParserLogLex:
			if len(genLexLogs) < 16 {
				genLexLogs = append(genLexLogs, message)
			}
		}
	})

	genTree, err := genParser.Parse(src)
	if err != nil {
		t.Fatalf("gen parse: %v", err)
	}
	refTree, err := gotreesitter.NewParser(refLang).Parse(src)
	if err != nil {
		t.Fatalf("ref parse: %v", err)
	}

	genRoot := genTree.RootNode()
	refRoot := refTree.RootNode()
	genRT := genTree.ParseRuntime()
	refRT := refTree.ParseRuntime()

	t.Logf("env: GOT_PARSE_NODE_LIMIT_SCALE=%q GOT_GLR_MAX_STACKS=%q sample=%q bytes=%d",
		os.Getenv("GOT_PARSE_NODE_LIMIT_SCALE"),
		os.Getenv("GOT_GLR_MAX_STACKS"),
		samplePath,
		len(src))
	for _, sym := range []int{90, int(genRT.LastTokenSymbol), 279} {
		if sym >= 0 && sym < len(genLang.SymbolNames) {
			t.Logf("gen-symbol[%d]=%q", sym, genLang.SymbolNames[sym])
		}
	}
	t.Logf("gen-runtime: %s", genRT.Summary())
	t.Logf("ref-runtime: %s", refRT.Summary())
	for i, msg := range genParseLogs {
		t.Logf("gen-parse-log[%d]: %s", i, msg)
	}
	for i, msg := range genLexLogs {
		t.Logf("gen-lex-log[%d]: %s", i, msg)
	}
	t.Logf("gen-root: sym=%d type=%q err=%v cc=%d range=[%d:%d]",
		genRoot.Symbol(), genRoot.Type(genLang), genRoot.HasError(), genRoot.ChildCount(), genRoot.StartByte(), genRoot.EndByte())
	t.Logf("ref-root: sym=%d type=%q err=%v cc=%d range=[%d:%d]",
		refRoot.Symbol(), refRoot.Type(refLang), refRoot.HasError(), refRoot.ChildCount(), refRoot.StartByte(), refRoot.EndByte())
	t.Logf("gen-sexp: %s", diagShortString(safeSExpr(genRoot, genLang, 64), 400))
	t.Logf("ref-sexp: %s", diagShortString(safeSExpr(refRoot, refLang, 64), 400))

	slashStarSyms := diagFindAllSymbols(ng, "/*")
	autoSemiSyms := diagFindAllSymbols(ng, "_automatic_semicolon")
	closeCommentSyms := diagFindAllSymbols(ng, "*/")
	blockCommentSyms := diagFindAllSymbols(ng, "block_comment")
	t.Logf("scala-diag symbols: /*=%v _automatic_semicolon=%v */=%v block_comment=%v extras=%v", slashStarSyms, autoSemiSyms, closeCommentSyms, blockCommentSyms, ng.ExtraSymbols)
	for i, prod := range ng.Productions {
		if diagProductionMentionsNames(ng, &prod, []string{"block_comment", "_comment_text", "comment"}) {
			t.Logf("scala-diag prod[%d]: %s", i, diagFormatProd(ng, i, -1))
		}
	}

	if len(slashStarSyms) > 0 {
		acts := tables.ActionTable[0][slashStarSyms[0]]
		t.Logf("scala-diag state=0 on %s actions=%s", diagSymbolName(ng, slashStarSyms[0]), diagFormatActions(ng, acts))
		for _, act := range acts {
			if act.kind != lrShift {
				continue
			}
			target := act.state
			mergeCount := 0
			if target < len(ctx.itemSets) {
				mergeCount = diagMergeCount(ctx, target)
			}
			remappedTarget := target + 1
			t.Logf("scala-diag target-state=%d remapped=%d merged=%d synthetic=%v", target, remappedTarget, mergeCount, target >= len(ctx.itemSets))
			if len(autoSemiSyms) > 0 {
				t.Logf("scala-diag target-state=%d on %s actions=%s",
					target, diagSymbolName(ng, autoSemiSyms[0]), diagFormatActions(ng, tables.ActionTable[target][autoSemiSyms[0]]))
			}
			if len(closeCommentSyms) > 0 {
				t.Logf("scala-diag target-state=%d on %s actions=%s",
					target, diagSymbolName(ng, closeCommentSyms[0]), diagFormatActions(ng, tables.ActionTable[target][closeCommentSyms[0]]))
			}
			if remappedTarget >= 0 && remappedTarget < len(genLang.LexModes) {
				mode := genLang.LexModes[remappedTarget]
				t.Logf("scala-diag remapped-state=%d lexState=%d externalLexState=%d", remappedTarget, mode.LexState, mode.ExternalLexState)
				if int(mode.ExternalLexState) < len(genLang.ExternalLexStates) {
					var names []string
					for i, ok := range genLang.ExternalLexStates[mode.ExternalLexState] {
						if !ok || i >= len(genLang.ExternalSymbols) {
							continue
						}
						sym := genLang.ExternalSymbols[i]
						if int(sym) < len(genLang.SymbolNames) {
							names = append(names, genLang.SymbolNames[sym])
						}
					}
					t.Logf("scala-diag remapped-state=%d external-valid=%v", remappedTarget, names)
				}
			}
			if target >= len(ctx.itemSets) {
				continue
			}
			for _, ce := range ctx.itemSets[target].cores {
				if diagProductionMentionsNames(ng, &ng.Productions[ce.prodIdx], []string{"block_comment", "comment", "_comment_text"}) {
					laPrefix := ""
					if len(autoSemiSyms) > 0 && ce.lookaheads.contains(autoSemiSyms[0]) {
						laPrefix += " LA(_automatic_semicolon)"
					}
					if len(closeCommentSyms) > 0 && ce.lookaheads.contains(closeCommentSyms[0]) {
						laPrefix += " LA(*/)"
					}
					t.Logf("scala-diag state=%d item%s %s", target, laPrefix, diagFormatProd(ng, ce.prodIdx, ce.dot))
				}
			}
		}
	}
}

func diagShortString(s string, max int) string {
	s = strings.ReplaceAll(s, "\n", "\\n")
	if len(s) <= max {
		return s
	}
	return s[:max] + "...(truncated)"
}
