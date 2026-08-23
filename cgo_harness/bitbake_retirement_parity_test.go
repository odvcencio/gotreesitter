//go:build cgo && treesitter_c_parity

package cgoharness

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/internal/benchfixtures"
	sitter "github.com/tree-sitter/go-tree-sitter"
)

const bitbakeAddtaskTriggerSource = `SUMMARY = "Test recipe for fetching git submodules"
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

const bitbakeFunctionFlagTriggerSource = `do_run_tests () {
    meson test -C "${B}" --no-rebuild
}
do_run_tests[doc] = "Run meson test using qemu-user"
addtask do_run_tests after do_compile`

const bitbakeAdjacentOverrideTriggerSource = `do_install:append() {
    ln -sf am335x-bonegreen-ext.dtb "${D}/boot/devicetree/am335x-bonegreen-ext-alias.dtb"
}

do_deploy:append() {
    ln -sf am335x-bonegreen-ext.dtb "${DEPLOYDIR}/devicetree/am335x-bonegreen-ext-alias.dtb"
}`

// TestBitbakeNormalizationCensusLockedCExact compares all A0 BitBake
// fixtures and the existing parser trigger sources against the pinned C grammar.
func TestBitbakeNormalizationCensusLockedCExact(t *testing.T) {
	entry, ok := parityEntriesByName["bitbake"]
	if !ok {
		t.Fatal("missing BitBake grammar entry")
	}
	language := entry.Language()
	cLanguage, err := COracleLanguage("bitbake")
	if err != nil {
		t.Fatal(err)
	}
	identity, err := COracleIdentity("bitbake")
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("C oracle contract=%s binding=%s runtime=%s grammar_commit=%s artifact_sha256=%s", identity.Contract, identity.BindingVersion, identity.RuntimeVersion, identity.GrammarCommit, identity.GrammarArtifactSHA256)

	tests := []struct {
		name   string
		file   string
		sha256 string
		source []byte
	}{
		{
			name:   "a0-small-error",
			file:   "small__error.bb",
			sha256: "fbdb85e443edd378e944e5a1416c0c4a1e485f0cd38f70a5ac75748073a15d12",
		},
		{
			name:   "a0-medium-clang-git",
			file:   "medium__clang_git.bb",
			sha256: "7deb41efd839d8b5b8b2c98589614377d12fd81fa6033824330084e07c5eaf9f",
		},
		{
			name:   "a0-large-linux-firmware",
			file:   "large__linux-firmware_20260519.bb",
			sha256: "eaa9e3f2354345d558717c4791a67144d8b27767674bf5468e157a0e0a332ff6",
		},
		{
			name:   "trigger-addtask-error-wrapper",
			sha256: "35ccf9d007ef76548258088b18adb39d7c0509452b4fb62bd7dfdc94cdcdf780",
			source: []byte(bitbakeAddtaskTriggerSource),
		},
		{
			name:   "trigger-function-flag-assignment",
			sha256: "8655832f5acd3b4ada197881f41b0110657572aa0c791ecf4524fbc10603a1eb",
			source: []byte(bitbakeFunctionFlagTriggerSource),
		},
		{
			name:   "trigger-adjacent-override-functions",
			sha256: "068037e13b101cf01d0a36da586f798972fa39b99ba0c2a86dc17996bc34d185",
			source: []byte(bitbakeAdjacentOverrideTriggerSource),
		},
	}

	for _, test := range tests {
		test := test
		if !t.Run(test.name, func(t *testing.T) {
			source := test.source
			if source == nil {
				var err error
				source, err = os.ReadFile(filepath.Join(
					"..", "testdata", "dispatcher_census_a0", "bitbake", test.file,
				))
				if err != nil {
					t.Fatal(err)
				}
			}
			if got := fmt.Sprintf("%x", sha256.Sum256(source)); got != test.sha256 {
				t.Fatalf("source SHA-256 = %s, want %s", got, test.sha256)
			}

			cParser := sitter.NewParser()
			t.Cleanup(cParser.Close)
			if err := cParser.SetLanguage(cLanguage); err != nil {
				t.Fatal(err)
			}
			cTree := cParser.Parse(source, nil)
			if cTree == nil || cTree.RootNode() == nil {
				t.Fatal("locked C parser returned a nil tree")
			}
			t.Cleanup(cTree.Close)
			cDigest, err := COracleDeepDigest(cTree)
			if err != nil {
				t.Fatalf("inspect locked C deep tree: %v", err)
			}

			rawParser := gotreesitter.NewParser(language)
			rawParser.SetAdmissionCandidateRoute(false)
			rawTree, err := rawParser.ParseNoResultCompatibilityBenchmarkOnly(source)
			if err != nil {
				t.Fatalf("raw parse: %v", err)
			}
			t.Cleanup(rawTree.Release)
			rawRuntime := rawTree.ParseRuntime()
			rawDigest := assertBitbakeLockedCTreeExact(t, "raw", rawTree, language, cTree, cDigest)

			productionParser := gotreesitter.NewParser(language)
			productionParser.SetAdmissionCandidateRoute(false)
			productionTree, err := productionParser.Parse(source)
			if err != nil {
				t.Fatalf("production parse: %v", err)
			}
			t.Cleanup(productionTree.Release)
			productionRuntime := productionTree.ParseRuntime()
			productionDigest := assertBitbakeLockedCTreeExact(t, "production", productionTree, language, cTree, cDigest)

			t.Logf("witness=%s bytes=%d source_sha256=%s c_digest=%s raw_digest=%s production_digest=%s raw_rewrites=%d production_rewrites=%d", test.name, len(source), test.sha256, cDigest, rawDigest, productionDigest, rawRuntime.NormalizationNodesRewritten, productionRuntime.NormalizationNodesRewritten)
		}) {
			return
		}
	}
}

func assertBitbakeLockedCTreeExact(
	t *testing.T,
	label string,
	goTree *gotreesitter.Tree,
	goLang *gotreesitter.Language,
	cTree *sitter.Tree,
	wantDigest string,
) string {
	t.Helper()
	goRoot := goTree.RootNode()
	cRoot := cTree.RootNode()
	if diff := FirstDivergenceDumpV1(goRoot, goLang, cRoot); diff != nil {
		t.Fatalf("%s tree diverges from the locked C oracle: %+v", label, diff)
	}
	if diff := firstLockedCTreeFlagDivergence(goRoot, goLang, cRoot, "/"+goRoot.Type(goLang)); diff != nil {
		t.Fatalf("%s tree has a missing or error flag divergence: %v", label, diff)
	}
	inspection, err := benchfixtures.InspectGoTree(goRoot, goLang)
	if err != nil {
		t.Fatalf("inspect %s Go deep tree: %v", label, err)
	}
	if inspection.SHA256 != wantDigest {
		t.Fatalf("%s deep digest Go=%s C=%s", label, inspection.SHA256, wantDigest)
	}
	t.Logf("%s route matches locked C exactly: symbols, fields, spans, points, child order, named/extra/missing/error flags, deep digest=%s", label, inspection.SHA256)
	return inspection.SHA256
}
