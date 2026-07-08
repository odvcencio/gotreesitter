package grammargen

import (
	"os"
	"testing"
	"time"

	"github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

func TestCSharpInterfaceDefaultMethodInvocationParity(t *testing.T) {
	genLang := loadGeneratedCSharpLanguageForParity(t)
	refLang := grammars.CSharpLanguage()
	adaptExternalScanner(refLang, genLang)

	sample := "interface MyDefault {\n" +
		"  void Log(string message) {\n" +
		"    Console.WriteLine(message);\n" +
		"  }\n" +
		"}\n"

	assertGeneratedAndReferenceDeepParity(t, genLang, refLang, sample)
}

func TestCSharpContextualFileInvocationParity(t *testing.T) {
	genLang := loadGeneratedCSharpLanguageForParity(t)
	refLang := grammars.CSharpLanguage()
	adaptExternalScanner(refLang, genLang)

	cases := []struct {
		name string
		src  string
	}{
		{
			name: "member_access",
			src:  "file.Method(1, 2);\n",
		},
		{
			name: "identifier_argument",
			src: "void m()\n" +
				"{\n" +
				"    m(file);\n" +
				"}\n",
		},
		{
			name: "scoped_lambdas",
			src: "void m()\n" +
				"{\n" +
				"    var l = scoped => null;\n" +
				"    var l = (scoped i) => null;\n" +
				"    var l = (scoped, i) => null;\n" +
				"    var l = scoped (int i, int j) => null;\n" +
				"}\n",
		},
		{
			name: "scoped_contextual_block",
			src: "void scoped() { }\n" +
				"void m(scoped p) { }\n" +
				"void m(scoped ref int p) { }\n" +
				"void m(scoped ref scoped p) { }\n" +
				"void m(int scoped) { }\n" +
				"void m()\n" +
				"{\n" +
				"    scoped v = null;\n" +
				"    scoped ref int v = null;\n" +
				"    scoped ref scoped v = null;\n" +
				"    int scoped = null;\n" +
				"\n" +
				"    scoped();\n" +
				"    m(scoped);\n" +
				"\n" +
				"    var x = scoped + 1;\n" +
				"    var l = scoped => null;\n" +
				"    var l = (scoped i) => null;\n" +
				"    var l = (scoped, i) => null;\n" +
				"    var l = scoped (int i, int j) => null;\n" +
				"}\n" +
				"\n" +
				"class scoped { }\n",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assertGeneratedAndReferenceDeepParity(t, genLang, refLang, tc.src)
		})
	}
}

func TestCSharpQueryJoinClauseParity(t *testing.T) {
	genLang := loadGeneratedCSharpLanguageForParity(t)
	refLang := grammars.CSharpLanguage()
	adaptExternalScanner(refLang, genLang)

	sample := "class C\n" +
		"{\n" +
		"    void M()\n" +
		"    {\n" +
		"        var x = from a in sourceA\n" +
		"                join b in sourceB on a.FK equals b.PK\n" +
		"                select a;\n" +
		"    }\n" +
		"}\n"

	assertGeneratedAndReferenceDeepParity(t, genLang, refLang, sample)
}

func TestCSharpConditionalStringLiteralParity(t *testing.T) {
	genLang := loadGeneratedCSharpLanguageForParity(t)
	refLang := grammars.CSharpLanguage()
	adaptExternalScanner(refLang, genLang)

	sample := "class C\n" +
		"{\n" +
		"    void M(bool x)\n" +
		"    {\n" +
		"        string y = x ? \"foo\" : \"bar\";\n" +
		"    }\n" +
		"}\n"

	assertGeneratedAndReferenceDeepParity(t, genLang, refLang, sample)
}

