package gotreesitter_test

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	"github.com/odvcencio/gotreesitter/internal/benchfixtures"
)

type rustDispatchProbeWitness struct {
	name string
	src  []byte
}

func TestRustDispatchRawProductionAndRouteProbe(t *testing.T) {
	t.Setenv("GTS_DISPATCHER_CENSUS", "1")
	fixtures := []rustDispatchProbeWitness{
		{name: "ownership-registry-smoke", src: []byte(grammars.ParseSmokeSample("rust"))},
		{name: "tracked-census-rust_ast.rs", src: mustRustProbeFile(t, "testdata/incremental_gate/rust_ast.rs")},
		{name: "admission-depth", src: []byte("fn main() {\n    let values = [1, 2, 3];\n    println!(\"{}\", values.len());\n}\n")},
		{name: "token-tree", src: []byte("fn main() {\n    let v = vec![1 + 2, 3 * 4, 5 - 6, a & b, c | d];\n    println!(\"{} {}\", x && y, p || q);\n    assert_eq!(lhs << 2, rhs >> 1);\n    my_macro!(a : b, c => d, e $ f, g ? h);\n    nested!(inner!(deep!(x % y ^ z, !flag)));\n    paths!(std::collections::HashMap, core::mem);\n    mixed! { key: value; arr[idx] = func(arg) + 1 }\n}\n")},
		{name: "recovered-impl-item", src: []byte("pub type ExplicitSelf = Spanned<SelfKind>;\n\nimpl Arg {\n    pub fn to_self(&self) -> Option<ExplicitSelf> {\n        if let PatKind::Ident(BindingMode::ByValue(mutbl), ident, _) = self.pat.node {\n            if ident.node.name == keywords::SelfValue.name() {\n                return match self.ty.node {\n                    TyKind::ImplicitSelf => Some(respan(self.pat.span, SelfKind::Value(mutbl))),\n                    _ => None,\n                };\n            }\n        }\n        None\n    }\n}\n")},
		{name: "doc-comment", src: []byte("//! crate docs\n/// item docs\nfn f() {}\n")},
		{name: "token-binding", src: []byte("macro_rules! m { ($e:expr) => {} }\n")},
		{name: "pattern-statement", src: []byte("if let A(x) | B(x) = expr {\n    do_stuff_with(x);\n}\n")},
		{name: "struct-expression", src: []byte("let a = SomeStruct { field1, field2: expression, field3, };\n")},
	}

	lang := grammars.RustLanguage()
	for _, witness := range fixtures {
		witness := witness
		t.Run(witness.name, func(t *testing.T) {
			sourceSHA := sha256.Sum256(witness.src)
			rawParser := gotreesitter.NewParser(lang)
			raw, err := rawParser.ParseNoResultCompatibilityBenchmarkOnly(witness.src)
			if err != nil {
				t.Fatalf("raw parse: %v", err)
			}
			defer raw.Release()
			productionParser := gotreesitter.NewParser(lang)
			productionParser.SetAdmissionCandidateRoute(false)
			production, err := productionParser.Parse(witness.src)
			if err != nil {
				t.Fatalf("production parse: %v", err)
			}
			defer production.Release()
			rawInspection, err := benchfixtures.InspectGoTree(raw.RootNode(), lang)
			if err != nil {
				t.Fatal(err)
			}
			productionInspection, err := benchfixtures.InspectGoTree(production.RootNode(), lang)
			if err != nil {
				t.Fatal(err)
			}
			rawRuntime := raw.ParseRuntime()
			productionRuntime := production.ParseRuntime()
			t.Logf("witness=%s bytes=%d source_sha256=%s raw_digest=%s production_digest=%s raw_rewrites=%d production_rewrites=%d raw_errors=%t production_errors=%t production_runtime=%s", witness.name, len(witness.src), hex.EncodeToString(sourceSHA[:]), rawInspection.SHA256, productionInspection.SHA256, rawRuntime.NormalizationNodesRewritten, productionRuntime.NormalizationNodesRewritten, raw.RootNode().HasError(), production.RootNode().HasError(), productionRuntime.Summary())
			if rawInspection.SHA256 != productionInspection.SHA256 {
				t.Errorf("raw and production deep digests differ: raw=%s production=%s", rawInspection.SHA256, productionInspection.SHA256)
			}
			logRustDispatchPass(t, "production", productionRuntime)

			routeSource := append(append([]byte(nil), witness.src...), '\n')
			routeProduction := parseRustProbeProduction(t, lang, routeSource)
			defer routeProduction.Release()
			routeProductionInspection, err := benchfixtures.InspectGoTree(routeProduction.RootNode(), lang)
			if err != nil {
				t.Fatal(err)
			}
			logRustRoute(t, "production-route", routeProduction, routeProductionInspection.SHA256)

			compactParser := gotreesitter.NewParser(lang)
			compactParser.SetAdmissionCandidateRoute(true)
			compact, err := compactParser.Parse(routeSource)
			if err != nil {
				t.Fatalf("compact route: %v", err)
			}
			defer compact.Release()
			compactInspection, err := benchfixtures.InspectGoTree(compact.RootNode(), lang)
			if err != nil {
				t.Fatal(err)
			}
			routed, fallback := gotreesitter.AdmissionCandidateCounters()
			t.Logf("route=compact candidate_routed=%d candidate_fallback=%d fallback_reason=%q digest=%s errors=%t runtime=%s", routed, fallback, gotreesitter.AdmissionCandidateLastFallbackReason(), compactInspection.SHA256, compact.RootNode().HasError(), compact.ParseRuntime().Summary())
			logRustDispatchPass(t, "compact", compact.ParseRuntime())
			if compactInspection.SHA256 != routeProductionInspection.SHA256 {
				t.Errorf("compact digest=%s, want production-route %s", compactInspection.SHA256, routeProductionInspection.SHA256)
			}

			forestParser := gotreesitter.NewParser(lang)
			forest, forestOK := forestParser.ParseForestExperimental(routeSource)
			if !forestOK || forest == nil {
				offset, symbol, reason, states := forestParser.ForestDeclineInfo()
				t.Logf("route=forest outcome=declined offset=%d symbol=%d reason=%q states=%v", offset, symbol, reason, states)
			} else {
				defer forest.Release()
				forestInspection, inspectErr := benchfixtures.InspectGoTree(forest.RootNode(), lang)
				if inspectErr != nil {
					t.Fatal(inspectErr)
				}
				t.Logf("route=forest outcome=accepted digest=%s errors=%t runtime=%s", forestInspection.SHA256, forest.RootNode().HasError(), forest.ParseRuntime().Summary())
				logRustDispatchPass(t, "forest", forest.ParseRuntime())
				if forestInspection.SHA256 != routeProductionInspection.SHA256 {
					t.Errorf("forest digest=%s, want production-route %s", forestInspection.SHA256, routeProductionInspection.SHA256)
				}
			}

			oldTree := parseRustProbeProduction(t, lang, witness.src)
			defer oldTree.Release()
			oldEnd := rustProbePointAtByte(witness.src, len(witness.src))
			oldTree.Edit(gotreesitter.InputEdit{StartByte: uint32(len(witness.src)), OldEndByte: uint32(len(witness.src)), NewEndByte: uint32(len(routeSource)), StartPoint: oldEnd, OldEndPoint: oldEnd, NewEndPoint: rustProbePointAtByte(routeSource, len(routeSource))})
			incrementalParser := gotreesitter.NewParser(lang)
			incremental, profile, err := incrementalParser.ParseIncrementalProfiled(routeSource, oldTree)
			if err != nil {
				t.Fatalf("incremental route: %v", err)
			}
			defer incremental.Release()
			incrementalInspection, err := benchfixtures.InspectGoTree(incremental.RootNode(), lang)
			if err != nil {
				t.Fatal(err)
			}
			t.Logf("route=incremental digest=%s errors=%t profile=%+v runtime=%s", incrementalInspection.SHA256, incremental.RootNode().HasError(), profile, incremental.ParseRuntime().Summary())
			logRustDispatchPass(t, "incremental", incremental.ParseRuntime())
			if incrementalInspection.SHA256 != routeProductionInspection.SHA256 {
				t.Errorf("incremental digest=%s, want production-route %s", incrementalInspection.SHA256, routeProductionInspection.SHA256)
			}
		})
	}
}

