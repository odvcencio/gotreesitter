//go:build !grammar_subset || grammar_subset_authzed

package grammars

import (
	"fmt"
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
)

func TestAuthzedTokenSourceRebuildStartsFreshAndPreservesCursor(t *testing.T) {
	lang := AuthzedLanguage()
	original, err := NewAuthzedTokenSource([]byte("definition old {}"), lang)
	if err != nil {
		t.Fatal(err)
	}
	original.Next()
	var rebuilder gotreesitter.TokenSourceRebuilder = original
	source := []byte("definition next {}")
	rebuilt, err := rebuilder.RebuildTokenSource(source, nil)
	if err != nil {
		t.Fatal(err)
	}
	if rebuilt == original {
		t.Fatal("rebuild must return an independent token stream")
	}
	tree, err := gotreesitter.NewParser(lang).ParseWithTokenSource(source, rebuilt)
	if err != nil {
		t.Fatal(err)
	}
	defer tree.Release()
	if tree.RootNode().HasError() || tree.RootNode().EndByte() != uint32(len(source)) {
		t.Fatal("rebuilt stream must parse the complete new definition from its start")
	}
	if token := original.Next(); token.StartByte != uint32(len("definition")) {
		t.Fatalf("rebuild changed the original cursor: next token starts at %d, want the space after definition", token.StartByte)
	}
}

func TestAuthzedTokenSourceParsesSpiceDBSchemaBasic(t *testing.T) {
	src := []byte("caveat foo(someParam int) {\n\tsomeParam == 42\n}\n\n" +
		"definition document {\n" +
		"\trelation viewer: user | user:*\n" +
		"\trelation editor: user | group#member with foo\n" +
		"\trelation parent: organization\n" +
		"\tpermission edit = editor\n" +
		"\tpermission view = viewer + edit + parent->view\n" +
		"\tpermission other = viewer - edit\n" +
		"\tpermission intersect = viewer & edit\n" +
		"\tpermission with_nil = (viewer - edit) & parent->view & nil\n" +
		"}\n")

	lang := AuthzedLanguage()
	parser := gotreesitter.NewParser(lang)
	ts := &recordingAuthzedTokenSource{
		base: NewAuthzedTokenSourceOrEOF(src, lang),
		lang: lang,
	}
	tree, err := parser.ParseWithTokenSource(src, ts)
	if err != nil {
		t.Fatalf("ParseWithTokenSource failed: %v", err)
	}
	defer tree.Release()

	root := tree.RootNode()
	if got := root.Type(lang); got != "source_file" {
		t.Logf("tokens: %v", ts.tokens)
		t.Fatalf("root type = %q, want source_file", got)
	}
	if root.HasError() {
		t.Logf("tokens: %v", ts.tokens)
		t.Fatal("root has syntax errors")
	}
	if got, want := root.EndByte(), uint32(len(src)); got != want {
		t.Logf("tokens: %v", ts.tokens)
		t.Fatalf("root end byte = %d, want %d", got, want)
	}
}