func TestCSharpExpressionCorpusParity(t *testing.T) {
	genLang := loadGeneratedCSharpLanguageForParity(t)
	refLang := grammars.CSharpLanguage()
	adaptExternalScanner(refLang, genLang)

	cases := []struct {
		name string
		src  string
	}{
		{
			name: "element_binding_expression",
			src:  "var x = [ y, ];\n",
		},
		{
			name: "logical_and_cast_dereference",
			src:  "bool c = (a) && b;\n",
		},
		{
			name: "generic_invocation_argument",
			src:  "MyFunction<A,B>(1);\n",
		},
		{
			name: "conditional_is_pattern_initializer",
			src:  "int a = 1 is Object ? 1 : 2;\n",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assertGeneratedAndReferenceDeepParity(t, genLang, refLang, tc.src)
		})
	}

	t.Run("parser_reuse", func(t *testing.T) {
		genParser := gotreesitter.NewParser(genLang)
		refParser := gotreesitter.NewParser(refLang)
		for _, tc := range cases {
			data := []byte(tc.src)
			genTree, err := genParser.Parse(data)
			if err != nil {
				t.Fatalf("%s generated parse: %v", tc.name, err)
			}
			refTree, err := refParser.Parse(data)
			if err != nil {
				t.Fatalf("%s reference parse: %v", tc.name, err)
			}
			if divs := compareTreesDeep(genTree.RootNode(), genLang, refTree.RootNode(), refLang, "root", 10); len(divs) > 0 {
				t.Fatalf("%s parser reuse deep mismatch\nGEN: %s\nREF: %s\nDIVS: %v",
					tc.name,
					safeSExpr(genTree.RootNode(), genLang, 256),
					safeSExpr(refTree.RootNode(), refLang, 256),
					divs)
			}
			genTree.Release()
			refTree.Release()
		}
	})

	t.Run("parser_reuse_after_highlight_samples", func(t *testing.T) {
		prefixPaths := []string{
			"/tmp/grammar_parity/c_sharp/test/highlight/baseline.cs",
			"/tmp/grammar_parity/c_sharp/test/highlight/operators.cs",
			"/tmp/grammar_parity/c_sharp/test/highlight/types.cs",
			"/tmp/grammar_parity/c_sharp/test/highlight/variableDeclarations.cs",
		}
		genParser := gotreesitter.NewParser(genLang)
		refParser := gotreesitter.NewParser(refLang)
		assertProbe := func(path string) {
			tc := cases[0]
			probe := []byte(tc.src)
			freshGenTree, err := gotreesitter.NewParser(genLang).Parse(probe)
			if err != nil {
				t.Fatalf("fresh generated parse after %s: %v", path, err)
			}
			freshRefTree, err := gotreesitter.NewParser(refLang).Parse(probe)
			if err != nil {
				t.Fatalf("fresh reference parse after %s: %v", path, err)
			}
			if divs := compareTreesDeep(freshGenTree.RootNode(), genLang, freshRefTree.RootNode(), refLang, "root", 10); len(divs) > 0 {
				t.Fatalf("fresh parser after %s deep mismatch\nGEN: %s\nREF: %s\nDIVS: %v",
					path,
					safeSExpr(freshGenTree.RootNode(), genLang, 256),
					safeSExpr(freshRefTree.RootNode(), refLang, 256),
					divs)
			}
			freshGenTree.Release()
			freshRefTree.Release()
		}
		for _, path := range prefixPaths {
			data, err := os.ReadFile(path)
			if err != nil {
				t.Skipf("C# highlight fixture not available: %v", err)
			}
			genTree, _ := genParser.Parse(data)
			if genTree != nil {
				genTree.Release()
			}
			refTree, _ := refParser.Parse(data)
			if refTree != nil {
				refTree.Release()
			}
			assertProbe(path)
		}
		tc := cases[0]
		data := []byte(tc.src)
		genTree, err := genParser.Parse(data)
		if err != nil {
			t.Fatalf("generated parse after prefix: %v", err)
		}
		refTree, err := refParser.Parse(data)
		if err != nil {
			t.Fatalf("reference parse after prefix: %v", err)
		}
		if divs := compareTreesDeep(genTree.RootNode(), genLang, refTree.RootNode(), refLang, "root", 10); len(divs) > 0 {
			t.Fatalf("parser reuse after highlight prefix deep mismatch\nGEN: %s\nREF: %s\nDIVS: %v",
				safeSExpr(genTree.RootNode(), genLang, 256),
				safeSExpr(refTree.RootNode(), refLang, 256),
				divs)
		}
		genTree.Release()
		refTree.Release()
	})
}

