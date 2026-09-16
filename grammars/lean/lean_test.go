package lean

import (
	"bytes"
	"crypto/sha256"
	"os/exec"
	"reflect"
	"strings"
	"testing"

	"github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammargen"
	"github.com/odvcencio/gotreesitter/grammars"
)

func TestPackageDoesNotDependOnAggregateGrammars(t *testing.T) {
	out, err := exec.Command("go", "list", "-deps", ".").Output()
	if err != nil {
		t.Fatal(err)
	}
	for _, dependency := range strings.Fields(string(out)) {
		if dependency == "github.com/odvcencio/gotreesitter/grammars" {
			t.Fatal("standalone Lean package imports the aggregate grammar registry")
		}
	}
}

func TestRegistration(t *testing.T) {
	entry := grammars.DetectLanguage("Main.lean")
	if entry == nil || entry.Name != "lean" {
		t.Fatalf("DetectLanguage(Main.lean) = %#v, want Lean", entry)
	}
	if entry.GrammarSource != grammars.GrammarSourceGrammargenBlob {
		t.Fatalf("GrammarSource = %q, want %q", entry.GrammarSource, grammars.GrammarSourceGrammargenBlob)
	}
	alias := grammars.DetectLanguageByName("lean4")
	if alias == nil || alias.Name != "lean" {
		t.Fatalf("DetectLanguageByName(lean4) = %#v, want Lean", alias)
	}
	if entry.Language() != Language() {
		t.Fatal("registry and package returned different language pointers")
	}
}

func TestAggregateBlobLoaderPreservesScanner(t *testing.T) {
	if grammars.LookupExternalScanner("lean") == nil {
		t.Fatal("aggregate scanner lookup lost the Lean registration")
	}
	lang, err := grammars.LoadLanguage("lean", languageBlob)
	if err != nil {
		t.Fatal(err)
	}
	if lang.ExternalScanner == nil {
		t.Fatal("aggregate blob loader omitted the Lean scanner")
	}
	source := []byte("/- outer /- inner -/ comment -/\ndef answer := 42\n")
	tree, err := gotreesitter.NewParser(lang).Parse(source)
	if err != nil {
		t.Fatal(err)
	}
	defer tree.Release()
	if tree.RootNode() == nil || tree.RootNode().HasError() {
		t.Fatal("aggregate Lean parser rejected nested comments")
	}
}

func TestRegistrationLookupDoesNotAllocate(t *testing.T) {
	grammars.DetectLanguage("Main.lean")
	if allocations := testing.AllocsPerRun(100, func() {
		grammars.DetectLanguage("Main.lean")
	}); allocations != 0 {
		t.Fatalf("registered Lean lookup allocated %g times", allocations)
	}
}

func TestPackagedBlobMatchesNativeGrammar(t *testing.T) {
	if ReferenceVersion != grammargen.LeanGrammarReferenceVersion {
		t.Fatalf("package reference version = %q, grammar reference version = %q",
			ReferenceVersion, grammargen.LeanGrammarReferenceVersion)
	}
	_, generated, err := grammargen.GenerateLanguageAndBlob(grammargen.LeanGrammar())
	if err != nil {
		t.Fatalf("generate native Lean blob: %v", err)
	}
	packagedLanguage, err := gotreesitter.LoadLanguage(languageBlob)
	if err != nil {
		t.Fatalf("load packaged Lean blob: %v", err)
	}
	// Re-encode the packaged payload with the current Language schema. Gob
	// includes exported struct field descriptors. A blob that predates a new
	// zero-value field can use different bytes but still describe the same
	// language.
	canonicalPackaged, err := gotreesitter.EncodeLanguageBlob(packagedLanguage)
	if err != nil {
		t.Fatalf("canonicalize packaged Lean blob: %v", err)
	}
	if !bytes.Equal(canonicalPackaged, generated) {
		generatedLanguage, err := gotreesitter.LoadLanguage(generated)
		if err != nil {
			t.Fatalf("load generated Lean blob: %v", err)
		}
		packagedValue := reflect.ValueOf(packagedLanguage).Elem()
		generatedValue := reflect.ValueOf(generatedLanguage).Elem()
		languageType := packagedValue.Type()
		var changed []string
		for i := 0; i < languageType.NumField(); i++ {
			field := languageType.Field(i)
			if !field.IsExported() || reflect.DeepEqual(packagedValue.Field(i).Interface(), generatedValue.Field(i).Interface()) {
				continue
			}
			changed = append(changed, field.Name)
		}
		t.Fatalf("packaged Lean blob SHA-256 = %x, canonical SHA-256 = %x, generated SHA-256 = %x; changed language fields: %s",
			sha256.Sum256(languageBlob), sha256.Sum256(canonicalPackaged), sha256.Sum256(generated), strings.Join(changed, ", "))
	}
}

