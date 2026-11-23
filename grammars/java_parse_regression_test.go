//go:build !grammar_subset || grammar_subset_java

package grammars

import (
	"strings"
	"testing"

	"github.com/odvcencio/gotreesitter"
)

func findFirstNamedDescendant(node *gotreesitter.Node, lang *gotreesitter.Language, typ string) *gotreesitter.Node {
	if node == nil {
		return nil
	}
	if node.IsNamed() && node.Type(lang) == typ {
		return node
	}
	for i := 0; i < node.NamedChildCount(); i++ {
		if found := findFirstNamedDescendant(node.NamedChild(i), lang, typ); found != nil {
			return found
		}
	}
	return nil
}

func assertMainStringArrayShape(t *testing.T, tree *gotreesitter.Tree, lang *gotreesitter.Language, src []byte) {
	t.Helper()

	if tree == nil || tree.RootNode() == nil {
		t.Fatal("parse returned nil root")
	}

	root := tree.RootNode()
	if root.HasError() {
		t.Fatalf("expected parse without syntax errors, got sexpr: %s", root.SExpr(lang))
	}
	if root.NamedChildCount() != 2 {
		t.Fatalf("expected root to have 2 named children, got %d: %s", root.NamedChildCount(), root.SExpr(lang))
	}
	if got := root.NamedChild(0).Type(lang); got != "package_declaration" {
		t.Fatalf("root child[0] = %q, want package_declaration", got)
	}
	if got := root.NamedChild(1).Type(lang); got != "class_declaration" {
		t.Fatalf("root child[1] = %q, want class_declaration", got)
	}

	methodDecl := findFirstNamedDescendant(root, lang, "method_declaration")
	if methodDecl == nil {
		t.Fatalf("no method_declaration in parse tree: %s", root.SExpr(lang))
	}
	nameNode := methodDecl.ChildByFieldName("name", lang)
	if nameNode == nil || nameNode.Text(src) != "main" {
		got := "<nil>"
		if nameNode != nil {
			got = nameNode.Text(src)
		}
		t.Fatalf("method name = %q, want %q", got, "main")
	}

	params := findFirstNamedDescendant(methodDecl, lang, "formal_parameters")
	if params == nil {
		t.Fatalf("method_declaration missing formal_parameters: %s", methodDecl.SExpr(lang))
	}
	paramText := strings.Join(strings.Fields(params.Text(src)), "")
	if !strings.Contains(paramText, "String[]args") {
		t.Fatalf("formal_parameters = %q, want to contain String[]args", params.Text(src))
	}

	invocation := findFirstNamedDescendant(methodDecl, lang, "method_invocation")
	if invocation == nil {
		t.Fatalf("method_declaration missing method_invocation: %s", methodDecl.SExpr(lang))
	}
	if !strings.Contains(invocation.Text(src), "System.out.println") {
		t.Fatalf("method_invocation text = %q, want to contain System.out.println", invocation.Text(src))
	}
}

func assertJavaSourceParsesWithoutErrors(t *testing.T, src []byte, tokenSource bool) {
	t.Helper()

	lang := JavaLanguage()
	parser := gotreesitter.NewParser(lang)

	var (
		tree *gotreesitter.Tree
		err  error
	)
	if tokenSource {
		tree, err = parser.ParseWithTokenSourceFactory(src, func(source []byte) (gotreesitter.TokenSource, error) {
			return NewJavaTokenSource(source, lang)
		})
	} else {
		tree, err = parser.Parse(src)
	}
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if tree == nil || tree.RootNode() == nil {
		t.Fatal("parse returned nil root")
	}
	if root := tree.RootNode(); root.HasError() {
		t.Fatalf("expected Java source to parse without syntax errors, got: %s", root.SExpr(lang))
	}
}

func TestJavaParseMainStringArrayRegression(t *testing.T) {
	lang := JavaLanguage()
	parser := gotreesitter.NewParser(lang)

	src := []byte(`package com.example;

public class App {
    public static void main(String[] args) {
        System.out.println("hello");
    }
}
`)

	tree, err := parser.Parse(src)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	assertMainStringArrayShape(t, tree, lang, src)
}

