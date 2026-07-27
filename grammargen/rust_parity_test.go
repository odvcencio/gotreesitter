package grammargen

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"

	"github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

func TestRustForLifetimeAbstractTypeParity(t *testing.T) {
	jsonPath := rustGrammarJSONPathForTest(t)
	source, err := os.ReadFile(jsonPath)
	if err != nil {
		t.Skipf("Rust grammar.json not available: %v", err)
	}
	gram, err := ImportGrammarJSON(source)
	if err != nil {
		t.Fatalf("import Rust grammar.json: %v", err)
	}
	genLang, err := generateWithTimeout(gram, 90*time.Second)
	if err != nil {
		t.Fatalf("generate Rust language: %v", err)
	}
	refLang := grammars.RustLanguage()
	adaptExternalScanner(refLang, genLang)

	sample := "fn main() {}\n\n" +
		"fn add(x: i32, y: i32) -> i32 {\n" +
		"    return x + y;\n" +
		"}\n\n" +
		"fn takes_slice(slice: &str) {\n" +
		"    println!(\"Got: {}\", slice);\n" +
		"}\n\n" +
		"fn foo() -> [u32; 2] {\n" +
		"    return [1, 2];\n" +
		"}\n\n" +
		"fn foo() -> (u32, u16) {\n" +
		"    return (1, 2);\n" +
		"}\n\n" +
		"fn foo() {\n" +
		"    return\n" +
		"}\n\n" +
		"fn foo(x: impl FnOnce() -> result::Result<T, E>) {}\n\n" +
		"fn foo(#[attr] x: i32, #[attr] x: i64) {}\n\n" +
		"fn accumulate(self) -> Machine<{State::Accumulate}> {}\n\n" +
		"fn foo(bar: impl for<'a> Baz<Quux<'a>>) {}\n"

	assertGeneratedAndReferenceDeepParity(t, genLang, refLang, sample)
}

func TestRustStructExpressionParity(t *testing.T) {
	jsonPath := rustGrammarJSONPathForTest(t)
	source, err := os.ReadFile(jsonPath)
	if err != nil {
		t.Skipf("Rust grammar.json not available: %v", err)
	}
	gram, err := ImportGrammarJSON(source)
	if err != nil {
		t.Fatalf("import Rust grammar.json: %v", err)
	}
	genLang, err := generateWithTimeout(gram, 90*time.Second)
	if err != nil {
		t.Fatalf("generate Rust language: %v", err)
	}
	refLang := grammars.RustLanguage()
	adaptExternalScanner(refLang, genLang)

	sample := "NothingInMe {};\n" +
		"Point {x: 10.0, y: 20.0};\n" +
		"let a = SomeStruct { field1, field2: expression, field3, };\n" +
		"let u = game::User {name: \"Joe\", age: 35, score: 100_000};\n" +
		"let i = Instant { 0: Duration::from_millis(0) };\n"

	assertGeneratedAndReferenceDeepParity(t, genLang, refLang, sample)
}

func TestRustClosureMethodCallParity(t *testing.T) {
	jsonPath := rustGrammarJSONPathForTest(t)
	source, err := os.ReadFile(jsonPath)
	if err != nil {
		t.Skipf("Rust grammar.json not available: %v", err)
	}
	gram, err := ImportGrammarJSON(source)
	if err != nil {
		t.Fatalf("import Rust grammar.json: %v", err)
	}
	genLang, err := generateWithTimeout(gram, 90*time.Second)
	if err != nil {
		t.Fatalf("generate Rust language: %v", err)
	}
	refLang := grammars.RustLanguage()
	adaptExternalScanner(refLang, genLang)

	cases := []string{
		"fn f() { self.params.iter().any(|param| param.is_lifetime_param()); }\n",
		"impl Generics { pub fn f(&self) -> bool { self.params.iter().any(|param| param.is_lifetime_param()) } }\n",
	}
	for _, sample := range cases {
		assertGeneratedAndReferenceDeepParity(t, genLang, refLang, sample)
	}
}

