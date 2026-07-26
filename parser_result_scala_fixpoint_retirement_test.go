package gotreesitter

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestScalaFormerSecondPassIsInertAfterCanonicalCompatibility(t *testing.T) {
	lang := loadScalaLanguageForFixpointRetirementTest(t)
	fixtures := map[string][]byte{
		"object_fragments": []byte(`object Outer {
  object Inner {
    val value = 1
  }
}`),
		"block_comment": []byte(`object Outer {
  /* comment */
  val value = 1
}`),
		"definition_fields": []byte(`object Outer {
  def choose(value: Int): Int =
    if value == 0 then 1 else 2
}`),
		"annotation": []byte(`object Outer {
  @deprecated
  def value = 1
}`),
	}

	for name, source := range fixtures {
		t.Run(name, func(t *testing.T) {
			assertScalaFormerSecondPassIsInert(t, lang, source)
		})
	}
}

func TestScalaFormerSecondPassIsInertOverRealCorpus(t *testing.T) {
	const corpusRoot = "cgo_harness/corpus_real/scala"
	info, err := os.Stat(corpusRoot)
	if os.IsNotExist(err) {
		t.Skipf("optional authenticated Scala corpus is absent: %s", corpusRoot)
	}
	if err != nil {
		t.Fatal(err)
	}
	if !info.IsDir() {
		t.Fatalf("authenticated Scala corpus path is not a directory: %s", corpusRoot)
	}

	lang := loadScalaLanguageForFixpointRetirementTest(t)
	files := 0
	err = filepath.Walk(corpusRoot, func(path string, fileInfo os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if fileInfo.IsDir() || !strings.HasSuffix(path, ".scala") {
			return nil
		}
		source, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		t.Run(filepath.Base(path), func(t *testing.T) {
			assertScalaFormerSecondPassIsInert(t, lang, source)
		})
		files++
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if files == 0 {
		t.Fatalf("authenticated Scala corpus is empty: %s", corpusRoot)
	}
}

func assertScalaFormerSecondPassIsInert(t *testing.T, lang *Language, source []byte) {
	t.Helper()
	tree, err := NewParser(lang).Parse(source)
	if err != nil {
		t.Fatal(err)
	}
	defer tree.Release()

	root := rawRootOrNil(tree)
	before := scalaFixpointRetirementTreeFingerprint(root)
	normalizeScalaTemplateBodyObjectFragments(root, source, nil, lang)
	normalizeScalaRecoveredObjectTemplateBodies(root, source, nil, lang)
	normalizeScalaDefinitionFields(root, source, lang)
	normalizeScalaTemplateBodyFunctionAnnotations(root, source, nil, lang)
	after := scalaFixpointRetirementTreeFingerprint(root)
	if after != before {
		t.Fatalf("former Scala second pass mutated the canonical tree: %x -> %x", before, after)
	}
}

func loadScalaLanguageForFixpointRetirementTest(t *testing.T) *Language {
	t.Helper()
	blob, err := os.ReadFile("grammars/grammar_blobs/scala.bin")
	if err != nil {
		t.Fatal(err)
	}
	lang, err := LoadLanguage(blob)
	if err != nil {
		t.Fatal(err)
	}
	return lang
}

func scalaFixpointRetirementTreeFingerprint(root *Node) [32]byte {
	hash := sha256.New()
	var walk func(*Node)
	walk = func(node *Node) {
		if node == nil {
			fmt.Fprint(hash, "nil;")
			return
		}
		fmt.Fprintf(
			hash,
			"%d:%d:%d:%d:%d:%d:%d:%t:%t:%t:%d:%v;",
			node.symbol,
			node.startByte,
			node.endByte,
			node.startPoint.Row,
			node.startPoint.Column,
			node.endPoint.Row,
			node.endPoint.Column,
			node.isNamed(),
			node.IsExtra(),
			node.HasError(),
			node.productionID,
			node.fieldIDs(),
		)
		for _, child := range node.children {
			walk(child)
		}
	}
	walk(root)
	var fingerprint [32]byte
	copy(fingerprint[:], hash.Sum(nil))
	return fingerprint
}