func TestJavaParseWithTokenSourceMainStringArrayRegression(t *testing.T) {
	lang := JavaLanguage()
	parser := gotreesitter.NewParser(lang)

	src := []byte(`package com.example;

public class App {
    public static void main(String[] args) {
        System.out.println("hello");
    }
}
`)

	ts, err := NewJavaTokenSource(src, lang)
	if err != nil {
		t.Fatalf("NewJavaTokenSource failed: %v", err)
	}
	tree, err := parser.ParseWithTokenSource(src, ts)
	if err != nil {
		t.Fatalf("parse with token source failed: %v", err)
	}
	assertMainStringArrayShape(t, tree, lang, src)
}

func TestJavaParseWithTokenSourceContextualPermitsIdentifierRegression(t *testing.T) {
	lang := JavaLanguage()
	parser := gotreesitter.NewParser(lang)

	src := []byte(`class T {
  void f() {
    int permits = 1;
    permits++;
  }
}
`)

	tree, err := parser.ParseWithTokenSourceFactory(src, func(source []byte) (gotreesitter.TokenSource, error) {
		return NewJavaTokenSource(source, lang)
	})
	if err != nil {
		t.Fatalf("parse with token source failed: %v", err)
	}
	if tree == nil || tree.RootNode() == nil {
		t.Fatal("parse returned nil root")
	}
	if root := tree.RootNode(); root.HasError() {
		t.Fatalf("expected contextual permits identifier to parse without syntax errors, got: %s", root.SExpr(lang))
	}
}

func TestJavaParseWithTokenSourceSealedPermitsClauseRegression(t *testing.T) {
	lang := JavaLanguage()
	parser := gotreesitter.NewParser(lang)

	src := []byte(`sealed class A permits B {
}

final class B extends A {
}
`)

	tree, err := parser.ParseWithTokenSourceFactory(src, func(source []byte) (gotreesitter.TokenSource, error) {
		return NewJavaTokenSource(source, lang)
	})
	if err != nil {
		t.Fatalf("parse with token source failed: %v", err)
	}
	if tree == nil || tree.RootNode() == nil {
		t.Fatal("parse returned nil root")
	}
	if root := tree.RootNode(); root.HasError() {
		t.Fatalf("expected sealed permits clause to parse without syntax errors, got: %s", root.SExpr(lang))
	}
}

func TestJavaEightArgumentCallDoesNotShiftAcrossUnconsumedArgument(t *testing.T) {
	lang := JavaLanguage()
	parser := gotreesitter.NewParser(lang)

	src := []byte(`class T {
  void f() {
    target.call(arg1, arg2, arg3, arg4, arg5, arg6, arg7, arg8);
  }
}
`)

	tree, err := parser.Parse(src)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	root := tree.RootNode()
	if root == nil {
		t.Fatal("parse returned nil root")
	}
	if root.HasError() {
		t.Fatalf("expected Java source to parse without syntax errors, got: %s", root.SExpr(lang))
	}

	args := findFirstNamedDescendant(root, lang, "argument_list")
	if args == nil {
		t.Fatalf("missing argument_list: %s", root.SExpr(lang))
	}
	compactArgs := strings.Join(strings.Fields(args.Text(src)), "")
	if got, want := compactArgs, "(arg1,arg2,arg3,arg4,arg5,arg6,arg7,arg8)"; got != want {
		t.Fatalf("argument_list text = %q, want %q; tree=%s", got, want, root.SExpr(lang))
	}
}

