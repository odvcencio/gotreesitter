package gotreesitter_test

import (
	"testing"

	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

func TestCppMalformedClassFunctionDefinitionRecovery(t *testing.T) {
	src := []byte(`int main() {
  a<T>();
  // <- function

  a::b();
  // ^ function

  a::b<C, D>();
  // ^ function

  this->b<C, D>();
  //    ^ function

  auto x = y;
  // <- type

  vector<T> a;
  // <- type

  std::vector<T> a;
  //   ^ type
}

class C : D{
  A();
  // <- function

  void efg() {
    // ^ function
  }
}

void A::b() {
  //    ^ function
}
`)
	lang := grammars.CppLanguage()
	tree, err := gts.NewParser(lang).ParseWithTokenSource(src, grammars.NewCTokenSourceOrEOF(src, lang))
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	defer tree.Release()
	root := tree.RootNode()
	if got, want := root.Type(lang), "translation_unit"; got != want {
		t.Fatalf("root type = %q, want %q\n%s", got, want, root.SExpr(lang))
	}
	if !root.HasError() {
		t.Fatalf("root.HasError = false, want true")
	}
	// The C oracle folds the malformed class and following `void A::b() {}`
	// into one recovered function_definition. Keep the cpp compatibility
	// normalizer scoped to this C shape instead of enabling cpp C-recovery
	// globally; the latter regressed corpus agreement in earlier A/B runs.
	if got, want := root.ChildCount(), 2; got != want {
		t.Fatalf("root child count = %d, want %d\n%s", got, want, root.SExpr(lang))
	}
	recovered := root.Child(1)
	if got, want := recovered.Type(lang), "function_definition"; got != want {
		t.Fatalf("root.Child(1) = %q, want %q\n%s", got, want, root.SExpr(lang))
	}
	if !recovered.HasError() {
		t.Fatalf("recovered function_definition HasError = false, want true\n%s", recovered.SExpr(lang))
	}
	if got, want := recovered.StartByte(), uint32(234); got != want {
		t.Fatalf("recovered function start = %d, want %d", got, want)
	}
	if got, want := recovered.EndByte(), uint32(346); got != want {
		t.Fatalf("recovered function end = %d, want %d", got, want)
	}
	if got, want := recovered.ChildCount(), 3; got != want {
		t.Fatalf("recovered child count = %d, want %d\n%s", got, want, recovered.SExpr(lang))
	}
	if got, want := recovered.Child(0).Type(lang), "class_specifier"; got != want {
		t.Fatalf("recovered child[0] = %q, want %q\n%s", got, want, recovered.SExpr(lang))
	}
	declarator := recovered.Child(1)
	if got, want := declarator.Type(lang), "function_declarator"; got != want {
		t.Fatalf("recovered child[1] = %q, want %q\n%s", got, want, recovered.SExpr(lang))
	}
	qualified := declarator.Child(0)
	if got, want := qualified.Type(lang), "qualified_identifier"; got != want {
		t.Fatalf("declarator child[0] = %q, want %q\n%s", got, want, declarator.SExpr(lang))
	}
	if got, want := qualified.ChildCount(), 4; got != want {
		t.Fatalf("qualified_identifier child count = %d, want %d\n%s", got, want, qualified.SExpr(lang))
	}
	if got, want := qualified.Child(0).Type(lang), "namespace_identifier"; got != want {
		t.Fatalf("qualified child[0] = %q, want %q\n%s", got, want, qualified.SExpr(lang))
	}
	if got, want := qualified.Child(0).Text(src), "void"; got != want {
		t.Fatalf("qualified child[0] text = %q, want %q", got, want)
	}
	errNode := qualified.Child(1)
	if got, want := errNode.Type(lang), "ERROR"; got != want {
		t.Fatalf("qualified child[1] = %q, want %q\n%s", got, want, qualified.SExpr(lang))
	}
	if !errNode.HasError() || !errNode.IsExtra() {
		t.Fatalf("qualified ERROR flags extra=%v hasError=%v, want both true\n%s", errNode.IsExtra(), errNode.HasError(), qualified.SExpr(lang))
	}
	if got, want := errNode.Child(0).Type(lang), "identifier"; got != want {
		t.Fatalf("qualified ERROR child = %q, want %q\n%s", got, want, errNode.SExpr(lang))
	}
	if got, want := errNode.Child(0).Text(src), "A"; got != want {
		t.Fatalf("qualified ERROR child text = %q, want %q", got, want)
	}
}

// TestCppMalformedClassFunctionDefinitionRecoveryFields locks the field
// names the malformed-class/out-of-class-method recovery rewrite assigns to
// the nodes it rebuilds. The C reference runtime assigns type/declarator/body
// on the recovered function_definition, name/body on the synthesized
// class_specifier, declarator/parameters on the rebuilt function_declarator,
// and scope/name on the rebuilt qualified_identifier.
//
// parser_result_cpp.go's node builders (cppNewParent and
// cppCloneParentWithChildren) once cleared field metadata on every rebuilt
// node instead of restoring it. That gap produced nine FieldName parity
// divergences against the C oracle in cgo_harness's
// TestCppMalformedClassFunctionDefinitionRecoveryParity, a cgo-only test
// that had never run in CI and so never caught the regression. This test
// locks the same field assignments without cgo, so a host-side `go test`
// run catches a recurrence.
func TestCppMalformedClassFunctionDefinitionRecoveryFields(t *testing.T) {
	src := []byte(`int main() {
  a<T>();
  // <- function

  a::b();
  // ^ function

  a::b<C, D>();
  // ^ function

  this->b<C, D>();
  //    ^ function

  auto x = y;
  // <- type

  vector<T> a;
  // <- type

  std::vector<T> a;
  //   ^ type
}

class C : D{
  A();
  // <- function

  void efg() {
    // ^ function
  }
}

void A::b() {
  //    ^ function
}
`)
	lang := grammars.CppLanguage()
	tree, err := gts.NewParser(lang).ParseWithTokenSource(src, grammars.NewCTokenSourceOrEOF(src, lang))
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	defer tree.Release()
	root := tree.RootNode()
	if got, want := root.ChildCount(), 2; got != want {
		t.Fatalf("root child count = %d, want %d\n%s", got, want, root.SExpr(lang))
	}

	recovered := root.Child(1)
	if got, want := recovered.Type(lang), "function_definition"; got != want {
		t.Fatalf("root.Child(1) = %q, want %q\n%s", got, want, root.SExpr(lang))
	}

	assertField := func(n *gts.Node, idx int, want, context string) {
		t.Helper()
		if n == nil {
			t.Fatalf("%s: node is nil", context)
		}
		if got := n.FieldNameForChild(idx, lang); got != want {
			t.Fatalf("%s: FieldNameForChild(%d) = %q, want %q\n%s", context, idx, got, want, n.SExpr(lang))
		}
	}

	// recovered function_definition: type=class_specifier,
	// declarator=function_declarator, body=compound_statement.
	assertField(recovered, 0, "type", "recovered function_definition")
	assertField(recovered, 1, "declarator", "recovered function_definition")
	assertField(recovered, 2, "body", "recovered function_definition")

	classSpec := recovered.Child(0)
	if got, want := classSpec.Type(lang), "class_specifier"; got != want {
		t.Fatalf("recovered.Child(0) = %q, want %q\n%s", got, want, recovered.SExpr(lang))
	}
	// synthesized class_specifier: name=type_identifier (child 1),
	// body=field_declaration_list (child 3). class/base_class_clause carry no
	// field in tree-sitter-cpp.
	assertField(classSpec, 1, "name", "synthesized class_specifier")
	assertField(classSpec, 3, "body", "synthesized class_specifier")

	declarator := recovered.Child(1)
	if got, want := declarator.Type(lang), "function_declarator"; got != want {
		t.Fatalf("recovered.Child(1) = %q, want %q\n%s", got, want, recovered.SExpr(lang))
	}
	// rebuilt function_declarator: declarator=qualified_identifier,
	// parameters=parameter_list.
	assertField(declarator, 0, "declarator", "rebuilt function_declarator")
	assertField(declarator, 1, "parameters", "rebuilt function_declarator")

	qualified := declarator.Child(0)
	if got, want := qualified.Type(lang), "qualified_identifier"; got != want {
		t.Fatalf("declarator.Child(0) = %q, want %q\n%s", got, want, declarator.SExpr(lang))
	}
	// rebuilt qualified_identifier: scope=namespace_identifier (child 0),
	// name=identifier (child 3). The ERROR wrapper and "::" carry no field.
	assertField(qualified, 0, "scope", "rebuilt qualified_identifier")
	assertField(qualified, 3, "name", "rebuilt qualified_identifier")
}
