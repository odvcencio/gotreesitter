//go:build !grammar_subset

package grammars

import (
	"os"
	"path/filepath"
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/internal/benchfixtures"
)

// bitbakeRetiredAddtaskSource is the recipe that once triggered the BitBake
// addtask ERROR-wrapper repair.
const bitbakeRetiredAddtaskSource = "SUMMARY = \"Test recipe for fetching git submodules\"\nHOMEPAGE = \"http://git.yoctoproject.org/cgit/cgit.cgi/git-submodule-test/\"\nLICENSE = \"MIT\"\nLIC_FILES_CHKSUM = \"file://${COMMON_LICENSE_DIR}/MIT;md5=0835ade698e0bcf8506ecda2f7b4f302\"\n\nINHIBIT_DEFAULT_DEPS = \"1\"\n\n# Note: this is intentionally not the latest version in the original .bb\nSRCREV = \"f280847494763cdcf71197557a81ba7d8a6bce42\"\nPV = \"0.1+git\"\nPR = \"r2\"\n\nSRC_URI = \"gitsm://git.yoctoproject.org/git-submodule-test;branch=master;protocol=https\"\nUPSTREAM_CHECK_COMMITS = \"1\"\nRECIPE_NO_UPDATE_REASON = \"This recipe is used to test devtool upgrade feature\"\n\nEXCLUDE_FROM_WORLD = \"1\"\n\ndo_test_git_as_user() {\n    cd ${S}\n    git status\n    git submodule status\n}\naddtask test_git_as_user after do_unpack\n\nfakeroot do_test_git_as_root() {\n    cd ${S}\n    git status\n    git submodule status\n}\ndo_test_git_as_root[depends] += \"virtual/fakeroot-native:do_populate_sysroot\"\naddtask test_git_as_root after do_unpack"

