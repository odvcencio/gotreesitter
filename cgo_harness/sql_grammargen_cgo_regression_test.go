//go:build cgo && treesitter_c_parity

package cgoharness

import (
	"bytes"
	"encoding/hex"
	"testing"
	"time"

	sitter "github.com/tree-sitter/go-tree-sitter"

	gotreesitter "github.com/odvcencio/gotreesitter"
)

func TestSQLGrammargenCGORegressionCases(t *testing.T) {
	var grammar grammargenCGOGrammar
	found := false
	for _, g := range grammargenCGOGrammars {
		if g.name == "sql" {
			grammar = g
			found = true
			break
		}
	}
	if !found {
		t.Fatal("missing sql grammargen CGO config")
	}

	gram, err := importGrammargenSource(grammar)
	if err != nil {
		t.Skipf("import unavailable: %v", err)
	}
	timeout := grammar.genTimeout
	if timeout == 0 {
		timeout = 30 * time.Second
	}
	genLang, err := grammargenGenerate(gram, timeout)
	if err != nil {
		t.Fatalf("generate grammar: %v", err)
	}
	refLang := grammar.blobFunc()
	adaptGrammargenCGOExternalScanner("sql", refLang, genLang)
	grammarHash, ok := genLang.GrammarBlobSHA256()
	if !ok {
		t.Fatal("generated SQL language has no authenticated grammar identity")
	}
	provider, ok := genLang.ExternalScanner.(gotreesitter.ExternalScannerCheckpointIdentityProvider)
	if !ok {
		t.Fatal("generated SQL scanner has no checkpoint identity provider")
	}
	expectedScanner := decodeSQLIdentity(t, "6be5f16fe226845e0bf490a42c33bcd00294b029dcc1160e676bb9fa7ae7901b")
	expectedGrammar := decodeSQLIdentity(t, "4ffb2a6d09e2000126f10101db9028d28e0752ac3e4f83e401f045c3b028ca7c")
	identity, ok := provider.CheckpointIdentity()
	if !ok || !bytes.Equal(identity.Scanner, expectedScanner) || !bytes.Equal(identity.Grammar, expectedGrammar) || !bytes.Equal(identity.Grammar, grammarHash[:]) {
		t.Fatalf("generated SQL scanner grammar identity does not match generated blob: ok=%t identity=%x blob=%x", ok, identity.Grammar, grammarHash)
	}
	t.Logf("generated SQL checkpoint identity: scanner=%x grammar=%x blob=%x", identity.Scanner, identity.Grammar, grammarHash)
	assertGeneratedSQLIncrementalCheckpointReuse(t, genLang, expectedScanner, expectedGrammar)
	assertGeneratedSQLStaleCheckpointFailsClosed(t, genLang, refLang, expectedGrammar)

	cLang, err := ParityCLanguage("sql")
	if err != nil {
		t.Skipf("C parser unavailable: %v", err)
	}
	cParser := sitter.NewParser()
	defer cParser.Close()
	if err := cParser.SetLanguage(cLang); err != nil {
		t.Fatalf("C SetLanguage: %v", err)
	}

	genParser := gotreesitter.NewParser(genLang)
	blobParser := gotreesitter.NewParser(refLang)

	cases := []struct {
		name string
		src  string
	}{
		{name: "select_identifier_list", src: "SELECT a, b;\n"},
		{name: "select_parenthesized_boolean", src: "SELECT (TRUE);\n"},
		{name: "select_dollar_quoted_string", src: "SELECT $$hey$$;\n"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			src := []byte(tc.src)

			cTree := cParser.Parse(src, nil)
			if cTree == nil || cTree.RootNode() == nil {
				t.Fatal("C nil tree")
			}
			defer cTree.Close()
			cRoot := cTree.RootNode()
			if cRoot.HasError() {
				t.Fatalf("C root has error:\n%s", dumpCTree(cRoot, 0))
			}

			genTree, err := genParser.Parse(src)
			if err != nil {
				t.Fatalf("generated parse error: %v", err)
			}
			genRoot := genTree.RootNode()

			blobTree, err := blobParser.Parse(src)
			if err != nil {
				t.Fatalf("blob parse error: %v", err)
			}
			blobRoot := blobTree.RootNode()

			var genVsCErrs []string
			compareNodes(genRoot, genLang, cRoot, "root", &genVsCErrs)
			if len(genVsCErrs) == 0 {
				return
			}

			var blobVsCErrs []string
			compareNodes(blobRoot, refLang, cRoot, "root", &blobVsCErrs)
			if len(blobVsCErrs) > 0 {
				t.Logf(
					"generated and blob both diverge from C; not treating as grammargen-specific regression\nGEN-vs-C:\n%s\n\nblob-vs-C:\n%s",
					joinTopErrors(genVsCErrs),
					joinTopErrors(blobVsCErrs),
				)
				return
			}

			var genVsBlobErrs []string
			compareGoTreesForLangs(genRoot, genLang, blobRoot, refLang, "root", &genVsBlobErrs)

			t.Fatalf(
				"generated-vs-C divergences:\n%s\n\ngenerated-vs-blob:\n%s\n\nblob-vs-C:\n%s\n\ngenerated:\n%s\n\nblob:\n%s\n\nc:\n%s",
				joinTopErrors(genVsCErrs),
				joinTopErrors(genVsBlobErrs),
				joinTopErrors(blobVsCErrs),
				genRoot.SExpr(genLang),
				blobRoot.SExpr(refLang),
				dumpCTree(cRoot, 0),
			)
		})
	}
}