func TestRustPatternStatementParity(t *testing.T) {
	jsonPath := rustGrammarJSONPathForTest(t)
	source, err := os.ReadFile(jsonPath)
	if err != nil {
		t.Skipf("Rust grammar.json not available: %v", err)
	}
	gram, err := ImportGrammarJSON(source)
	if err != nil {
		t.Fatalf("import Rust grammar.json: %v", err)
	}
	genLang, err := generateWithTimeout(gram, 90*time.Second)
	if err != nil {
		t.Fatalf("generate Rust language: %v", err)
	}
	refLang := grammars.RustLanguage()
	adaptExternalScanner(refLang, genLang)

	sample := "if let A(x) | B(x) = expr {\n" +
		"    do_stuff_with(x);\n" +
		"}\n\n" +
		"while let A(x) | B(x) = expr {\n" +
		"    do_stuff_with(x);\n" +
		"}\n\n" +
		"let Ok(index) | Err(index) = slice.binary_search(&x);\n\n" +
		"for ref a | b in c {}\n\n" +
		"let Ok(x) | Err(x) = binary_search(x);\n\n" +
		"for A | B | C in c {}\n\n" +
		"|(Ok(x) | Err(x))| expr();\n\n" +
		"let ref mut x @ (A | B | C);\n\n" +
		"fn foo((1 | 2 | 3): u8) {}\n\n" +
		"if let x!() | y!() = () {}\n"

	assertGeneratedAndReferenceDeepParity(t, genLang, refLang, sample)
}

func TestRustMacroInvocationParity(t *testing.T) {
	jsonPath := rustGrammarJSONPathForTest(t)
	source, err := os.ReadFile(jsonPath)
	if err != nil {
		t.Skipf("Rust grammar.json not available: %v", err)
	}
	gram, err := ImportGrammarJSON(source)
	if err != nil {
		t.Fatalf("import Rust grammar.json: %v", err)
	}
	genLang, err := generateWithTimeout(gram, 90*time.Second)
	if err != nil {
		t.Fatalf("generate Rust language: %v", err)
	}
	refLang := grammars.RustLanguage()
	adaptExternalScanner(refLang, genLang)

	sample := "a!(* a *);\n" +
		"a!(& a &);\n" +
		"a!(- a -);\n" +
		"a!(b + c + +);\n" +
		"a!('a'..='z');\n" +
		"a!('\\u{0}'..='\\u{2}');\n" +
		"a!('lifetime)\n" +
		"default!(a);\n" +
		"union!(a);\n" +
		"a!($);\n" +
		"a!($());\n" +
		"a!($ a $);\n" +
		"a!(${$([ a ])});\n" +
		"a!($a $a:ident $($a);*);\n"

	assertGeneratedAndReferenceDeepParity(t, genLang, refLang, sample)
}

func TestRustWeirdExpressionsParity(t *testing.T) {
	jsonPath := rustGrammarJSONPathForTest(t)
	source, err := os.ReadFile(jsonPath)
	if err != nil {
		t.Skipf("Rust grammar.json not available: %v", err)
	}
	gram, err := ImportGrammarJSON(source)
	if err != nil {
		t.Fatalf("import Rust grammar.json: %v", err)
	}
	genLang, err := generateWithTimeout(gram, 90*time.Second)
	if err != nil {
		t.Fatalf("generate Rust language: %v", err)
	}
	refLang := grammars.RustLanguage()
	adaptExternalScanner(refLang, genLang)

	sample := "fn angrydome() {\n" +
		"    loop { if break { } }\n" +
		"    let mut i = 0;\n" +
		"    loop { i += 1; if i == 1 { match (continue) { 1 => { }, _ => panic!(\"wat\") } }\n" +
		"      break; }\n" +
		"}\n\n" +
		"fn special_characters() {\n" +
		"    let val = !((|(..):(_,_),(|__@_|__)|__)((&*\"\\\\\",'🤔')/**/,{})=={&[..=..][..];})//\n" +
		"    ;\n" +
		"    assert!(!val);\n" +
		"}\n\n" +
		"fn function() {\n" +
		"    struct foo;\n" +
		"    impl Deref for foo {\n" +
		"        type Target = fn() -> Self;\n" +
		"        fn deref(&self) -> &Self::Target {\n" +
		"            &((|| foo) as _)\n" +
		"        }\n" +
		"    }\n" +
		"    let foo = foo () ()() ()()() ()()()() ()()()()();\n" +
		"}\n\n" +
		"fn closure_matching() {\n" +
		"    let x = |_| Some(1);\n" +
		"    let (|x| x) = match x(..) {\n" +
		"        |_| Some(2) => |_| Some(3),\n" +
		"        |_| _ => unreachable!(),\n" +
		"    };\n" +
		"    assert!(matches!(x(..), |_| Some(4)));\n" +
		"}\n"

	assertGeneratedAndReferenceDeepParity(t, genLang, refLang, sample)
}