func TestJavaTopLevelAnnotationsKeepDeclarationBranchRegression(t *testing.T) {
	cases := []struct {
		name      string
		src       []byte
		childType string
	}{
		{
			name: "annotation_type_declaration",
			src: []byte(`@Retention(RetentionPolicy.RUNTIME)
@Target(ElementType.TYPE)
public @interface Intercept {
  public Class<? extends JsonPostDeserializer> postDeserialize();
}
`),
			childType: "annotation_type_declaration",
		},
		{
			name: "repeatable_annotation_type_declaration",
			src: []byte(`@Target({ ElementType.TYPE, ElementType.METHOD, ElementType.FIELD, ElementType.PARAMETER })
@Retention(RetentionPolicy.RUNTIME)
@Documented
@Inherited
@Repeatable(Extensions.class)
@API(status = STABLE, since = "5.0")
public @interface ExtendWith {
  Class<? extends Extension>[] value();
}
`),
			childType: "annotation_type_declaration",
		},
		{
			name: "class_declaration",
			src: []byte(`@ClassTemplate
@ExtendWith(ClassTemplateDemo.MyClassTemplateInvocationContextProvider.class)
class ClassTemplateDemo {
}
`),
			childType: "class_declaration",
		},
	}

	for _, tc := range cases {
		for _, tokenSource := range []bool{false, true} {
			name := tc.name + "/dfa"
			if tokenSource {
				name = tc.name + "/token_source"
			}
			t.Run(name, func(t *testing.T) {
				assertJavaSourceParsesWithoutErrors(t, tc.src, tokenSource)

				lang := JavaLanguage()
				parser := gotreesitter.NewParser(lang)
				var (
					tree *gotreesitter.Tree
					err  error
				)
				if tokenSource {
					tree, err = parser.ParseWithTokenSourceFactory(tc.src, func(source []byte) (gotreesitter.TokenSource, error) {
						return NewJavaTokenSource(source, lang)
					})
				} else {
					tree, err = parser.Parse(tc.src)
				}
				if err != nil {
					t.Fatalf("parse failed: %v", err)
				}
				root := tree.RootNode()
				if got := root.NamedChild(0).Type(lang); got != tc.childType {
					t.Fatalf("root named child = %q, want %q; sexpr: %s", got, tc.childType, root.SExpr(lang))
				}
			})
		}
	}
}

func TestJavaTopLevelAnnotationDeclarationsAfterClassRegression(t *testing.T) {
	src := []byte(`class Host {
}

@Target(ElementType.PARAMETER)
@Retention(RetentionPolicy.RUNTIME)
@ExtendWith(MagicParameter.Extension.class)
@interface MagicParameter {
  String value();

  @NullMarked
  class Extension implements ParameterResolver {
    Object resolveParameter(ParameterContext parameterContext) {
      return parameterContext.findAnnotation(MagicParameter.class)
          .map(MagicParameter::value)
          .orElse("enigma");
    }
  }
}

@SuppressWarnings("unused")
@NullMarked
class BaseParameterExtension<T extends Annotation> implements ParameterResolver {
}

@Target(ElementType.PARAMETER)
@Retention(RetentionPolicy.RUNTIME)
@ExtendWith(ConstructorParameter.Extension.class)
@interface ConstructorParameter {
  class Extension extends BaseParameterExtension<ConstructorParameter> {
  }
}

@Target(ElementType.PARAMETER)
@Retention(RetentionPolicy.RUNTIME)
@ExtendWith(AfterAllParameter.Extension.class)
@interface AfterAllParameter {
  class Extension extends BaseParameterExtension<AfterAllParameter> {
  }
}
`)

	for _, tokenSource := range []bool{false, true} {
		name := "dfa"
		if tokenSource {
			name = "token_source"
		}
		t.Run(name, func(t *testing.T) {
			assertJavaSourceParsesWithoutErrors(t, src, tokenSource)

			lang := JavaLanguage()
			parser := gotreesitter.NewParser(lang)
			var (
				tree *gotreesitter.Tree
				err  error
			)
			if tokenSource {
				tree, err = parser.ParseWithTokenSourceFactory(src, func(source []byte) (gotreesitter.TokenSource, error) {
					return NewJavaTokenSource(source, lang)
				})
			} else {
				tree, err = parser.Parse(src)
			}
			if err != nil {
				t.Fatalf("parse failed: %v", err)
			}
			root := tree.RootNode()
			if got := root.NamedChildCount(); got < 4 {
				t.Fatalf("root named child count = %d, want at least 4; sexpr: %s", got, root.SExpr(lang))
			}
			if got := root.NamedChild(1).Type(lang); got != "annotation_type_declaration" {
				t.Fatalf("root named child[1] = %q, want annotation_type_declaration; sexpr: %s", got, root.SExpr(lang))
			}
		})
	}
}