// TestRetiredInertArmRouteReceipts covers the five result-compatibility arms
// that never rewrote a tree on any registered witness: bitbake, corn, dtd,
// templ, and wolfram. With the arms removed, every route must return
// the tree the engine builds without any result compatibility, the dispatcher
// must record no pass for the language, and no route may rewrite a node. The
// locked-C comparison for the same witnesses is
// cgo_harness/retired_inert_arm_parity_test.go.
func TestRetiredInertArmRouteReceipts(t *testing.T) {
	t.Setenv("GTS_DISPATCHER_CENSUS", "1")

	languages := map[string]func() *gotreesitter.Language{
		"bitbake": BitbakeLanguage,
		"corn":    CornLanguage,
		"dtd":     DtdLanguage,
		"templ":   TemplLanguage,
		"wolfram": WolframLanguage,
	}
	const fixtureRoot = "../testdata/retired_dispatch_arms"
	witnesses := []struct {
		language string
		name     string
		path     string
		source   string
	}{
		{language: "bitbake", name: "a0-small-error", path: "bitbake/small__error.bb"},
		{language: "bitbake", name: "a0-medium-clang-git", path: "bitbake/medium__clang_git.bb"},
		{language: "bitbake", name: "addtask-error-wrapper", source: bitbakeRetiredAddtaskSource},
		{language: "bitbake", name: "function-flag-assignment", source: "do_run_tests () {\n    meson test -C \"${B}\" --no-rebuild\n}\ndo_run_tests[doc] = \"Run meson test using qemu-user\"\naddtask do_run_tests after do_compile"},
		{language: "bitbake", name: "adjacent-override-functions", source: "do_install:append() {\n    ln -sf am335x-bonegreen-ext.dtb \"${D}/boot/devicetree/am335x-bonegreen-ext-alias.dtb\"\n}\n\ndo_deploy:append() {\n    ln -sf am335x-bonegreen-ext.dtb \"${DEPLOYDIR}/devicetree/am335x-bonegreen-ext-alias.dtb\"\n}"},
		{language: "bitbake", name: "plain-assignment-control", source: "SUMMARY = \"hello\"\n"},
		{language: "bitbake", name: "malformed-shell-function", source: "do_install() {\n    echo \"missing close\"\n"},
		{language: "corn", name: "a0-compact", path: "corn/small__compact.corn"},
		{language: "corn", name: "a0-complex", path: "corn/small__complex.corn"},
		{language: "corn", name: "a0-readme-example", path: "corn/small__readme_example.corn"},
		{language: "corn", name: "quoted-keys-trigger", source: "{\n    'foo.bar' = 42\n    'green.eggs'.and.ham = \"hello world\"\n    'with spaces' = true\n    'escaped\\'quote' = false\n    'escaped=equals' = -3\n}\n"},
		{language: "corn", name: "plain-path-control", source: "{\n    foo.bar = 42\n}\n"},
		{language: "corn", name: "malformed-quoted-path", source: "{\n    'foo.bar' = 42\n"},
		{language: "dtd", name: "parser-produced-pe-reference-trigger", source: "<!ELEMENT colspec %ho; EMPTY >"},
		{language: "dtd", name: "medium-calstblx", path: "dtd/medium__calstblx.dtd"},
		{language: "templ", name: "a0-medium-main", path: "templ/medium__main.templ"},
		{language: "templ", name: "a0-medium-template", path: "templ/medium__template.templ"},
		{language: "templ", name: "a0-small-template", path: "templ/small__template.templ"},
		{language: "templ", name: "clean-component-import", source: "package p\n\n@templ.JSONScript(\"scriptData\", scriptData)\n"},
		{language: "templ", name: "malformed-dangling-quote", source: "package p\n\n<div title=\" >\n"},
		{language: "wolfram", name: "a0-large-EvaluationUtilities", path: "wolfram/large__EvaluationUtilities.wl"},
		{language: "wolfram", name: "a0-medium-OutputHandlingUtilities", path: "wolfram/medium__OutputHandlingUtilities.wl"},
		{language: "wolfram", name: "a0-small-PacletInfo", path: "wolfram/small__PacletInfo.m"},
		{language: "wolfram", name: "plain-symbol", source: "a\n"},
		{language: "wolfram", name: "split-infix", source: "a + b\n"},
		{language: "wolfram", name: "malformed", source: "a +\n"},
	}

	for _, witness := range witnesses {
		witness := witness
		t.Run(witness.language+"/"+witness.name, func(t *testing.T) {
			source := []byte(witness.source)
			if witness.path != "" {
				var err error
				source, err = os.ReadFile(filepath.Join(fixtureRoot, filepath.FromSlash(witness.path)))
				if err != nil {
					t.Fatal(err)
				}
			}
			language := languages[witness.language]()
			dispatchName := "dispatch." + witness.language

			raw, err := gotreesitter.NewParser(language).
				ParseNoResultCompatibilityBenchmarkOnly(source)
			if err != nil {
				t.Fatalf("raw parse failed: %v", err)
			}
			t.Cleanup(raw.Release)
			rawInspection, err := benchfixtures.InspectGoTree(raw.RootNode(), language)
			if err != nil {
				t.Fatal(err)
			}

			receipts := retiredDispatchRouteReceiptsAllowCompactAndForestFallbackExactSource(t, language, source)
			for _, receipt := range receipts {
				inspection, err := benchfixtures.InspectGoTree(receipt.tree.RootNode(), language)
				if err != nil {
					t.Fatal(err)
				}
				if inspection.SHA256 != rawInspection.SHA256 {
					t.Fatalf("route %s digest=%s, want the raw engine tree %s", receipt.name, inspection.SHA256, rawInspection.SHA256)
				}
				runtime := receipt.tree.ParseRuntime()
				if runtime.NormalizationNodesRewritten != 0 {
					t.Fatalf("route %s rewrote %d nodes", receipt.name, runtime.NormalizationNodesRewritten)
				}
				if runtime.NormalizationPasses != nil {
					for _, pass := range *runtime.NormalizationPasses {
						if pass.Name == dispatchName {
							t.Fatalf("route %s recorded retired pass %s: %+v", receipt.name, dispatchName, pass)
						}
					}
				}
			}
			t.Logf("source_bytes=%d routes=%d digest=%s", len(source), len(receipts), rawInspection.SHA256)
		})
	}
}