func TestAuthzedTokenSourceRecoversSingleQuotedCaveatLiteralLikeC(t *testing.T) {
	src := []byte("definition another {}\n\n" +
		"caveat somecaveat(somecondition uint, somebool bool, somestring string) {\n" +
		"  somecondition == 42 && somebool && somestring == 'hello'\n" +
		"}\n\n" +
		"definition user {}")

	lang := AuthzedLanguage()
	parser := gotreesitter.NewParser(lang)
	tree, err := parser.ParseWithTokenSource(src, NewAuthzedTokenSourceOrEOF(src, lang))
	if err != nil {
		t.Fatalf("ParseWithTokenSource failed: %v", err)
	}
	defer tree.Release()

	root := tree.RootNode()
	if got := root.Type(lang); got != "source_file" {
		t.Fatalf("root type = %q, want source_file", got)
	}
	if !root.HasError() {
		t.Fatal("root HasError = false, want true")
	}
	if got, want := root.ChildCount(), 6; got != want {
		t.Fatalf("root child count = %d, want %d", got, want)
	}
	caveat := root.Child(2)
	if caveat == nil || caveat.Type(lang) != "caveat" {
		t.Fatalf("root child[2] type = %v, want caveat", nodeTypeForTest(caveat, lang))
	}
	if !caveat.HasError() {
		t.Fatal("caveat HasError = false, want true")
	}
	block := caveat.Child(3)
	if block == nil || block.Type(lang) != "block_c" {
		t.Fatalf("caveat child[3] type = %v, want block_c", nodeTypeForTest(block, lang))
	}
	if !block.HasError() {
		t.Fatal("block_c HasError = false, want true")
	}
	if got, want := block.ChildCount(), 5; got != want {
		t.Fatalf("block_c child count = %d, want %d", got, want)
	}
	errNode := block.Child(2)
	if errNode == nil || errNode.Type(lang) != "ERROR" {
		t.Fatalf("block_c child[2] type = %v, want ERROR", nodeTypeForTest(errNode, lang))
	}
	if got, want := errNode.StartByte(), uint32(145); got != want {
		t.Fatalf("ERROR start byte = %d, want %d", got, want)
	}
	if got, want := errNode.EndByte(), uint32(155); got != want {
		t.Fatalf("ERROR end byte = %d, want %d", got, want)
	}
	if got, want := errNode.ChildCount(), 1; got != want {
		t.Fatalf("ERROR child count = %d, want %d", got, want)
	}
	if got := errNode.Child(0).Type(lang); got != "==" {
		t.Fatalf("ERROR child[0] type = %q, want ==", got)
	}
}

func TestAuthzedTokenSourceRecoversStrayCaveatTailLikeC(t *testing.T) {
	src := []byte("definition user {}\n\n" +
		"caveat somecaveat(somecondition int) {\n" +
		"  somecondition == 42 `\n" +
		"}")

	lang := AuthzedLanguage()
	parser := gotreesitter.NewParser(lang)
	tree, err := parser.ParseWithTokenSource(src, NewAuthzedTokenSourceOrEOF(src, lang))
	if err != nil {
		t.Fatalf("ParseWithTokenSource failed: %v", err)
	}
	defer tree.Release()

	root := tree.RootNode()
	if got := root.Type(lang); got != "source_file" {
		t.Fatalf("root type = %q, want source_file", got)
	}
	if got, want := root.ChildCount(), 4; got != want {
		t.Fatalf("root child count = %d, want %d", got, want)
	}
	caveat := root.Child(2)
	if caveat == nil || caveat.Type(lang) != "caveat" {
		t.Fatalf("root child[2] type = %v, want caveat", nodeTypeForTest(caveat, lang))
	}
	block := caveat.Child(3)
	if block == nil || block.Type(lang) != "block_c" {
		t.Fatalf("caveat child[3] type = %v, want block_c", nodeTypeForTest(block, lang))
	}
	if got, want := block.ChildCount(), 5; got != want {
		t.Fatalf("block_c child count = %d, want %d", got, want)
	}
	errNode := block.Child(2)
	if errNode == nil || errNode.Type(lang) != "ERROR" {
		t.Fatalf("block_c child[2] type = %v, want ERROR", nodeTypeForTest(errNode, lang))
	}
	if got, want := errNode.StartByte(), uint32(81); got != want {
		t.Fatalf("ERROR start byte = %d, want %d", got, want)
	}
	if got, want := errNode.EndByte(), uint32(82); got != want {
		t.Fatalf("ERROR end byte = %d, want %d", got, want)
	}
}