func TestCSharpStatementCorpusParity(t *testing.T) {
	genLang := loadGeneratedCSharpLanguageForParity(t)
	refLang := grammars.CSharpLanguage()
	adaptExternalScanner(refLang, genLang)

	cases := []struct {
		name string
		src  string
	}{
		{
			name: "mixed_declarations_and_assignments",
			src: "class A {\n" +
				"  void Sample() {\n" +
				"    int a;\n" +
				"    int a = 1, b = 2;\n" +
				"    const int a = 1;\n" +
				"    const int a = 1, b = 2;\n" +
				"    ref var value = ref data[i];\n" +
				"    var g = args[0].Length;\n" +
				"\n" +
				"    numbers ??= new List<int>();\n" +
				"    b = obj ?? a == 0;\n" +
				"\n" +
				"    person = new Person(null!, null!);\n" +
				"\n" +
				"    MyClass myVar = MyFunction<MyOtherClass>(\"MyArg\");\n" +
				"  }\n" +
				"}\n",
		},
		{
			name: "using_statements",
			src: "class A {\n" +
				"  void Sample() {\n" +
				"    using (var a = b) {\n" +
				"      return;\n" +
				"    }\n" +
				"\n" +
				"    using (Stream a = File.OpenRead(\"a\"), b = new BinaryReader(a)) {\n" +
				"      return;\n" +
				"    }\n" +
				"\n" +
				"    using var a = new A();\n" +
				"\n" +
				"    using (Object a = b) {\n" +
				"      return;\n" +
				"    }\n" +
				"\n" +
				"    using (this) {\n" +
				"      return;\n" +
				"    }\n" +
				"  }\n" +
				"}\n",
		},
		{
			name: "variable_declarations_highlight",
			src: "class A\n" +
				"{\n" +
				"    public void M()\n" +
				"    {\n" +
				"        foreach (int i in new[] { 1 })\n" +
				"        //           ^ variable\n" +
				"        {\n" +
				"            int j = i;\n" +
				"            //  ^ variable\n" +
				"        }\n" +
				"\n" +
				"        var x = from a in sourceA\n" +
				"        //           ^ variable\n" +
				"        //                ^ variable\n" +
				"                join b in sourceB on a.FK equals b.PK\n" +
				"        //           ^ variable\n" +
				"        //                ^ variable\n" +
				"                group a by a.X into g\n" +
				"        //            ^ variable\n" +
				"        //                          ^ variable\n" +
				"                orderby g ascending\n" +
				"        //              ^ variable\n" +
				"                select new { A.A, B.B };\n" +
				"    }\n" +
				"}\n",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assertGeneratedAndReferenceDeepParity(t, genLang, refLang, tc.src)
		})
	}
}