func decodeSQLIdentity(t *testing.T, encoded string) []byte {
	t.Helper()
	identity, err := hex.DecodeString(encoded)
	if err != nil {
		t.Fatalf("decode pinned SQL identity: %v", err)
	}
	return identity
}

func generatedSQLIncrementalWitness(t *testing.T) ([]byte, []byte, gotreesitter.InputEdit) {
	t.Helper()
	const sourceText = "SELECT $$hey$$;\n"
	source := []byte(sourceText)
	edited := append([]byte(nil), source...)
	editOffset := bytes.IndexByte(source, 'h')
	if editOffset < 0 {
		t.Fatal("generated SQL incremental witness has no editable scanner content")
	}
	edited[editOffset] = 'H'
	return source, edited, gotreesitter.InputEdit{
		StartByte:   uint32(editOffset),
		OldEndByte:  uint32(editOffset + 1),
		NewEndByte:  uint32(editOffset + 1),
		StartPoint:  gotreesitter.Point{Column: uint32(editOffset)},
		OldEndPoint: gotreesitter.Point{Column: uint32(editOffset + 1)},
		NewEndPoint: gotreesitter.Point{Column: uint32(editOffset + 1)},
	}
}

func assertGeneratedSQLIncrementalCheckpointReuse(t *testing.T, lang *gotreesitter.Language, wantScanner, wantGrammar []byte) {
	t.Helper()
	source, edited, edit := generatedSQLIncrementalWitness(t)
	provider, ok := lang.ExternalScanner.(gotreesitter.ExternalScannerCheckpointIdentityProvider)
	if !ok {
		t.Fatal("generated SQL incremental witness lost checkpoint identity provider")
	}
	identity, ok := provider.CheckpointIdentity()
	if !ok || !bytes.Equal(identity.Scanner, wantScanner) || !bytes.Equal(identity.Grammar, wantGrammar) {
		t.Fatalf("generated SQL incremental witness identity changed: ok=%t identity=%x want_scanner=%x want_grammar=%x", ok, identity, wantScanner, wantGrammar)
	}

	parser := gotreesitter.NewParser(lang)
	oldTree, err := parser.Parse(source)
	if err != nil {
		t.Fatalf("generated SQL incremental old parse: %v", err)
	}
	defer oldTree.Release()
	oldRuntime := oldTree.ParseRuntime()
	if oldRuntime.ExternalScannerCheckpointRecords != 13 || oldRuntime.ExternalScannerCheckpointLeafNodes != 4 || oldRuntime.ExternalScannerSnapshotBytesAllocated != 4 {
		t.Fatalf("generated SQL old parse checkpoint counts changed: records=%d leaves=%d snapshots=%d", oldRuntime.ExternalScannerCheckpointRecords, oldRuntime.ExternalScannerCheckpointLeafNodes, oldRuntime.ExternalScannerSnapshotBytesAllocated)
	}
	oldTree.Edit(edit)
	incremental, profile, err := parser.ParseIncrementalProfiled(edited, oldTree)
	if err != nil {
		t.Fatalf("generated SQL incremental parse: %v", err)
	}
	if incremental == oldTree {
		t.Fatal("generated SQL incremental parse returned the edited old tree")
	}
	defer incremental.Release()
	fresh, err := gotreesitter.NewParser(lang).Parse(edited)
	if err != nil {
		t.Fatalf("generated SQL fresh edited parse: %v", err)
	}
	defer fresh.Release()
	if got, want := incremental.RootNode().SExpr(lang), fresh.RootNode().SExpr(lang); got != want {
		t.Fatalf("generated SQL incremental tree differs from fresh tree:\n got %s\nwant %s", got, want)
	}
	runtime := incremental.ParseRuntime()
	if profile.ReuseUnsupported || profile.ReusedSubtrees != 1 || profile.ReusedBytes != 16 {
		t.Fatalf("generated SQL incremental checkpoint reuse counts changed: profile=%+v runtime=%s", profile, runtime.Summary())
	}
	t.Logf("generated SQL incremental checkpoint reuse: old_checkpoints=%d old_leaves=%d old_snapshots=%d reused_subtrees=%d reused_bytes=%d old_tree_route=%t", oldRuntime.ExternalScannerCheckpointRecords, oldRuntime.ExternalScannerCheckpointLeafNodes, oldRuntime.ExternalScannerSnapshotBytesAllocated, profile.ReusedSubtrees, profile.ReusedBytes, profile.OldTreeReuseRoute)
}

