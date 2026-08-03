//go:build !grammar_subset

package parserresult_test

import (
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	"github.com/odvcencio/gotreesitter/internal/benchfixtures"
)

type materializationSubpassProbe struct {
	name               string
	language           func() *gotreesitter.Language
	source             string
	wantRawDigest      string
	wantResultDigest   string
	expectedSubpasses  []string
	rewrittenSubpasses []string
}

func TestMaterializationSubpassCompatibilityFreeProducerProbes(t *testing.T) {
	for _, probe := range materializationSubpassProbes() {
		probe := probe
		t.Run(probe.name, func(t *testing.T) {
			language := probe.language()
			source := []byte(probe.source)

			t.Setenv("GTS_DISPATCHER_CENSUS", "")
			rawDigest, _ := materializationSubpassParseDigest(
				t,
				language,
				source,
				true,
			)
			resultDigest, _ := materializationSubpassParseDigest(
				t,
				language,
				source,
				false,
			)

			t.Setenv("GTS_DISPATCHER_CENSUS", "1")
			censusDigest, runtime := materializationSubpassParseDigest(
				t,
				language,
				source,
				false,
			)
			if censusDigest != resultDigest {
				t.Fatalf(
					"census changed production output: got %s, want %s",
					censusDigest,
					resultDigest,
				)
			}
			if probe.wantRawDigest != "" && rawDigest != probe.wantRawDigest {
				t.Fatalf("raw digest = %s, want %s", rawDigest, probe.wantRawDigest)
			}
			if probe.wantResultDigest != "" && resultDigest != probe.wantResultDigest {
				t.Fatalf(
					"result digest = %s, want %s",
					resultDigest,
					probe.wantResultDigest,
				)
			}
			assertMaterializationSubpassReceipts(
				t,
				runtime,
				probe.expectedSubpasses,
				probe.rewrittenSubpasses,
			)
			t.Logf("raw=%s result=%s", rawDigest, resultDigest)
		})
	}
}

func materializationSubpassParseDigest(
	t *testing.T,
	language *gotreesitter.Language,
	source []byte,
	withoutCompatibility bool,
) (string, gotreesitter.ParseRuntime) {
	t.Helper()
	parser := gotreesitter.NewParser(language)
	parser.SetAdmissionCandidateRoute(false)
	var (
		tree *gotreesitter.Tree
		err  error
	)
	if withoutCompatibility {
		tree, err = parser.ParseNoResultCompatibilityBenchmarkOnly(source)
	} else {
		tree, err = parser.Parse(source)
	}
	if err != nil {
		t.Fatal(err)
	}
	if tree == nil || tree.RootNode() == nil {
		t.Fatal("parse returned no tree")
	}
	defer tree.Release()
	inspection, err := benchfixtures.InspectGoTree(tree.RootNode(), language)
	if err != nil {
		t.Fatal(err)
	}
	return inspection.SHA256, tree.ParseRuntime()
}

func assertMaterializationSubpassReceipts(
	t *testing.T,
	runtime gotreesitter.ParseRuntime,
	expected []string,
	rewritten []string,
) {
	t.Helper()
	if runtime.NormalizationPasses == nil {
		t.Fatal("census recorded no normalization passes")
	}
	passes := make(map[string]gotreesitter.NormalizationPassRuntime)
	for _, pass := range *runtime.NormalizationPasses {
		passes[pass.Name] = pass
	}
	wantRewritten := make(map[string]bool, len(rewritten))
	for _, name := range rewritten {
		wantRewritten[name] = true
	}
	for _, name := range expected {
		pass, ok := passes[name]
		if !ok {
			t.Errorf("census did not record %s: %+v", name, passes)
			continue
		}
		if pass.Checked == 0 || pass.Run == 0 || pass.NodesVisited == 0 {
			t.Errorf("%s has a vacuous census receipt: %+v", name, pass)
		}
		if got := pass.NodesRewritten > 0; got != wantRewritten[name] {
			t.Errorf(
				"%s rewrite state = %t, want %t: %+v",
				name,
				got,
				wantRewritten[name],
				pass,
			)
		}
		t.Logf("%s: %+v", name, pass)
	}
}