func TestCSharpAttributedTopLevelDeclarationsParity(t *testing.T) {
	genLang := loadGeneratedCSharpLanguageForParity(t)
	refLang := grammars.CSharpLanguage()
	adaptExternalScanner(refLang, genLang)

	cases := []struct {
		name string
		src  string
	}{
		{
			name: "top_level_classes",
			src: "[A(B.C)]\n" +
				"class D {}\n" +
				"\n" +
				"[NS.A(B.C)]\n" +
				"class E {}\n" +
				"\n" +
				"[One][Two]\n" +
				"[Three]\n" +
				"class F { }\n",
		},
		{
			name: "struct_and_field",
			src: "[A,B()][C]\n" +
				"struct A { }\n" +
				"\n" +
				"class Zzz {\n" +
				"  [A,B()][C]\n" +
				"  public int Z;\n" +
				"}\n",
		},
		{
			name: "method_targets",
			src: "class Methods {\n" +
				"  [ValidatedContract]\n" +
				"  int Method1() { return 0; }\n" +
				"\n" +
				"  [method: ValidatedContract]\n" +
				"  int Method2() { return 0; }\n" +
				"\n" +
				"  [return: ValidatedContract]\n" +
				"  int Method3() { return 0; }\n" +
				"}\n",
		},
		{
			name: "enum_and_event",
			src: "[Single]\n" +
				"enum A { B, C }\n" +
				"\n" +
				"class Zzz {\n" +
				"  [A,B()][C]\n" +
				"  public event EventHandler SomeEvent { add { } remove { } }\n" +
				"}\n",
		},
		{
			name: "generic_type_parameter_attributes",
			src: "class Class<[A, B][C()]T1> {\n" +
				"  void Method<[E] [F, G(1)] T2>() {\n" +
				"  }\n" +
				"}\n",
		},
		{
			name: "accessor_attributes",
			src: "class Zzz {\n" +
				"  public event EventHandler SomeEvent {\n" +
				"    [A,B()][C] add { }\n" +
				"    [A,B()][C] remove { }\n" +
				"  }\n" +
				"}\n",
		},
		{
			name: "named_attribute_arguments",
			src: "[RegularExpression(pattern: \"/.+\", ErrorMessage = \"The Callback Path Must start with a forward slash '/' followed by one or more characters\")]\n" +
				"class Validator { }\n" +
				"\n" +
				"[Route(Name: \"default\", Template = \"/api/{id}\")]\n" +
				"[Obsolete(message: \"Use NewMethod instead\", error: true)]\n" +
				"class Example { }\n",
		},
		{
			name: "non_global_attribute_targets",
			src: "[type: Obsolete]\n" +
				"class A<[typevar: B] TC>\n" +
				"{\n" +
				"  [field:JsonIgnore]\n" +
				"  [property: JsonIgnore]\n" +
				"  public int D { get; set; }\n" +
				"\n" +
				"  [method: Obsolete]\n" +
				"  [return: MaybeNull]\n" +
				"  public void E([param: AllowNull] int f) { }\n" +
				"\n" +
				"  [event: Obsolete]\n" +
				"  public event EventHandler OnG;\n" +
				"}\n",
		},
		{
			name: "combined_attribute_corpus",
			src: "[A(B.C)]\n" +
				"class D {}\n\n" +
				"[NS.A(B.C)]\n" +
				"class D {}\n\n" +
				"[One][Two]\n" +
				"[Three]\n" +
				"class A { }\n\n" +
				"[A,B()][C]\n" +
				"struct A { }\n\n" +
				"class Zzz {\n" +
				"  [A,B()][C]\n" +
				"  public int Z;\n" +
				"}\n\n" +
				"class Methods {\n" +
				"  [ValidatedContract]\n" +
				"  int Method1() { return 0; }\n\n" +
				"  [method: ValidatedContract]\n" +
				"  int Method2() { return 0; }\n\n" +
				"  [return: ValidatedContract]\n" +
				"  int Method3() { return 0; }\n" +
				"}\n\n" +
				"[Single]\n" +
				"enum A { B, C }\n\n" +
				"class Zzz {\n" +
				"  [A,B()][C]\n" +
				"  public event EventHandler SomeEvent { add { } remove { } }\n" +
				"}\n\n" +
				"class Class<[A, B][C()]T1> {\n" +
				"  void Method<[E] [F, G(1)] T2>() {\n" +
				"  }\n" +
				"}\n\n" +
				"class Zzz {\n" +
				"  public event EventHandler SomeEvent {\n" +
				"    [A,B()][C] add { }\n" +
				"    [A,B()][C] remove { }\n" +
				"  }\n" +
				"}\n",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assertGeneratedAndReferenceDeepParity(t, genLang, refLang, tc.src)
		})
	}
}