func TestRustWeirdTopLevelParity(t *testing.T) {
	jsonPath := rustGrammarJSONPathForTest(t)
	source, err := os.ReadFile(jsonPath)
	if err != nil {
		t.Skipf("Rust grammar.json not available: %v", err)
	}
	gram, err := ImportGrammarJSON(source)
	if err != nil {
		t.Fatalf("import Rust grammar.json: %v", err)
	}
	genLang, err := generateWithTimeout(gram, 90*time.Second)
	if err != nil {
		t.Fatalf("generate Rust language: %v", err)
	}
	refLang := grammars.RustLanguage()
	adaptExternalScanner(refLang, genLang)

	sample := "// Just a grab bag of stuff that you wouldn't want to actually write.\n\n" +
		"fn strange() -> bool { let _x: bool = return true; }\n\n" +
		"fn what() {\n" +
		"    fn the(x: &Cell<bool>) {\n" +
		"        return while !x.get() { x.set(true); };\n" +
		"    }\n" +
		"    let i = &Cell::new(false);\n" +
		"    let dont = {||the(i)};\n" +
		"    dont();\n" +
		"    assert!((i.get()));\n" +
		"}\n\n" +
		"fn punch_card() -> impl std::fmt::Debug {\n" +
		"    ..=..=.. ..    .. .. .. ..    .. .. .. ..    .. ..=.. ..\n" +
		"    ..=.. ..=..    .. .. .. ..    .. .. .. ..    ..=..=..=..\n" +
		"    ..=.. ..=..    ..=.. ..=..    .. ..=..=..    .. ..=.. ..\n" +
		"    ..=..=.. ..    ..=.. ..=..    ..=.. .. ..    .. ..=.. ..\n" +
		"    ..=.. ..=..    ..=.. ..=..    .. ..=.. ..    .. ..=.. ..\n" +
		"    ..=.. ..=..    ..=.. ..=..    .. .. ..=..    .. ..=.. ..\n" +
		"    ..=.. ..=..    .. ..=..=..    ..=..=.. ..    .. ..=.. ..\n" +
		"}\n"

	assertGeneratedAndReferenceDeepParity(t, genLang, refLang, sample)
}

func TestRustDotRangeTableParityBeforeCompatibility(t *testing.T) {
	jsonPath := rustGrammarJSONPathForTest(t)
	source, err := os.ReadFile(jsonPath)
	if err != nil {
		t.Skipf("Rust grammar.json not available: %v", err)
	}
	gram, err := ImportGrammarJSON(source)
	if err != nil {
		t.Fatalf("import Rust grammar.json: %v", err)
	}
	genLang, err := generateWithTimeout(gram, 90*time.Second)
	if err != nil {
		t.Fatalf("generate Rust language: %v", err)
	}
	refLang := grammars.RustLanguage()
	adaptExternalScanner(refLang, genLang)

	sample := "fn punch_card() -> impl std::fmt::Debug {\n" +
		"    ..=..=.. ..    .. .. .. ..    .. .. .. ..    .. ..=.. ..\n" +
		"    ..=.. ..=..    .. .. .. ..    .. .. .. ..    ..=..=..=..\n" +
		"    ..=.. ..=..    ..=.. ..=..    .. ..=..=..    .. ..=.. ..\n" +
		"    ..=..=.. ..    ..=.. ..=..    ..=.. .. ..    .. ..=.. ..\n" +
		"    ..=.. ..=..    ..=.. ..=..    .. ..=.. ..    .. ..=.. ..\n" +
		"    ..=.. ..=..    ..=.. ..=..    .. .. ..=..    .. ..=.. ..\n" +
		"    ..=.. ..=..    .. ..=..=..    ..=..=.. ..    .. ..=.. ..\n" +
		"}\n"

	assertGeneratedAndReferenceRawDeepParity(t, genLang, refLang, sample)
}

func assertGeneratedAndReferenceRawDeepParity(t *testing.T, genLang, refLang *gotreesitter.Language, src string) {
	t.Helper()
	data := []byte(src)
	genTree, err := gotreesitter.NewParser(genLang).ParseNoResultCompatibilityBenchmarkOnly(data)
	if err != nil {
		t.Fatalf("generated parse: %v", err)
	}
	defer genTree.Release()
	refTree, err := gotreesitter.NewParser(refLang).ParseNoResultCompatibilityBenchmarkOnly(data)
	if err != nil {
		t.Fatalf("reference parse: %v", err)
	}
	defer refTree.Release()

	divs := compareTreesDeep(genTree.RootNode(), genLang, refTree.RootNode(), refLang, "root", 10)
	if len(divs) > 0 {
		t.Fatalf("raw deep mismatch\nGEN: %s\nREF: %s\nDIVS: %v",
			safeSExpr(genTree.RootNode(), genLang, 256),
			safeSExpr(refTree.RootNode(), refLang, 256),
			divs)
	}
}

