//go:build cgo && treesitter_c_parity

package cgoharness

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	gts "github.com/odvcencio/gotreesitter"
	sitter "github.com/tree-sitter/go-tree-sitter"
)

// TestParityQueryQuantifiedPublicSurfaces checks full ordered results through
// every public execution method against C, including the overseer's witness.
func TestParityQueryQuantifiedPublicSurfaces(t *testing.T) {
	for _, comments := range []int{0, 1, 16, 32, 64} {
		for _, suffix := range []string{"func F() {}\n", "type T int\n"} {
			for _, quantifier := range []string{"*", "+", "?"} {
				name := fmt.Sprintf("comments=%d/suffix=%s/quantifier=%s", comments, strings.Fields(suffix)[0], quantifier)
				t.Run(name, func(t *testing.T) {
					source := []byte("package audit\n" + strings.Repeat("// audit\n", comments) + suffix)
					query := fmt.Sprintf(`(source_file (comment)%s @comment . (type_declaration) @type) @root`, quantifier)
					assertQueryPublicSurfaceParity(t, "go", source, query)
				})
			}
		}
	}
}

func assertQueryPublicSurfaceParity(t *testing.T, language string, source []byte, query string) {
	t.Helper()
	tree, lang, err := parseWithGo(parityCase{name: language, source: string(source)}, source, nil)
	if err != nil || tree == nil {
		t.Fatalf("Go parse: %v", err)
	}
	defer tree.Release()
	cLang, err := ParityCLanguage(language)
	if err != nil {
		t.Fatal(err)
	}
	cp := sitter.NewParser()
	defer cp.Close()
	if err := cp.SetLanguage(cLang); err != nil {
		t.Fatal(err)
	}
	ct := cp.Parse(source, nil)
	if ct == nil || ct.RootNode() == nil {
		t.Fatal("C parse returned no tree")
	}
	defer ct.Close()
	var differences []string
	compareNodes(tree.RootNode(), lang, ct.RootNode(), "root", &differences)
	if len(differences) > 0 {
		t.Fatalf("trees differ before query execution: %s", strings.Join(differences, "\n"))
	}
	want := collectCExactQueryMatches(t, cLang, ct, query, source)
	q, err := gts.NewQuery(query, lang)
	if err != nil {
		t.Fatal(err)
	}
	cursor := q.Exec(tree.RootNode(), lang, source)
	var streamed []gts.QueryMatch
	for {
		match, ok := cursor.NextMatch()
		if !ok {
			break
		}
		streamed = append(streamed, match)
	}
	if cursor.DidExceedMatchLimit() {
		t.Fatal("exact fixture exceeded the matcher budget")
	}
	prefix := gts.QueryMatch{PatternIndex: -1}
	appended := q.ExecuteInto(tree, []gts.QueryMatch{prefix})
	if len(appended) == 0 || appended[0].PatternIndex != -1 {
		t.Fatal("ExecuteInto changed the destination prefix")
	}
	for _, operation := range []struct {
		name    string
		matches []gts.QueryMatch
	}{
		{"Execute", q.Execute(tree)},
		{"ExecuteInto", appended[1:]},
		{"ExecuteNode", q.ExecuteNode(tree.RootNode(), lang, source)},
		{"Cursor", streamed},
	} {
		var got []exactQueryMatch
		for _, match := range operation.matches {
			snapshot := exactQueryMatch{PatternIndex: match.PatternIndex}
			for _, capture := range match.Captures {
				node := capture.Node
				snapshot.Captures = append(snapshot.Captures, exactQueryCapture{
					Name: capture.Name, Type: node.Type(lang), Named: node.IsNamed(),
					StartByte: node.StartByte(), EndByte: node.EndByte(), Text: capture.Text(source),
				})
			}
			got = append(got, snapshot)
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("%s differs from C for %s\nGo:\n%s\nC:\n%s", operation.name, query, formatExactQueryMatches(got), formatExactQueryMatches(want))
		}
	}
}

// BenchmarkQueryQuantifiedC includes cursor creation, complete enumeration,
// and cursor destruction, matching the Go public-operation benchmarks.
func BenchmarkQueryQuantifiedC(b *testing.B) {
	source := []byte("package audit\n" + strings.Repeat("// audit\n", 32) + "func F() {}\n")
	lang, err := ParityCLanguage("go")
	if err != nil {
		b.Fatal(err)
	}
	parser := sitter.NewParser()
	defer parser.Close()
	if err := parser.SetLanguage(lang); err != nil {
		b.Fatal(err)
	}
	tree := parser.Parse(source, nil)
	if tree == nil || tree.RootNode().HasError() {
		b.Fatal("C parse failed")
	}
	defer tree.Close()
	q, err := sitter.NewQuery(lang, `(source_file (comment)+ @comment . (type_declaration) @type)`)
	if err != nil {
		b.Fatal(err)
	}
	defer q.Close()
	root := tree.RootNode()
	b.ReportAllocs()
	b.SetBytes(int64(len(source)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cursor := sitter.NewQueryCursor()
		matches := cursor.Matches(q, root, source)
		if matches.Next() != nil {
			b.Fatal("C returned a match for an absent type declaration")
		}
		cursor.Close()
	}
}