func TestAuthzedTokenSourceRecoversUnclosedCaveatLikeC(t *testing.T) {
	src := []byte("definition another {}\n\n" +
		"caveat somecaveat(somecondition uint, somebool bool) {\n" +
		"  somemap{\n\n" +
		"  \n" +
		"}\n\n" +
		"definition user {}")

	lang := AuthzedLanguage()
	parser := gotreesitter.NewParser(lang)
	tree, err := parser.ParseWithTokenSource(src, NewAuthzedTokenSourceOrEOF(src, lang))
	if err != nil {
		t.Fatalf("ParseWithTokenSource failed: %v", err)
	}
	defer tree.Release()

	root := tree.RootNode()
	if got := root.Type(lang); got != "source_file" {
		t.Fatalf("root type = %q, want source_file", got)
	}
	if got, want := root.ChildCount(), 6; got != want {
		t.Fatalf("root child count = %d, want %d", got, want)
	}
	caveat := root.Child(2)
	if caveat == nil || caveat.Type(lang) != "caveat" {
		t.Fatalf("root child[2] type = %v, want caveat", nodeTypeForTest(caveat, lang))
	}
	if got, want := caveat.EndByte(), uint32(94); got != want {
		t.Fatalf("caveat end byte = %d, want %d", got, want)
	}
	block := caveat.Child(3)
	if block == nil || block.Type(lang) != "block_c" {
		t.Fatalf("caveat child[3] type = %v, want block_c", nodeTypeForTest(block, lang))
	}
	if got, want := block.ChildCount(), 5; got != want {
		t.Fatalf("block_c child count = %d, want %d", got, want)
	}
	errNode := block.Child(2)
	if errNode == nil || errNode.Type(lang) != "ERROR" {
		t.Fatalf("block_c child[2] type = %v, want ERROR", nodeTypeForTest(errNode, lang))
	}
	if got, want := errNode.StartByte(), uint32(87); got != want {
		t.Fatalf("ERROR start byte = %d, want %d", got, want)
	}
	if got, want := errNode.EndByte(), uint32(88); got != want {
		t.Fatalf("ERROR end byte = %d, want %d", got, want)
	}
	if got := root.Child(4).Type(lang); got != "definition" {
		t.Fatalf("root child[4] type = %q, want definition", got)
	}
}

func TestAuthzedTokenSourceRecoversUnsupportedUseDirectiveLikeC(t *testing.T) {
	src := []byte("use import\n\n" +
		"import \"subjects.zed\"\n\n" +
		"definition resource {\n" +
		"  relation viewer: user\n" +
		"  permission view = viewer\n" +
		"}\n")

	lang := AuthzedLanguage()
	parser := gotreesitter.NewParser(lang)
	tree, err := parser.ParseWithTokenSource(src, NewAuthzedTokenSourceOrEOF(src, lang))
	if err != nil {
		t.Fatalf("ParseWithTokenSource failed: %v", err)
	}
	defer tree.Release()

	root := tree.RootNode()
	if got := root.Type(lang); got != "source_file" {
		t.Fatalf("root type = %q, want source_file", got)
	}
	if !root.HasError() {
		t.Fatal("root HasError = false, want true")
	}
	if got, want := root.ChildCount(), 1; got != want {
		t.Fatalf("root child count = %d, want %d", got, want)
	}
	errNode := root.Child(0)
	if errNode == nil {
		t.Fatal("root child is nil")
	}
	if got := errNode.Type(lang); got != "ERROR" {
		t.Fatalf("root child type = %q, want ERROR", got)
	}
	if !errNode.IsExtra() {
		t.Fatal("root child IsExtra = false, want true")
	}
	if got, want := errNode.ChildCount(), 10; got != want {
		t.Fatalf("error child count = %d, want %d", got, want)
	}
	if got, want := root.EndByte(), uint32(len(src)); got != want {
		t.Fatalf("root end byte = %d, want %d", got, want)
	}
}

