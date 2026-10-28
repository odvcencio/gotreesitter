package parse_test

import (
	"testing"

	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

func TestBitbakeUnwrapsAddtaskErrorWrapper(t *testing.T) {
	src := []byte(`SUMMARY = "Test recipe for fetching git submodules"
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
addtask test_git_as_root after do_unpack`)

	lang := grammars.BitbakeLanguage()
	tree, err := gts.NewParser(lang).Parse(src)
	if err != nil || tree == nil || tree.RootNode() == nil {
		t.Fatalf("parse failed: tree=%v err=%v", tree, err)
	}
	defer tree.Release()

	root := tree.RootNode()
	last := root.Child(root.ChildCount() - 1)
	if got, want := last.Type(lang), "addtask_statement"; got != want {
		t.Fatalf("last child type = %q, want %q; tree=%s", got, want, root.SExpr(lang))
	}
}

func TestBitbakeSplitsRecoveredFunctionFlagAssignment(t *testing.T) {
	src := []byte(`do_run_tests () {
    meson test -C "${B}" --no-rebuild
}
do_run_tests[doc] = "Run meson test using qemu-user"
addtask do_run_tests after do_compile`)

	lang := grammars.BitbakeLanguage()
	tree, err := gts.NewParser(lang).Parse(src)
	if err != nil || tree == nil || tree.RootNode() == nil {
		t.Fatalf("parse failed: tree=%v err=%v", tree, err)
	}
	defer tree.Release()

	root := tree.RootNode()
	if got, want := root.ChildCount(), 3; got != want {
		t.Fatalf("root child count = %d, want %d; tree=%s", got, want, root.SExpr(lang))
	}
	if got, want := root.Child(0).Type(lang), "function_definition"; got != want {
		t.Fatalf("first child type = %q, want %q; tree=%s", got, want, root.SExpr(lang))
	}
	if got, want := root.Child(1).Type(lang), "variable_assignment"; got != want {
		t.Fatalf("second child type = %q, want %q; tree=%s", got, want, root.SExpr(lang))
	}
	if got, want := root.Child(2).Type(lang), "addtask_statement"; got != want {
		t.Fatalf("third child type = %q, want %q; tree=%s", got, want, root.SExpr(lang))
	}
}

func TestBitbakeSplitsRecoveredAdjacentOverrideFunctions(t *testing.T) {
	src := []byte(`do_install:append() {
    ln -sf am335x-bonegreen-ext.dtb "${D}/boot/devicetree/am335x-bonegreen-ext-alias.dtb"
}

do_deploy:append() {
    ln -sf am335x-bonegreen-ext.dtb "${DEPLOYDIR}/devicetree/am335x-bonegreen-ext-alias.dtb"
}`)

	lang := grammars.BitbakeLanguage()
	tree, err := gts.NewParser(lang).Parse(src)
	if err != nil || tree == nil || tree.RootNode() == nil {
		t.Fatalf("parse failed: tree=%v err=%v", tree, err)
	}
	defer tree.Release()

	root := tree.RootNode()
	if got, want := root.ChildCount(), 2; got != want {
		t.Fatalf("root child count = %d, want %d; tree=%s", got, want, root.SExpr(lang))
	}
	for i := 0; i < 2; i++ {
		if got, want := root.Child(i).Type(lang), "function_definition"; got != want {
			t.Fatalf("child %d type = %q, want %q; tree=%s", i, got, want, root.SExpr(lang))
		}
	}
}