func assertGeneratedSQLStaleCheckpointFailsClosed(t *testing.T, generated, stale *gotreesitter.Language, wantGrammar []byte) {
	t.Helper()
	staleProvider, ok := stale.ExternalScanner.(gotreesitter.ExternalScannerCheckpointIdentityProvider)
	if !ok {
		t.Fatal("reference SQL scanner has no checkpoint identity provider")
	}
	staleIdentity, ok := staleProvider.CheckpointIdentity()
	if !ok || len(staleIdentity.Grammar) == 0 || bytes.Equal(staleIdentity.Grammar, wantGrammar) {
		t.Fatalf("reference SQL scanner did not provide a distinct stale identity: ok=%t grammar=%x", ok, staleIdentity.Grammar)
	}
	source := []byte("SELECT $$hey$$;\n")
	edited := []byte("SELECT $$heyX$$;\n")
	edit := gotreesitter.InputEdit{
		StartByte:   12,
		OldEndByte:  12,
		NewEndByte:  13,
		StartPoint:  gotreesitter.Point{Column: 12},
		OldEndPoint: gotreesitter.Point{Column: 12},
		NewEndPoint: gotreesitter.Point{Column: 13},
	}
	parser := gotreesitter.NewParser(generated)
	oldTree, err := parser.Parse(source)
	if err != nil {
		t.Fatalf("generated SQL stale-checkpoint old parse: %v", err)
	}
	defer oldTree.Release()
	oldTree.Edit(edit)
	originalScanner := generated.ExternalScanner
	generated.ExternalScanner = stale.ExternalScanner
	defer func() { generated.ExternalScanner = originalScanner }()
	incremental, profile, err := parser.ParseIncrementalProfiled(edited, oldTree)
	generated.ExternalScanner = originalScanner
	if err != nil {
		t.Fatalf("generated SQL stale-checkpoint incremental parse: %v", err)
	}
	defer incremental.Release()
	if profile.ReusedSubtrees != 0 || profile.ReusedBytes != 0 {
		t.Fatalf("stale generated SQL checkpoint was reused: profile=%+v", profile)
	}
	fresh, err := gotreesitter.NewParser(generated).Parse(edited)
	if err != nil {
		t.Fatalf("generated SQL stale-checkpoint fresh parse: %v", err)
	}
	defer fresh.Release()
	if got, want := incremental.RootNode().SExpr(generated), fresh.RootNode().SExpr(generated); got != want {
		t.Fatalf("stale generated SQL incremental tree differs from fresh tree:\n got %s\nwant %s", got, want)
	}
	t.Logf("generated SQL stale checkpoint rejected: stale_grammar=%x reused_subtrees=%d reused_bytes=%d", staleIdentity.Grammar, profile.ReusedSubtrees, profile.ReusedBytes)
}