func TestAuthzedTokenSourceRecoversUnsupportedUseAfterDefinitionLikeC(t *testing.T) {
	src := []byte("definition resource {}\n\nuse expiration")

	lang := AuthzedLanguage()
	parser := gotreesitter.NewParser(lang)
	tree, err := parser.ParseWithTokenSource(src, NewAuthzedTokenSourceOrEOF(src, lang))
	if err != nil {
		t.Fatalf("ParseWithTokenSource failed: %v", err)
	}
	defer tree.Release()

	root := tree.RootNode()
	if got := root.Type(lang); got != "source_file" {
		t.Fatalf("root type = %q, want source_file", got)
	}
	if !root.HasError() {
		t.Fatal("root HasError = false, want true")
	}
	if got, want := root.ChildCount(), 3; got != want {
		t.Fatalf("root child count = %d, want %d", got, want)
	}
	if got := root.Child(0).Type(lang); got != "definition" {
		t.Fatalf("root child[0] type = %q, want definition", got)
	}
	if got := root.Child(1).Type(lang); got != "\n" {
		t.Fatalf("root child[1] type = %q, want newline", got)
	}
	errNode := root.Child(2)
	if errNode == nil || errNode.Type(lang) != "ERROR" {
		t.Fatalf("root child[2] type = %v, want ERROR", nodeTypeForTest(errNode, lang))
	}
	if !errNode.IsExtra() {
		t.Fatal("root child[2] IsExtra = false, want true")
	}
	if got, want := errNode.StartByte(), uint32(24); got != want {
		t.Fatalf("ERROR start byte = %d, want %d", got, want)
	}
	if got, want := errNode.EndByte(), uint32(len(src)); got != want {
		t.Fatalf("ERROR end byte = %d, want %d", got, want)
	}
	if got, want := errNode.ChildCount(), 0; got != want {
		t.Fatalf("ERROR child count = %d, want %d", got, want)
	}
}

func TestAuthzedTokenSourceIncrementalRetryHandlesAssociativityFirstByteToggle(t *testing.T) {
	src := []byte("definition resource {\n" +
		"    permission union = a + b + c\n" +
		"    permission exclusion = a - b - c\n" +
		"    permission intersection = a & b & c\n\n" +
		"    permission combined1 = a + b - c\n" +
		"    permission combined2 = a - b + c\n" +
		"}")
	edited := append([]byte(nil), src...)
	edited[0] = 'e'
	edit := gotreesitter.InputEdit{
		StartByte:   0,
		OldEndByte:  1,
		NewEndByte:  1,
		StartPoint:  gotreesitter.Point{},
		OldEndPoint: gotreesitter.Point{Column: 1},
		NewEndPoint: gotreesitter.Point{Column: 1},
	}

	lang := AuthzedLanguage()
	parser := gotreesitter.NewParser(lang)
	freshOriginal := parseAuthzedForTest(t, parser, lang, src)
	defer freshOriginal.Release()
	freshEdited := parseAuthzedForTest(t, parser, lang, edited)
	defer freshEdited.Release()

	oldTree := parseAuthzedForTest(t, parser, lang, src)
	defer oldTree.Release()
	oldTree.Edit(edit)
	incrEdited, _, err := parser.ParseIncrementalWithTokenSourceProfiled(edited, oldTree, NewAuthzedTokenSourceOrEOF(edited, lang))
	if err != nil {
		t.Fatalf("ParseIncrementalWithTokenSourceProfiled edited failed: %v", err)
	}
	defer incrEdited.Release()
	requireCompleteAuthzedTreeForTest(t, incrEdited, edited)
	assertAuthzedNodeShapeEqual(t, lang, incrEdited.RootNode(), freshEdited.RootNode(), "root")

	incrEdited.Edit(edit)
	incrOriginal, profile, err := parser.ParseIncrementalWithTokenSourceProfiled(src, incrEdited, NewAuthzedTokenSourceOrEOF(src, lang))
	if err != nil {
		t.Fatalf("ParseIncrementalWithTokenSourceProfiled original failed: %v", err)
	}
	defer incrOriginal.Release()
	requireCompleteAuthzedTreeForTest(t, incrOriginal, src)
	assertAuthzedNodeShapeEqual(t, lang, incrOriginal.RootNode(), freshOriginal.RootNode(), "root")
	// Independently corroborate the recursive shape check with an
	// S-expression text comparison (named-node type/nesting) against a
	// genuine from-scratch parse of src. Both must agree that the
	// associativity-sensitive recovery (the mixed +/- "combined1"/
	// "combined2" permissions require precedence disambiguation the grammar
	// resolves via an ERROR production) round-trips identically after the
	// detour through the toggled-first-byte "edited" parse.
	if got, want := incrOriginal.RootNode().SExpr(lang), freshOriginal.RootNode().SExpr(lang); got != want {
		t.Fatalf("incremental reparse SExpr diverges from fresh parse:\n incr:  %s\n fresh: %s", got, want)
	}
	// Prior to the C-faithful error-recovery fix, the incremental dispatch
	// could not resolve this associativity-sensitive recovery on its own
	// (toggling the first byte away from and back to the "definition"
	// keyword invalidates essentially the whole tree) and fell back to a
	// full re-parse, which set ReuseUnsupportedReason to
	// "incremental_parse_full_retry". Recovery now succeeds directly inside
	// the incremental dispatch, so no full retry is triggered:
	// ReuseUnsupported stays false and the reason is empty. The shape/SExpr
	// checks above (plus ProfileReuseRejectHasError/ReuseRejectAncestorDirty
	// accounting showing the dirtied span was correctly NOT reused) are what
	// actually guard against a stale/incorrect-reuse regression here; this
	// just pins the (now empty) retry bookkeeping so a regression back to
	// the old fallback path would be caught too.
	if profile.ReuseUnsupported {
		t.Fatalf("incremental parse unexpectedly fell back to full retry (reason=%q); C-faithful recovery should resolve this directly", profile.ReuseUnsupportedReason)
	}
	if got, want := profile.ReuseUnsupportedReason, ""; got != want {
		t.Fatalf("incremental fallback reason = %q, want %q (no full retry expected)", got, want)
	}
}