func TestJavaParseWithTokenSourceCompactNestedGenericRegression(t *testing.T) {
	lang := JavaLanguage()
	parser := gotreesitter.NewParser(lang)

	src := []byte(`class T {
  Queue<IOConsumer<IndexWriter>> queue = new ConcurrentLinkedQueue<>();
}
`)

	tree, err := parser.ParseWithTokenSourceFactory(src, func(source []byte) (gotreesitter.TokenSource, error) {
		return NewJavaTokenSource(source, lang)
	})
	if err != nil {
		t.Fatalf("parse with token source failed: %v", err)
	}
	if tree == nil || tree.RootNode() == nil {
		t.Fatal("parse returned nil root")
	}
	if root := tree.RootNode(); root.HasError() {
		t.Fatalf("expected compact nested generic to parse without syntax errors, got: %s", root.SExpr(lang))
	}
}

func TestJavaArrayInitializerTrailingCommaRegression(t *testing.T) {
	lang := JavaLanguage()
	parser := gotreesitter.NewParser(lang)

	src := []byte(`class T {
  int[] values = {
    1,
    2, // trailing comma remains optional
  };
}
`)

	tree, err := parser.Parse(src)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if tree == nil || tree.RootNode() == nil {
		t.Fatal("parse returned nil root")
	}
	if root := tree.RootNode(); root.HasError() {
		t.Fatalf("expected trailing comma array initializer to parse without syntax errors, got: %s", root.SExpr(lang))
	}
}

func TestJavaParseSwitchRuleThenConditionalExpressionRegression(t *testing.T) {
	lang := JavaLanguage()
	parser := gotreesitter.NewParser(lang)

	src := []byte(`class T {
  private static void processLongColumn(LongColumn column, PerField pf, IndexableFieldType fieldType)
      throws IOException {
    final boolean hasPoints = fieldType.pointDimensionCount() != 0;
    if (hasPoints == false) {
      LongTupleCursor cursor = column.tuples();
      switch (fieldType.docValuesType()) {
        case NUMERIC -> {
          NumericDocValuesWriter writer = (NumericDocValuesWriter) pf.docValuesWriter;
          int batchDocID;
          while ((batchDocID = cursor.nextDoc()) != DocIdSetIterator.NO_MORE_DOCS) {
            writer.addValue(batchDocID, cursor.longValue());
          }
        }
        default ->
            throw new IllegalArgumentException(
                "LongColumn \"" + column.name() + "\" has incompatible docValuesType");
      }
      return;
    }
    final int byteWidth =
        (column.numericKind() == LongColumn.NumericKind.INT || column.numericKind() == LongColumn.NumericKind.FLOAT)
            ? Integer.BYTES
            : Long.BYTES;
  }
}
`)

	tree, err := parser.Parse(src)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if tree == nil || tree.RootNode() == nil {
		t.Fatal("parse returned nil root")
	}
	if root := tree.RootNode(); root.HasError() {
		t.Fatalf("expected switch rule followed by conditional expression to parse without syntax errors, got: %s", root.SExpr(lang))
	}
}

