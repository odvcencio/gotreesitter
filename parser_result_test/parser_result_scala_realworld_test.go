package parserresult_test

import (
	"strings"
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
)

func TestScalaPathResolverRecoversTopLevelObjectAndClass(t *testing.T) {
	if raceEnabled {
		t.Skip("skip heavyweight Scala PathResolver realworld parse under -race; non-race coverage keeps the full recovery assertions")
	}
	src := readRealworldCorpusOrSkip(t, "cgo_harness/corpus_real/scala/medium__PathResolver.scala")

	tree, lang := parseByLanguageName(t, "scala", string(src))
	root := tree.RootNode()
	if got, want := root.Type(lang), "compilation_unit"; got != want {
		t.Fatalf("root type = %q, want %q: %s", got, want, root.SExpr(lang))
	}
	if root.HasError() {
		t.Fatalf("unexpected scala parse error: %s", root.SExpr(lang))
	}
	if got, want := root.EndByte(), uint32(len(src)); got != want {
		t.Fatalf("root endByte = %d, want %d", got, want)
	}

	object := firstNode(root, func(n *gotreesitter.Node) bool {
		return n.Type(lang) == "object_definition" &&
			strings.Contains(n.Text(src), "object PathResolver")
	})
	if object == nil {
		t.Fatalf("missing PathResolver object_definition: %s", root.SExpr(lang))
	}
	var sawObjectBody bool
	for i := 0; i < object.ChildCount(); i++ {
		child := object.Child(i)
		if child == nil || child.Type(lang) != "template_body" {
			continue
		}
		sawObjectBody = true
		if got, want := object.FieldNameForChild(i, lang), "body"; got != want {
			t.Fatalf("object template_body field = %q, want %q: %s", got, want, object.SExpr(lang))
		}
	}
	if !sawObjectBody {
		t.Fatalf("missing PathResolver object template_body: %s", object.SExpr(lang))
	}
	class := firstNode(root, func(n *gotreesitter.Node) bool {
		return n.Type(lang) == "class_definition" &&
			strings.Contains(n.Text(src), "final class PathResolver")
	})
	if class == nil {
		t.Fatalf("missing PathResolver class_definition: %s", root.SExpr(lang))
	}
	var sawClassName, sawClassBody bool
	for i := 0; i < class.ChildCount(); i++ {
		child := class.Child(i)
		if child == nil {
			continue
		}
		switch child.Type(lang) {
		case "identifier":
			sawClassName = true
			if got, want := class.FieldNameForChild(i, lang), "name"; got != want {
				t.Fatalf("class identifier field = %q, want %q: %s", got, want, class.SExpr(lang))
			}
		case "template_body":
			sawClassBody = true
			if got, want := class.FieldNameForChild(i, lang), "body"; got != want {
				t.Fatalf("class template_body field = %q, want %q: %s", got, want, class.SExpr(lang))
			}
		}
	}
	if !sawClassName || !sawClassBody {
		t.Fatalf("missing class name/body children: %s", class.SExpr(lang))
	}

	fn := firstNode(class, func(n *gotreesitter.Node) bool {
		return n.Type(lang) == "function_definition"
	})
	if fn == nil {
		t.Fatalf("missing nested function_definition: %s", class.SExpr(lang))
	}
	var sawFnName, sawFnParameters bool
	for i := 0; i < fn.ChildCount(); i++ {
		child := fn.Child(i)
		if child == nil {
			continue
		}
		switch child.Type(lang) {
		case "identifier":
			sawFnName = true
			if got, want := fn.FieldNameForChild(i, lang), "name"; got != want {
				t.Fatalf("function identifier field = %q, want %q: %s", got, want, fn.SExpr(lang))
			}
		case "parameters":
			sawFnParameters = true
			if got, want := fn.FieldNameForChild(i, lang), "parameters"; got != want {
				t.Fatalf("function parameters field = %q, want %q: %s", got, want, fn.SExpr(lang))
			}
		case "type_identifier":
			if got, want := fn.FieldNameForChild(i, lang), "return_type"; got != want {
				t.Fatalf("function return_type field = %q, want %q: %s", got, want, fn.SExpr(lang))
			}
		case "block":
			if got, want := fn.FieldNameForChild(i, lang), "body"; got != want {
				t.Fatalf("function body field = %q, want %q: %s", got, want, fn.SExpr(lang))
			}
		}
	}
	if !sawFnName || !sawFnParameters {
		t.Fatalf("missing function name/parameters fields in recovered nested function: %s", fn.SExpr(lang))
	}

	environmentObject := firstNode(root, func(n *gotreesitter.Node) bool {
		return n.Type(lang) == "object_definition" &&
			strings.Contains(n.Text(src), "object Environment")
	})
	if environmentObject == nil {
		t.Fatalf("missing Environment object_definition: %s", root.SExpr(lang))
	}
	searchFn := firstNode(environmentObject, func(n *gotreesitter.Node) bool {
		return n.Type(lang) == "function_definition" &&
			strings.Contains(n.Text(src), "private def searchForBootClasspath")
	})
	sourcePathFn := firstNode(environmentObject, func(n *gotreesitter.Node) bool {
		return n.Type(lang) == "function_definition" &&
			strings.Contains(n.Text(src), "def sourcePathEnv")
	})
	if searchFn == nil || sourcePathFn == nil {
		t.Fatalf("missing Environment functions after recovery: %s", environmentObject.SExpr(lang))
	}
	if got, want := searchFn.EndByte(), sourcePathFn.StartByte(); got != want {
		t.Fatalf("searchForBootClasspath endByte = %d, want %d at next function start", got, want)
	}
	searchBody := firstNode(searchFn, func(n *gotreesitter.Node) bool {
		return n.Type(lang) == "indented_block"
	})
	if searchBody == nil {
		t.Fatalf("missing searchForBootClasspath indented_block: %s", searchFn.SExpr(lang))
	}
	if got, want := searchBody.EndByte(), sourcePathFn.StartByte(); got != want {
		t.Fatalf("searchForBootClasspath indented_block endByte = %d, want %d at next function start", got, want)
	}

	supplementalObject := firstNode(root, func(n *gotreesitter.Node) bool {
		return n.Type(lang) == "object_definition" &&
			strings.HasPrefix(n.Text(src), "object SupplementalLocations")
	})
	if supplementalObject == nil {
		t.Fatalf("missing SupplementalLocations object_definition: %s", root.SExpr(lang))
	}
	sawSupplementalName := false
	for i := 0; i < supplementalObject.ChildCount(); i++ {
		child := supplementalObject.Child(i)
		if child == nil || child.Type(lang) != "identifier" {
			continue
		}
		sawSupplementalName = true
		if got, want := supplementalObject.FieldNameForChild(i, lang), "name"; got != want {
			t.Fatalf("SupplementalLocations identifier field = %q, want %q: %s", got, want, supplementalObject.SExpr(lang))
		}
	}
	if !sawSupplementalName {
		t.Fatalf("missing SupplementalLocations identifier child: %s", supplementalObject.SExpr(lang))
	}
	supplementalBody := firstNode(supplementalObject, func(n *gotreesitter.Node) bool {
		return n.Type(lang) == "template_body"
	})
	if supplementalBody == nil {
		t.Fatalf("missing SupplementalLocations template_body: %s", supplementalObject.SExpr(lang))
	}
	var sawSupplementalComment bool
	supplementalFns := 0
	for i := 0; i < supplementalBody.ChildCount(); i++ {
		child := supplementalBody.Child(i)
		if child == nil {
			continue
		}
		switch child.Type(lang) {
		case "block_comment":
			sawSupplementalComment = true
			if !child.IsExtra() {
				t.Fatalf("SupplementalLocations block_comment should be extra: %s", supplementalBody.SExpr(lang))
			}
		case "block_comment_repeat1":
			t.Fatalf("SupplementalLocations template_body still contains block_comment_repeat1 fragments: %s", supplementalBody.SExpr(lang))
		case "function_definition":
			supplementalFns++
		}
	}
	if !sawSupplementalComment || supplementalFns < 2 {
		t.Fatalf("unexpected SupplementalLocations template_body shape: %s", supplementalBody.SExpr(lang))
	}

	calculatedObject := firstNode(class, func(n *gotreesitter.Node) bool {
		return n.Type(lang) == "object_definition" &&
			strings.HasPrefix(n.Text(src), "object Calculated")
	})
	if calculatedObject == nil {
		t.Fatalf("missing Calculated object_definition inside PathResolver: %s", class.SExpr(lang))
	}
	calculatedBody := firstNode(calculatedObject, func(n *gotreesitter.Node) bool {
		return n.Type(lang) == "template_body"
	})
	if calculatedBody == nil {
		t.Fatalf("missing Calculated template_body: %s", calculatedObject.SExpr(lang))
	}
	var sawCalculatedBlockComment, sawCalculatedLineComment, sawCalculatedVal bool
	for i := 0; i < calculatedBody.ChildCount(); i++ {
		child := calculatedBody.Child(i)
		if child == nil {
			continue
		}
		switch child.Type(lang) {
		case "block_comment_repeat1":
			t.Fatalf("Calculated template_body still contains block_comment_repeat1 fragments: %s", calculatedBody.SExpr(lang))
		case "block_comment":
			sawCalculatedBlockComment = true
			if !child.IsExtra() {
				t.Fatalf("Calculated block_comment should be extra: %s", calculatedBody.SExpr(lang))
			}
		case "comment":
			sawCalculatedLineComment = true
			if !child.IsExtra() {
				t.Fatalf("Calculated comment should be extra: %s", calculatedBody.SExpr(lang))
			}
		case "val_definition":
			sawCalculatedVal = true
			if !strings.Contains(child.Text(src), "lazy val containers") {
				t.Fatalf("unexpected Calculated val_definition recovered: %s", child.SExpr(lang))
			}
		}
	}
	if !sawCalculatedBlockComment || !sawCalculatedLineComment || !sawCalculatedVal {
		t.Fatalf("Calculated template_body missing recovered comment/val nodes: %s", calculatedBody.SExpr(lang))
	}

	caseBlock := firstNode(root, func(n *gotreesitter.Node) bool {
		return n.Type(lang) == "case_block" && n.ChildCount() >= 4
	})
	if caseBlock == nil {
		t.Fatalf("missing multi-clause case_block: %s", root.SExpr(lang))
	}
	var clauses []*gotreesitter.Node
	var closeBrace *gotreesitter.Node
	for i := 0; i < caseBlock.ChildCount(); i++ {
		child := caseBlock.Child(i)
		if child == nil {
			continue
		}
		switch child.Type(lang) {
		case "case_clause":
			clauses = append(clauses, child)
		case "}":
			closeBrace = child
		}
	}
	if len(clauses) < 2 || closeBrace == nil {
		t.Fatalf("unexpected case_block shape: %s", caseBlock.SExpr(lang))
	}
	for i := 0; i+1 < len(clauses); i++ {
		if got, want := clauses[i].EndByte(), clauses[i+1].StartByte(); got != want {
			t.Fatalf("case_clause[%d] endByte = %d, want %d at next sibling start", i, got, want)
		}
	}
	if got, want := clauses[len(clauses)-1].EndByte(), closeBrace.StartByte(); got != want {
		t.Fatalf("final case_clause endByte = %d, want %d at close brace", got, want)
	}

	asURLs := firstNode(class, func(n *gotreesitter.Node) bool {
		return n.Type(lang) == "function_definition" &&
			strings.Contains(n.Text(src), "def asURLs: List[URL] = resultAsURLs.toList")
	})
	if asURLs == nil {
		t.Fatalf("missing annotated asURLs function_definition: %s", class.SExpr(lang))
	}
	if got, want := asURLs.StartPoint().Row, uint32(284); got != want {
		t.Fatalf("asURLs start row = %d, want %d", got, want)
	}
	sawAnnotation := false
	for i := 0; i < asURLs.ChildCount(); i++ {
		child := asURLs.Child(i)
		if child == nil || child.Type(lang) != "annotation" {
			continue
		}
		sawAnnotation = true
	}
	if !sawAnnotation {
		t.Fatalf("missing annotation child on asURLs: %s", asURLs.SExpr(lang))
	}
}