// TestRustDispatchRewriteTraceExpandedCorpus expands the receipt beyond the
// registered smoke and tracked witnesses. It records every Rust arm rewrite.
func TestRustDispatchRewriteTraceExpandedCorpus(t *testing.T) {
	t.Setenv("GTS_DISPATCHER_CENSUS", "1")
	fixtures := []rustDispatchProbeWitness{
		{name: "admission-direct-external-payload", src: mustRustProbeFile(t, "testdata/admission_direct/external_payload/rust.rs")},
		{name: "outline-rust-lib", src: mustRustProbeFile(t, "testdata/outline/rust/lib.rs")},
		{name: "parity-lifetime-and-abstract-types", src: []byte("fn main() {}\n\nfn add(x: i32, y: i32) -> i32 { return x + y; }\n\nfn foo(x: impl FnOnce() -> result::Result<T, E>) {}\n\nfn foo(bar: impl for<'a> Baz<Quux<'a>>) {}\n")},
		{name: "parity-pattern-statements", src: []byte("if let A(x) | B(x) = expr { do_stuff_with(x); }\nwhile let A(x) | B(x) = expr { do_stuff_with(x); }\nlet Ok(index) | Err(index) = slice.binary_search(&x);\nfor A | B | C in c {}\n")},
		{name: "parity-macro-invocations", src: []byte("a!(* a *);\na!(& a &);\na!(- a -);\na!(b + c + +);\na!('a'..='z');\na!($);\na!($());\na!($ a $);\na!(${$([ a ])});\na!($a $a:ident $($a);*);\n")},
		{name: "parity-weird-expressions", src: []byte("fn angrydome() {\n    loop { if break { } }\n    let mut i = 0;\n    loop { i += 1; if i == 1 { match (continue) { 1 => { }, _ => panic!(\"wat\") } }\n      break; }\n}\n\nfn special_characters() {\n    let val = !((|(..):(_,_),(|__@_|__)|__)((&*\"\\\\\",'🤔')/**/,{})=={&[..=..][..];})//\n    ;\n    assert!(!val);\n}\n\nfn function() {\n    struct foo;\n    impl Deref for foo {\n        type Target = fn() -> Self;\n        fn deref(&self) -> &Self::Target {\n            &((|| foo) as _)\n        }\n    }\n    let foo = foo () ()() ()()() ()()()() ()()()()();\n}\n\nfn closure_matching() {\n    let x = |_| Some(1);\n    let (|x| x) = match x(..) {\n        |_| Some(2) => |_| Some(3),\n        |_| _ => unreachable!(),\n    };\n    assert!(matches!(x(..), |_| Some(4)));\n}\n")},
		{name: "parity-weird-top-level", src: []byte("// Just a grab bag of stuff that you would not want to write.\n\nfn strange() -> bool { let _x: bool = return true; }\n\nfn what() {\n    fn the(x: &Cell<bool>) {\n        return while !x.get() { x.set(true); };\n    }\n    let i = &Cell::new(false);\n    let dont = {||the(i)};\n    dont();\n    assert!((i.get()));\n}\n\nfn punch_card() -> impl std::fmt::Debug {\n    ..=..=.. ..    .. .. .. ..    .. .. .. ..    .. ..=.. ..\n    ..=.. ..=..    .. .. .. ..    .. .. .. ..    ..=..=..=..\n    ..=.. ..=..    ..=.. ..=..    .. ..=..=..    .. ..=.. ..\n    ..=..=.. ..    ..=.. ..=..    ..=.. .. ..    .. ..=.. ..\n    ..=.. ..=..    ..=.. ..=..    .. ..=.. ..    .. ..=.. ..\n    ..=.. ..=..    ..=.. ..=..    .. .. ..=..    .. ..=.. ..\n    ..=.. ..=..    .. ..=..=..    ..=..=.. ..    .. ..=.. ..\n}\n")},
	}
	lang := grammars.RustLanguage()
	for _, fixture := range fixtures {
		fixture := fixture
		t.Run(fixture.name, func(t *testing.T) {
			rawParser := gotreesitter.NewParser(lang)
			raw, err := rawParser.ParseNoResultCompatibilityBenchmarkOnly(fixture.src)
			if err != nil {
				t.Fatalf("raw parse: %v", err)
			}
			defer raw.Release()
			productionParser := gotreesitter.NewParser(lang)
			productionParser.SetAdmissionCandidateRoute(false)
			production, err := productionParser.Parse(fixture.src)
			if err != nil {
				t.Fatalf("production parse: %v", err)
			}
			defer production.Release()
			rawInspection, err := benchfixtures.InspectGoTree(raw.RootNode(), lang)
			if err != nil {
				t.Fatal(err)
			}
			productionInspection, err := benchfixtures.InspectGoTree(production.RootNode(), lang)
			if err != nil {
				t.Fatal(err)
			}
			t.Logf("expanded=%s bytes=%d raw_digest=%s production_digest=%s raw_rewrites=%d production_rewrites=%d raw_errors=%t production_errors=%t", fixture.name, len(fixture.src), rawInspection.SHA256, productionInspection.SHA256, raw.ParseRuntime().NormalizationNodesRewritten, production.ParseRuntime().NormalizationNodesRewritten, raw.RootNode().HasError(), production.RootNode().HasError())
			logRustDispatchPass(t, "expanded-production", production.ParseRuntime())
			if rawInspection.SHA256 != productionInspection.SHA256 {
				t.Logf("expanded raw/production deep mismatch: raw=%s production=%s", rawInspection.SHA256, productionInspection.SHA256)
			}
		})
	}
}