func TestJavaParseSwitchRuleThenPrivateVoidMethodRegression(t *testing.T) {
	lang := JavaLanguage()
	parser := gotreesitter.NewParser(lang)

	src := []byte(`class T {
  private void storeVectorValues(Info info, IndexableField vectorField) {
    assert vectorField instanceof KnnFloatVectorField || vectorField instanceof KnnByteVectorField;
    switch (info.fieldInfo.getVectorEncoding()) {
      case BYTE -> {
        if (vectorField instanceof KnnByteVectorField byteVectorField) {
          if (info.byteVectorCount == 1) {
            throw new IllegalArgumentException(
                "Only one value per field allowed for byte vector field ["
                    + vectorField.name()
                    + "]");
          }
          info.byteVectorCount++;
          if (info.byteVectorValues == null) {
            info.byteVectorValues = new byte[1][];
          }
          info.byteVectorValues[0] =
              ArrayUtil.copyOfSubArray(
                  byteVectorField.vectorValue(), 0, info.fieldInfo.getVectorDimension());
          return;
        }
        throw new IllegalArgumentException(
            "Field ["
                + vectorField.name()
                + "] is not a byte vector field, but the field info is configured for byte vectors");
      }
      case FLOAT32 -> {
        if (vectorField instanceof KnnFloatVectorField floatVectorField) {
          if (info.floatVectorCount == 1) {
            throw new IllegalArgumentException(
                "Only one value per field allowed for float vector field ["
                    + vectorField.name()
                    + "]");
          }
          info.floatVectorCount++;
          if (info.floatVectorValues == null) {
            info.floatVectorValues = new float[1][];
          }
          info.floatVectorValues[0] =
              ArrayUtil.copyOfSubArray(
                  floatVectorField.vectorValue(), 0, info.fieldInfo.getVectorDimension());
          return;
        }
        throw new IllegalArgumentException(
            "Field ["
                + vectorField.name()
                + "] is not a float vector field, but the field info is configured for float vectors");
      }
    }
  }

  private void storeValues(Info info, IndexableField field) {
    if (info.storedValues == null) {
      info.storedValues = new ArrayList<>();
    }
    BytesRef binaryValue = field.binaryValue();
    if (binaryValue != null) {
      info.storedValues.add(binaryValue);
      return;
    }
  }
}
`)

	tree, err := parser.Parse(src)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if tree == nil || tree.RootNode() == nil {
		t.Fatal("parse returned nil root")
	}
	if root := tree.RootNode(); root.HasError() {
		t.Fatalf("expected switch rule followed by private void method to parse without syntax errors, got: %s", root.SExpr(lang))
	}
}

func TestJavaParseSwitchRuleThenClassLiteralArgumentRegression(t *testing.T) {
	lang := JavaLanguage()
	parser := gotreesitter.NewParser(lang)

	src := []byte(`class T {
  protected void addRandomFields(Document doc) {
    switch (vectorEncoding) {
      case BYTE -> doc.add(new KnnByteVectorField("v2", randomVector8(30), similarityFunction));
      case FLOAT32 ->
          doc.add(new KnnFloatVectorField("v2", randomNormalizedVector(30), similarityFunction));
    }
  }

  protected boolean mergeIsStable() {
    return false;
  }

  private int getVectorsMaxDimensions(String fieldName) {
    return Codec.getDefault().knnVectorsFormat().getMaxDimensions(fieldName);
  }

  public void testFieldConstructor() {
    float[] v = new float[1];
    KnnFloatVectorField field = new KnnFloatVectorField("f", v);
    assertEquals(1, field.fieldType().vectorDimension());
    assertEquals(VectorSimilarityFunction.EUCLIDEAN, field.fieldType().vectorSimilarityFunction());
    assertSame(v, field.vectorValue());
  }

  public void testFieldConstructorExceptions() {
    expectThrows(IllegalArgumentException.class, () -> new KnnFloatVectorField(null, new float[1]));
    expectThrows(IllegalArgumentException.class, () -> new KnnFloatVectorField("f", null));
    expectThrows(
        IllegalArgumentException.class,
        () -> new KnnFloatVectorField("f", new float[1], (VectorSimilarityFunction) null));
    expectThrows(IllegalArgumentException.class, () -> new KnnFloatVectorField("f", new float[0]));
  }
}
`)

	tree, err := parser.Parse(src)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if tree == nil || tree.RootNode() == nil {
		t.Fatal("parse returned nil root")
	}
	if root := tree.RootNode(); root.HasError() {
		t.Fatalf("expected switch rule followed by class literal invocation argument to parse without syntax errors, got: %s", root.SExpr(lang))
	}
}

func TestJavaSwitchRuleLabelBeatsLambdaShiftWhenPostReduceCanConsumeArrow(t *testing.T) {
	src := []byte(`class T {
  enum K {
    A,
    B
  }

  int f(K k) {
    return switch (k) {
      case A -> 1;
      case B -> 2;
    };
  }
}
`)

	assertJavaSourceParsesWithoutErrors(t, src, false)
}

func TestJavaLambdaArrowStillParsesAfterSwitchDisambiguator(t *testing.T) {
	src := []byte(`import java.util.function.Function;

class T {
  Function<Integer, Integer> f() {
    return x -> x + 1;
  }
}
`)

	assertJavaSourceParsesWithoutErrors(t, src, false)
}

