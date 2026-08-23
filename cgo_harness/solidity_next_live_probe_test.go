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

type solidityNextWitness struct {
	name                    string
	source                  []byte
	sourceSHA256            string
	wantError               bool
	wantForest              bool
	wantCompact             string
	wantC                   string
	wantRaw                 string
	wantProduction          string
	wantCompactDigest       string
	wantForestDigest        string
	wantIncremental         string
	wantRawDiff             *DumpV1Divergence
	wantProductionDiff      *DumpV1Divergence
	wantCompactDiff         *DumpV1Divergence
	wantForestDiff          *DumpV1Divergence
	wantIncrementalDiff     *DumpV1Divergence
	wantRawDispatch         string
	wantProductionDispatch  string
	wantCompactDispatch     string
	wantForestDispatch      string
	wantIncrementalDispatch string
	wantReusedSubtrees      uint64
	wantReusedBytes         uint64
}

// TestSolidityNextLiveArmLockedCRoutes records all required Solidity routes.
func TestSolidityNextLiveArmLockedCRoutes(t *testing.T) {
	t.Setenv("GTS_DISPATCHER_CENSUS", "1")
	t.Setenv("GOT_PARSE_PHASE_TIMING", "1")
	goLanguage := grammars.SolidityLanguage()
	cLanguage, err := COracleLanguage("solidity")
	if err != nil {
		t.Fatal(err)
	}
	memberDiff := &DumpV1Divergence{
		Path:     "/source_file/contract_declaration[0]/contract_body[2]/function_definition[1]/function_body[8]/statement[1]/return_statement[0]/expression[1]/member_expression[0]/expression[0]",
		Category: "type",
		GoValue:  "expression",
		CValue:   "identifier",
	}
	callDiff := &DumpV1Divergence{
		Path:     "/source_file/contract_declaration[0]/contract_body[2]/function_definition[1]/function_body[8]/statement[1]/return_statement[0]/expression[1]/call_expression[0]",
		Category: "type",
		GoValue:  "call_expression",
		CValue:   "type_cast_expression",
	}
	initializableDiff := &DumpV1Divergence{
		Path:     "/source_file/contract_declaration[4]/contract_body[3]/modifier_definition[12]/function_body[4]/statement[4]/variable_declaration_statement[0]/expression[2]/member_expression[0]",
		Category: "type",
		GoValue:  "member_expression",
		CValue:   "unary_expression",
	}
	initializableForestDiff := &DumpV1Divergence{
		Path:     "/source_file/contract_declaration[4]/contract_body[3]/modifier_definition[12]/function_body[4]/statement[2]/variable_declaration_statement[0]/expression[2]/call_expression[0]/expression[0]/_primary_expression[0]",
		Category: "type",
		GoValue:  "_primary_expression",
		CValue:   "identifier",
	}
	packingForestDiff := &DumpV1Divergence{
		Path:     "/source_file/library_declaration[6]/contract_body[2]/function_definition[58]/function_body[10]/statement[1]/if_statement[0]/expression[2]/binary_expression[0]/expression[0]/_primary_expression[0]",
		Category: "type",
		GoValue:  "_primary_expression",
		CValue:   "identifier",
	}
	malformedDiff := &DumpV1Divergence{
		Path:     "/source_file/contract_declaration[0]/contract_body[2]/function_definition[1]/function_body[8]/statement[1]/return_statement[0]",
		Category: "shape",
		GoValue:  "children=3",
		CValue:   "children=4",
	}
	positiveForestDiff := &DumpV1Divergence{
		Path:     "/source_file/contract_declaration[0]/contract_body[2]/function_definition[1]/function_body[8]/statement[1]/return_statement[0]/expression[1]/_primary_expression[0]",
		Category: "type",
		GoValue:  "_primary_expression",
		CValue:   "identifier",
	}
	noActionFallback := "fallback:compact route declined at no_action [mechanism=scheduler-frontier-shape]: converged-path reduction split no-action drop descends from an unproved historical boundary resurrection"
	recoveryFallback := "fallback:compact route declined at recovery [mechanism=recovery-entered]: did not accept EOF: generic scheduler has no table action for the elected token"
	witnesses := []solidityNextWitness{
		{
			name: "a0-small-IERC3156", source: mustSolidityFile(t, "../testdata/dispatcher_census_a0/solidity/small__IERC3156.sol"), sourceSHA256: "9fbd10c6970c328f348c9a86604bdad336743caeda2547f94b6a86d8a906c961",
			wantC: "e930abf94bedfcdfaaade28d76373c3ed9b2587fb075d800c1de3357320ce415", wantRaw: "e930abf94bedfcdfaaade28d76373c3ed9b2587fb075d800c1de3357320ce415", wantProduction: "e930abf94bedfcdfaaade28d76373c3ed9b2587fb075d800c1de3357320ce415", wantCompactDigest: "e930abf94bedfcdfaaade28d76373c3ed9b2587fb075d800c1de3357320ce415", wantForestDigest: "e930abf94bedfcdfaaade28d76373c3ed9b2587fb075d800c1de3357320ce415", wantIncremental: "e930abf94bedfcdfaaade28d76373c3ed9b2587fb075d800c1de3357320ce415", wantCompact: "accepted", wantForest: true,
			wantRawDispatch: "none", wantProductionDispatch: "none", wantCompactDispatch: "none", wantForestDispatch: "1/1/31/0", wantIncrementalDispatch: "1/1/31/0", wantReusedSubtrees: 15, wantReusedBytes: 179,
		},
		{
			name: "a0-medium-Initializable", source: mustSolidityFile(t, "../testdata/dispatcher_census_a0/solidity/medium__Initializable.sol"), sourceSHA256: "f527a063813c2bf60c153fb08e38539578935402894fcc36fac42324ca325d3b",
			wantC: "9c73deee203b676abf35a10a7dfa02c6ed90ee21209f9745bcb0256fd935526f", wantRaw: "b38a5f0babca0fec5a4b6c6fad6169ad0f201e0606f8400553ca2034e731c8dd", wantProduction: "8f424e55a8dc92e0e3f8d5e7408c0a15120881a5815255a35256fb8ecd188083", wantCompactDigest: "8f424e55a8dc92e0e3f8d5e7408c0a15120881a5815255a35256fb8ecd188083", wantForestDigest: "e7f3c1838b6d50dbcf9c94241f068a282e34cd7dc46111bd2968560cf32ef512", wantIncremental: "8f424e55a8dc92e0e3f8d5e7408c0a15120881a5815255a35256fb8ecd188083", wantRawDiff: initializableDiff, wantProductionDiff: initializableDiff, wantCompactDiff: initializableDiff, wantForestDiff: initializableForestDiff, wantIncrementalDiff: initializableDiff, wantCompact: noActionFallback, wantForest: true,
			wantRawDispatch: "none", wantProductionDispatch: "1/1/798/666", wantCompactDispatch: "1/1/798/666", wantForestDispatch: "1/1/817/604", wantIncrementalDispatch: "1/1/798/666", wantReusedSubtrees: 46, wantReusedBytes: 840,
		},
		{
			name: "a0-large-Packing", source: mustSolidityFile(t, "../testdata/dispatcher_census_a0/solidity/large__Packing.sol"), sourceSHA256: "766829f6d9758a1318dd009143912d7aa6bbafa4f4b2a137c94d7f81a73b38ac",
			wantC: "7ebe5bde35327a5138ff647e0b0d3d807c8ee33fb8db2589ef1196fdea5ee6e8", wantRaw: "7ebe5bde35327a5138ff647e0b0d3d807c8ee33fb8db2589ef1196fdea5ee6e8", wantProduction: "7ebe5bde35327a5138ff647e0b0d3d807c8ee33fb8db2589ef1196fdea5ee6e8", wantCompactDigest: "7ebe5bde35327a5138ff647e0b0d3d807c8ee33fb8db2589ef1196fdea5ee6e8", wantForestDigest: "7c1d74398a8a9023f2aabc44c8274cdd752a73c043362314ce4addf0e264ad82", wantIncremental: "7ebe5bde35327a5138ff647e0b0d3d807c8ee33fb8db2589ef1196fdea5ee6e8", wantForestDiff: packingForestDiff, wantCompact: "fallback:compact route error: parser-core phase zero: shared (101,1721) live-link cap exceeded: 9 > 8", wantForest: true,
			wantRawDispatch: "none", wantProductionDispatch: "1/1/26068/0", wantCompactDispatch: "1/1/26068/0", wantForestDispatch: "1/1/26458/0", wantIncrementalDispatch: "1/1/26068/0", wantReusedSubtrees: 6898, wantReusedBytes: 26115,
		},
		{
			name: "clean-member", source: []byte("contract C { function f(address a) public view returns (address) { return a.owner; } }\n"), sourceSHA256: "6858437cbe0360e44ac599c49810e7a86f2b94ccfabab38112d751f203f05674",
			wantC: "58e3573f7d0a876346fed1636144f061175594f507e5813d2e07aca6c6f2ed8c", wantRaw: "59d346b564a497fa8299c68724f8d3bae4f40e041552c4ec2b4431e1892da4fb", wantProduction: "58e3573f7d0a876346fed1636144f061175594f507e5813d2e07aca6c6f2ed8c", wantCompactDigest: "58e3573f7d0a876346fed1636144f061175594f507e5813d2e07aca6c6f2ed8c", wantForestDigest: "58e3573f7d0a876346fed1636144f061175594f507e5813d2e07aca6c6f2ed8c", wantIncremental: "58e3573f7d0a876346fed1636144f061175594f507e5813d2e07aca6c6f2ed8c", wantRawDiff: memberDiff, wantCompact: noActionFallback, wantForest: true,
			wantRawDispatch: "none", wantProductionDispatch: "1/1/42/7", wantCompactDispatch: "1/1/42/7", wantForestDispatch: "1/1/41/0", wantIncrementalDispatch: "1/1/42/7", wantReusedSubtrees: 14, wantReusedBytes: 53,
		},
		{
			name: "clean-call-alias", source: []byte("contract C { function f(uint256 x) public pure returns (uint256) { return uint256(x); } }\n"), sourceSHA256: "0d946989580a17703c05871cd035ca8083048e600aa3bbe451f344b77298cca4",
			wantC: "3b4933bab1c7f82173bbc78c423f30c596521f1ec34a12a2588571d501e518d6", wantRaw: "3b4933bab1c7f82173bbc78c423f30c596521f1ec34a12a2588571d501e518d6", wantProduction: "9882705b4b5b2001a012dbde7971ecec760ff3e9f59b94544c30881daf5184ff", wantCompactDigest: "9882705b4b5b2001a012dbde7971ecec760ff3e9f59b94544c30881daf5184ff", wantForestDigest: "d9c040b976230d7454ac85d3928b5b6041c11776eaccef1d48b6c979a6c4e7e8", wantIncremental: "9882705b4b5b2001a012dbde7971ecec760ff3e9f59b94544c30881daf5184ff", wantProductionDiff: callDiff, wantCompactDiff: callDiff, wantForestDiff: callDiff, wantIncrementalDiff: callDiff, wantCompact: noActionFallback, wantForest: true,
			wantRawDispatch: "none", wantProductionDispatch: "1/1/45/12", wantCompactDispatch: "1/1/45/12", wantForestDispatch: "1/1/46/13", wantIncrementalDispatch: "1/1/45/12", wantReusedSubtrees: 16, wantReusedBytes: 61,
		},
		{
			name: "malformed-member", source: []byte("contract C { function f(address a) public view returns (address) { return a.; } }\n"), sourceSHA256: "188f62c3e17fdbf9f022c0120d8f8015ae8a21468cf2f6f92ec74892354d4dc7", wantError: true,
			wantC: "7ae2871bd3028093215d8118e3f0a58065e4276834db38329a8b97e65f8df912", wantRaw: "64d54df15129a3845acd6eda9e9470f40dc50f40108b7646c72c956516072d69", wantProduction: "4d876136804ad8a663cdd5fce91f04cdfab2f3bd215c7ea499b92d72dd577690", wantCompactDigest: "4d876136804ad8a663cdd5fce91f04cdfab2f3bd215c7ea499b92d72dd577690", wantIncremental: "4d876136804ad8a663cdd5fce91f04cdfab2f3bd215c7ea499b92d72dd577690", wantRawDiff: malformedDiff, wantProductionDiff: malformedDiff, wantCompactDiff: malformedDiff, wantIncrementalDiff: malformedDiff, wantCompact: noActionFallback,
			wantRawDispatch: "none", wantProductionDispatch: "1/1/42/7", wantCompactDispatch: "1/1/42/7", wantForestDispatch: "none", wantIncrementalDispatch: "1/1/42/7", wantReusedSubtrees: 13, wantReusedBytes: 52,
		},
		{
			name: "malformed-call", source: []byte("contract C { function f(uint256 x) public pure returns (uint256) { return uint256(; } }\n"), sourceSHA256: "523a3016d484bee308c1b237183c553186d3b2732541c8bca6b4175cc1ddd0f7", wantError: true,
			wantC: "00b5fccbcf99a1bf710682cfa4d01db50e74e3bddad5c3a78977269ba76053cb", wantRaw: "9272deb09841144e85fd5ed32a3a6bc6b6f1d39b2221e0692db918c7f3c33d2d", wantProduction: "9272deb09841144e85fd5ed32a3a6bc6b6f1d39b2221e0692db918c7f3c33d2d", wantCompactDigest: "9272deb09841144e85fd5ed32a3a6bc6b6f1d39b2221e0692db918c7f3c33d2d", wantIncremental: "afb72aead66613b4cb37a32bdf9aacd8a945963e1af1b6c47f0f2999359e071d", wantRawDiff: malformedDiff, wantProductionDiff: malformedDiff, wantCompactDiff: malformedDiff, wantIncrementalDiff: malformedDiff, wantCompact: recoveryFallback,
			wantRawDispatch: "none", wantProductionDispatch: "1/1/43/0", wantCompactDispatch: "1/1/43/0", wantForestDispatch: "none", wantIncrementalDispatch: "1/1/42/0", wantReusedSubtrees: 14, wantReusedBytes: 59,
		},
		{
			name: "positive-control", source: []byte("contract C { function f(uint256 x) public pure returns (uint256) { return x; } }\n"), sourceSHA256: "565524cb2e35aa2dc11cc39720d2a92c57e794e9120f8eaf3cd0160ef818c68a",
			wantC: "90e10f0667bbcac3ad8f10774370c052084020d6782bc3b5e22a6670308b4bd2", wantRaw: "90e10f0667bbcac3ad8f10774370c052084020d6782bc3b5e22a6670308b4bd2", wantProduction: "90e10f0667bbcac3ad8f10774370c052084020d6782bc3b5e22a6670308b4bd2", wantCompactDigest: "90e10f0667bbcac3ad8f10774370c052084020d6782bc3b5e22a6670308b4bd2", wantForestDigest: "c25f7b71de7473b83eac6e6cb582d4d155da05f239818b7d597b8166ae48a69c", wantIncremental: "90e10f0667bbcac3ad8f10774370c052084020d6782bc3b5e22a6670308b4bd2", wantForestDiff: positiveForestDiff, wantCompact: noActionFallback, wantForest: true,
			wantRawDispatch: "none", wantProductionDispatch: "1/1/38/0", wantCompactDispatch: "1/1/38/0", wantForestDispatch: "1/1/39/0", wantIncrementalDispatch: "1/1/38/0", wantReusedSubtrees: 14, wantReusedBytes: 53,
		},
	}
	for _, witness := range witnesses {
		witness := witness
		t.Run(witness.name, func(t *testing.T) {
			if got := fmt.Sprintf("%x", sha256.Sum256(witness.source)); got != witness.sourceSHA256 {
				t.Fatalf("source SHA-256 = %s, want %s", got, witness.sourceSHA256)
			}
			cParser := sitter.NewParser()
			t.Cleanup(cParser.Close)
			if err := cParser.SetLanguage(cLanguage); err != nil {
				t.Fatal(err)
			}
			cTree := cParser.Parse(witness.source, nil)
			if cTree == nil || cTree.RootNode() == nil {
				t.Fatal("locked C parser returned no tree")
			}
			t.Cleanup(cTree.Close)
			cDigest, err := COracleDeepDigest(cTree)
			if err != nil {
				t.Fatal(err)
			}
			if cDigest != witness.wantC {
				t.Fatalf("locked C digest = %s, want %s", cDigest, witness.wantC)
			}
			raw := solidityNextParseRoute(t, goLanguage, witness.source, "raw", func(p *gotreesitter.Parser, source []byte) (*gotreesitter.Tree, error) {
				return p.ParseNoResultCompatibilityBenchmarkOnly(source)
			})
			production := solidityNextParseRoute(t, goLanguage, witness.source, "production", func(p *gotreesitter.Parser, source []byte) (*gotreesitter.Tree, error) {
				return p.Parse(source)
			})
			assertSolidityNextRoute(t, "raw", raw, goLanguage, cTree, cDigest, witness.wantError, witness.wantRaw, witness.wantRawDiff, witness.wantRawDispatch)
			assertSolidityNextRoute(t, "production", production, goLanguage, cTree, cDigest, witness.wantError, witness.wantProduction, witness.wantProductionDiff, witness.wantProductionDispatch)

			_, fallbackBefore := gotreesitter.AdmissionCandidateCounters()
			compactParser := gotreesitter.NewParser(goLanguage)
			compactParser.SetAdmissionCandidateRoute(true)
			compact, err := compactParser.Parse(witness.source)
			if err != nil {
				t.Fatalf("compact parse: %v", err)
			}
			t.Cleanup(compact.Release)
			_, fallbackAfter := gotreesitter.AdmissionCandidateCounters()
			compactRoute := "accepted"
			if fallbackAfter > fallbackBefore {
				compactRoute = "fallback:" + gotreesitter.AdmissionCandidateLastFallbackReason()
			}
			if compactRoute != witness.wantCompact {
				t.Fatalf("compact route = %q, want %q", compactRoute, witness.wantCompact)
			}
			assertSolidityNextRoute(t, "compact", compact, goLanguage, cTree, cDigest, witness.wantError, witness.wantCompactDigest, witness.wantCompactDiff, witness.wantCompactDispatch)

			forestParser := gotreesitter.NewParser(goLanguage)
			forest, forestOK := forestParser.ParseForestExperimental(witness.source)
			if forestOK != witness.wantForest || (forestOK && forest == nil) {
				t.Fatalf("forest accepted=%t, tree nil=%t, want accepted=%t", forestOK, forest == nil, witness.wantForest)
			}
			if forestOK {
				t.Cleanup(forest.Release)
				assertSolidityNextRoute(t, "forest", forest, goLanguage, cTree, cDigest, witness.wantError, witness.wantForestDigest, witness.wantForestDiff, witness.wantForestDispatch)
			}

			base := bytes.TrimSuffix(witness.source, []byte{'\n'})
			incrementalParser := gotreesitter.NewParser(goLanguage)
			oldTree, err := incrementalParser.Parse(base)
			if err != nil {
				t.Fatalf("incremental base parse: %v", err)
			}
			t.Cleanup(oldTree.Release)
			oldTree.Edit(gotreesitter.InputEdit{StartByte: uint32(len(base)), OldEndByte: uint32(len(base)), NewEndByte: uint32(len(witness.source)), StartPoint: solidityNextPointAtByte(base), OldEndPoint: solidityNextPointAtByte(base), NewEndPoint: solidityNextPointAtByte(witness.source)})
			incremental, profile, err := incrementalParser.ParseIncrementalProfiled(witness.source, oldTree)
			if err != nil {
				t.Fatalf("incremental parse: %v", err)
			}
			t.Cleanup(incremental.Release)
			if !profile.OldTreeReuseRoute || profile.ReuseUnsupported || profile.ReusedSubtrees != witness.wantReusedSubtrees || profile.ReusedBytes != witness.wantReusedBytes {
				t.Fatalf("incremental profile = reuse:%t unsupported:%t subtrees:%d bytes:%d, want reuse:true unsupported:false subtrees:%d bytes:%d", profile.OldTreeReuseRoute, profile.ReuseUnsupported, profile.ReusedSubtrees, profile.ReusedBytes, witness.wantReusedSubtrees, witness.wantReusedBytes)
			}
			assertSolidityNextRoute(t, "incremental", incremental, goLanguage, cTree, cDigest, witness.wantError, witness.wantIncremental, witness.wantIncrementalDiff, witness.wantIncrementalDispatch)
			t.Logf("witness=%s bytes=%d source_sha256=%s c_digest=%s reuse=%d/%d", witness.name, len(witness.source), witness.sourceSHA256, cDigest, profile.ReusedSubtrees, profile.ReusedBytes)
		})
	}
}