func TestRustGeneratedMatchArmDiagnostic(t *testing.T) {
	if os.Getenv("GTS_RUST_GENERATED_MATCH_DIAGNOSTIC") == "" {
		t.Skip("set GTS_RUST_GENERATED_MATCH_DIAGNOSTIC=1 to reproduce the generated Rust match_arm divergence")
	}
	jsonPath := rustGrammarJSONPathForTest(t)
	source, err := os.ReadFile(jsonPath)
	if err != nil {
		t.Skipf("Rust grammar.json not available: %v", err)
	}
	gram, err := ImportGrammarJSON(source)
	if err != nil {
		t.Fatalf("import Rust grammar.json: %v", err)
	}
	ng, err := Normalize(gram)
	if err != nil {
		t.Fatalf("normalize Rust grammar.json: %v", err)
	}
	logRustMatchDiagnostic(t, ng, nil)
	genLang, err := generateWithTimeout(gram, 90*time.Second)
	if err != nil {
		t.Fatalf("generate Rust language: %v", err)
	}
	refLang := grammars.RustLanguage()
	adaptExternalScanner(refLang, genLang)
	logRustMatchDiagnostic(t, ng, genLang)

	cases := []string{
		"match x {\n" +
			"    1 => { \"one\" }\n" +
			"}\n",
		"match x {\n" +
			"    1 => { \"one\" }\n" +
			"    2 => \"two\",\n" +
			"}\n",
	}
	for _, sample := range cases {
		t.Run(sample, func(t *testing.T) {
			assertGeneratedAndReferenceDeepParity(t, genLang, refLang, sample)
		})
	}
}
func TestRustIfBreakBlockDiagnostic(t *testing.T) {
	if os.Getenv("GTS_RUST_IF_BREAK_DIAGNOSTIC") == "" {
		t.Skip("set GTS_RUST_IF_BREAK_DIAGNOSTIC=1 to reproduce the generated Rust if-break block divergence")
	}
	jsonPath := rustGrammarJSONPathForTest(t)
	source, err := os.ReadFile(jsonPath)
	if err != nil {
		t.Skipf("Rust grammar.json not available: %v", err)
	}
	gram, err := ImportGrammarJSON(source)
	if err != nil {
		t.Fatalf("import Rust grammar.json: %v", err)
	}
	ng, err := Normalize(gram)
	if err != nil {
		t.Fatalf("normalize Rust grammar.json: %v", err)
	}
	logRustIfBreakConflictDiagnostic(t, ng)

	genLang, err := generateWithTimeout(gram, 90*time.Second)
	if err != nil {
		t.Fatalf("generate Rust language: %v", err)
	}
	refLang := grammars.RustLanguage()
	adaptExternalScanner(refLang, genLang)

	assertGeneratedAndReferenceDeepParity(t, genLang, refLang, "fn f() { loop { if break { } } }\n")
}

type rustLRActionKey struct {
	state int
	sym   int
}

func logRustIfBreakConflictDiagnostic(t *testing.T, ng *NormalizedGrammar) {
	t.Helper()

	tables, _, err := buildLRTablesWithProvenance(ng)
	if err != nil {
		t.Fatalf("build Rust LR tables: %v", err)
	}
	keys := logRustIfBreakActionEntries(t, "unresolved", tables, ng, nil)
	stats, err := resolveConflicts(context.Background(), tables, ng)
	if err != nil {
		t.Fatalf("resolve Rust LR conflicts: %v", err)
	}
	t.Logf("rust if-break diag resolved conflicts: %+v", stats)
	logRustIfBreakActionEntries(t, "resolved", tables, ng, keys)
}

