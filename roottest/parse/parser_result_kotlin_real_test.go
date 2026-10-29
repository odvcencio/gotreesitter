package parse_test

import (
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

func TestKotlinGenericCallTypeArgumentsCompatibilityRealParser(t *testing.T) {
	lang := grammars.KotlinLanguage()
	parser := gotreesitter.NewParser(lang)
	source := []byte(`tasks.named<KotlinCompile>("compile") {}`)
	tree, err := parser.Parse(source)
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}
	if tree == nil || tree.RootNode() == nil {
		t.Fatal("Parse returned nil tree")
	}
	defer tree.Release()

	call := findNodeByText(tree.RootNode(), lang, source, "call_expression", string(source))
	if call == nil {
		t.Fatalf("call_expression not found:\n%s", tree.RootNode().SExpr(lang))
	}
	if got, want := call.ChildCount(), 2; got != want {
		t.Fatalf("call child count = %d, want %d:\n%s", got, want, tree.RootNode().SExpr(lang))
	}
	suffix := call.Child(1)
	if got, want := suffix.Type(lang), "call_suffix"; got != want {
		t.Fatalf("call child[1] = %q, want %q:\n%s", got, want, tree.RootNode().SExpr(lang))
	}
	wantTypes := []string{"type_arguments", "value_arguments", "annotated_lambda"}
	if got, want := suffix.ChildCount(), len(wantTypes); got != want {
		t.Fatalf("suffix child count = %d, want %d:\n%s", got, want, tree.RootNode().SExpr(lang))
	}
	for i, want := range wantTypes {
		if got := suffix.Child(i).Type(lang); got != want {
			t.Fatalf("suffix child[%d] = %q, want %q:\n%s", i, got, want, tree.RootNode().SExpr(lang))
		}
	}
}

func TestKotlinGenericCallTypeArgumentsLabeledLambdaCompatibilityRealParser(t *testing.T) {
	lang := grammars.KotlinLanguage()
	parser := gotreesitter.NewParser(lang)
	source := []byte(`suspendCoroutineUninterceptedOrReturn<Unit> sc@{}`)
	tree, err := parser.Parse(source)
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}
	if tree == nil || tree.RootNode() == nil {
		t.Fatal("Parse returned nil tree")
	}
	defer tree.Release()

	call := findNodeByText(tree.RootNode(), lang, source, "call_expression", string(source))
	if call == nil {
		t.Fatalf("call_expression not found:\n%s", tree.RootNode().SExpr(lang))
	}
	if got, want := call.ChildCount(), 2; got != want {
		t.Fatalf("call child count = %d, want %d:\n%s", got, want, tree.RootNode().SExpr(lang))
	}
	suffix := call.Child(1)
	if got, want := suffix.Type(lang), "call_suffix"; got != want {
		t.Fatalf("call child[1] = %q, want %q:\n%s", got, want, tree.RootNode().SExpr(lang))
	}
	wantTypes := []string{"type_arguments", "annotated_lambda"}
	if got, want := suffix.ChildCount(), len(wantTypes); got != want {
		t.Fatalf("suffix child count = %d, want %d:\n%s", got, want, tree.RootNode().SExpr(lang))
	}
	for i, want := range wantTypes {
		if got := suffix.Child(i).Type(lang); got != want {
			t.Fatalf("suffix child[%d] = %q, want %q:\n%s", i, got, want, tree.RootNode().SExpr(lang))
		}
	}
}

func TestKotlinGenericCallTypeArgumentsChainedLambdaCompatibilityRealParser(t *testing.T) {
	lang := grammars.KotlinLanguage()
	parser := gotreesitter.NewParser(lang)
	source := []byte(`tasks.withType<Test>().configureEach {}`)
	tree, err := parser.Parse(source)
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}
	if tree == nil || tree.RootNode() == nil {
		t.Fatal("Parse returned nil tree")
	}
	defer tree.Release()

	call := findNodeByText(tree.RootNode(), lang, source, "call_expression", string(source))
	if call == nil {
		t.Fatalf("call_expression not found:\n%s", tree.RootNode().SExpr(lang))
	}
	if got, want := call.ChildCount(), 2; got != want {
		t.Fatalf("call child count = %d, want %d:\n%s", got, want, tree.RootNode().SExpr(lang))
	}
	nav := call.Child(0)
	if got, want := nav.Type(lang), "navigation_expression"; got != want {
		t.Fatalf("call child[0] = %q, want %q:\n%s", got, want, tree.RootNode().SExpr(lang))
	}
	innerCall := nav.Child(0)
	if got, want := innerCall.Type(lang), "call_expression"; got != want {
		t.Fatalf("navigation child[0] = %q, want %q:\n%s", got, want, tree.RootNode().SExpr(lang))
	}
	innerSuffix := innerCall.Child(1)
	wantInnerTypes := []string{"type_arguments", "value_arguments"}
	for i, want := range wantInnerTypes {
		if got := innerSuffix.Child(i).Type(lang); got != want {
			t.Fatalf("inner suffix child[%d] = %q, want %q:\n%s", i, got, want, tree.RootNode().SExpr(lang))
		}
	}
	if got, want := call.Child(1).Child(0).Type(lang), "annotated_lambda"; got != want {
		t.Fatalf("outer suffix child[0] = %q, want %q:\n%s", got, want, tree.RootNode().SExpr(lang))
	}
}

