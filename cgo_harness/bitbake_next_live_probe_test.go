//go:build cgo && treesitter_c_parity

package cgoharness

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	"github.com/odvcencio/gotreesitter/internal/benchfixtures"
	sitter "github.com/tree-sitter/go-tree-sitter"
)

const bitbakeNextAddtaskSource = `SUMMARY = "Test recipe for fetching git submodules"
HOMEPAGE = "http://git.yoctoproject.org/cgit/cgit.cgi/git-submodule-test/"
LICENSE = "MIT"
LIC_FILES_CHKSUM = "file://${COMMON_LICENSE_DIR}/MIT;md5=0835ade698e0bcf8506ecda2f7b4f302"

INHIBIT_DEFAULT_DEPS = "1"

# Note: this is intentionally not the latest version in the original .bb
SRCREV = "f280847494763cdcf71197557a81ba7d8a6bce42"
PV = "0.1+git"
PR = "r2"

SRC_URI = "gitsm://git.yoctoproject.org/git-submodule-test;branch=master;protocol=https"
UPSTREAM_CHECK_COMMITS = "1"
RECIPE_NO_UPDATE_REASON = "This recipe is used to test devtool upgrade feature"

EXCLUDE_FROM_WORLD = "1"

do_test_git_as_user() {
    cd ${S}
    git status
    git submodule status
}
addtask test_git_as_user after do_unpack

fakeroot do_test_git_as_root() {
    cd ${S}
    git status
    git submodule status
}
do_test_git_as_root[depends] += "virtual/fakeroot-native:do_populate_sysroot"
addtask test_git_as_root after do_unpack`

const bitbakeNextFunctionFlagSource = `do_run_tests () {
    meson test -C "${B}" --no-rebuild
}
do_run_tests[doc] = "Run meson test using qemu-user"
addtask do_run_tests after do_compile`

const bitbakeNextOverrideSource = `do_install:append() {
    ln -sf am335x-bonegreen-ext.dtb "${D}/boot/devicetree/am335x-bonegreen-ext-alias.dtb"
}

do_deploy:append() {
    ln -sf am335x-bonegreen-ext.dtb "${DEPLOYDIR}/devicetree/am335x-bonegreen-ext-alias.dtb"
}`

const bitbakeNextMalformedSource = `do_install() {
    echo "missing close"
`

// TestBitbakeNextLiveArmProbe records every route before a possible retirement.
// It keeps recovery, shell-function, and flag-assignment behavior visible.
func TestBitbakeNextLiveArmProbe(t *testing.T) {
	t.Setenv("GTS_DISPATCHER_CENSUS", "1")
	language := grammars.BitbakeLanguage()
	if language.ExternalScanner == nil {
		t.Fatal("BitBake language has no registered external scanner")
	}
	if reusable, ok := language.ExternalScanner.(gotreesitter.IncrementalReuseExternalScanner); ok && reusable.SupportsIncrementalReuse() {
		t.Fatal("BitBake external scanner unexpectedly supports incremental reuse")
	}
	cLanguage, err := COracleLanguage("bitbake")
	if err != nil {
		t.Fatal(err)
	}

	witnesses := []struct {
		name      string
		file      string
		source    []byte
		malformed bool
	}{
		{name: "a0-small-error", file: "small__error.bb"},
		{name: "a0-medium-clang-git", file: "medium__clang_git.bb"},
		{name: "a0-large-linux-firmware", file: "large__linux-firmware_20260519.bb"},
		{name: "addtask-error-wrapper", source: []byte(bitbakeNextAddtaskSource)},
		{name: "function-flag-assignment", source: []byte(bitbakeNextFunctionFlagSource)},
		{name: "adjacent-override-functions", source: []byte(bitbakeNextOverrideSource)},
		{name: "plain-assignment-control", source: []byte("SUMMARY = \"hello\"\n")},
		{name: "malformed-shell-function", source: []byte(bitbakeNextMalformedSource), malformed: true},
	}

	for _, witness := range witnesses {
		witness := witness
		t.Run(witness.name, func(t *testing.T) {
			source := witness.source
			if source == nil {
				var err error
				source, err = os.ReadFile(filepath.Join(
					"..", "testdata", "dispatcher_census_a0", "bitbake", witness.file,
				))
				if err != nil {
					t.Fatal(err)
				}
			}
			t.Logf("witness=%s malformed=%t bytes=%d source_sha256=%x", witness.name, witness.malformed, len(source), sha256.Sum256(source))

			cParser := sitter.NewParser()
			t.Cleanup(cParser.Close)
			if err := cParser.SetLanguage(cLanguage); err != nil {
				t.Fatal(err)
			}
			cTree := cParser.Parse(source, nil)
			if cTree == nil || cTree.RootNode() == nil {
				t.Fatal("C parser returned no root")
			}
			t.Cleanup(cTree.Close)
			cDigest, err := COracleDeepDigest(cTree)
			if err != nil {
				t.Fatal(err)
			}

			rawParser := gotreesitter.NewParser(language)
			rawParser.SetAdmissionCandidateRoute(false)
			raw, err := rawParser.ParseNoResultCompatibilityBenchmarkOnly(source)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(raw.Release)
			t.Logf("route=raw %s", bitbakeNextReceipt(raw, language, cTree, cDigest))

			productionParser := gotreesitter.NewParser(language)
			productionParser.SetAdmissionCandidateRoute(false)
			production, err := productionParser.Parse(source)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(production.Release)
			t.Logf("route=production %s", bitbakeNextReceipt(production, language, cTree, cDigest))

			routedBefore, fallbackBefore := gotreesitter.AdmissionCandidateCounters()
			compactParser := gotreesitter.NewParser(language)
			compactParser.SetAdmissionCandidateRoute(true)
			compact, err := compactParser.Parse(source)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(compact.Release)
			routedAfter, fallbackAfter := gotreesitter.AdmissionCandidateCounters()
			compactMode := fmt.Sprintf("counters=%d/%d->%d/%d", routedBefore, fallbackBefore, routedAfter, fallbackAfter)
			if routedAfter == routedBefore && fallbackAfter == fallbackBefore+1 {
				compactMode += " fallback:" + gotreesitter.AdmissionCandidateLastFallbackReason()
			} else if routedAfter == routedBefore+1 && fallbackAfter == fallbackBefore {
				compactMode += " accepted"
			}
			t.Logf("route=compact mode=%s %s", compactMode, bitbakeNextReceipt(compact, language, cTree, cDigest))

			forestParser := gotreesitter.NewParser(language)
			forest, forestOK := forestParser.ParseForestExperimental(source)
			if forestOK && forest != nil {
				t.Cleanup(forest.Release)
				t.Logf("route=forest mode=accepted %s", bitbakeNextReceipt(forest, language, cTree, cDigest))
			} else {
				t.Log("route=forest mode=declined")
			}

			incrementalParser := gotreesitter.NewParser(language)
			incrementalParser.SetAdmissionCandidateRoute(false)
			base := bytes.TrimSuffix(source, []byte{'\n'})
			oldTree, err := incrementalParser.Parse(base)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(oldTree.Release)
			oldTree.Edit(gotreesitter.InputEdit{
				StartByte:   uint32(len(base)),
				OldEndByte:  uint32(len(base)),
				NewEndByte:  uint32(len(source)),
				StartPoint:  bitbakeNextPointAtByte(base),
				OldEndPoint: bitbakeNextPointAtByte(base),
				NewEndPoint: bitbakeNextPointAtByte(source),
			})
			incremental, profile, err := incrementalParser.ParseIncrementalProfiled(source, oldTree)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(incremental.Release)
			t.Logf("route=incremental reuse=%t unsupported=%t reason=%q reused_subtrees=%d reused_bytes=%d %s", profile.OldTreeReuseRoute, profile.ReuseUnsupported, profile.ReuseUnsupportedReason, profile.ReusedSubtrees, profile.ReusedBytes, bitbakeNextReceipt(incremental, language, cTree, cDigest))
		})
	}
}