func logRustIfBreakActionEntries(t *testing.T, label string, tables *LRTables, ng *NormalizedGrammar, keys map[rustLRActionKey]bool) map[rustLRActionKey]bool {
	t.Helper()

	selected := make(map[rustLRActionKey]bool)
	states := make([]int, 0, len(tables.ActionTable))
	for state := range tables.ActionTable {
		states = append(states, state)
	}
	sort.Ints(states)

	logged := 0
	for _, state := range states {
		syms := make([]int, 0, len(tables.ActionTable[state]))
		for sym := range tables.ActionTable[state] {
			syms = append(syms, sym)
		}
		sort.Ints(syms)
		for _, sym := range syms {
			key := rustLRActionKey{state: state, sym: sym}
			actions := tables.ActionTable[state][sym]
			if keys == nil {
				if !rustIfBreakActionEntryRelevant(sym, actions, ng) {
					continue
				}
				selected[key] = true
			} else if !keys[key] {
				continue
			}
			t.Logf("rust if-break diag %s state=%d lookahead=%d/%q actions=%d",
				label, state, sym, rustSymbolName(ng, sym), len(actions))
			for _, action := range actions {
				logRustLRAction(t, "  "+label, action, ng)
			}
			logged++
			if logged >= 12 {
				t.Logf("rust if-break diag %s truncated after %d entries", label, logged)
				return selected
			}
		}
	}
	if logged == 0 {
		t.Logf("rust if-break diag %s found no { shift/break_expression reduce entries", label)
	}
	return selected
}

func rustIfBreakActionEntryRelevant(sym int, actions []lrAction, ng *NormalizedGrammar) bool {
	if rustSymbolName(ng, sym) != "{" {
		return false
	}
	hasBreakReduce := false
	hasShift := false
	for _, action := range actions {
		switch action.kind {
		case lrShift:
			hasShift = true
		case lrReduce:
			if action.prodIdx >= 0 && action.prodIdx < len(ng.Productions) &&
				rustSymbolName(ng, ng.Productions[action.prodIdx].LHS) == "break_expression" {
				hasBreakReduce = true
			}
		}
	}
	return hasBreakReduce && hasShift
}

func logRustLRAction(t *testing.T, label string, action lrAction, ng *NormalizedGrammar) {
	t.Helper()

	switch action.kind {
	case lrShift:
		t.Logf("%s shift target=%d prec=%d hasPrec=%v assoc=%s lhs=%s lhsSyms=%v repeat=%v repeatLHS=%s repeatLHSSyms=%v",
			label, action.state, action.prec, action.hasPrec, rustAssocName(action.assoc),
			rustSymbolName(ng, action.lhsSym), rustSymbolNames(ng, action.lhsSyms),
			action.repeat, rustSymbolName(ng, action.repeatLHS), rustSymbolNames(ng, action.repeatLHSSyms))
	case lrReduce:
		if action.prodIdx < 0 || action.prodIdx >= len(ng.Productions) {
			t.Logf("%s reduce prod=%d", label, action.prodIdx)
			return
		}
		prod := ng.Productions[action.prodIdx]
		t.Logf("%s reduce prod=%d lhs=%s rhs=%v prec=%d hasPrec=%v assoc=%s dynPrec=%d",
			label, action.prodIdx, rustSymbolName(ng, prod.LHS), rustSymbolNames(ng, prod.RHS),
			prod.Prec, prod.HasExplicitPrec, rustAssocName(prod.Assoc), prod.DynPrec)
	case lrAccept:
		t.Logf("%s accept", label)
	default:
		t.Logf("%s action kind=%d", label, action.kind)
	}
}

func rustSymbolNames(ng *NormalizedGrammar, syms []int) []string {
	names := make([]string, 0, len(syms))
	for _, sym := range syms {
		names = append(names, rustSymbolName(ng, sym))
	}
	return names
}

func rustSymbolName(ng *NormalizedGrammar, sym int) string {
	if ng == nil || sym < 0 || sym >= len(ng.Symbols) {
		return ""
	}
	return ng.Symbols[sym].Name
}

