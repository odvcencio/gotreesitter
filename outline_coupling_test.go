package gotreesitter_test

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"strings"
	"testing"

	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

// outlineSourceFiles lists every file that carries the outline mechanism.
// The static scan below reads exactly these files, so a new outline source
// file must be added here or the scan will not cover it.
var outlineSourceFiles = []string{
	"outline.go",
	"outline_owner.go",
	"outline_report.go",
}

// outlineForbiddenIdentifiers names the parser-core surfaces the outline must
// never touch. The outline is a read-only projection over a tree somebody else
// already built: it must not parse, must not choose a parse route, must not
// enter admission, and must not run result compatibility.
//
// The list mixes parse entry points, route dispatch, admission, and result
// compatibility, which are the four surfaces whose behavior the parse gates
// pin byte for byte.
var outlineForbiddenIdentifiers = []string{
	"parseInternal",
	"parseIncrementalInternal",
	"parseWithTokenSource",
	"parseForRecovery",
	"parseForest",
	"dispatchParse",
	"NewParser(",
	"Parse(",
	"admission",
	"Admission",
	"resultCompatibility",
	"ResultCompatibility",
	"normalizeResultCompatibility",
	"compactRoute",
	"runLanguageResultCompatibility",
}

// TestOutlineSourceHasNoParserCoreCoupling is gate G-4a: a static scan. It
// proves by reading the shipped source that the outline mechanism names no
// parse, route, admission, or result-compatibility identifier. A projection
// that never mentions those surfaces cannot change what a parse produces.
func TestOutlineSourceHasNoParserCoreCoupling(t *testing.T) {
	for _, file := range outlineSourceFiles {
		data, err := os.ReadFile(file)
		if err != nil {
			t.Fatalf("read outline source %s: %v", file, err)
		}
		text := string(data)
		for _, forbidden := range outlineForbiddenIdentifiers {
			if strings.Contains(text, forbidden) {
				t.Errorf("%s names the parser-core identifier %q; the outline must stay a read-only projection over an already-built tree",
					file, forbidden)
			}
		}
	}
}

// TestOutlineSourceScanCoversEveryOutlineFile keeps the static scan honest. A
// scan that reads a stale file list is the exact failure this repository has
// seen before: a test that exists, passes, and covers nothing.
func TestOutlineSourceScanCoversEveryOutlineFile(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("read package directory: %v", err)
	}
	listed := map[string]bool{}
	for _, file := range outlineSourceFiles {
		listed[file] = true
	}
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasPrefix(name, "outline") || !strings.HasSuffix(name, ".go") {
			continue
		}
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		if !listed[name] {
			t.Errorf("%s carries outline code but is missing from outlineSourceFiles, so the coupling scan skips it", name)
		}
	}
	for _, file := range outlineSourceFiles {
		if _, err := os.Stat(file); err != nil {
			t.Errorf("outlineSourceFiles names %s, which does not exist: %v", file, err)
		}
	}
}

// TestOutlineSourceScanDetectsCoupling proves the scan can fail. A gate that
// cannot fail is not a gate.
func TestOutlineSourceScanDetectsCoupling(t *testing.T) {
	sample := "func project() { p := NewParser(lang); _ = p }"
	hit := false
	for _, forbidden := range outlineForbiddenIdentifiers {
		if strings.Contains(sample, forbidden) {
			hit = true
			break
		}
	}
	if !hit {
		t.Fatal("the forbidden-identifier list does not catch a direct parser construction; the scan is vacuous")
	}
}

// TestOutlineTreeLeavesTheTreeUnchanged is gate G-4b: the behavioral half. It
// parses once, digests the whole tree as an s-expression, projects the
// outline, and digests again. A projection that changed any node, span, or
// field would move the digest.
func TestOutlineTreeLeavesTheTreeUnchanged(t *testing.T) {
	for _, fixture := range outlineGoldenFixtures {
		fixture := fixture
		t.Run(fixture.File, func(t *testing.T) {
			entry := grammars.DetectLanguageByName(fixture.Language)
			if entry == nil {
				t.Fatalf("language %q is not registered", fixture.Language)
			}
			lang := entry.Language()
			source, err := os.ReadFile(outlineGoldenRoot + "/" + fixture.File)
			if err != nil {
				t.Fatalf("read fixture: %v", err)
			}

			parser := gts.NewParser(lang)
			tree, err := parser.Parse(source)
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			defer tree.Release()

			before := sExprDigest(tree.RootNode().SExpr(lang))
			beforeSource := sha256.Sum256(tree.Source())

			outliner, err := gts.NewOutliner(lang, grammars.ResolveTagsQuery(*entry))
			if err != nil {
				t.Fatalf("build outliner: %v", err)
			}
			// Project twice: a projection that mutated the tree would also
			// have to be idempotent to hide, and this catches the rest.
			firstSymbols, firstReport := outliner.OutlineTree(tree)
			secondSymbols, secondReport := outliner.OutlineTree(tree)

			after := sExprDigest(tree.RootNode().SExpr(lang))
			afterSource := sha256.Sum256(tree.Source())

			if before != after {
				t.Errorf("tree s-expression digest moved across OutlineTree: %s -> %s", before, after)
			}
			if beforeSource != afterSource {
				t.Error("tree source bytes changed across OutlineTree")
			}
			if firstReport != secondReport {
				t.Errorf("OutlineTree is not repeatable: %+v then %+v", firstReport, secondReport)
			}
			if !outlineSymbolsEqual(firstSymbols, secondSymbols) {
				t.Error("OutlineTree returned different symbols on the second call over the same tree")
			}
		})
	}
}

func sExprDigest(sexpr string) string {
	sum := sha256.Sum256([]byte(sexpr))
	return hex.EncodeToString(sum[:])
}

func outlineSymbolsEqual(a, b []gts.OutlineSymbol) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].Kind != b[i].Kind ||
			a[i].Name != b[i].Name ||
			a[i].NodeType != b[i].NodeType ||
			a[i].Range != b[i].Range ||
			a[i].NameRange != b[i].NameRange ||
			a[i].Owner != b[i].Owner {
			return false
		}
		if !outlineSymbolsEqual(a[i].Children, b[i].Children) {
			return false
		}
	}
	return true
}