func TestScalaPathResolverSimpleImportDotsUsePathField(t *testing.T) {
	if raceEnabled {
		t.Skip("skip heavyweight Scala PathResolver realworld parse under -race; smaller Scala tests still cover field normalization")
	}
	src := readRealworldCorpusOrSkip(t, "cgo_harness/corpus_real/scala/medium__PathResolver.scala")

	tree, lang := parseByLanguageName(t, "scala", string(src))
	root := tree.RootNode()
	if root.HasError() {
		t.Fatalf("unexpected scala parse error: %s", root.SExpr(lang))
	}

	imp := firstNode(root, func(n *gotreesitter.Node) bool {
		return n.Type(lang) == "import_declaration" &&
			strings.Contains(n.Text(src), "import PartialFunction.condOpt")
	})
	if imp == nil {
		t.Fatalf("missing PartialFunction.condOpt import_declaration: %s", root.SExpr(lang))
	}

	sawDot := false
	for i := 0; i < imp.ChildCount(); i++ {
		child := imp.Child(i)
		if child == nil || child.Type(lang) != "." {
			continue
		}
		sawDot = true
		if got, want := imp.FieldNameForChild(i, lang), "path"; got != want {
			t.Fatalf("import dot field = %q, want %q: %s", got, want, imp.SExpr(lang))
		}
	}
	if !sawDot {
		t.Fatalf("missing dot child in PartialFunction.condOpt import: %s", imp.SExpr(lang))
	}
}