// TestRustDispatchRewriteTraceMalformedCorpus probes the recovery-only paths
// that the clean corpus does not reach. Keep this diagnostic receipt separate.
func TestRustDispatchRewriteTraceMalformedCorpus(t *testing.T) {
	t.Setenv("GTS_DISPATCHER_CENSUS", "1")
	fixtures := []rustDispatchProbeWitness{
		{name: "malformed-function-impl-type", src: []byte("fn foo(bar: impl for<'a> Baz<Quux<'a>>) {\n")},
		{name: "malformed-top-level-impl", src: []byte("impl Arg {\n    pub fn f(&self) -> bool { self.value }\n")},
		{name: "malformed-let-closure", src: []byte("fn f() { let x = (|a| a + 1; }\n")},
		{name: "malformed-token-tree", src: []byte("macro_rules! m { ($e:expr => {\n")},
		{name: "malformed-pattern-statement", src: []byte("if let A(x) | B(x) = expr {\n")},
		{name: "malformed-struct-expression", src: []byte("let a = SomeStruct { field1, field2: expression\n")},
		{name: "malformed-doc-comment", src: []byte("/// docs\nfn f(\n")},
	}
	lang := grammars.RustLanguage()
	for _, fixture := range fixtures {
		fixture := fixture
		t.Run(fixture.name, func(t *testing.T) {
			rawParser := gotreesitter.NewParser(lang)
			raw, err := rawParser.ParseNoResultCompatibilityBenchmarkOnly(fixture.src)
			if err != nil {
				t.Fatalf("raw parse: %v", err)
			}
			defer raw.Release()
			productionParser := gotreesitter.NewParser(lang)
			productionParser.SetAdmissionCandidateRoute(false)
			production, err := productionParser.Parse(fixture.src)
			if err != nil {
				t.Fatalf("production parse: %v", err)
			}
			defer production.Release()
			rawInspection, err := benchfixtures.InspectGoTree(raw.RootNode(), lang)
			if err != nil {
				t.Fatal(err)
			}
			productionInspection, err := benchfixtures.InspectGoTree(production.RootNode(), lang)
			if err != nil {
				t.Fatal(err)
			}
			t.Logf("malformed=%s bytes=%d raw_digest=%s production_digest=%s raw_rewrites=%d production_rewrites=%d raw_errors=%t production_errors=%t", fixture.name, len(fixture.src), rawInspection.SHA256, productionInspection.SHA256, raw.ParseRuntime().NormalizationNodesRewritten, production.ParseRuntime().NormalizationNodesRewritten, raw.RootNode().HasError(), production.RootNode().HasError())
			logRustDispatchPass(t, "malformed-production", production.ParseRuntime())
			if rawInspection.SHA256 != productionInspection.SHA256 {
				t.Logf("malformed raw/production deep mismatch: raw=%s production=%s", rawInspection.SHA256, productionInspection.SHA256)
			}
		})
	}
}