func TestAuthzedTokenSourceRecoversMalformedPermissionMethodLikeC(t *testing.T) {
	src := []byte("definition resource {\n" +
		"    permission view = a.foo(bar)\n" +
		"}")

	lang := AuthzedLanguage()
	parser := gotreesitter.NewParser(lang)
	tree, err := parser.ParseWithTokenSource(src, NewAuthzedTokenSourceOrEOF(src, lang))
	if err != nil {
		t.Fatalf("ParseWithTokenSource failed: %v", err)
	}
	defer tree.Release()

	root := tree.RootNode()
	if got := root.Type(lang); got != "source_file" {
		t.Fatalf("root type = %q, want source_file", got)
	}
	if !root.HasError() {
		t.Fatal("root HasError = false, want true")
	}
	if got, want := root.ChildCount(), 1; got != want {
		t.Fatalf("root child count = %d, want %d", got, want)
	}
	errNode := root.Child(0)
	if errNode == nil {
		t.Fatal("root child is nil")
	}
	if got := errNode.Type(lang); got != "ERROR" {
		t.Fatalf("root child type = %q, want ERROR", got)
	}
	if !errNode.IsExtra() {
		t.Fatal("root child IsExtra = false, want true")
	}
	if got, want := errNode.ChildCount(), 9; got != want {
		t.Fatalf("error child count = %d, want %d", got, want)
	}
	last := errNode.Child(errNode.ChildCount() - 1)
	if last == nil {
		t.Fatal("last error child is nil")
	}
	if got := last.Type(lang); got != "\n" {
		t.Fatalf("last error child type = %q, want newline", got)
	}
}

func TestAuthzedTokenSourceRecoversMalformedChainedPermissionMethodLikeC(t *testing.T) {
	src := []byte("definition resource {\n" +
		"    permission view = parent1.any(member1) + parent2.all(member2)\n" +
		"    permission chained = parent1.any(member1).all(member2)->member3\n" +
		"}")

	lang := AuthzedLanguage()
	parser := gotreesitter.NewParser(lang)
	tree, err := parser.ParseWithTokenSource(src, NewAuthzedTokenSourceOrEOF(src, lang))
	if err != nil {
		t.Fatalf("ParseWithTokenSource failed: %v", err)
	}
	defer tree.Release()

	root := tree.RootNode()
	if got := root.Type(lang); got != "source_file" {
		t.Fatalf("root type = %q, want source_file", got)
	}
	if got, want := root.ChildCount(), 1; got != want {
		t.Fatalf("root child count = %d, want %d", got, want)
	}
	errNode := root.Child(0)
	if errNode == nil {
		t.Fatal("root child is nil")
	}
	if got := errNode.Type(lang); got != "ERROR" {
		t.Fatalf("root child type = %q, want ERROR", got)
	}
	if got, want := errNode.ChildCount(), 6; got != want {
		t.Fatalf("error child count = %d, want %d", got, want)
	}
	if got := errNode.Child(3).Type(lang); got != "permission" {
		t.Fatalf("error child[3] type = %q, want permission", got)
	}
	if got := errNode.Child(4).Type(lang); got != "\n" {
		t.Fatalf("error child[4] type = %q, want newline", got)
	}
	if got := errNode.Child(5).Type(lang); got != "\n" {
		t.Fatalf("error child[5] type = %q, want newline", got)
	}
}