// TestBitbakeNextLiveArmReceiptDocument guards the blocker receipt markers.
func TestBitbakeNextLiveArmReceiptDocument(t *testing.T) {
	raw, err := os.ReadFile("../docs/root-normalization-retirement.md")
	if err != nil {
		t.Fatal(err)
	}
	document := strings.Join(strings.Fields(string(raw)), " ")
	for _, marker := range []string{
		"The BitBake arm remains live.",
		"A0 has three BitBake files, three checked, three run, and zero rewrites.",
		"The medium A0 witness differs from locked C at the recipe shape.",
		"The malformed shell-function witness remains a recovery blocker.",
		"BitBake has a registered external scanner. It does not implement incremental reuse support.",
		"Keep dispatch.bitbake live until scheduler_action_semantics emits the locked-C shell-function tree.",
	} {
		marker = strings.Join(strings.Fields(marker), " ")
		if !strings.Contains(document, marker) {
			t.Fatalf("BitBake blocker receipt lacks marker %q", marker)
		}
	}
}

func bitbakeNextReceipt(tree *gotreesitter.Tree, language *gotreesitter.Language, cTree *sitter.Tree, cDigest string) string {
	if tree == nil || tree.RootNode() == nil {
		return "tree=nil"
	}
	inspection, err := benchfixtures.InspectGoTree(tree.RootNode(), language)
	if err != nil {
		return fmt.Sprintf("inspect_error=%v", err)
	}
	diff := FirstDivergenceDumpV1(tree.RootNode(), language, cTree.RootNode())
	return fmt.Sprintf("error_root=%t digest=%s c_digest=%s exact=%t rewrites=%d passes=%s divergence=%+v", tree.RootNode().HasError(), inspection.SHA256, cDigest, diff == nil && inspection.SHA256 == cDigest, tree.ParseRuntime().NormalizationNodesRewritten, bitbakeNextPasses(tree), diff)
}

func bitbakeNextPasses(tree *gotreesitter.Tree) string {
	runtime := tree.ParseRuntime()
	if runtime.NormalizationPasses == nil {
		return "none"
	}
	parts := make([]string, 0, len(*runtime.NormalizationPasses))
	for _, pass := range *runtime.NormalizationPasses {
		parts = append(parts, fmt.Sprintf("%s:%d/%d/%d/%d", pass.Name, pass.Checked, pass.Run, pass.NodesVisited, pass.NodesRewritten))
	}
	return fmt.Sprintf("%v", parts)
}

func bitbakeNextPointAtByte(source []byte) gotreesitter.Point {
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
