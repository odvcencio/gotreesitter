//go:build cgo && treesitter_c_parity

package cgoharness

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"os"
	"strings"
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	"github.com/odvcencio/gotreesitter/internal/benchfixtures"
	sitter "github.com/tree-sitter/go-tree-sitter"
)

type cooklangNextDivergence struct {
	path     string
	category string
	goValue  string
	cValue   string
}

type cooklangNextPass struct {
	checked   uint64
	run       uint64
	visited   uint64
	rewritten uint64
}

type cooklangNextRoute struct {
	digest     string
	errorRoot  bool
	divergence *cooklangNextDivergence
	pass       *cooklangNextPass
}

type cooklangNextWitness struct {
	name         string
	source       []byte
	sourceSHA    string
	cDigest      string
	routes       map[string]cooklangNextRoute
	cExpectation string
	forest       string
}

// TestCooklangNextLiveArmLockedCRoutes records all five routes for Cooklang.
// It keeps the producer, recovery, scanner, and forest gaps visible.
func TestCooklangNextLiveArmLockedCRoutes(t *testing.T) {
	t.Setenv("GTS_DISPATCHER_CENSUS", "1")
	t.Setenv("GOT_PARSE_PHASE_TIMING", "1")
	language := grammars.CooklangLanguage()
	cLanguage, err := COracleLanguage("cooklang")
	if err != nil {
		t.Fatal(err)
	}

	witnesses := cooklangNextWitnesses(t)
	for _, witness := range witnesses {
		witness := witness
		t.Run(witness.name, func(t *testing.T) {
			if got := fmt.Sprintf("%x", sha256.Sum256(witness.source)); got != witness.sourceSHA {
				t.Fatalf("source SHA-256=%s, want %s", got, witness.sourceSHA)
			}
			cTree := cooklangNextLockedCTree(t, cLanguage, witness.source)
			t.Cleanup(cTree.Close)
			cDigest, err := COracleDeepDigest(cTree)
			if err != nil {
				t.Fatal(err)
			}
			if cDigest != witness.cDigest {
				t.Fatalf("locked C digest=%s, want %s", cDigest, witness.cDigest)
			}

			raw := cooklangNextParseRoute(t, language, witness.source, false, func(p *gotreesitter.Parser, source []byte) (*gotreesitter.Tree, error) {
				return p.ParseNoResultCompatibilityBenchmarkOnly(source)
			})
			cooklangNextAssertRoute(t, "raw", raw, language, cTree, cDigest, witness.routes["raw"])

			production := cooklangNextParseRoute(t, language, witness.source, false, func(p *gotreesitter.Parser, source []byte) (*gotreesitter.Tree, error) {
				return p.Parse(source)
			})
			cooklangNextAssertRoute(t, "production", production, language, cTree, cDigest, witness.routes["production"])

			routedBefore, fallbackBefore := gotreesitter.AdmissionCandidateCounters()
			compactParser := gotreesitter.NewParser(language)
			compactParser.SetAdmissionCandidateRoute(true)
			compact, err := compactParser.Parse(witness.source)
			if err != nil {
				t.Fatalf("compact parse: %v", err)
			}
			t.Cleanup(compact.Release)
			routedAfter, fallbackAfter := gotreesitter.AdmissionCandidateCounters()
			compactRoute := "accepted"
			if fallbackAfter > fallbackBefore {
				compactRoute = "fallback"
				if !strings.Contains(gotreesitter.AdmissionCandidateLastFallbackReason(), "did not accept EOF") {
					t.Fatalf("compact fallback reason=%q", gotreesitter.AdmissionCandidateLastFallbackReason())
				}
			} else if routedAfter != routedBefore+1 || fallbackAfter != fallbackBefore {
				t.Fatalf("compact counters before=(%d,%d) after=(%d,%d)", routedBefore, fallbackBefore, routedAfter, fallbackAfter)
			}
			if compactRoute != witness.cExpectation {
				t.Fatalf("compact route=%s, want %s", compactRoute, witness.cExpectation)
			}
			cooklangNextAssertRoute(t, "compact", compact, language, cTree, cDigest, witness.routes["compact"])

			forestParser := gotreesitter.NewParser(language)
			forest, forestOK := forestParser.ParseForestExperimental(witness.source)
			forestRoute := "declined"
			if forestOK && forest != nil {
				forestRoute = "accepted"
				t.Cleanup(forest.Release)
				cooklangNextAssertRoute(t, "forest", forest, language, cTree, cDigest, witness.routes["forest"])
			} else if witness.forest == "accepted" {
				t.Fatal("forest route declined for an accepted witness")
			}
			if forestRoute != witness.forest {
				t.Fatalf("forest route=%s, want %s", forestRoute, witness.forest)
			}

			incrementalParser := gotreesitter.NewParser(language)
			incrementalParser.SetAdmissionCandidateRoute(false)
			base := bytes.TrimSuffix(witness.source, []byte{'\n'})
			oldTree, err := incrementalParser.Parse(base)
			if err != nil {
				t.Fatalf("incremental base parse: %v", err)
			}
			t.Cleanup(oldTree.Release)
			oldTree.Edit(gotreesitter.InputEdit{
				StartByte:   uint32(len(base)),
				OldEndByte:  uint32(len(base)),
				NewEndByte:  uint32(len(witness.source)),
				StartPoint:  cooklangNextPointAtByte(base),
				OldEndPoint: cooklangNextPointAtByte(base),
				NewEndPoint: cooklangNextPointAtByte(witness.source),
			})
			incremental, profile, err := incrementalParser.ParseIncrementalProfiled(witness.source, oldTree)
			if err != nil {
				t.Fatalf("incremental parse: %v", err)
			}
			t.Cleanup(incremental.Release)
			if profile.OldTreeReuseRoute || !profile.ReuseUnsupported || profile.ReuseUnsupportedReason != "external_scanner_unsupported" || profile.ReusedSubtrees != 0 || profile.ReusedBytes != 0 {
				t.Fatalf("incremental profile=%+v, want external-scanner fallback with zero reuse", profile)
			}
			cooklangNextAssertRoute(t, "incremental", incremental, language, cTree, cDigest, witness.routes["incremental"])
			t.Logf("witness=%s bytes=%d source_sha256=%s c_digest=%s compact=%s forest=%s incremental_reuse=%t unsupported=%t reason=%q", witness.name, len(witness.source), witness.sourceSHA, cDigest, compactRoute, forestRoute, profile.OldTreeReuseRoute, profile.ReuseUnsupported, profile.ReuseUnsupportedReason)
		})
	}
}