func TestJavaParseWithTokenSourceShiftExpressionRegression(t *testing.T) {
	lang := JavaLanguage()
	parser := gotreesitter.NewParser(lang)

	src := []byte(`class T {
  void f() {
    int shifted = value >> 1;
  }
}
`)

	tree, err := parser.ParseWithTokenSourceFactory(src, func(source []byte) (gotreesitter.TokenSource, error) {
		return NewJavaTokenSource(source, lang)
	})
	if err != nil {
		t.Fatalf("parse with token source failed: %v", err)
	}
	if tree == nil || tree.RootNode() == nil {
		t.Fatal("parse returned nil root")
	}
	if root := tree.RootNode(); root.HasError() {
		t.Fatalf("expected shift expression to parse without syntax errors, got: %s", root.SExpr(lang))
	}
}

func TestJavaParseShiftExpressionWithParenthesizedRightOperandRegression(t *testing.T) {
	lang := JavaLanguage()
	src := []byte(`class T {
  void f(int bits, int numMantissaBits) {
    int smallfloat = bits >> (24 - numMantissaBits);
  }
}
`)

	t.Run("dfa", func(t *testing.T) {
		parser := gotreesitter.NewParser(lang)
		tree, err := parser.Parse(src)
		if err != nil {
			t.Fatalf("parse failed: %v", err)
		}
		if tree == nil || tree.RootNode() == nil {
			t.Fatal("parse returned nil root")
		}
		if root := tree.RootNode(); root.HasError() {
			t.Fatalf("expected parenthesized shift expression to parse without syntax errors, got: %s", root.SExpr(lang))
		}
	})

	t.Run("token_source", func(t *testing.T) {
		parser := gotreesitter.NewParser(lang)
		tree, err := parser.ParseWithTokenSourceFactory(src, func(source []byte) (gotreesitter.TokenSource, error) {
			return NewJavaTokenSource(source, lang)
		})
		if err != nil {
			t.Fatalf("parse with token source failed: %v", err)
		}
		if tree == nil || tree.RootNode() == nil {
			t.Fatal("parse returned nil root")
		}
		if root := tree.RootNode(); root.HasError() {
			t.Fatalf("expected parenthesized shift expression to parse without syntax errors, got: %s", root.SExpr(lang))
		}
	})
}

func TestJavaShiftExpressionBeforeCallDoesNotSplitCompactAngles(t *testing.T) {
	src := []byte(`class T {
  void f(int value) {
    consume(value >> (mask()));
    consume(value >>> (mask()));
  }
}
`)

	t.Run("dfa", func(t *testing.T) {
		assertJavaSourceParsesWithoutErrors(t, src, false)
	})
	t.Run("token_source", func(t *testing.T) {
		assertJavaSourceParsesWithoutErrors(t, src, true)
	})
}

func TestJavaParseWithTokenSourceUnsignedShiftExpressionRegression(t *testing.T) {
	lang := JavaLanguage()
	parser := gotreesitter.NewParser(lang)

	src := []byte(`class T {
  void f() {
    int shifted = value >>> 1;
  }
}
`)

	tree, err := parser.ParseWithTokenSourceFactory(src, func(source []byte) (gotreesitter.TokenSource, error) {
		return NewJavaTokenSource(source, lang)
	})
	if err != nil {
		t.Fatalf("parse with token source failed: %v", err)
	}
	if tree == nil || tree.RootNode() == nil {
		t.Fatal("parse returned nil root")
	}
	if root := tree.RootNode(); root.HasError() {
		t.Fatalf("expected unsigned shift expression to parse without syntax errors, got: %s", root.SExpr(lang))
	}
}

func TestJavaParseWithTokenSourceTripleCompactGenericRegression(t *testing.T) {
	lang := JavaLanguage()
	parser := gotreesitter.NewParser(lang)

	src := []byte(`class T {
  Map<Class<? extends TW>, List<Class<? extends X>>> entries;
}
`)

	tree, err := parser.ParseWithTokenSourceFactory(src, func(source []byte) (gotreesitter.TokenSource, error) {
		return NewJavaTokenSource(source, lang)
	})
	if err != nil {
		t.Fatalf("parse with token source failed: %v", err)
	}
	if tree == nil || tree.RootNode() == nil {
		t.Fatal("parse returned nil root")
	}
	if root := tree.RootNode(); root.HasError() {
		t.Fatalf("expected triple compact generic to parse without syntax errors, got: %s", root.SExpr(lang))
	}
}