func TestCSharpQuerySyntaxClauseParity(t *testing.T) {
	genLang := loadGeneratedCSharpLanguageForParity(t)
	refLang := grammars.CSharpLanguage()
	adaptExternalScanner(refLang, genLang)

	cases := []struct {
		name string
		src  string
	}{
		{
			name: "select_conditional",
			src:  "var x = from a in source select a.B() ? c : c * 2;\n",
		},
		{
			name: "select_assignment",
			src:  "var x = from a in source select somevar = a;\n",
		},
		{
			name: "select_anonymous_object",
			src:  "var x = from a in source select new { Name = a.B };\n",
		},
		{
			name: "where_clause",
			src: "var x = from a in source\n" +
				"  where a.B == \"A\"\n" +
				"  select new { Name = a.B };\n",
		},
		{
			name: "order_by_clause",
			src: "var x = from a in source\n" +
				"  orderby a.A descending\n" +
				"  orderby a.C ascending\n" +
				"  orderby 1\n" +
				"  select a;\n",
		},
		{
			name: "let_clause",
			src: "var x = from a in source\n" +
				"  let z = new { a.A, a.B }\n" +
				"  select z;\n",
		},
		{
			name: "nested_from_clause",
			src: "var x = from a in sourceA\n" +
				"  from b in sourceB\n" +
				"  where a.FK == b.FK\n" +
				"  select new { A.A, B.B };\n",
		},
		{
			name: "group_into_clause",
			src: "var x = from a in sourceA\n" +
				"  group a by a.Country into g\n" +
				"  select new { Country = g.Key, Population = g.Sum(p => p.Population) };\n",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assertGeneratedAndReferenceDeepParity(t, genLang, refLang, tc.src)
		})
	}
}

func TestCSharpConstrainedTypeDeclarationParity(t *testing.T) {
	genLang := loadGeneratedCSharpLanguageForParity(t)
	refLang := grammars.CSharpLanguage()
	adaptExternalScanner(refLang, genLang)

	cases := []struct {
		name string
		src  string
	}{
		{
			name: "class_constraint",
			src:  "public class F<T> where T:struct {}\n",
		},
		{
			name: "struct_constraint",
			src:  "public struct F<T> where T:struct {}\n",
		},
		{
			name: "record_constraints",
			src:  "private record F<T1, T2> where T1 : I1, I2, new() where T2 : I2 { }\n",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assertGeneratedAndReferenceDeepParity(t, genLang, refLang, tc.src)
		})
	}
}

func TestCSharpSourceFileStructureParity(t *testing.T) {
	genLang := loadGeneratedCSharpLanguageForParity(t)
	refLang := grammars.CSharpLanguage()
	adaptExternalScanner(refLang, genLang)

	cases := []struct {
		name string
		src  string
	}{
		{
			name: "namespace_with_using",
			src: "namespace Foo {\n" +
				"  using A;\n" +
				"}\n",
		},
		{
			name: "extern_alias_then_namespace",
			src: "extern alias A;\n" +
				"namespace Foo {\n" +
				"  using A;\n" +
				"}\n",
		},
		{
			name: "highlight_types",
			src: `class A : B, C
//    ^ type
//        ^ type
//           ^ type
{
    public void M()
    {
        int a;
        // <- type.builtin
        var a;
        // <- keyword

        int? a;
        // <- type.builtin
        // ^ operator
        A? a;
        // <- type
         // <- operator

        int* a;
        // <- type.builtin
        // ^ operator
        A* a;
        // <- type
         // <- operator

        ref A* a;
        // <- keyword
        //  ^ type
        //   ^ operator

        var a = x is int;
        //           ^ type.builtin
        var a = x is A;
        //           ^

        var a = x as int;
        //           ^ type.builtin
        var a = x as A;
        //           ^ type

        var a = (int)x;
        //       ^ type.builtin
        var a = (A)x;
        //       ^ type

        A<int, A> a = new A<int, A>();
        // <- type
        //^ type.builtin
        //     ^ type
        //                ^ type
        //                  ^ type.builtin
        //                       ^ type
    }
}

record A(int a, B b) : B(), I;
//     ^ type
//       ^ type.builtin
//              ^ type
//                     ^ type
//                          ^ type

record A : B, I;
//     ^ type
//         ^ type
//            ^ type
`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assertGeneratedAndReferenceDeepParity(t, genLang, refLang, tc.src)
		})
	}
}