// TestCooklangNextLiveArmReceiptDocument guards the blocker receipt markers.
func TestCooklangNextLiveArmReceiptDocument(t *testing.T) {
	raw, err := os.ReadFile("../docs/root-normalization-retirement.md")
	if err != nil {
		t.Fatal(err)
	}
	document := strings.Join(strings.Fields(string(raw)), " ")
	for _, marker := range []string{
		"The Cooklang dispatcher arm remains live.",
		"The A0 (authenticated dispatcher census) manifest has 14 languages, 42 files, and 14 receipts. Cooklang has three files, three checked, three run, 1037 nodes visited, 1021 rewrites, two error roots, and zero parse errors.",
		"The tracked census has seven fixtures across six languages. It omits Cooklang. Its aggregate has nine checked, nine run, 26022 nodes visited, 2107 rewrites, and zero error roots. The authenticated real-corpus census is unavailable because `cgo_harness/corpus_real` is absent.",
		"The three raw-digest controls from rejected pull request (PR) #793 are retained.",
		"Forest declines the three A0 witnesses and the three recovered-recipe controls.",
		"Incremental parsing reports `external_scanner_unsupported` for every witness. It reuses zero subtrees and zero bytes.",
		"Keep dispatch.cooklang live until scheduler_action_semantics emits the locked-C tree on every route.",
	} {
		marker = strings.Join(strings.Fields(marker), " ")
		if !strings.Contains(document, marker) {
			t.Fatalf("Cooklang blocker receipt lacks marker %q", marker)
		}
	}
}

