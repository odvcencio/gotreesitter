//go:build cgo && treesitter_c_parity

package cgoharness

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	"github.com/odvcencio/gotreesitter/internal/benchfixtures"
	sitter "github.com/tree-sitter/go-tree-sitter"
)

// bitbakeRetiredAddtaskSource is the recipe that once triggered the BitBake
// addtask ERROR-wrapper repair.
const bitbakeRetiredAddtaskSource = "SUMMARY = \"Test recipe for fetching git submodules\"\nHOMEPAGE = \"http://git.yoctoproject.org/cgit/cgit.cgi/git-submodule-test/\"\nLICENSE = \"MIT\"\nLIC_FILES_CHKSUM = \"file://${COMMON_LICENSE_DIR}/MIT;md5=0835ade698e0bcf8506ecda2f7b4f302\"\n\nINHIBIT_DEFAULT_DEPS = \"1\"\n\n# Note: this is intentionally not the latest version in the original .bb\nSRCREV = \"f280847494763cdcf71197557a81ba7d8a6bce42\"\nPV = \"0.1+git\"\nPR = \"r2\"\n\nSRC_URI = \"gitsm://git.yoctoproject.org/git-submodule-test;branch=master;protocol=https\"\nUPSTREAM_CHECK_COMMITS = \"1\"\nRECIPE_NO_UPDATE_REASON = \"This recipe is used to test devtool upgrade feature\"\n\nEXCLUDE_FROM_WORLD = \"1\"\n\ndo_test_git_as_user() {\n    cd ${S}\n    git status\n    git submodule status\n}\naddtask test_git_as_user after do_unpack\n\nfakeroot do_test_git_as_root() {\n    cd ${S}\n    git status\n    git submodule status\n}\ndo_test_git_as_root[depends] += \"virtual/fakeroot-native:do_populate_sysroot\"\naddtask test_git_as_root after do_unpack"

type retiredInertArmWitness struct {
	language     string
	name         string
	file         string // under testdata/retired_dispatch_arms
	source       string
	sourceSHA256 string
	cDigest      string
	goDigest     string
	hasError     bool
	// divergence is the first difference from locked C. It is nil when the
	// Go tree matches locked C exactly.
	divergence *DumpV1Divergence
}