func TestCSharpTopLevelChunkParity(t *testing.T) {
	genLang := loadGeneratedCSharpLanguageForParity(t)
	refLang := grammars.CSharpLanguage()
	adaptExternalScanner(refLang, genLang)

	cases := []struct {
		name string
		src  string
	}{
		{
			name: "extern_usings_namespace",
			src: "extern alias A;\n" +
				"// alias comment\n" +
				"using System;\n" +
				"// using comment\n" +
				"using static System.Console;\n" +
				"namespace Foo {\n" +
				"  using A;\n" +
				"}\n",
		},
		{
			name: "multiple_top_level_classes",
			src: "public class F {}\n" +
				"public class G<T> where T:struct {}\n" +
				"file class A {}\n" +
				"public class NoBody;\n",
		},
		{
			name: "globals_then_class",
			src: "(string a, bool b) c = default;\n" +
				"A<B> a = null;\n" +
				"class A {\n" +
				"  int Sample() {\n" +
				"    return 1;\n" +
				"  }\n" +
				"}\n",
		},
		{
			name: "global_lambda_chain",
			src:  "var result = list.Select(c => (c.f1, c.f2)).Where(t => t.f2 == 1);\n",
		},
		{
			name: "top_level_local_function_switch_patterns",
			src: "int Sample9(int a) {\n" +
				"  switch (a, a) {\n" +
				"    case (1, 1):\n" +
				"      return 1;\n" +
				"    default:\n" +
				"      return 0;\n" +
				"  }\n" +
				"\n" +
				"  switch (A, B) {\n" +
				"      case (_, _) when !c:\n" +
				"        break;\n" +
				"  }\n" +
				"\n" +
				"  switch (A) {\n" +
				"      case {Length: 2} when !c:\n" +
				"        break;\n" +
				"  }\n" +
				"\n" +
				"  int i = 123;\n" +
				"  switch (i)\n" +
				"  {\n" +
				"      case int when i < 5:\n" +
				"          break;\n" +
				"  }\n" +
				"}\n",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assertGeneratedAndReferenceDeepParity(t, genLang, refLang, tc.src)
		})
	}
}

func TestCSharpAttributeDeclarationParity(t *testing.T) {
	genLang := loadGeneratedCSharpLanguageForParity(t)
	refLang := grammars.CSharpLanguage()
	adaptExternalScanner(refLang, genLang)

	cases := []struct {
		name string
		src  string
	}{
		{
			name: "attribute_member_access_argument",
			src:  "[A(B.C)] class D {}\n",
		},
		{
			name: "qualified_attribute_member_access_argument",
			src:  "[NS.A(B.C)] class D {}\n",
		},
		{
			name: "stacked_attribute_lists",
			src: "[One][Two]\n" +
				"[Three]\n" +
				"class A { }\n",
		},
		{
			name: "multi_attribute_struct",
			src:  "[A,B()][C] struct A { }\n",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assertGeneratedAndReferenceDeepParity(t, genLang, refLang, tc.src)
		})
	}
}

func TestCSharpTypeDeclarationBodyParity(t *testing.T) {
	genLang := loadGeneratedCSharpLanguageForParity(t)
	refLang := grammars.CSharpLanguage()
	adaptExternalScanner(refLang, genLang)

	cases := []struct {
		name string
		src  string
	}{
		{
			name: "using_declaration_method",
			src: "class A {\n" +
				"  void Sample() {\n" +
				"    using var a = new A();\n" +
				"  }\n" +
				"}\n",
		},
		{
			name: "local_function_tuple_method",
			src: "class A {\n" +
				"  void Sample() {\n" +
				"    (bool a, bool b) M2() {\n" +
				"      return (true, false);\n" +
				"    }\n" +
				"  }\n" +
				"}\n",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assertGeneratedAndReferenceDeepParity(t, genLang, refLang, tc.src)
		})
	}
}