func TestSolidityNextLiveArmReceiptDocument(t *testing.T) {
	document, err := os.ReadFile("../docs/root-normalization-retirement.md")
	if err != nil {
		t.Fatal(err)
	}
	documentText := strings.Join(strings.Fields(string(document)), " ")
	for _, marker := range []string{
		"Status: NO-GO. KEEP LIVE: `dispatch.solidity`.",
		"The A0 manifest has 14 languages and 14 receipts.",
		"Solidity records three files, three checks, three runs, 26897 visited nodes, 666 rewrites",
		"The focused receipt covers eight witnesses",
		"The authenticated real-corpus census is unavailable.",
		"Keep `dispatch.solidity` live until the producer closes every locked-C divergence",
	} {
		marker = strings.Join(strings.Fields(marker), " ")
		if !strings.Contains(documentText, marker) {
			t.Fatalf("Solidity blocker receipt lacks marker %q", marker)
		}
	}
}

func TestSolidityNextLiveChangelogReceipt(t *testing.T) {
	changelog, err := os.ReadFile("../CHANGELOG.md")
	if err != nil {
		t.Fatal(err)
	}
	changelogText := strings.Join(strings.Fields(string(changelog)), " ")
	for _, marker := range []string{
		"Record the `dispatch.solidity` blocker receipt at base",
		"The A0 manifest records three Solidity files, three checks, three runs, and 666 rewrites.",
		"No registry or production code changes are included.",
	} {
		marker = strings.Join(strings.Fields(marker), " ")
		if !strings.Contains(changelogText, marker) {
			t.Fatalf("Solidity changelog receipt lacks marker %q", marker)
		}
	}
}