func TestJavaCompactGenericAnglesSplitOnlyWithUnmatchedOpenAngle(t *testing.T) {
	src := []byte(`class T {
  Map<String, List<Integer>> values;

  void f(int bits) {
    int shifted = bits >> (mask());
  }
}
`)

	t.Run("dfa", func(t *testing.T) {
		assertJavaSourceParsesWithoutErrors(t, src, false)
	})
	t.Run("token_source", func(t *testing.T) {
		assertJavaSourceParsesWithoutErrors(t, src, true)
	})
}

func TestJavaParseWithTokenSourceUnderscoreResourceRegression(t *testing.T) {
	lang := JavaLanguage()
	parser := gotreesitter.NewParser(lang)

	src := []byte(`class T {
  void f() throws Exception {
    try (Closeable _ = resource()) {
    }
  }
}
`)

	tree, err := parser.ParseWithTokenSourceFactory(src, func(source []byte) (gotreesitter.TokenSource, error) {
		return NewJavaTokenSource(source, lang)
	})
	if err != nil {
		t.Fatalf("parse with token source failed: %v", err)
	}
	if tree == nil || tree.RootNode() == nil {
		t.Fatal("parse returned nil root")
	}
	if root := tree.RootNode(); root.HasError() {
		t.Fatalf("expected underscore resource to parse without syntax errors, got: %s", root.SExpr(lang))
	}
}

func TestJavaParseEnhancedForCompactNestedGenericRegression(t *testing.T) {
	lang := JavaLanguage()
	parser := gotreesitter.NewParser(lang)

	src := []byte(`class T {
  void f() {
    for (Map.Entry<String, List<X>> ent : xs.entrySet()) {
      String field = ent.getKey();
    }
  }
}
`)

	tree, err := parser.Parse(src)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if tree == nil || tree.RootNode() == nil {
		t.Fatal("parse returned nil root")
	}
	if root := tree.RootNode(); root.HasError() {
		t.Fatalf("expected compact nested generic enhanced-for to parse without syntax errors, got: %s", root.SExpr(lang))
	}
}

func TestJavaParseShiftExpressionAfterCompactAngleSplitter(t *testing.T) {
	lang := JavaLanguage()
	parser := gotreesitter.NewParser(lang)

	src := []byte(`class T {
  void f() {
    int shifted = value >> 1;
  }
}
`)

	tree, err := parser.Parse(src)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if tree == nil || tree.RootNode() == nil {
		t.Fatal("parse returned nil root")
	}
	if root := tree.RootNode(); root.HasError() {
		t.Fatalf("expected Java shift expression to parse without syntax errors, got: %s", root.SExpr(lang))
	}
}

func TestJavaTokenSourceKeepsRelationalShiftOperator(t *testing.T) {
	for _, operator := range []string{">>", ">>>"} {
		t.Run(operator, func(t *testing.T) {
			lang := JavaLanguage()
			input := []byte("class Main { void f() { boolean x = a < b " + operator + " c; var y = foo(); } }")
			parser := gotreesitter.NewParser(lang)
			parser.SetAdmissionCandidateRoute(false)
			lexer, err := NewJavaTokenSource(input, lang)
			if err != nil {
				t.Fatal(err)
			}
			tree, err := parser.ParseWithTokenSourceStrict(input, lexer)
			if err != nil || tree == nil {
				t.Fatalf("parse failed: %v", err)
			}
			defer tree.Release()
			root := tree.RootNode()
			expr := findFirstNamedDescendant(root, lang, "binary_expression")
			if root.HasError() || expr == nil {
				t.Fatalf("comparison with a shift did not parse: %s", root.SExpr(lang))
			}
			lt := expr.ChildByFieldName("operator", lang)
			right := expr.ChildByFieldName("right", lang)
			if lt == nil || lt.Text(input) != "<" || right == nil || right.Type(lang) != "binary_expression" {
				t.Fatalf("expected relational expression with a shift on the right: %s", expr.SExpr(lang))
			}
			shift := right.ChildByFieldName("operator", lang)
			if shift == nil || shift.Text(input) != operator {
				t.Fatalf("shift operator %q missing from %s", operator, right.SExpr(lang))
			}
		})
	}
}