func TestGeneratedCSharpTypeDeclarationBodyRecovery(t *testing.T) {
	genLang := loadGeneratedCSharpLanguageForParity(t)
	refLang := grammars.CSharpLanguage()
	adaptExternalScanner(refLang, genLang)
	parser := gotreesitter.NewParser(genLang)

	cases := []struct {
		name     string
		src      string
		wantStmt string
	}{
		{
			name: "initializers_prefix_method",
			src: "class A {\n" +
				"  void Sample() {\n" +
				"    int a;\n" +
				"    int a = 1, b = 2;\n" +
				"    const int a = 1;\n" +
				"    const int a = 1, b = 2;\n" +
				"    ref var value = ref data[i];\n" +
				"    var g = args[0].Length;\n" +
				"  }\n" +
				"}\n",
			wantStmt: "local_declaration_statement",
		},
		{
			name: "using_prefix_method",
			src: "class A {\n" +
				"  void Sample() {\n" +
				"    using (var a = b) {\n" +
				"      return;\n" +
				"    }\n" +
				"\n" +
				"    using (Stream a = File.OpenRead(\"a\"), b = new BinaryReader(a)) {\n" +
				"      return;\n" +
				"    }\n" +
				"  }\n" +
				"}\n",
			wantStmt: "using_statement",
		},
		{
			name: "variable_declarations_prefix_method",
			src: "class A\n" +
				"{\n" +
				"    public void M()\n" +
				"    {\n" +
				"        foreach (int i in new[] { 1 })\n" +
				"        {\n" +
				"            int j = i;\n" +
				"        }\n" +
				"\n" +
				"        var x = from a in sourceA\n" +
				"                join b in sourceB on a.FK equals b.PK\n" +
				"                group a by a.X into g\n" +
				"                orderby g ascending\n" +
				"                select new { A.A, B.B };\n" +
				"    }\n" +
				"}\n",
			wantStmt: "foreach_statement",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tree, err := parser.Parse([]byte(tc.src))
			if err != nil {
				t.Fatalf("parse failed: %v", err)
			}
			root := tree.RootNode()
			if root == nil || root.HasError() {
				t.Fatalf("expected no-error tree, got %s", root.SExpr(genLang))
			}
			if got := findFirstNamedDescendantOfType(root, genLang, "class_declaration"); got == nil {
				t.Fatalf("missing class_declaration: %s", root.SExpr(genLang))
			}
			method := findFirstNamedDescendantOfType(root, genLang, "method_declaration")
			if method == nil {
				t.Fatalf("missing method_declaration: %s", root.SExpr(genLang))
			}
			if got := findFirstNamedDescendantOfType(method, genLang, tc.wantStmt); got == nil {
				t.Fatalf("missing %s in recovered method: %s", tc.wantStmt, method.SExpr(genLang))
			}
		})
	}
}

func TestCSharpUnicodeIdentifierParity(t *testing.T) {
	genLang := loadGeneratedCSharpLanguageForParity(t)
	refLang := grammars.CSharpLanguage()
	adaptExternalScanner(refLang, genLang)

	sample := "int ග්\u200dරහලෝකය = 0;\n"
	assertGeneratedAndReferenceDeepParity(t, genLang, refLang, sample)
}

func loadGeneratedCSharpLanguageForParity(t *testing.T) *gotreesitter.Language {
	t.Helper()

	candidates := []string{
		"/tmp/grammar_parity/c_sharp/src/grammar.json",
		".claude/worktrees/grammargen-pr9-resume/harness_out/grammar_seeds/c_sharp/src/grammar.json",
		"../.claude/worktrees/grammargen-pr9-resume/harness_out/grammar_seeds/c_sharp/src/grammar.json",
	}

	var grammarPath string
	for _, path := range candidates {
		if _, err := os.Stat(path); err == nil {
			grammarPath = path
			break
		}
	}
	if grammarPath == "" {
		t.Skip("C# grammar.json not available")
	}

	source, err := os.ReadFile(grammarPath)
	if err != nil {
		t.Fatalf("read C# grammar.json: %v", err)
	}
	gram, err := ImportGrammarJSON(source)
	if err != nil {
		t.Fatalf("import C# grammar.json: %v", err)
	}
	genLang, err := generateWithTimeout(gram, 90*time.Second)
	if err != nil {
		t.Fatalf("generate C# language: %v", err)
	}
	return genLang
}

func findFirstNamedDescendantOfType(node *gotreesitter.Node, lang *gotreesitter.Language, typ string) *gotreesitter.Node {
	if node == nil {
		return nil
	}
	if node.IsNamed() && node.Type(lang) == typ {
		return node
	}
	for i := 0; i < node.NamedChildCount(); i++ {
		if got := findFirstNamedDescendantOfType(node.NamedChild(i), lang, typ); got != nil {
			return got
		}
	}
	return nil
}