func mustSolidityFile(t *testing.T, path string) []byte {
	t.Helper()
	source, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return source
}

func solidityNextParseRoute(t *testing.T, language *gotreesitter.Language, source []byte, route string, parse func(*gotreesitter.Parser, []byte) (*gotreesitter.Tree, error)) *gotreesitter.Tree {
	t.Helper()
	parser := gotreesitter.NewParser(language)
	tree, err := parse(parser, source)
	if err != nil {
		t.Fatalf("%s parse: %v", route, err)
	}
	t.Cleanup(tree.Release)
	return tree
}

func assertSolidityNextRoute(t *testing.T, route string, tree *gotreesitter.Tree, language *gotreesitter.Language, cTree *sitter.Tree, cDigest string, wantError bool, wantDigest string, wantDiff *DumpV1Divergence, wantDispatch string) {
	t.Helper()
	root := tree.RootNode()
	if root == nil || root.HasError() != wantError {
		t.Fatalf("%s root error=%t, want %t", route, root != nil && root.HasError(), wantError)
	}
	inspection, err := benchfixtures.InspectGoTree(root, language)
	if err != nil {
		t.Fatalf("%s inspect Go tree: %v", route, err)
	}
	if inspection.SHA256 != wantDigest {
		t.Fatalf("%s digest=%s, want %s", route, inspection.SHA256, wantDigest)
	}
	diff := FirstDivergenceDumpV1(root, language, cTree.RootNode())
	if diff == nil {
		if wantDiff != nil {
			t.Fatalf("%s divergence is nil, want %+v", route, wantDiff)
		}
		if inspection.SHA256 != cDigest {
			t.Fatalf("%s exact digest=%s, C=%s", route, inspection.SHA256, cDigest)
		}
	} else if wantDiff == nil || *diff != *wantDiff {
		t.Fatalf("%s divergence=%+v, want %+v", route, diff, wantDiff)
	}
	if got := solidityNextDispatchReceipt(tree); got != wantDispatch {
		t.Fatalf("%s dispatch=%q, want %q", route, got, wantDispatch)
	}
	t.Logf("route=%s error=%t digest=%s c_digest=%s divergence=%v dispatch=%s", route, root.HasError(), inspection.SHA256, cDigest, diff, wantDispatch)
}

func solidityNextDispatchReceipt(tree *gotreesitter.Tree) string {
	if tree == nil || tree.ParseRuntime().NormalizationPasses == nil {
		return "none"
	}
	for _, pass := range *tree.ParseRuntime().NormalizationPasses {
		if pass.Name == "dispatch.solidity" {
			return fmt.Sprintf("%d/%d/%d/%d", pass.Checked, pass.Run, pass.NodesVisited, pass.NodesRewritten)
		}
	}
	return "none"
}

func solidityNextPointAtByte(source []byte) gotreesitter.Point {
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