func mustRustProbeFile(t *testing.T, path string) []byte {
	t.Helper()
	source, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return source
}

func parseRustProbeProduction(t *testing.T, lang *gotreesitter.Language, source []byte) *gotreesitter.Tree {
	t.Helper()
	parser := gotreesitter.NewParser(lang)
	parser.SetAdmissionCandidateRoute(false)
	tree, err := parser.Parse(source)
	if err != nil {
		t.Fatalf("production parse: %v", err)
	}
	return tree
}

func logRustDispatchPass(t *testing.T, route string, runtime gotreesitter.ParseRuntime) {
	t.Helper()
	if runtime.NormalizationPasses == nil {
		t.Logf("route=%s dispatch.rust=absent", route)
		return
	}
	for _, pass := range *runtime.NormalizationPasses {
		if pass.Name == "dispatch.rust" || pass.Name == "rust_source_file_root_pre" || pass.Name == "rust_recovered_pattern_statements_root" || pass.Name == "rust_recovered_function_items" || pass.Name == "rust_recovered_struct_expression_root" || pass.Name == "rust_token_binding_patterns" || pass.Name == "rust_source_file_root_post" || pass.Name == "rust_doc_comment_ranges" {
			t.Logf("route=%s pass=%s checked=%d run=%d visited=%d rewritten=%d", route, pass.Name, pass.Checked, pass.Run, pass.NodesVisited, pass.NodesRewritten)
		}
	}
}

func logRustRoute(t *testing.T, route string, tree *gotreesitter.Tree, digest string) {
	t.Helper()
	t.Logf("route=%s digest=%s errors=%t runtime=%s", route, digest, tree.RootNode().HasError(), tree.ParseRuntime().Summary())
	logRustDispatchPass(t, route, tree.ParseRuntime())
}

func rustProbePointAtByte(source []byte, offset int) gotreesitter.Point {
	if offset < 0 || offset > len(source) {
		panic(fmt.Sprintf("point offset %d outside source length %d", offset, len(source)))
	}
	var point gotreesitter.Point
	for _, value := range source[:offset] {
		if value == '\n' {
			point.Row++
			point.Column = 0
		} else {
			point.Column++
		}
	}
	return point
}