func TestAuthzedTokenSourceRecoversDigitPermissionNamesLikeC(t *testing.T) {
	src := []byte("definition resource {\n" +
		"    permission union = a + b + c\n" +
		"    permission exclusion = a - b - c\n" +
		"    permission intersection = a & b & c\n\n" +
		"    permission combined1 = a + b - c\n" +
		"    permission combined2 = a - b + c\n" +
		"}")

	lang := AuthzedLanguage()
	parser := gotreesitter.NewParser(lang)
	tree, err := parser.ParseWithTokenSource(src, NewAuthzedTokenSourceOrEOF(src, lang))
	if err != nil {
		t.Fatalf("ParseWithTokenSource failed: %v", err)
	}
	defer tree.Release()

	root := tree.RootNode()
	if got := root.Type(lang); got != "source_file" {
		t.Fatalf("root type = %q, want source_file", got)
	}
	if got, want := root.ChildCount(), 1; got != want {
		t.Fatalf("root child count = %d, want %d", got, want)
	}
	errNode := root.Child(0)
	if errNode == nil {
		t.Fatal("root child is nil")
	}
	if got, want := errNode.ChildCount(), 10; got != want {
		t.Fatalf("error child count = %d, want %d", got, want)
	}
	if got := errNode.Child(6).Type(lang); got != "permission_literal" {
		t.Fatalf("error child[6] type = %q, want permission_literal", got)
	}
	if got := errNode.Child(7).Type(lang); got != "identifier" {
		t.Fatalf("error child[7] type = %q, want identifier", got)
	}
	if got := errNode.Child(8).Type(lang); got != "\n" {
		t.Fatalf("error child[8] type = %q, want newline", got)
	}
	if got := errNode.Child(9).Type(lang); got != "\n" {
		t.Fatalf("error child[9] type = %q, want newline", got)
	}
}

func TestAuthzedTokenSourceRecoversWildcardPermissionNameLikeC(t *testing.T) {
	src := []byte("definition org {\n" +
		"    relation admin: user\n" +
		"    relation member: user\n\n" +
		"    permission read = admin + member\n" +
		"    permission create = admin\n" +
		"    permission update = admin\n" +
		"    permission delete = admin\n" +
		"    permission * = read + create + update + delete\n" +
		"}")

	lang := AuthzedLanguage()
	parser := gotreesitter.NewParser(lang)
	tree, err := parser.ParseWithTokenSource(src, NewAuthzedTokenSourceOrEOF(src, lang))
	if err != nil {
		t.Fatalf("ParseWithTokenSource failed: %v", err)
	}
	defer tree.Release()

	root := tree.RootNode()
	if got := root.Type(lang); got != "source_file" {
		t.Fatalf("root type = %q, want source_file", got)
	}
	if got, want := root.ChildCount(), 1; got != want {
		t.Fatalf("root child count = %d, want %d", got, want)
	}
	errNode := root.Child(0)
	if errNode == nil || errNode.Type(lang) != "ERROR" {
		t.Fatalf("root child[0] type = %v, want ERROR", nodeTypeForTest(errNode, lang))
	}
	if got, want := errNode.ChildCount(), 11; got != want {
		t.Fatalf("error child count = %d, want %d", got, want)
	}
	if got := errNode.Child(9).Type(lang); got != "permission_literal" {
		t.Fatalf("error child[9] type = %q, want permission_literal", got)
	}
	if got := errNode.Child(10).Type(lang); got != "\n" {
		t.Fatalf("error child[10] type = %q, want newline", got)
	}
}