func TestKotlinGenericCallTypeArgumentsDirectLambdaCompatibilityRealParser(t *testing.T) {
	lang := grammars.KotlinLanguage()
	parser := gotreesitter.NewParser(lang)
	source := []byte(`extensions.configure<DokkaExtension> {}`)
	tree, err := parser.Parse(source)
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}
	if tree == nil || tree.RootNode() == nil {
		t.Fatal("Parse returned nil tree")
	}
	defer tree.Release()

	call := findNodeByText(tree.RootNode(), lang, source, "call_expression", string(source))
	if call == nil {
		t.Fatalf("call_expression not found:\n%s", tree.RootNode().SExpr(lang))
	}
	suffix := call.Child(1)
	wantTypes := []string{"type_arguments", "annotated_lambda"}
	if got, want := suffix.ChildCount(), len(wantTypes); got != want {
		t.Fatalf("suffix child count = %d, want %d:\n%s", got, want, tree.RootNode().SExpr(lang))
	}
	for i, want := range wantTypes {
		if got := suffix.Child(i).Type(lang); got != want {
			t.Fatalf("suffix child[%d] = %q, want %q:\n%s", i, got, want, tree.RootNode().SExpr(lang))
		}
	}
}

func TestKotlinGenericCallTypeArgumentsDirectValueArgumentsCompatibilityRealParser(t *testing.T) {
	lang := grammars.KotlinLanguage()
	parser := gotreesitter.NewParser(lang)
	source := []byte(`Flux.from<Int>(toBeMaxed.apply(word))`)
	tree, err := parser.Parse(source)
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}
	if tree == nil || tree.RootNode() == nil {
		t.Fatal("Parse returned nil tree")
	}
	defer tree.Release()

	call := findNodeByText(tree.RootNode(), lang, source, "call_expression", string(source))
	if call == nil {
		t.Fatalf("call_expression not found:\n%s", tree.RootNode().SExpr(lang))
	}
	if got, want := call.ChildCount(), 2; got != want {
		t.Fatalf("call child count = %d, want %d:\n%s", got, want, tree.RootNode().SExpr(lang))
	}
	suffix := call.Child(1)
	wantTypes := []string{"type_arguments", "value_arguments"}
	for i, want := range wantTypes {
		if got := suffix.Child(i).Type(lang); got != want {
			t.Fatalf("suffix child[%d] = %q, want %q:\n%s", i, got, want, tree.RootNode().SExpr(lang))
		}
	}
}

func TestKotlinNestedGenericCallTrailingLambdaRecoveryRealParser(t *testing.T) {
	lang := grammars.KotlinLanguage()
	parser := gotreesitter.NewParser(lang)
	source := []byte(`package p

class C {
    fun f() {
        val x = Function<String, Flux<Long>> { word ->
            Flux.from(a.apply(word)
                .flatMap<Map.Entry<Int, LongWrapper>> { map -> Flux.fromIterable<Map.Entry<Int, LongWrapper>>(Iterable { map.entries.iterator() }) }
                .flatMap(blank)
                .reduce { a, b -> sum(a, b) })
        }
    }
}`)
	tree, err := parser.Parse(source)
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}
	if tree == nil || tree.RootNode() == nil {
		t.Fatal("Parse returned nil tree")
	}
	defer tree.Release()

	root := tree.RootNode()
	if root.HasError() {
		t.Fatalf("root HasError = true, want false:\n%s", root.SExpr(lang))
	}
	classDecl := findNodeByText(root, lang, source, "class_declaration", `class C {
    fun f() {
        val x = Function<String, Flux<Long>> { word ->
            Flux.from(a.apply(word)
                .flatMap<Map.Entry<Int, LongWrapper>> { map -> Flux.fromIterable<Map.Entry<Int, LongWrapper>>(Iterable { map.entries.iterator() }) }
                .flatMap(blank)
                .reduce { a, b -> sum(a, b) })
        }
    }
}`)
	if classDecl == nil {
		t.Fatalf("class_declaration not found:\n%s", root.SExpr(lang))
	}
	innerCall := findNodeByText(root, lang, source, "call_expression", `Flux.fromIterable<Map.Entry<Int, LongWrapper>>(Iterable { map.entries.iterator() })`)
	if innerCall == nil {
		t.Fatalf("inner generic call not found:\n%s", root.SExpr(lang))
	}
	if got, want := innerCall.ChildCount(), 2; got != want {
		t.Fatalf("inner call child count = %d, want %d:\n%s", got, want, root.SExpr(lang))
	}
	suffix := innerCall.Child(1)
	wantTypes := []string{"type_arguments", "value_arguments"}
	for i, want := range wantTypes {
		if got := suffix.Child(i).Type(lang); got != want {
			t.Fatalf("inner suffix child[%d] = %q, want %q:\n%s", i, got, want, root.SExpr(lang))
		}
	}
}

func TestKotlinPrefixIncrementComparisonCompatibilityRealParser(t *testing.T) {
	lang := grammars.KotlinLanguage()
	parser := gotreesitter.NewParser(lang)
	source := []byte(`fun f() { if (++consumed < count) {} }`)
	tree, err := parser.Parse(source)
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}
	if tree == nil || tree.RootNode() == nil {
		t.Fatal("Parse returned nil tree")
	}
	defer tree.Release()

	cmp := findNodeByText(tree.RootNode(), lang, source, "comparison_expression", "++consumed < count")
	if cmp == nil {
		t.Fatalf("comparison_expression not found:\n%s", tree.RootNode().SExpr(lang))
	}
	if got, want := cmp.ChildCount(), 3; got != want {
		t.Fatalf("comparison child count = %d, want %d:\n%s", got, want, tree.RootNode().SExpr(lang))
	}
	if got, want := cmp.Child(0).Type(lang), "prefix_expression"; got != want {
		t.Fatalf("comparison child[0] = %q, want %q:\n%s", got, want, tree.RootNode().SExpr(lang))
	}
}