func materializationSubpassProbes() []materializationSubpassProbe {
	return []materializationSubpassProbe{
		{
			name:     "ada_locked_named_array_aggregate",
			language: grammars.AdaLanguage,
			// tree-sitter-ada test/corpus/arrays.txt, commit 6b58259a.
			source: "package P is\n" +
				"   type A is array (1 .. 2) of Boolean;\n" +
				"   V : constant A := (1 => True, 2 => False);\n" +
				"   V2 : constant A := (1 => True, others => False);\n" +
				"end P;\n",
			// Both digests moved with the inherited-field projection repair
			// (PR #638). The old digests pinned a "subtype_mark" field on
			// range_g that the C runtime never assigns. C receipt, taken
			// with field comparison on: this source now reports 0 mismatches
			// against the locked C oracle on the raw route and 0 on the
			// production route.
			wantRawDigest:    "d04b5a44207f4e4691e8aea35d0b15c01f225b5bbbd2f2bef3f75532fc5a517c",
			wantResultDigest: "d04b5a44207f4e4691e8aea35d0b15c01f225b5bbbd2f2bef3f75532fc5a517c",
			expectedSubpasses: []string{
				"dispatch.ada",
			},
		},
		{
			name:     "ada_locked_positional_array_aggregate",
			language: grammars.AdaLanguage,
			// tree-sitter-ada test/corpus/arrays.txt, commit 6b58259a.
			source: "package P is\n" +
				"   type A is array (1 .. 3) of Boolean;\n" +
				"   V : constant A := (1, 2, 3);\n" +
				"end;\n",
			// Both digests moved with the inherited-field projection repair
			// (PR #638). The old digests pinned a "subtype_mark" field on
			// range_g that the C runtime never assigns. C receipt, taken
			// with field comparison on: the production route now reports 0
			// mismatches against the locked C oracle. The raw route keeps
			// only the 2 pre-existing aggregate-kind election mismatches
			// (record_aggregate for positional_array_aggregate) that
			// dispatch.ada.aggregate-kind-election repairs, and 0 field
			// mismatches.
			wantRawDigest:    "03ed84353f6116db108013a4124c9e573333fbc9454464fe28711527b1e7d6ec",
			wantResultDigest: "78ef4359be68f12055d075c272b5fbf63be2a9f0fe8672f5c26979308ba149ff",
			expectedSubpasses: []string{
				"dispatch.ada",
				"dispatch.ada.aggregate-kind-election",
			},
			rewrittenSubpasses: []string{
				"dispatch.ada",
				"dispatch.ada.aggregate-kind-election",
			},
		},
		{
			name:     "ada_locked_record_aggregate",
			language: grammars.AdaLanguage,
			// tree-sitter-ada test/corpus/records.txt, commit 6b58259a.
			source: "procedure P is\n" +
				"begin\n" +
				"   A := (F1 => 1, F2 => 2);\n" +
				"end;\n",
			wantRawDigest:    "ef26cc01b0dee3da50728dbfa242162ba837798c34450674d8b3ce3ed06955d6",
			wantResultDigest: "ef26cc01b0dee3da50728dbfa242162ba837798c34450674d8b3ce3ed06955d6",
			expectedSubpasses: []string{
				"dispatch.ada",
			},
		},
		{
			name:     "ada_array_others_choice",
			language: grammars.AdaLanguage,
			source: "procedure P is\n" +
				"begin\n" +
				"   A := (others => 0);\n" +
				"end;\n",
			// Both digests moved with the inherited-field projection repair
			// (PR #638). The old digests pinned a "name" field on
			// named_array_aggregate that the C runtime never assigns. C
			// receipt, taken with field comparison on: the production route
			// now reports 0 mismatches against the locked C oracle. The raw
			// route keeps only the 6 pre-existing aggregate-kind and
			// association-choice election mismatches that
			// dispatch.ada.aggregate-kind-election and
			// dispatch.ada.association-choice-materialization repair, and 0
			// field mismatches.
			wantRawDigest:    "ad2718a1e309fadba012881bc82689ff1dabec69814106b0dcbe3a39b6d70b27",
			wantResultDigest: "95edae06bdbba04a19749854656b7c439a334cf9ae7bba367ed762386cf28d15",
			expectedSubpasses: []string{
				"dispatch.ada",
				"dispatch.ada.aggregate-kind-election",
				"dispatch.ada.association-choice-materialization",
			},
			rewrittenSubpasses: []string{
				"dispatch.ada",
				"dispatch.ada.aggregate-kind-election",
				"dispatch.ada.association-choice-materialization",
			},
		},
		{
			name:     "ada_attribute_constraint",
			language: grammars.AdaLanguage,
			source: "procedure P is\n" +
				"begin\n" +
				"   A := new T (F => Pkg.Obj'Access);\n" +
				"end;\n",
			wantRawDigest:    "a35538112ad4ae10df6d5544c7f0a58f3e6a3ecb5fd5fb842808d0e31ac27ac2",
			wantResultDigest: "d0006efd9f34dce85ac48330642b378dc2b5f995004c9c11a20aee98ac6be8a2",
			expectedSubpasses: []string{
				"dispatch.ada",
				"dispatch.ada.constraint-kind-election",
			},
			rewrittenSubpasses: []string{
				"dispatch.ada",
				"dispatch.ada.constraint-kind-election",
			},
		},
		{
			name:     "apex_generic_local_declaration",
			language: grammars.ApexLanguage,
			source: "public class C {\n" +
				"  void m() {\n" +
				"    List<List<SObject>> searchResults = [FIND :keyword IN ALL FIELDS];\n" +
				"  }\n" +
				"}\n",
			wantRawDigest:    "7d39cbbd2e1ae5eb23bfd005f4a893688c380f3366cc0c2894d66d752e93f0a3",
			wantResultDigest: "7d39cbbd2e1ae5eb23bfd005f4a893688c380f3366cc0c2894d66d752e93f0a3",
			expectedSubpasses: []string{
				"dispatch.apex",
				"dispatch.apex.class-literal-alias",
			},
		},
		{
			name:     "apex_class_literal_alias",
			language: grammars.ApexLanguage,
			source: "public class C {\n" +
				"  void m() {\n" +
				"    Object t = RecordPage.class;\n" +
				"  }\n" +
				"}\n",
			wantRawDigest:    "a7ce1bb2fb703f6c85a85bac1116a31f9d14f03cd4560f8fe81f518b19d36f33",
			wantResultDigest: "691872382855aafce9519a115ca30ac191c4a118d4ab7170e88e0193fd2f5bb6",
			expectedSubpasses: []string{
				"dispatch.apex",
				"dispatch.apex.class-literal-alias",
			},
			rewrittenSubpasses: []string{
				"dispatch.apex",
				"dispatch.apex.class-literal-alias",
			},
		},
		{
			name:             "bash_assignment_wrapper",
			language:         grammars.BashLanguage,
			source:           "a=1 b=2 c=3",
			wantRawDigest:    "acf5436fdf47d1af14d387f9bc7ebc30ec09b4874d6a9e9caf452678e387c9f6",
			wantResultDigest: "81ace1f9c8a394d177e5ea909ff3012329ec78b255e45a235fc8a3eea4a67a16",
			expectedSubpasses: []string{
				"dispatch.bash",
				"dispatch.bash.variable-assignment-wrapper-flattening",
				"dispatch.bash.generated-command-assignment",
			},
			rewrittenSubpasses: []string{
				"dispatch.bash",
				"dispatch.bash.variable-assignment-wrapper-flattening",
			},
		},
		{
			name:             "bash_generated_command_assignment",
			language:         grammars.BashLanguage,
			source:           "zipname=npm-$(node ../cli.js -v).zip",
			wantRawDigest:    "a2dac68ad605a69720f13549488b1519627653226bb252bd89db912ae1558e8b",
			wantResultDigest: "a2dac68ad605a69720f13549488b1519627653226bb252bd89db912ae1558e8b",
			expectedSubpasses: []string{
				"dispatch.bash",
				"dispatch.bash.generated-command-assignment",
			},
		},
		{
			name:             "bash_command_name_concatenation",
			language:         grammars.BashLanguage,
			source:           "${0%/*}/check-unsafe-assertions.sh",
			wantRawDigest:    "a5b4479ddfaa21239153effb0ee7b1f886155c25e2ea6ba43d56c5087a211819",
			wantResultDigest: "a5b4479ddfaa21239153effb0ee7b1f886155c25e2ea6ba43d56c5087a211819",
			expectedSubpasses: []string{
				"dispatch.bash",
				"dispatch.bash.generated-command-assignment",
			},
		},
		{
			name:     "bash_if_condition_field",
			language: grammars.BashLanguage,
			source: "if [ $ret -eq 0 ]; then\n" +
				"  (exit 0)\n" +
				"else\n" +
				"  rm npm-install-$$.sh\n" +
				"  echo \"Failed to download script\" >&2\n" +
				"  exit $ret\n" +
				"fi\n",
			wantRawDigest:    "7d110bcd4998cf43995020d6ee6326ef0e060b587ce44fa75943d95d86156cd5",
			wantResultDigest: "7d110bcd4998cf43995020d6ee6326ef0e060b587ce44fa75943d95d86156cd5",
			expectedSubpasses: []string{
				"dispatch.bash",
				"dispatch.bash.if-condition-field-projection",
				"dispatch.bash.generated-command-assignment",
			},
		},
		{
			name:             "cooklang_punctuation_recipe",
			language:         grammars.CooklangLanguage,
			source:           "Add @salt{1%tsp}.\n",
			wantRawDigest:    "0e6880ec4902576c2a6de014424c3cba7eef99cdc5fd8fded8ceb6382a6df9cd",
			wantResultDigest: "c6e4535b725516550ca7a0ee4c69974799c2d2d10fed4e5f1ba6b71e43c5ba8a",
			expectedSubpasses: []string{
				"dispatch.cooklang",
				"dispatch.cooklang.recovered-recipe",
			},
			rewrittenSubpasses: []string{
				"dispatch.cooklang",
				"dispatch.cooklang.recovered-recipe",
			},
		},
		{
			name:     "cooklang_recovered_recipe",
			language: grammars.CooklangLanguage,
			source: "---\n" +
				"servings: 4\n" +
				"emoji: 🥟\n" +
				"tags: warm, fried, starter\n" +
				"---\n\n" +
				"Serve hot.\n",
			wantRawDigest:    "896d9f79d941c3869dca7b855bae45738392d02519f0fbe3ac45cc2623fcfa2f",
			wantResultDigest: "814240a8aff9c3e253b37ce1ff535ff2cd96510c6afea40680a270405699967f",
			expectedSubpasses: []string{
				"dispatch.cooklang",
				"dispatch.cooklang.recovered-recipe",
			},
			rewrittenSubpasses: []string{
				"dispatch.cooklang",
				"dispatch.cooklang.recovered-recipe",
			},
		},
		{
			name:             "cooklang_recovered_recipe_without_newline",
			language:         grammars.CooklangLanguage,
			source:           "Add @salt{1%tsp}.",
			wantRawDigest:    "f49ca1a85a0b2ee7ed7f07993d6bc8b103d66311f83d4a026e085cf2013a69ec",
			wantResultDigest: "dd3692a1a0e9145af9f2d082126a1e798d60cbe74942427746d3f0e83bd31e1c",
			expectedSubpasses: []string{
				"dispatch.cooklang",
				"dispatch.cooklang.recovered-recipe",
			},
			rewrittenSubpasses: []string{
				"dispatch.cooklang",
				"dispatch.cooklang.recovered-recipe",
			},
		},
		{
			// Pins all three Go arm census subpass ids and one real rewrite
			// receipt. dispatch.go.source-file-root is inert on this fresh
			// route by design; its live route is included ranges, covered by
			// cgo_harness/included_ranges_go_root_parity_cgo_test.go.
			name:     "go_new_make_type_argument",
			language: grammars.GoLanguage,
			source:   "package p\n\nfunc f() {\n\t_ = new(dirInfo)\n}\n",
			expectedSubpasses: []string{
				"dispatch.go",
				"dispatch.go.source-file-root",
				"dispatch.go.compat-walk",
				"dispatch.go.new-make-type",
			},
			rewrittenSubpasses: []string{
				"dispatch.go",
				"dispatch.go.new-make-type",
			},
		},
		{
			// Pins the Kotlin arm's recovered-root subpass id and its
			// INERTNESS off the included-ranges route. The member was retired
			// on 2026-08-02 as dead code and restored on 2026-08-03 because it
			// is live on the included-ranges route only. This probe is the
			// other half of that claim: on the fresh route it is reached and
			// rewrites nothing. Its live route is covered by
			// parser_included_ranges_kotlin_root_test.go and
			// cgo_harness/included_ranges_kotlin_root_parity_cgo_test.go.
			name:     "kotlin_clean_source_file_root_is_inert",
			language: grammars.KotlinLanguage,
			source:   "package demo\n\nfun main() {\n    println(\"hi\")\n}\n",
			expectedSubpasses: []string{
				"dispatch.kotlin",
				"dispatch.kotlin.recovered-source-file-root",
			},
			rewrittenSubpasses: nil,
		},
	}
}
