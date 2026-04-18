//go:build diagnostic

package grammargen

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"testing"
	"unicode/utf8"

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
	immediateTokens := make(map[int]bool)
	for _, term := range ng.Terminals {
		if term.Immediate {
			immediateTokens[term.SymbolID] = true
		}
	}
	keywordSet := make(map[int]bool, len(ng.KeywordSymbols))
	for _, ks := range ng.KeywordSymbols {
		keywordSet[ks] = true
	}
	diagLexModes, diagStateToMode, _ := computeLexModes(
		tables.StateCount,
		ng.TokenCount(),
		func(state, sym int) bool {
			if acts, ok := tables.ActionTable[state]; ok {
				if entry, ok := acts[sym]; ok && len(entry) > 0 {
					return true
				}
			}
			return false
		},
		computeStringPrefixExtensions(ng.Terminals),
		ng.ExtraSymbols,
		tables.ExtraChainStateStart,
		immediateTokens,
		ng.ExternalSymbols,
		ng.WordSymbolID,
		keywordSet,
		terminalPatternSymSet(ng),
		nil, nil,
	)
	var diagLexStates []gotreesitter.LexState
	var diagLexModeOffsets []int
	if os.Getenv("DIAG_SCALA_LEX_BYTE") != "" {
		skipExtras := computeSkipExtras(ng)
		diagLexStates, diagLexModeOffsets, err = buildLexDFA(context.Background(), ng.Terminals, ng.ExtraSymbols, skipExtras, diagLexModes)
		if err != nil {
			t.Fatalf("build diag lex DFA: %v", err)
		}
	}

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
	t.Logf("gen-children: %v", diagNodeChildren(genRoot, genLang, 12))
	t.Logf("ref-children: %v", diagNodeChildren(refRoot, refLang, 12))
	t.Logf("gen-sexp: %s", diagShortString(safeSExpr(genRoot, genLang, 64), 400))
	t.Logf("ref-sexp: %s", diagShortString(safeSExpr(refRoot, refLang, 64), 400))

	slashStarSyms := diagFindAllSymbols(ng, "/*")
	slashSlashSyms := diagFindAllSymbols(ng, "//")
	autoSemiSyms := diagFindAllSymbols(ng, "_automatic_semicolon")
	closeCommentSyms := diagFindAllSymbols(ng, "*/")
	blockCommentToken2Syms := diagFindAllSymbols(ng, "_block_comment_token2")
	blockCommentSyms := diagFindAllSymbols(ng, "block_comment")
	eofSyms := []int{0}
	t.Logf("scala-diag symbols: /*=%v _automatic_semicolon=%v */=%v block_comment=%v extras=%v", slashStarSyms, autoSemiSyms, closeCommentSyms, blockCommentSyms, ng.ExtraSymbols)
	for _, term := range ng.Terminals {
		if containsInt(closeCommentSyms, term.SymbolID) || containsInt(blockCommentToken2Syms, term.SymbolID) {
			ruleKind := "<nil>"
			ruleVal := ""
			if term.Rule != nil {
				ruleKind = ruleKindName(term.Rule.Kind)
				ruleVal = term.Rule.Value
			}
			t.Logf("scala-diag terminal sym=%d name=%q priority=%d immediate=%v kind=%s value=%q",
				term.SymbolID, ng.Symbols[term.SymbolID].Name, term.Priority, term.Immediate, ruleKind, ruleVal)
		}
	}
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
			diagLogStateActions(t, "scala-diag", ng, tables, target, slashStarSyms, slashSlashSyms, closeCommentSyms, autoSemiSyms)
			if len(autoSemiSyms) > 0 {
				t.Logf("scala-diag target-state=%d on %s actions=%s",
					target, diagSymbolName(ng, autoSemiSyms[0]), diagFormatActions(ng, tables.ActionTable[target][autoSemiSyms[0]]))
			}
			if len(closeCommentSyms) > 0 {
				t.Logf("scala-diag target-state=%d on %s actions=%s",
					target, diagSymbolName(ng, closeCommentSyms[0]), diagFormatActions(ng, tables.ActionTable[target][closeCommentSyms[0]]))
				for _, closeAct := range tables.ActionTable[target][closeCommentSyms[0]] {
					if closeAct.kind != lrShift {
						continue
					}
					closeTarget := closeAct.state
					t.Logf("scala-diag close-target-state=%d from state=%d on %s",
						closeTarget, target, diagSymbolName(ng, closeCommentSyms[0]))
					diagLogStateActions(t, "scala-diag close", ng, tables, closeTarget, eofSyms, slashStarSyms, slashSlashSyms, closeCommentSyms, autoSemiSyms)
				}
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
				if diagProductionMentionsNames(ng, &ng.Productions[int(ce.prodIdx)], []string{"block_comment", "comment", "_comment_text"}) {
					laPrefix := ""
					if len(autoSemiSyms) > 0 && ce.lookaheads.contains(autoSemiSyms[0]) {
						laPrefix += " LA(_automatic_semicolon)"
					}
					if len(closeCommentSyms) > 0 && ce.lookaheads.contains(closeCommentSyms[0]) {
						laPrefix += " LA(*/)"
					}
					t.Logf("scala-diag state=%d item%s %s", target, laPrefix, diagFormatProd(ng, int(ce.prodIdx), int(ce.dot)))
				}
			}
			if len(slashStarSyms) > 0 {
				for _, nestedAct := range tables.ActionTable[target][slashStarSyms[0]] {
					if nestedAct.kind != lrShift || !nestedAct.isExtra {
						continue
					}
					nestedTarget := nestedAct.state
					t.Logf("scala-diag nested-target-state=%d from state=%d on %s", nestedTarget, target, diagSymbolName(ng, slashStarSyms[0]))
					diagLogStateActions(t, "scala-diag nested", ng, tables, nestedTarget, eofSyms, slashStarSyms, slashSlashSyms, closeCommentSyms, autoSemiSyms)
				}
			}
		}
	}
	if len(closeCommentSyms) > 0 && len(blockCommentSyms) > 0 && tables.ExtraChainStateStart >= 0 {
		for state := tables.ExtraChainStateStart; state < tables.StateCount; state++ {
			acts := tables.ActionTable[state][closeCommentSyms[0]]
			for _, act := range acts {
				if act.kind != lrShift || act.lhsSym != blockCommentSyms[0] {
					continue
				}
				t.Logf("scala-diag block-comment-close state=%d on %s target=%d actions=%s",
					state, diagSymbolName(ng, closeCommentSyms[0]), act.state, diagFormatActions(ng, acts))
				diagLogStateActions(t, "scala-diag block-comment-close-target", ng, tables, act.state, eofSyms, slashStarSyms, slashSlashSyms, closeCommentSyms, autoSemiSyms)
			}
		}
	}
	if raw := strings.TrimSpace(os.Getenv("DIAG_SCALA_STATES")); raw != "" {
		lexProbeByte := -1
		if rawByte := strings.TrimSpace(os.Getenv("DIAG_SCALA_LEX_BYTE")); rawByte != "" {
			if n, err := strconv.Atoi(rawByte); err == nil {
				lexProbeByte = n
			} else {
				t.Logf("scala-diag invalid DIAG_SCALA_LEX_BYTE=%q: %v", rawByte, err)
			}
		}
		for _, part := range strings.Split(raw, ",") {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}
			state, err := strconv.Atoi(part)
			if err != nil {
				t.Logf("scala-diag invalid state %q: %v", part, err)
				continue
			}
			if state < 0 || state >= tables.StateCount {
				t.Logf("scala-diag state=%d out of range (StateCount=%d)", state, tables.StateCount)
				continue
			}
			if state < len(diagStateToMode) && diagStateToMode[state] < len(diagLexModes) {
				modeIdx := diagStateToMode[state]
				valid := diagLexModes[modeIdx].validSymbols
				t.Logf("scala-diag explicit state=%d mode=%d validClose=%v validToken2=%v",
					state,
					modeIdx,
					len(closeCommentSyms) > 0 && valid[closeCommentSyms[0]],
					len(blockCommentToken2Syms) > 0 && valid[blockCommentToken2Syms[0]],
				)
			}
			if lexProbeByte >= 0 && lexProbeByte <= len(src) && state+1 < len(genLang.LexModes) {
				lexState := genLang.LexModes[state+1].LexState
				lexer := gotreesitter.NewLexer(genLang.LexStates, src[lexProbeByte:])
				tok := lexer.Next(lexState)
				tokName := ""
				if int(tok.Symbol) < len(genLang.SymbolNames) {
					tokName = genLang.SymbolNames[tok.Symbol]
				}
				t.Logf("scala-diag explicit state=%d runtimeState=%d lexState=%d probeByte=%d tok=%d %q span=%d",
					state, state+1, lexState, lexProbeByte, tok.Symbol, tokName, tok.EndByte-tok.StartByte)
				if state < len(diagStateToMode) && diagStateToMode[state] < len(diagLexModeOffsets) {
					modeIdx := diagStateToMode[state]
					computedLexState := uint16(diagLexModeOffsets[modeIdx])
					diagLexer := gotreesitter.NewLexer(diagLexStates, src[lexProbeByte:])
					diagTok := diagLexer.Next(computedLexState)
					diagName := ""
					if int(diagTok.Symbol) < len(genLang.SymbolNames) {
						diagName = genLang.SymbolNames[diagTok.Symbol]
					}
					t.Logf("scala-diag explicit state=%d computedMode=%d computedLexState=%d probeByte=%d tok=%d %q span=%d",
						state, modeIdx, computedLexState, lexProbeByte, diagTok.Symbol, diagName, diagTok.EndByte-diagTok.StartByte)
					if int(computedLexState) < len(diagLexStates) {
						startState := diagLexStates[computedLexState]
						starNext := -1
						for _, tr := range startState.Transitions {
							if '*' >= tr.Lo && '*' <= tr.Hi {
								starNext = tr.NextState
								break
							}
						}
						t.Logf("scala-diag explicit state=%d computedLexState=%d accept=%d starNext=%d",
							state, computedLexState, startState.AcceptToken, starNext)
						if starNext >= 0 && int(starNext) < len(diagLexStates) {
							nextState := diagLexStates[starNext]
							slashNext := -1
							for _, tr := range nextState.Transitions {
								if '/' >= tr.Lo && '/' <= tr.Hi {
									slashNext = tr.NextState
									break
								}
							}
							t.Logf("scala-diag explicit state=%d afterStar=%d accept=%d slashNext=%d",
								state, starNext, nextState.AcceptToken, slashNext)
							if slashNext >= 0 && int(slashNext) < len(diagLexStates) {
								closeState := diagLexStates[slashNext]
								t.Logf("scala-diag explicit state=%d afterSlash=%d accept=%d prio=%d trans=%d default=%d eof=%d",
									state, slashNext, closeState.AcceptToken, closeState.AcceptPriority, len(closeState.Transitions), closeState.Default, closeState.EOF)
							}
						}
					}
					if getenvOr("DIAG_SCALA_LEX_TRACE", "") == "1" && int(computedLexState) < len(diagLexStates) {
						limit := len(src) - lexProbeByte
						if limit > 4 {
							limit = 4
						}
						if limit < 0 {
							limit = 0
						}
						diagTraceLexScan(t, "scala-diag computed-trace", diagLexStates, genLang.SymbolNames, int(computedLexState), src[lexProbeByte:lexProbeByte+limit])
					}
				}
			}
			diagLogStateActions(t, fmt.Sprintf("scala-diag explicit state=%d", state), ng, tables, state, eofSyms, slashStarSyms, slashSlashSyms, closeCommentSyms, autoSemiSyms)
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

func diagNodeChildren(n *gotreesitter.Node, lang *gotreesitter.Language, max int) []string {
	if n == nil {
		return nil
	}
	limit := n.ChildCount()
	if limit > max {
		limit = max
	}
	out := make([]string, 0, limit)
	for i := 0; i < limit; i++ {
		c := n.Child(i)
		if c == nil {
			continue
		}
		out = append(out, fmt.Sprintf("%s[%d:%d]", c.Type(lang), c.StartByte(), c.EndByte()))
	}
	return out
}

func diagLogStateActions(t *testing.T, label string, ng *NormalizedGrammar, tables *LRTables, state int, syms ...[]int) {
	t.Helper()
	interesting := make(map[int]struct{})
	for _, group := range syms {
		for _, sym := range group {
			interesting[sym] = struct{}{}
		}
	}
	interestingSyms := make([]int, 0, len(interesting))
	for sym := range interesting {
		if acts := tables.ActionTable[state][sym]; len(acts) > 0 {
			interestingSyms = append(interestingSyms, sym)
		}
	}
	sort.Ints(interestingSyms)
	for _, sym := range interestingSyms {
		t.Logf("%s state=%d on %s actions=%s",
			label, state, diagSymbolName(ng, sym), diagFormatActions(ng, tables.ActionTable[state][sym]))
	}
}

func containsInt(vals []int, target int) bool {
	for _, v := range vals {
		if v == target {
			return true
		}
	}
	return false
}

func ruleKindName(kind RuleKind) string {
	switch kind {
	case RuleBlank:
		return "blank"
	case RuleString:
		return "string"
	case RulePattern:
		return "pattern"
	case RuleSeq:
		return "seq"
	case RuleChoice:
		return "choice"
	case RuleRepeat:
		return "repeat"
	case RuleRepeat1:
		return "repeat1"
	case RuleOptional:
		return "optional"
	case RuleSymbol:
		return "symbol"
	case RuleField:
		return "field"
	case RuleAlias:
		return "alias"
	case RuleToken:
		return "token"
	case RuleImmToken:
		return "immediate_token"
	case RulePrec:
		return "prec"
	case RulePrecLeft:
		return "prec_left"
	case RulePrecRight:
		return "prec_right"
	case RulePrecDynamic:
		return "prec_dynamic"
	default:
		return fmt.Sprintf("kind(%d)", kind)
	}
}

func diagTraceLexScan(t *testing.T, label string, states []gotreesitter.LexState, symbolNames []string, startState int, src []byte) {
	t.Helper()
	curState := startState
	scanPos := 0
	acceptPos := -1
	acceptSym := gotreesitter.Symbol(0)
	acceptPrio := int16(32767)

	for step := 0; step < 8; step++ {
		if curState < 0 || curState >= len(states) {
			t.Logf("%s step=%d state=%d out-of-range", label, step, curState)
			return
		}
		st := states[curState]
		nextRune := rune(0)
		nextSize := 0
		if scanPos < len(src) {
			nextRune, nextSize = utf8.DecodeRune(src[scanPos:])
		}
		t.Logf("%s step=%d state=%d pos=%d accept=%d(%q) prio=%d skip=%v next=%q size=%d default=%d eof=%d",
			label,
			step,
			curState,
			scanPos,
			st.AcceptToken,
			diagLexSymbolName(symbolNames, st.AcceptToken),
			st.AcceptPriority,
			st.Skip,
			nextRune,
			nextSize,
			st.Default,
			st.EOF,
		)
		if st.AcceptToken > 0 || st.Skip {
			if acceptPos < 0 || st.AcceptPriority < acceptPrio || (st.AcceptPriority == acceptPrio && scanPos > acceptPos) {
				acceptPos = scanPos
				acceptSym = st.AcceptToken
				acceptPrio = st.AcceptPriority
				t.Logf("%s step=%d candidate pos=%d sym=%d(%q) prio=%d",
					label, step, acceptPos, acceptSym, diagLexSymbolName(symbolNames, acceptSym), acceptPrio)
			}
		}
		if scanPos >= len(src) {
			t.Logf("%s stop=EOF bestPos=%d bestSym=%d(%q) bestPrio=%d",
				label, acceptPos, acceptSym, diagLexSymbolName(symbolNames, acceptSym), acceptPrio)
			return
		}
		nextState := -1
		for _, tr := range st.Transitions {
			if nextRune >= tr.Lo && nextRune <= tr.Hi {
				nextState = tr.NextState
				t.Logf("%s step=%d transition=%q -> %d range=[%q:%q] skip=%v",
					label, step, nextRune, nextState, tr.Lo, tr.Hi, tr.Skip)
				break
			}
		}
		if nextState < 0 && st.Default >= 0 {
			nextState = st.Default
			t.Logf("%s step=%d default -> %d", label, step, nextState)
		}
		if nextState < 0 {
			t.Logf("%s stop=no-transition bestPos=%d bestSym=%d(%q) bestPrio=%d",
				label, acceptPos, acceptSym, diagLexSymbolName(symbolNames, acceptSym), acceptPrio)
			return
		}
		scanPos += nextSize
		curState = nextState
	}
}

func diagLexSymbolName(symbolNames []string, sym gotreesitter.Symbol) string {
	if int(sym) >= 0 && int(sym) < len(symbolNames) {
		return symbolNames[sym]
	}
	return ""
}