func TestJavaTokenSourceClosesGenericBeforeRelationalOperator(t *testing.T) {
	for _, rhs := range []string{"A<B>", "pkg.A<@Mark B>", "A<B<C>>"} {
		t.Run(rhs, func(t *testing.T) {
			lang := JavaLanguage()
			input := []byte("class Main { void f() { boolean x = o instanceof " + rhs + ">y; } }")
			parser := gotreesitter.NewParser(lang)
			parser.SetAdmissionCandidateRoute(false)
			lexer, err := NewJavaTokenSource(input, lang)
			if err != nil {
				t.Fatal(err)
			}
			tree, err := parser.ParseWithTokenSourceStrict(input, lexer)
			if err != nil || tree == nil {
				t.Fatalf("parse failed: %v", err)
			}
			defer tree.Release()
			root := tree.RootNode()
			expr := findFirstNamedDescendant(root, lang, "binary_expression")
			if root.HasError() || expr == nil {
				t.Fatalf("generic followed by a comparison did not parse: %s", root.SExpr(lang))
			}
			operator := expr.ChildByFieldName("operator", lang)
			left := expr.ChildByFieldName("left", lang)
			if operator == nil || operator.Text(input) != ">" || left == nil || left.Type(lang) != "instanceof_expression" {
				t.Fatalf("generic closer consumed the comparison: %s", expr.SExpr(lang))
			}
		})
	}
}

func TestJavaTokenSourceClosesAnnotatedNestedGeneric(t *testing.T) {
	src := []byte("class Main { void f() { Object x = new A<B<@Mark(v=1) C<D>>>(arg); } }")
	lang := JavaLanguage()
	parser := gotreesitter.NewParser(lang)
	parser.SetAdmissionCandidateRoute(false)
	lexer, err := NewJavaTokenSource(src, lang)
	if err != nil {
		t.Fatal(err)
	}
	tree, err := parser.ParseWithTokenSourceStrict(src, lexer)
	if err != nil || tree == nil {
		t.Fatalf("parse failed: %v", err)
	}
	defer tree.Release()
	root := tree.RootNode()
	creation := findFirstNamedDescendant(root, lang, "object_creation_expression")
	if root.HasError() || creation == nil {
		t.Fatalf("annotated generic did not parse: %s", root.SExpr(lang))
	}
	objectType := creation.ChildByFieldName("type", lang)
	if objectType == nil || objectType.Text(src) != "A<B<@Mark(v=1) C<D>>>" {
		t.Fatalf("wrong object creation type: %s", creation.SExpr(lang))
	}
}

func TestJavaTokenSourcePreservesComparisonAfterGenericAndShiftAssignment(t *testing.T) {
	lang := JavaLanguage()
	for _, tc := range []struct{ statement, kind, operator string }{
		{"boolean y = o instanceof A<B>>=v;", "binary_expression", ">="},
		{"boolean y = o instanceof A<B<C>>>=v;", "binary_expression", ">="},
		{"v >>= 1;", "assignment_expression", ">>="},
		{"v >>>= 1;", "assignment_expression", ">>>="},
	} {
		t.Run(tc.statement, func(t *testing.T) {
			input := []byte("class A { void f() { " + tc.statement + " } }")
			p := gotreesitter.NewParser(lang)
			p.SetAdmissionCandidateRoute(false)
			s, err := NewJavaTokenSource(input, lang)
			if err != nil {
				t.Fatal(err)
			}
			tree, err := p.ParseWithTokenSourceStrict(input, s)
			if err != nil || tree == nil {
				t.Fatalf("parse: %v", err)
			}
			defer tree.Release()
			root := tree.RootNode()
			n := findFirstNamedDescendant(root, lang, tc.kind)
			if root.HasError() || n == nil {
				t.Fatalf("expected %s: %s", tc.kind, root.SExpr(lang))
			}
			op := n.ChildByFieldName("operator", lang)
			if op == nil || op.Text(input) != tc.operator {
				t.Fatalf("expected operator %q: %s", tc.operator, n.SExpr(lang))
			}
		})
	}
}