func cooklangNextWitnesses(t *testing.T) []cooklangNextWitness {
	t.Helper()
	pass := func(visited, rewritten uint64) *cooklangNextPass {
		return &cooklangNextPass{checked: 1, run: 1, visited: visited, rewritten: rewritten}
	}
	div := func(path, category, goValue, cValue string) *cooklangNextDivergence {
		return &cooklangNextDivergence{path: path, category: category, goValue: goValue, cValue: cValue}
	}
	allRoutes := func(raw, normalized cooklangNextRoute) map[string]cooklangNextRoute {
		return map[string]cooklangNextRoute{
			"raw": raw, "production": normalized, "compact": normalized,
			"forest": normalized, "incremental": normalized,
		}
	}
	exact := func(digest string, p *cooklangNextPass) cooklangNextRoute {
		return cooklangNextRoute{digest: digest, pass: p}
	}
	divergent := func(digest string, rootError bool, d *cooklangNextDivergence, p *cooklangNextPass) cooklangNextRoute {
		return cooklangNextRoute{digest: digest, errorRoot: rootError, divergence: d, pass: p}
	}
	witnesses := []cooklangNextWitness{
		{
			name: "a0-medium-complex", source: mustCooklangFile(t, "../testdata/dispatcher_census_a0/cooklang/medium__complex_test_recipe.cook"),
			sourceSHA: "6120e9cafce48a745c0f5dade752499883bc2b07230cae93d6a452a788c7ba74", cDigest: "17c3be167109ab72b919cfc700715330f5b0c33e3fdea559ef25f01ae8ad7a1f",
			routes: allRoutes(
				divergent("edc7662c358880c8fd35c182092aa7a15547f9d624bdcb7f31ed30fee1248e36", true, div("/recipe", "shape", "children=22", "children=42"), nil),
				divergent("585034f453f485b2c3ccc7cf20e204d0dd210465e871d3e449fe1547e5684d87", true, div("/recipe", "shape", "children=23", "children=42"), pass(450, 442)),
			), cExpectation: "fallback", forest: "declined",
		},
		{
			name: "a0-medium-frontmatter", source: mustCooklangFile(t, "../testdata/dispatcher_census_a0/cooklang/medium__frontmatter_test_recipe.cook"),
			sourceSHA: "956fcaf1c14e0e915efded324450789cb5d9a896cf60e4feafb71b918c8e621a", cDigest: "cc2dc5fd1f73c2def6c0f7370a431c795a229506d9ad7e762243382f95cf8327",
			routes: allRoutes(
				divergent("76560c38e365ea67b8f84f4f2c5b36b760df71daa880803c4a1f610ce797d7fc", true, div("/recipe", "shape", "children=36", "children=47"), nil),
				divergent("3f433923a14de13456e84914cab679db1e59bc5c7e6cb6e13e2ec9168b4f5eaf", true, div("/recipe", "shape", "children=35", "children=47"), pass(289, 287)),
			), cExpectation: "fallback", forest: "declined",
		},
		{
			name: "a0-medium-recipe", source: mustCooklangFile(t, "../testdata/dispatcher_census_a0/cooklang/medium__test_recipe.cook"),
			sourceSHA: "1acb11626700218ebc8ff8b7d445e1a257b12af35dea7dbc0fcef0587a79468f", cDigest: "ff61d92143c45498b4db19a499414be137894e6c3d5b71c6917140edc8eb3b0b",
			routes: allRoutes(
				divergent("f2992b217d2bef245959891d67abbca56b91252f683f3f64170eecda9e85ee94", true, div("/recipe", "shape", "children=34", "children=43"), nil),
				divergent("83a5757e3dc00f198115e21f8a34b368a3f4da118764ba899300fe47bce23189", false, div("/recipe", "error", "false", "true"), pass(298, 292)),
			), cExpectation: "fallback", forest: "declined",
		},
		{
			name: "pr793-punctuation", source: []byte("Add @salt{1%tsp}.\n"),
			sourceSHA: "8dd8b584db0c0ef919fdcc229645c2cb5d7697c7f555624b3c075e7f1a4eb53a", cDigest: "c6e4535b725516550ca7a0ee4c69974799c2d2d10fed4e5f1ba6b71e43c5ba8a",
			routes: allRoutes(
				divergent("0e6880ec4902576c2a6de014424c3cba7eef99cdc5fd8fded8ceb6382a6df9cd", true, div("/recipe", "error", "true", "false"), nil),
				exact("c6e4535b725516550ca7a0ee4c69974799c2d2d10fed4e5f1ba6b71e43c5ba8a", pass(13, 4)),
			), cExpectation: "fallback", forest: "declined",
		},
		{
			name: "pr793-recovered", source: []byte("---\nservings: 4\nemoji: 🥟\ntags: warm, fried, starter\n---\n\nServe hot.\n"),
			sourceSHA: "8fefd1eb97742b1ef8349e9e51b5260c28295ed0d583d12eb5fbc04db579ce8a", cDigest: "3ae5ffba70cd0922976d24ed3e4d254cbb9d356639e8485b8b4b3abdc2667133",
			routes: allRoutes(
				divergent("896d9f79d941c3869dca7b855bae45738392d02519f0fbe3ac45cc2623fcfa2f", true, div("/recipe", "shape", "children=6", "children=7"), nil),
				divergent("814240a8aff9c3e253b37ce1ff535ff2cd96510c6afea40680a270405699967f", true, div("/recipe", "shape", "children=5", "children=7"), pass(20, 18)),
			), cExpectation: "fallback", forest: "declined",
		},
		{
			name: "pr793-without-newline", source: []byte("Add @salt{1%tsp}."),
			sourceSHA: "6d60ac3d4e9155e84ead7b1f9e751728dcebafe03d6561d913f3f18f58a14297", cDigest: "dd3692a1a0e9145af9f2d082126a1e798d60cbe74942427746d3f0e83bd31e1c",
			routes: allRoutes(
				divergent("f49ca1a85a0b2ee7ed7f07993d6bc8b103d66311f83d4a026e085cf2013a69ec", true, div("/recipe", "error", "true", "false"), nil),
				exact("dd3692a1a0e9145af9f2d082126a1e798d60cbe74942427746d3f0e83bd31e1c", pass(13, 4)),
			), cExpectation: "fallback", forest: "declined",
		},
		{
			name: "clean-control", source: []byte("Add @salt{1%tsp}\n"),
			sourceSHA: "c31fe39c742b150ec7daec33319e21b39511207fa10fe97d04deac94a31db43e", cDigest: "5bbe67b1d36f2d57144a29d329e44cd39b5a90f6191ed0fc05f307ce983e67c2",
			routes:       cooklangNextControlRoutes("5bbe67b1d36f2d57144a29d329e44cd39b5a90f6191ed0fc05f307ce983e67c2", 11, 1),
			cExpectation: "accepted", forest: "accepted",
		},
		{
			name: "no-op-ingredient-control", source: []byte("Add @salt{}\n"),
			sourceSHA: "e28998769b7ac8e1f8f615bd7ee4e8f1dff629890201c96abba1b2a60d70362b", cDigest: "ed03c94ecef09c4724f5a4931b33d9d0b476c85a49b5121ad3dc0f39cc944461",
			routes:       cooklangNextControlRoutes("ed03c94ecef09c4724f5a4931b33d9d0b476c85a49b5121ad3dc0f39cc944461", 7, 1),
			cExpectation: "accepted", forest: "accepted",
		},
		{
			name: "malformed-frontmatter", source: []byte("---\nservings:\n---\nServe hot.\n"),
			sourceSHA: "f2c2b1b02d9b3497d42c1cc53b94373974bd7b525af438d7dcf73391725f3615", cDigest: "b7b9d810d3d03756017e082edb333785c5fa196fe46a2f69d34191eeec6b9cf0",
			routes: allRoutes(
				divergent("21557aa2d5b8757be18c010fb673b8611171dcd7232784565b11113c741e3092", true, div("/recipe", "error", "true", "false"), nil),
				exact("b7b9d810d3d03756017e082edb333785c5fa196fe46a2f69d34191eeec6b9cf0", pass(13, 11)),
			), cExpectation: "fallback", forest: "declined",
		},
	}
	return witnesses
}