func TestParseCoreAndNestedComments(t *testing.T) {
	source := []byte(`/- outer /- nested -/ comment -/
/-! Module documentation. -/

module

prelude
public import Init.Prelude
public meta import Init.Grind.Tactics
public section

namespace Demo
/-- Double a natural number. -/
def double (n : Nat) : Nat := n + n
theorem double_zero : double 0 = 0 := by
  rfl
end Demo
`)
	tree, err := gotreesitter.NewParser(Language()).Parse(source)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	defer tree.Release()
	root := tree.RootNode()
	if root == nil || root.HasError() {
		t.Fatalf("Lean parse has error: %s", root.SExpr(Language()))
	}
	sexpr := root.SExpr(Language())
	for _, nodeType := range []string{"block_comment", "module_doc_comment", "doc_comment", "namespace_declaration", "definition_declaration", "theorem_declaration"} {
		if !strings.Contains(sexpr, "("+nodeType) {
			t.Fatalf("tree lacks %s: %s", nodeType, sexpr)
		}
	}
}

func TestQueriesAndTags(t *testing.T) {
	lang := Language()
	if _, err := gotreesitter.NewQuery(HighlightQuery, lang); err != nil {
		t.Fatalf("compile highlight query: %v", err)
	}
	tagger, err := gotreesitter.NewTagger(lang, TagsQuery)
	if err != nil {
		t.Fatalf("compile tags query: %v", err)
	}
	tags := tagger.Tag([]byte("namespace Demo\n@[inline] def double (n : Nat) := n + n\n@[simp] protected theorem sound : True := by trivial\n"))
	var names []string
	for _, tag := range tags {
		names = append(names, tag.Name)
	}
	if got, want := strings.Join(names, ","), "Demo,double,sound"; got != want {
		t.Fatalf("tag names = %q, want %q; tags=%+v", got, want, tags)
	}
}

func TestIncrementalEditMatchesFresh(t *testing.T) {
	oldSource := []byte(`namespace Demo
def double (n : Nat) : Nat := n + n
theorem double_zero : double 0 = 0 := by
  rfl
end Demo
`)
	newSource := append([]byte(nil), oldSource...)
	site := bytes.Index(newSource, []byte("zero"))
	if site < 0 {
		t.Fatal("edit site not found")
	}
	newSource[site] = 'h'

	lang := Language()
	parser := gotreesitter.NewParser(lang)
	oldTree, err := parser.Parse(oldSource)
	if err != nil {
		t.Fatalf("parse old source: %v", err)
	}
	defer oldTree.Release()
	point := leanPointAt(oldSource, site)
	oldTree.Edit(gotreesitter.InputEdit{
		StartByte:   uint32(site),
		OldEndByte:  uint32(site + 1),
		NewEndByte:  uint32(site + 1),
		StartPoint:  point,
		OldEndPoint: gotreesitter.Point{Row: point.Row, Column: point.Column + 1},
		NewEndPoint: gotreesitter.Point{Row: point.Row, Column: point.Column + 1},
	})

	incremental, profile, err := parser.ParseIncrementalProfiled(newSource, oldTree)
	if err != nil {
		t.Fatalf("parse incremental source: %v", err)
	}
	defer incremental.Release()
	fresh, err := gotreesitter.NewParser(lang).Parse(newSource)
	if err != nil {
		t.Fatalf("parse fresh source: %v", err)
	}
	defer fresh.Release()
	if profile.ReusedSubtrees == 0 {
		t.Fatalf("incremental parse reused no subtrees: %+v", profile)
	}
	if incremental.RootNode().HasError() || incremental.ParseStoppedEarly() {
		t.Fatalf("incremental parse is not clean: %s", incremental.RootNode().SExpr(lang))
	}
	if got, want := incremental.RootNode().SExpr(lang), fresh.RootNode().SExpr(lang); got != want {
		t.Fatalf("incremental tree differs from fresh tree:\nincremental: %s\nfresh: %s", got, want)
	}
	if got, want := incremental.RootNode().EndByte(), uint32(len(newSource)); got != want {
		t.Fatalf("incremental root ends at %d, want %d", got, want)
	}
}

func leanPointAt(source []byte, offset int) gotreesitter.Point {
	prefix := source[:offset]
	row := bytes.Count(prefix, []byte{'\n'})
	lineStart := bytes.LastIndexByte(prefix, '\n') + 1
	return gotreesitter.Point{Row: uint32(row), Column: uint32(offset - lineStart)}
}