func TestScalaPathResolverNamespaceSelectorDotDoesNotUsePathField(t *testing.T) {
	if raceEnabled {
		t.Skip("skip heavyweight Scala PathResolver realworld parse under -race; smaller Scala tests still cover field normalization")
	}
	src := readRealworldCorpusOrSkip(t, "cgo_harness/corpus_real/scala/medium__PathResolver.scala")

	tree, lang := parseByLanguageName(t, "scala", string(src))
	root := tree.RootNode()
	if root.HasError() {
		t.Fatalf("unexpected scala parse error: %s", root.SExpr(lang))
	}

	imp := firstNode(root, func(n *gotreesitter.Node) bool {
		return n.Type(lang) == "import_declaration" &&
			strings.Contains(n.Text(src), "scala.reflect.io.{Directory, File, Path}")
	})
	if imp == nil {
		t.Fatalf("missing namespace-selector import_declaration: %s", root.SExpr(lang))
	}

	for i := 0; i < imp.ChildCount(); i++ {
		child := imp.Child(i)
		if child == nil || child.Type(lang) != "." {
			continue
		}
		if next := imp.Child(i + 1); next != nil && next.Type(lang) == "namespace_selectors" {
			if got := imp.FieldNameForChild(i, lang); got != "" {
				t.Fatalf("namespace-selector dot field = %q, want empty: %s", got, imp.SExpr(lang))
			}
			return
		}
	}
	t.Fatalf("missing dot before namespace_selectors: %s", imp.SExpr(lang))
}