func cooklangNextControlRoutes(digest string, visited, rewritten uint64) map[string]cooklangNextRoute {
	pass := &cooklangNextPass{checked: 1, run: 1, visited: visited, rewritten: rewritten}
	return map[string]cooklangNextRoute{
		"raw":         {digest: digest},
		"production":  {digest: digest, pass: pass},
		"compact":     {digest: digest},
		"forest":      {digest: digest, pass: pass},
		"incremental": {digest: digest, pass: pass},
	}
}

func cooklangNextLockedCTree(t *testing.T, language *sitter.Language, source []byte) *sitter.Tree {
	t.Helper()
	parser := sitter.NewParser()
	t.Cleanup(parser.Close)
	if err := parser.SetLanguage(language); err != nil {
		t.Fatal(err)
	}
	tree := parser.Parse(source, nil)
	if tree == nil || tree.RootNode() == nil {
		t.Fatal("locked C parser returned no root")
	}
	return tree
}

func cooklangNextParseRoute(t *testing.T, language *gotreesitter.Language, source []byte, admission bool, parse func(*gotreesitter.Parser, []byte) (*gotreesitter.Tree, error)) *gotreesitter.Tree {
	t.Helper()
	parser := gotreesitter.NewParser(language)
	parser.SetAdmissionCandidateRoute(admission)
	tree, err := parse(parser, source)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(tree.Release)
	return tree
}