func TestAuthzedTokenSourceRecoversInvalidRelationWildcardLikeC(t *testing.T) {
	src := []byte("definition mydefinition {\n" +
		"    /**\n" +
		"     * some doc comment\n" +
		"     */\n" +
		"    relation foo: sometype#... | anothertype#somerel\n\n" +
		"    // My cool permission\n" +
		"    permission bar = foo + baz - meh\n" +
		"    permission another = (foo - meh) + bar\n" +
		"}")

	lang := AuthzedLanguage()
	parser := gotreesitter.NewParser(lang)
	tree, err := parser.ParseWithTokenSource(src, NewAuthzedTokenSourceOrEOF(src, lang))
	if err != nil {
		t.Fatalf("ParseWithTokenSource failed: %v", err)
	}
	defer tree.Release()

	root := tree.RootNode()
	if got := root.Type(lang); got != "source_file" {
		t.Fatalf("root type = %q, want source_file", got)
	}
	if got, want := root.ChildCount(), 1; got != want {
		t.Fatalf("root child count = %d, want %d", got, want)
	}
	errNode := root.Child(0)
	if errNode == nil {
		t.Fatal("root child is nil")
	}
	if got, want := errNode.ChildCount(), 10; got != want {
		t.Fatalf("error child count = %d, want %d", got, want)
	}
	if got := errNode.Child(3).Type(lang); got != "comment" {
		t.Fatalf("error child[3] type = %q, want comment", got)
	}
	relation := errNode.Child(4)
	if relation == nil {
		t.Fatal("error child[4] is nil")
	}
	if got := relation.Type(lang); got != "relation" {
		t.Fatalf("error child[4] type = %q, want relation", got)
	}
	if relation.HasError() {
		t.Fatal("partial relation HasError = true, want false")
	}
	if got, want := relation.EndByte(), uint32(93); got != want {
		t.Fatalf("partial relation end byte = %d, want %d", got, want)
	}
	if got := errNode.Child(5).Type(lang); got != "\n" {
		t.Fatalf("error child[5] type = %q, want newline", got)
	}
}

type recordingAuthzedTokenSource struct {
	base   gotreesitter.TokenSource
	lang   *gotreesitter.Language
	tokens []string
}

func (ts *recordingAuthzedTokenSource) Next() gotreesitter.Token {
	tok := ts.base.Next()
	name := "<invalid>"
	if int(tok.Symbol) >= 0 && int(tok.Symbol) < len(ts.lang.SymbolNames) {
		name = ts.lang.SymbolNames[tok.Symbol]
	}
	ts.tokens = append(ts.tokens, fmt.Sprintf("%s@%d:%d", name, tok.StartByte, tok.EndByte))
	return tok
}

func (ts *recordingAuthzedTokenSource) SetParserState(state gotreesitter.StateID) {
	if stateful, ok := ts.base.(interface{ SetParserState(gotreesitter.StateID) }); ok {
		stateful.SetParserState(state)
	}
}

func (ts *recordingAuthzedTokenSource) SetGLRStates(states []gotreesitter.StateID) {
	if stateful, ok := ts.base.(interface{ SetGLRStates([]gotreesitter.StateID) }); ok {
		stateful.SetGLRStates(states)
	}
}

func TestAuthzedTokenSourceEmitsWildcardType(t *testing.T) {
	src := []byte("user:*")
	lang := AuthzedLanguage()
	ts := NewAuthzedTokenSourceOrEOF(src, lang)
	tok := ts.Next()

	if got := lang.SymbolNames[tok.Symbol]; got != "wildcard_type" {
		t.Fatalf("first token symbol = %q, want wildcard_type", got)
	}
	if got := tok.Text; got != "user:*" {
		t.Fatalf("first token text = %q, want user:*", got)
	}
	if got, want := tok.EndByte, uint32(len(src)); got != want {
		t.Fatalf("first token end byte = %d, want %d", got, want)
	}
}