func rustAssocName(assoc Assoc) string {
	switch assoc {
	case AssocLeft:
		return "left"
	case AssocRight:
		return "right"
	default:
		return "none"
	}
}
func logRustMatchDiagnostic(t *testing.T, ng *NormalizedGrammar, lang *gotreesitter.Language) {
	t.Helper()
	if ng != nil {
		matchSym := -1
		identifierSym := -1
		for i, sym := range ng.Symbols {
			switch sym.Name {
			case "match":
				if matchSym == -1 || sym.Kind == SymbolTerminal {
					matchSym = i
				}
			case "identifier":
				if identifierSym == -1 || sym.Kind == SymbolNamedToken {
					identifierSym = i
				}
			}
		}
		matchKeyword := false
		for _, symID := range ng.KeywordSymbols {
			if symID == matchSym {
				matchKeyword = true
				break
			}
		}
		t.Logf("rust match diag normalized: word=%d identifier=%d match=%d match_keyword=%v keywords=%d keyword_entries=%d terminals=%d productions=%d reserved_sets=%d",
			ng.WordSymbolID, identifierSym, matchSym, matchKeyword, len(ng.KeywordSymbols), len(ng.KeywordEntries), len(ng.Terminals), len(ng.Productions), len(ng.ReservedWordSets))
	}
	if lang == nil {
		return
	}
	matchSym := -1
	identifierSym := -1
	for i, name := range lang.SymbolNames {
		switch name {
		case "match":
			if matchSym == -1 {
				matchSym = i
			}
		case "identifier":
			if identifierSym == -1 {
				identifierSym = i
			}
		}
	}
	kwTok := gotreesitter.NewLexer(lang.KeywordLexStates, []byte("match")).Next(0)
	kwName := ""
	if int(kwTok.Symbol) < len(lang.SymbolNames) {
		kwName = lang.SymbolNames[kwTok.Symbol]
	}
	t.Logf("rust match diag generated: initial_state=%d keyword_capture=%d keyword_states=%d token_count=%d state_count=%d match=%d identifier=%d keyword_lex_match=%d/%q",
		lang.InitialState, lang.KeywordCaptureToken, len(lang.KeywordLexStates), lang.TokenCount, lang.StateCount, matchSym, identifierSym, kwTok.Symbol, kwName)
	for _, state := range []int{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10} {
		if state >= len(lang.LexModes) {
			continue
		}
		matchAction := uint16(0)
		identifierAction := uint16(0)
		if matchSym >= 0 {
			matchAction = lookupActionIndexForLanguage(lang, gotreesitter.StateID(state), gotreesitter.Symbol(matchSym))
		}
		if identifierSym >= 0 {
			identifierAction = lookupActionIndexForLanguage(lang, gotreesitter.StateID(state), gotreesitter.Symbol(identifierSym))
		}
		mode := lang.LexModes[state]
		t.Logf("rust match diag state=%d lex=%d after_ws=%d match_action=%d identifier_action=%d",
			state, mode.LexStateIndex(), mode.AfterWhitespaceLexStateIndex(), matchAction, identifierAction)
	}
}
func TestRustCorpusMatchLetGuardParity(t *testing.T) {
	jsonPath := rustGrammarJSONPathForTest(t)
	source, err := os.ReadFile(jsonPath)
	if err != nil {
		t.Skipf("Rust grammar.json not available: %v", err)
	}
	gram, err := ImportGrammarJSON(source)
	if err != nil {
		t.Fatalf("import Rust grammar.json: %v", err)
	}
	genLang, err := generateWithTimeout(gram, 90*time.Second)
	if err != nil {
		t.Fatalf("generate Rust language: %v", err)
	}
	refLang := grammars.RustLanguage()
	adaptExternalScanner(refLang, genLang)

	sample := "match x {\n" +
		"    1 => { \"one\" }\n" +
		"    2 => \"two\",\n" +
		"    -1 => 1,\n" +
		"    -3.14 => 3,\n\n" +
		"    #[attr1]\n" +
		"    3 => \"three\",\n" +
		"    macro!(4) => \"four\",\n" +
		"    _ => \"something else\",\n" +
		"}\n\n" +
		"let msg = match x {\n" +
		"    0 | 1 | 10 => \"one of zero, one, or ten\",\n" +
		"    y if y < 20 => \"less than 20, but not zero, one, or ten\",\n" +
		"    y if y == 200 =>\n" +
		"      if a {\n" +
		"        \"200 (but this is not very stylish)\"\n" +
		"      }\n" +
		"    y if let Some(z) = foo && z && let Some(w) = bar => \"very chained\",\n" +
		"    _ => \"something else\",\n" +
		"};\n"

	assertGeneratedAndReferenceDeepParity(t, genLang, refLang, sample)
}

func rustGrammarJSONPathForTest(t *testing.T) string {
	t.Helper()

	candidates := []string{
		"/tmp/grammar_parity/rust/src/grammar.json",
	}
	globs := []string{
		"/tmp/gotreesitter-parity-*/repos/rust/src/grammar.json",
	}
	for _, pattern := range globs {
		matches, err := filepath.Glob(pattern)
		if err == nil && len(matches) > 0 {
			candidates = append(candidates, matches...)
		}
	}
	for _, path := range candidates {
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}
	t.Skip("Rust grammar.json not available")
	return ""
}