func cooklangNextAssertRoute(t *testing.T, route string, tree *gotreesitter.Tree, language *gotreesitter.Language, cTree *sitter.Tree, cDigest string, want cooklangNextRoute) {
	t.Helper()
	root := tree.RootNode()
	if root == nil || root.HasError() != want.errorRoot {
		t.Fatalf("%s root error=%t, want %t", route, root != nil && root.HasError(), want.errorRoot)
	}
	inspection, err := benchfixtures.InspectGoTree(root, language)
	if err != nil {
		t.Fatalf("%s inspect: %v", route, err)
	}
	if inspection.SHA256 != want.digest {
		t.Fatalf("%s digest=%s, want %s", route, inspection.SHA256, want.digest)
	}
	diff := FirstDivergenceDumpV1(root, language, cTree.RootNode())
	if want.divergence == nil {
		if diff != nil || inspection.SHA256 != cDigest {
			t.Fatalf("%s locked-C divergence=%+v digest=%s C=%s", route, diff, inspection.SHA256, cDigest)
		}
	} else if diff == nil || diff.Path != want.divergence.path || diff.Category != want.divergence.category || diff.GoValue != want.divergence.goValue || diff.CValue != want.divergence.cValue {
		t.Fatalf("%s divergence=%+v, want %+v", route, diff, want.divergence)
	}
	if got := cooklangNextPassReceipt(tree, "dispatch.cooklang"); want.pass == nil {
		if got != nil {
			t.Fatalf("%s unexpected dispatch receipt=%+v", route, got)
		}
	} else {
		if got == nil || *got != *want.pass {
			t.Fatalf("%s dispatch.cooklang receipt=%+v, want %+v", route, got, want.pass)
		}
		if recovered := cooklangNextPassReceipt(tree, "dispatch.cooklang.recovered-recipe"); recovered == nil || *recovered != *want.pass {
			t.Fatalf("%s recovered-recipe receipt=%+v, want %+v", route, recovered, want.pass)
		}
	}
	t.Logf("route=%s error=%t digest=%s c_digest=%s divergence=%+v dispatch=%+v", route, root.HasError(), inspection.SHA256, cDigest, diff, want.pass)
}

func cooklangNextPassReceipt(tree *gotreesitter.Tree, name string) *cooklangNextPass {
	if tree == nil || tree.ParseRuntime().NormalizationPasses == nil {
		return nil
	}
	for _, pass := range *tree.ParseRuntime().NormalizationPasses {
		if pass.Name == name {
			return &cooklangNextPass{checked: pass.Checked, run: pass.Run, visited: pass.NodesVisited, rewritten: pass.NodesRewritten}
		}
	}
	return nil
}

func cooklangNextPointAtByte(source []byte) gotreesitter.Point {
	var point gotreesitter.Point
	for _, value := range source {
		if value == '\n' {
			point.Row++
			point.Column = 0
		} else {
			point.Column++
		}
	}
	return point
}

func mustCooklangFile(t *testing.T, path string) []byte {
	t.Helper()
	source, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return source
}