// TestRetiredInertArmsLockedCParity compares the plain engine tree with the
// locked C tree for the witnesses of five retired result-compatibility arms:
// bitbake, corn, dtd, templ, and wolfram. None of the arms rewrote a
// node on any of these inputs, so the tree after retirement is the tree
// before it. Witnesses that match C must keep matching it. Witnesses that
// differ from C keep the pinned difference and the pinned Go digest. The
// route-by-route comparison without C is
// grammars/retired_inert_arm_route_receipt_test.go.
func TestRetiredInertArmsLockedCParity(t *testing.T) {
	languages := map[string]func() *gotreesitter.Language{
		"bitbake": grammars.BitbakeLanguage,
		"corn":    grammars.CornLanguage,
		"dtd":     grammars.DtdLanguage,
		"templ":   grammars.TemplLanguage,
		"wolfram": grammars.WolframLanguage,
	}
	witnesses := []retiredInertArmWitness{
		{language: "bitbake", name: "a0-small-error", file: "bitbake/small__error.bb", sourceSHA256: "fbdb85e443edd378e944e5a1416c0c4a1e485f0cd38f70a5ac75748073a15d12", cDigest: "6154c15dcc0a1f365b7cfb0bf0cc120f3e365b55a491ca793a0cf170bc5cfe92", goDigest: "6154c15dcc0a1f365b7cfb0bf0cc120f3e365b55a491ca793a0cf170bc5cfe92", hasError: false, divergence: nil},
		{language: "bitbake", name: "a0-medium-clang-git", file: "bitbake/medium__clang_git.bb", sourceSHA256: "7deb41efd839d8b5b8b2c98589614377d12fd81fa6033824330084e07c5eaf9f", cDigest: "7eb2e581801fd0dbf1ed79a9eaf2ad357062725d6b26a238a168b3e14473d064", goDigest: "d115b90c9c7c42e7ca7f2a6cbd089a799b3b139b15669ea142584b9fc2cc0091", hasError: true, divergence: &DumpV1Divergence{Path: "/recipe/function_definition[40]/ERROR[5]", Category: "shape", GoValue: "children=15", CValue: "children=13"}},
		{language: "bitbake", name: "addtask-error-wrapper", source: bitbakeRetiredAddtaskSource, sourceSHA256: "35ccf9d007ef76548258088b18adb39d7c0509452b4fb62bd7dfdc94cdcdf780", cDigest: "464f3db4c1ad8be4dabc16c9631524f4cee1d99c9c1c6f4abfad6d6bfe8cabed", goDigest: "464f3db4c1ad8be4dabc16c9631524f4cee1d99c9c1c6f4abfad6d6bfe8cabed", hasError: true, divergence: nil},
		{language: "bitbake", name: "function-flag-assignment", source: "do_run_tests () {\n    meson test -C \"${B}\" --no-rebuild\n}\ndo_run_tests[doc] = \"Run meson test using qemu-user\"\naddtask do_run_tests after do_compile", sourceSHA256: "8655832f5acd3b4ada197881f41b0110657572aa0c791ecf4524fbc10603a1eb", cDigest: "dd7147f575544eadae2a0c5aa1ecd571495d66f40da096cba1fd9b309635bd61", goDigest: "80fa26e7c69b77d21be836e2dbfa3f99da37d3c45e975533da1328e8b86f70d9", hasError: true, divergence: &DumpV1Divergence{Path: "/recipe/function_definition[0]/ERROR[4]", Category: "shape", GoValue: "children=10", CValue: "children=9"}},
		{language: "bitbake", name: "adjacent-override-functions", source: "do_install:append() {\n    ln -sf am335x-bonegreen-ext.dtb \"${D}/boot/devicetree/am335x-bonegreen-ext-alias.dtb\"\n}\n\ndo_deploy:append() {\n    ln -sf am335x-bonegreen-ext.dtb \"${DEPLOYDIR}/devicetree/am335x-bonegreen-ext-alias.dtb\"\n}", sourceSHA256: "068037e13b101cf01d0a36da586f798972fa39b99ba0c2a86dc17996bc34d185", cDigest: "f972c3f88404f41643400bfa2c200c72f2ddbcb93295a258c8b1f404e2cb7d9b", goDigest: "ae67876071cf4c9cbe3f8bb52c59efbe0bc216db8af43ccc9dd027a5873f2801", hasError: true, divergence: &DumpV1Divergence{Path: "/recipe", Category: "shape", GoValue: "children=2", CValue: "children=3"}},
		{language: "bitbake", name: "plain-assignment-control", source: "SUMMARY = \"hello\"\n", sourceSHA256: "b6a30f2e69cfe6ad0a3ca514e855b86b3c7e9403c5106d7d2239c3f42abea292", cDigest: "99187413f4fea3d1d6a10fb61768341c4188db1bd8582cdb041837d8ff49306b", goDigest: "99187413f4fea3d1d6a10fb61768341c4188db1bd8582cdb041837d8ff49306b", hasError: false, divergence: nil},
		{language: "bitbake", name: "malformed-shell-function", source: "do_install() {\n    echo \"missing close\"\n", sourceSHA256: "248dddc07429da83d3f05a14da1137270b7715dca43aa41abf58a51109b14477", cDigest: "f3ad263fcb44ab306ad7b1cc6a510840ce86c2683c7c94df7333fdec57edeae0", goDigest: "f3ad263fcb44ab306ad7b1cc6a510840ce86c2683c7c94df7333fdec57edeae0", hasError: true, divergence: nil},
		{language: "corn", name: "a0-compact", file: "corn/small__compact.corn", sourceSHA256: "e9793277f21b19593024cf7f670934333deefb54eae122caaa1f66cf41c7606a", cDigest: "6925a38d869d585460c85ab53d4d3486e675f6e6ed8e97f92e6c5702bcf951ad", goDigest: "6925a38d869d585460c85ab53d4d3486e675f6e6ed8e97f92e6c5702bcf951ad", hasError: false, divergence: nil},
		{language: "corn", name: "a0-complex", file: "corn/small__complex.corn", sourceSHA256: "98aaba0d478418a7855fa538cb9cd52ab81b2d2e581402d212bc3951a6a5db02", cDigest: "2e25bed381c2d79a7ba0bdd909dfbd30e738b3c8be29a7fc7352ea1fd975969a", goDigest: "2e25bed381c2d79a7ba0bdd909dfbd30e738b3c8be29a7fc7352ea1fd975969a", hasError: false, divergence: nil},
		{language: "corn", name: "a0-readme-example", file: "corn/small__readme_example.corn", sourceSHA256: "7d412d6e3c5e396818885601df14ad092b8cbe4aac690a45d7a4c29e0410da94", cDigest: "8d705f274c5421a5a60b9ef31013e8768f0c12dbd737bedca57ce3730a644a50", goDigest: "8d705f274c5421a5a60b9ef31013e8768f0c12dbd737bedca57ce3730a644a50", hasError: false, divergence: nil},
		{language: "corn", name: "quoted-keys-trigger", source: "{\n    'foo.bar' = 42\n    'green.eggs'.and.ham = \"hello world\"\n    'with spaces' = true\n    'escaped\\'quote' = false\n    'escaped=equals' = -3\n}\n", sourceSHA256: "7bcdf348ffbe92ef87dcb79b45ebae007811fe7af717c2d2cdea09f8c61872c8", cDigest: "92b1b5257e650d85af9692640cf30b287453eb7ec15af25f1c1560899d3b7615", goDigest: "714e0b57f2920716ba5ac832d2284ac17db8189e7e12c21f4f6978e3d3936e2e", hasError: true, divergence: &DumpV1Divergence{Path: "/source_file/object[0]", Category: "shape", GoValue: "children=7", CValue: "children=8"}},
		{language: "corn", name: "plain-path-control", source: "{\n    foo.bar = 42\n}\n", sourceSHA256: "40dc0c83705af7393578499a43eab93269e4b91b99f3c1ec901dad1fd76f4bd0", cDigest: "1c9dd43135dfa64e87c1af706f3fa9d66c420ec4003d54dd0e37e5d2f9896a08", goDigest: "1c9dd43135dfa64e87c1af706f3fa9d66c420ec4003d54dd0e37e5d2f9896a08", hasError: false, divergence: nil},
		{language: "corn", name: "malformed-quoted-path", source: "{\n    'foo.bar' = 42\n", sourceSHA256: "b86b200a916bc043d21c2b9b45f35968db1b88733c789984ae070cde7d56d324", cDigest: "e55d7e35692aa8895adedf98d61b37f5b4d6fbdbc78f5a4106ae2cac94badbdf", goDigest: "e55d7e35692aa8895adedf98d61b37f5b4d6fbdbc78f5a4106ae2cac94badbdf", hasError: true, divergence: nil},
		{language: "dtd", name: "pe-reference-trigger", source: "<!ELEMENT colspec %ho; EMPTY >", sourceSHA256: "f6903445e1a330ae0fd42b19c43538ea30b0da6261c1fe5ae452fc713597f0c7", cDigest: "5c2393834cf7a941dfc5e0c86dacb344cb122822631b379e21f9bf607544c860", goDigest: "5c2393834cf7a941dfc5e0c86dacb344cb122822631b379e21f9bf607544c860", hasError: true, divergence: nil},
		{language: "dtd", name: "medium-calstblx", file: "dtd/medium__calstblx.dtd", sourceSHA256: "54c96c2aa55e2a95b4d0f9ac30df90cfdd717fa1c52f6d3547f1cbd3c8ad4b85", cDigest: "6316281505e3891906174c07c691814c0b187d3619aa455fc01174efd2736a3e", goDigest: "6316281505e3891906174c07c691814c0b187d3619aa455fc01174efd2736a3e", hasError: true, divergence: nil},
		{language: "templ", name: "a0-medium-main", file: "templ/medium__main.templ", sourceSHA256: "4415618a310cc880cb67fcd902bb7e9f82e91b9d0f461349e0cfb5cd0b1fa007", cDigest: "efab90f3a4a75a4deba8c94d67c741dd842a7c8c6708bed3f59e37e0a994a11f", goDigest: "efab90f3a4a75a4deba8c94d67c741dd842a7c8c6708bed3f59e37e0a994a11f", hasError: false, divergence: nil},
		{language: "templ", name: "a0-medium-template", file: "templ/medium__template.templ", sourceSHA256: "e4a5934ad709206e1c5ca82ab9bc86cd20467df61484357096e6378b5dbb7791", cDigest: "7de9788750436a485bee98ec6200da09d5062700368333fe380562d71f171891", goDigest: "7de9788750436a485bee98ec6200da09d5062700368333fe380562d71f171891", hasError: false, divergence: nil},
		{language: "templ", name: "a0-small-template", file: "templ/small__template.templ", sourceSHA256: "bdc8798d13311d9f459108d3fad77f291dde4156fe68295671706684b8dd3eb3", cDigest: "cb81fe10587416eae568216d16d2f7258bda32d00136030d8a4fcd2198e12594", goDigest: "cb81fe10587416eae568216d16d2f7258bda32d00136030d8a4fcd2198e12594", hasError: false, divergence: nil},
		{language: "templ", name: "clean-component-import", source: "package p\n\n@templ.JSONScript(\"scriptData\", scriptData)\n", sourceSHA256: "54d6f6d873afb7b4155c87b08e3c70dedede40dfec36a0fc583d7129cbafc2e0", cDigest: "be90e19cd2f34f20303a530e3af710dae87e72d681d336aadcf7e5b605050cb6", goDigest: "93a6867460ccc0f0ce787d49d6a622d2bfa4839f5226a6ef197fb40623de22a9", hasError: true, divergence: &DumpV1Divergence{Path: "/source_file/ERROR[1]", Category: "shape", GoValue: "children=5", CValue: "children=7"}},
		{language: "templ", name: "malformed-dangling-quote", source: "package p\n\n<div title=\" >\n", sourceSHA256: "d5f89458e03f1f63c9b42ab2a7b76ddd3d4c7117b4b8c9b579374e631cd5cafc", cDigest: "6d468842ea01aeb519c3e7cf49e000862800c3b5324e4b2a6429616788c4cd42", goDigest: "7521467f2167c771e8d7ab1e66ad47f7e1bc0ecda761f64d8ce10e059b7f64e0", hasError: true, divergence: &DumpV1Divergence{Path: "/source_file", Category: "shape", GoValue: "children=4", CValue: "children=2"}},
		{language: "wolfram", name: "a0-large-EvaluationUtilities", file: "wolfram/large__EvaluationUtilities.wl", sourceSHA256: "e03c8588214ce3a0a5ba48d1f1335276c1826356052c33df1f3184a6d6303a53", cDigest: "44bd3aae9d0f0134a0728842d5a163e6fb1313baed36f6909667e44653a60346", goDigest: "12857be13d8a0b53c4bf08fb37748871a26ed36e46f7ca32d5747914add18253", hasError: true, divergence: &DumpV1Divergence{Path: "/source_file", Category: "type", GoValue: "source_file", CValue: "ERROR"}},
		{language: "wolfram", name: "a0-medium-OutputHandlingUtilities", file: "wolfram/medium__OutputHandlingUtilities.wl", sourceSHA256: "45a6287c3c8ad5f4f37298d4915d1bfb29e6e91ee0eccde1c842efb7c90e3dec", cDigest: "ba8e1803d5dc2f1ab722053029293532f3d2f179a3747046e05cf7def0190e75", goDigest: "1c30608767df40afdf461ecbe90dab00bf9d7b99eb0e68b30e868fb40885730e", hasError: true, divergence: &DumpV1Divergence{Path: "/source_file", Category: "type", GoValue: "source_file", CValue: "ERROR"}},
		{language: "wolfram", name: "a0-small-PacletInfo", file: "wolfram/small__PacletInfo.m", sourceSHA256: "55be9b6143e5dd68ddb433bb9c95c0388a505b65c452fb6036e064d537e3f602", cDigest: "8e9966945e16f3a6fa173cc41ed6a171821f4a196bb4198057786ba4edbd464d", goDigest: "8e9966945e16f3a6fa173cc41ed6a171821f4a196bb4198057786ba4edbd464d", hasError: true, divergence: nil},
		{language: "wolfram", name: "split-infix", source: "a + b\n", sourceSHA256: "2ff5f5b73b79baf991a11cf00ca251c8c436a9e4c768b907cd6c589211a87dec", cDigest: "f55efd4d7590d69c7a0cf938c4276db5a91c67fe864951e58fd6e222bb9dc3e8", goDigest: "f55efd4d7590d69c7a0cf938c4276db5a91c67fe864951e58fd6e222bb9dc3e8", hasError: false, divergence: nil},
		{language: "wolfram", name: "plain-symbol", source: "a\n", sourceSHA256: "87428fc522803d31065e7bce3cf03fe475096631e5e07bbd7a0fde60c4cf25c7", cDigest: "d417ec327539473cae7cc3baffce13f6391e31563ee60835f2f289baf5c1dcba", goDigest: "d417ec327539473cae7cc3baffce13f6391e31563ee60835f2f289baf5c1dcba", hasError: false, divergence: nil},
		{language: "wolfram", name: "malformed", source: "a +\n", sourceSHA256: "0743fffb28bab9c8a803d05f63061f6e9578b01cdcac165608c71392395fd5af", cDigest: "0239eda45e2781be67dce8d9a1cf61168e8b41e46c67f5d2be17795830b9c99a", goDigest: "647d0ecc7542b5eeab97d968c582b031258772a0d694f654a2662a88bdf96c3e", hasError: true, divergence: &DumpV1Divergence{Path: "/source_file", Category: "shape", GoValue: "children=2", CValue: "children=1"}},
	}
	for _, witness := range witnesses {
		witness := witness
		t.Run(witness.language+"/"+witness.name, func(t *testing.T) {
			scheduleParityMemoryScavenge(t)
			source := []byte(witness.source)
			if witness.file != "" {
				var err error
				source, err = os.ReadFile(filepath.Join("..", "testdata", "retired_dispatch_arms", filepath.FromSlash(witness.file)))
				if err != nil {
					t.Fatal(err)
				}
			}
			if got := fmt.Sprintf("%x", sha256.Sum256(source)); got != witness.sourceSHA256 {
				t.Fatalf("source SHA-256 = %s, want %s", got, witness.sourceSHA256)
			}
			language := languages[witness.language]()
			cLanguage, err := COracleLanguage(witness.language)
			if err != nil {
				t.Fatal(err)
			}
			cParser := sitter.NewParser()
			t.Cleanup(cParser.Close)
			if err := cParser.SetLanguage(cLanguage); err != nil {
				t.Fatal(err)
			}
			cTree := cParser.Parse(source, nil)
			if cTree == nil || cTree.RootNode() == nil {
				t.Fatal("locked C parser returned no root")
			}
			t.Cleanup(cTree.Close)
			cDigest, err := COracleDeepDigest(cTree)
			if err != nil {
				t.Fatal(err)
			}
			if cDigest != witness.cDigest {
				t.Fatalf("locked C digest = %s, want %s", cDigest, witness.cDigest)
			}

			parsers := []struct {
				route string
				parse func(*gotreesitter.Parser) (*gotreesitter.Tree, error)
			}{
				{"raw", func(p *gotreesitter.Parser) (*gotreesitter.Tree, error) {
					return p.ParseNoResultCompatibilityBenchmarkOnly(source)
				}},
				{"production", func(p *gotreesitter.Parser) (*gotreesitter.Tree, error) { return p.Parse(source) }},
			}
			for _, route := range parsers {
				parser := gotreesitter.NewParser(language)
				parser.SetAdmissionCandidateRoute(false)
				tree, err := route.parse(parser)
				if err != nil {
					t.Fatalf("%s parse: %v", route.route, err)
				}
				t.Cleanup(tree.Release)
				inspection, err := benchfixtures.InspectGoTree(tree.RootNode(), language)
				if err != nil {
					t.Fatal(err)
				}
				diff := FirstDivergenceDumpV1(tree.RootNode(), language, cTree.RootNode())
				if inspection.SHA256 != witness.goDigest ||
					tree.RootNode().HasError() != witness.hasError ||
					(diff == nil) != (witness.divergence == nil) ||
					(diff != nil && *diff != *witness.divergence) {
					t.Fatalf("%s tree digest=%s error=%t divergence=%+v, want digest=%s error=%t divergence=%+v",
						route.route, inspection.SHA256, tree.RootNode().HasError(), diff,
						witness.goDigest, witness.hasError, witness.divergence)
				}
				if rewritten := tree.ParseRuntime().NormalizationNodesRewritten; rewritten != 0 {
					t.Fatalf("%s route rewrote %d nodes", route.route, rewritten)
				}
				if witness.divergence == nil {
					if witness.goDigest != witness.cDigest {
						t.Fatalf("pinned exact witness has Go digest %s and C digest %s", witness.goDigest, witness.cDigest)
					}
					if err := firstLockedCTreeFlagDivergence(tree.RootNode(), language, cTree.RootNode(), "/"+tree.RootNode().Type(language)); err != nil {
						t.Fatalf("%s tree flags differ from locked C: %v", route.route, err)
					}
				}
			}
			t.Logf("source_bytes=%d go_digest=%s c_digest=%s exact=%t", len(source), witness.goDigest, witness.cDigest, witness.divergence == nil)
		})
	}
}