func TestAuthzedTokenSourceEmitsBlockComment(t *testing.T) {
	src := []byte("/**\n * doc\n */")
	lang := AuthzedLanguage()
	ts := NewAuthzedTokenSourceOrEOF(src, lang)
	tok := ts.Next()

	if got := lang.SymbolNames[tok.Symbol]; got != "comment" {
		t.Fatalf("first token symbol = %q, want comment", got)
	}
	if got := tok.Text; got != string(src) {
		t.Fatalf("first token text = %q, want %q", got, string(src))
	}
	if got, want := tok.EndByte, uint32(len(src)); got != want {
		t.Fatalf("first token end byte = %d, want %d", got, want)
	}
	if got, want := tok.EndPoint.Row, uint32(2); got != want {
		t.Fatalf("first token end row = %d, want %d", got, want)
	}
}

func nodeTypeForTest(node *gotreesitter.Node, lang *gotreesitter.Language) string {
	if node == nil {
		return "<nil>"
	}
	return node.Type(lang)
}

func parseAuthzedForTest(t *testing.T, parser *gotreesitter.Parser, lang *gotreesitter.Language, src []byte) *gotreesitter.Tree {
	t.Helper()
	tree, err := parser.ParseWithTokenSource(src, NewAuthzedTokenSourceOrEOF(src, lang))
	if err != nil {
		t.Fatalf("ParseWithTokenSource failed: %v", err)
	}
	requireCompleteAuthzedTreeForTest(t, tree, src)
	return tree
}

func requireCompleteAuthzedTreeForTest(t *testing.T, tree *gotreesitter.Tree, src []byte) {
	t.Helper()
	if tree == nil || tree.RootNode() == nil {
		t.Fatal("tree root is nil")
	}
	if got, want := tree.RootNode().EndByte(), uint32(len(src)); got != want {
		t.Fatalf("root end byte = %d, want %d (%s)", got, want, tree.ParseRuntime().Summary())
	}
}

func assertAuthzedNodeShapeEqual(t *testing.T, lang *gotreesitter.Language, left, right *gotreesitter.Node, path string) {
	t.Helper()
	if left == nil || right == nil {
		if left != right {
			t.Fatalf("%s nil mismatch left=%v right=%v", path, left == nil, right == nil)
		}
		return
	}
	if got, want := left.Type(lang), right.Type(lang); got != want {
		t.Fatalf("%s type = %q, want %q", path, got, want)
	}
	if got, want := left.StartByte(), right.StartByte(); got != want {
		t.Fatalf("%s start byte = %d, want %d", path, got, want)
	}
	if got, want := left.EndByte(), right.EndByte(); got != want {
		t.Fatalf("%s end byte = %d, want %d", path, got, want)
	}
	if got, want := left.IsNamed(), right.IsNamed(); got != want {
		t.Fatalf("%s IsNamed = %v, want %v", path, got, want)
	}
	if got, want := left.IsMissing(), right.IsMissing(); got != want {
		t.Fatalf("%s IsMissing = %v, want %v", path, got, want)
	}
	if got, want := left.IsExtra(), right.IsExtra(); got != want {
		t.Fatalf("%s IsExtra = %v, want %v", path, got, want)
	}
	if got, want := left.HasError(), right.HasError(); got != want {
		t.Fatalf("%s HasError = %v, want %v", path, got, want)
	}
	if got, want := left.ChildCount(), right.ChildCount(); got != want {
		t.Fatalf("%s child count = %d, want %d", path, got, want)
	}
	for i := 0; i < int(left.ChildCount()); i++ {
		if got, want := left.FieldNameForChild(i, lang), right.FieldNameForChild(i, lang); got != want {
			t.Fatalf("%s[%d] field = %q, want %q", path, i, got, want)
		}
		assertAuthzedNodeShapeEqual(t, lang, left.Child(i), right.Child(i), fmt.Sprintf("%s[%d]", path, i))
	}
}
