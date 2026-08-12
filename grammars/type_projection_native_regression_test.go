//go:build !grammar_subset

package grammars

import (
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/internal/benchfixtures"
)

type typeProjectionRetirementCase struct {
	name                   string
	source                 []byte
	language               *gotreesitter.Language
	assert                 func(*testing.T, *gotreesitter.Node, *gotreesitter.Language, []byte)
	reuseUnsupportedReason string
	compactDecline         bool
}

func TestTypeProjectionNeedsNoResultCompatibility(t *testing.T) {
	for _, test := range typeProjectionRetirementCases() {
		test := test
		t.Run(test.name, func(t *testing.T) {
			tree, err := gotreesitter.NewParser(test.language).
				ParseNoResultCompatibilityBenchmarkOnly(test.source)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(tree.Release)
			test.assert(t, tree.RootNode(), test.language, test.source)
		})
	}
}

func TestTypeProjectionRoutes(t *testing.T) {
	gotreesitter.SetGLRForestEnabled(false)
	t.Cleanup(func() { gotreesitter.SetGLRForestEnabled(true) })

	for _, test := range typeProjectionRetirementCases() {
		test := test
		t.Run(test.name, func(t *testing.T) {
			source := append(append([]byte(nil), test.source...), '\n')

			productionParser := gotreesitter.NewParser(test.language)
			productionParser.SetAdmissionCandidateRoute(false)
			production, err := productionParser.Parse(source)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(production.Release)
			test.assert(t, production.RootNode(), test.language, source)

			routedBefore, fallbackBefore := gotreesitter.AdmissionCandidateCounters()
			compactParser := gotreesitter.NewParser(test.language)
			compactParser.SetAdmissionCandidateRoute(true)
			compact, err := compactParser.Parse(source)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(compact.Release)
			test.assert(t, compact.RootNode(), test.language, source)
			routedAfter, fallbackAfter := gotreesitter.AdmissionCandidateCounters()
			if test.compactDecline {
				if routedAfter != routedBefore || fallbackAfter != fallbackBefore+1 {
					t.Fatalf(
						"compact decline counters routed=%d/%d fallback=%d/%d",
						routedBefore,
						routedAfter,
						fallbackBefore,
						fallbackAfter,
					)
				}
			} else if routedAfter != routedBefore+1 || fallbackAfter != fallbackBefore {
				t.Fatalf(
					"compact route counters routed=%d/%d fallback=%d/%d",
					routedBefore,
					routedAfter,
					fallbackBefore,
					fallbackAfter,
				)
			}

			gotreesitter.SetGLRForestEnabled(true)
			forestParser := gotreesitter.NewParser(test.language)
			forest, ok := forestParser.ParseForestExperimental(source)
			gotreesitter.SetGLRForestEnabled(false)
			if !ok || forest == nil {
				offset, symbol, reason, _ := forestParser.ForestDeclineInfo()
				t.Fatalf("forest declined at %d symbol=%d reason=%s", offset, symbol, reason)
			}
			t.Cleanup(forest.Release)
			test.assert(t, forest.RootNode(), test.language, source)
			if !forest.ParseRuntime().ForestFastPath {
				t.Fatal("forest parse did not report the forest route")
			}

			incrementalParser := gotreesitter.NewParser(test.language)
			incrementalParser.SetAdmissionCandidateRoute(false)
			oldTree, err := incrementalParser.Parse(test.source)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(oldTree.Release)
			endPoint := retiredDispatchPointAtByte(test.source, len(test.source))
			oldTree.Edit(gotreesitter.InputEdit{
				StartByte:   uint32(len(test.source)),
				OldEndByte:  uint32(len(test.source)),
				NewEndByte:  uint32(len(source)),
				StartPoint:  endPoint,
				OldEndPoint: endPoint,
				NewEndPoint: gotreesitter.Point{Row: endPoint.Row + 1},
			})
			incremental, profile, err := incrementalParser.ParseIncrementalProfiled(source, oldTree)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(incremental.Release)
			test.assert(t, incremental.RootNode(), test.language, source)
			if test.reuseUnsupportedReason != "" {
				if !profile.ReuseUnsupported ||
					profile.ReuseUnsupportedReason != test.reuseUnsupportedReason {
					t.Fatalf("incremental reuse status = %+v", profile)
				}
			} else if !profile.OldTreeReuseRoute || profile.ReusedSubtrees == 0 ||
				profile.ReusedBytes == 0 {
				t.Fatalf("incremental route did not reuse the old tree: %+v", profile)
			}

			want := typeProjectionTreeDigest(t, production, test.language)
			for route, tree := range map[string]*gotreesitter.Tree{
				"compact":     compact,
				"forest":      forest,
				"incremental": incremental,
			} {
				if got := typeProjectionTreeDigest(t, tree, test.language); got != want {
					t.Fatalf("%s digest = %s, want %s", route, got, want)
				}
			}
		})
	}
}

func typeProjectionRetirementCases() []typeProjectionRetirementCase {
	return []typeProjectionRetirementCase{
		{
			name:                   "arduino_builtin_primitive_types",
			source:                 []byte("int f(void) { char c = 0; return c; }"),
			language:               ArduinoLanguage(),
			assert:                 assertArduinoBuiltinPrimitiveTypes,
			reuseUnsupportedReason: "external_scanner_unsupported",
		},
		{
			name:           "objc_protocol_type_identifier",
			source:         []byte("@interface CallbackClient : NSObject <ClientProtocol>\n@end"),
			language:       ObjcLanguage(),
			assert:         assertObjcProtocolTypeIdentifier,
			compactDecline: true,
		},
		{
			name: "objc_method_type_sequence_scalar_first",
			source: []byte(
				"@interface T : NSObject\n" +
					"- (NSUInteger)count;\n" +
					"+ (NSArray*)items;\n" +
					"- (void)reset;\n" +
					"@end",
			),
			language:       ObjcLanguage(),
			assert:         assertObjcMethodTypeSequence,
			compactDecline: true,
		},
		{
			name: "objc_method_type_sequence_pointer_first",
			source: []byte(
				"@interface T : NSObject\n" +
					"+ (NSArray*)items;\n" +
					"- (NSUInteger)count;\n" +
					"- (void)reset;\n" +
					"@end",
			),
			language:       ObjcLanguage(),
			assert:         assertObjcMethodTypeSequence,
			compactDecline: true,
		},
		{
			name:     "hyprlang_boolean_assignment_values",
			source:   []byte("resize_on_border = true\nname = myBezier\n"),
			language: HyprlangLanguage(),
			assert:   assertHyprlangBooleanAssignmentValues,
		},
	}
}

func typeProjectionTreeDigest(
	t *testing.T,
	tree *gotreesitter.Tree,
	language *gotreesitter.Language,
) string {
	t.Helper()
	receipt, err := benchfixtures.InspectGoTree(tree.RootNode(), language)
	if err != nil {
		t.Fatal(err)
	}
	return receipt.SHA256
}

func assertArduinoBuiltinPrimitiveTypes(
	t *testing.T,
	root *gotreesitter.Node,
	language *gotreesitter.Language,
	source []byte,
) {
	t.Helper()
	for _, text := range []string{"int", "void", "char"} {
		node := findTypeProjectionNode(root, language, source, "primitive_type", text)
		if node == nil || !node.IsNamed() || node.ChildCount() != 0 {
			t.Fatalf("Arduino primitive %q = %v: %s", text, node, root.SExpr(language))
		}
	}
}

func assertObjcProtocolTypeIdentifier(
	t *testing.T,
	root *gotreesitter.Node,
	language *gotreesitter.Language,
	source []byte,
) {
	t.Helper()
	assertTypeProjectionLeaf(t, root, language, source, "type_identifier", "ClientProtocol")
}

func assertObjcMethodTypeSequence(
	t *testing.T,
	root *gotreesitter.Node,
	language *gotreesitter.Language,
	source []byte,
) {
	t.Helper()
	scalar := findTypeProjectionNode(
		root,
		language,
		source,
		"type_name",
		"NSUInteger",
	)
	if scalar == nil || scalar.ChildCount() != 1 ||
		scalar.Child(0).Type(language) != "type_identifier" {
		t.Fatalf("Objective-C scalar method type = %v: %s", scalar, root.SExpr(language))
	}

	pointer := findTypeProjectionNode(
		root,
		language,
		source,
		"type_name",
		"NSArray*",
	)
	if pointer == nil || pointer.ChildCount() != 2 ||
		pointer.Child(0).Type(language) != "type_identifier" ||
		pointer.Child(1).Type(language) != "abstract_pointer_declarator" {
		t.Fatalf("Objective-C pointer method type = %v: %s", pointer, root.SExpr(language))
	}

	primitive := findTypeProjectionNode(
		root,
		language,
		source,
		"primitive_type",
		"void",
	)
	if primitive == nil {
		t.Fatalf("Objective-C primitive method type is missing: %s", root.SExpr(language))
	}
}

// assertHyprlangBooleanAssignmentValues checks that a bare boolean keyword
// value (here "true", preceded by the leading whitespace the DFA's word
// token — declared as `word: $ => $.string` in the upstream grammar — always
// captures along with it) is projected to a named boolean node wrapping the
// specific unnamed keyword leaf, while a non-keyword value stays a plain
// string leaf.
func assertHyprlangBooleanAssignmentValues(
	t *testing.T,
	root *gotreesitter.Node,
	language *gotreesitter.Language,
	source []byte,
) {
	t.Helper()
	boolNode := findTypeProjectionNode(root, language, source, "boolean", " true")
	if boolNode == nil || !boolNode.IsNamed() || boolNode.ChildCount() != 1 {
		t.Fatalf("Hyprlang boolean value = %v: %s", boolNode, root.SExpr(language))
	}
	child := boolNode.Child(0)
	if child == nil || child.Type(language) != "true" || child.IsNamed() || child.ChildCount() != 0 {
		t.Fatalf("Hyprlang boolean child = %v: %s", child, root.SExpr(language))
	}

	stringNode := findTypeProjectionNode(root, language, source, "string", " myBezier")
	if stringNode == nil || !stringNode.IsNamed() || stringNode.ChildCount() != 0 {
		t.Fatalf("Hyprlang non-boolean value = %v: %s", stringNode, root.SExpr(language))
	}
}

func assertTypeProjectionLeaf(
	t *testing.T,
	root *gotreesitter.Node,
	language *gotreesitter.Language,
	source []byte,
	nodeType string,
	text string,
) {
	t.Helper()
	node := findTypeProjectionNode(root, language, source, nodeType, text)
	if node == nil || !node.IsNamed() || node.ChildCount() != 0 {
		t.Fatalf("%s %q = %v: %s", nodeType, text, node, root.SExpr(language))
	}
}

func findTypeProjectionNode(
	node *gotreesitter.Node,
	language *gotreesitter.Language,
	source []byte,
	nodeType string,
	text string,
) *gotreesitter.Node {
	if node == nil {
		return nil
	}
	if node.Type(language) == nodeType && node.Text(source) == text {
		return node
	}
	for index := 0; index < node.ChildCount(); index++ {
		if found := findTypeProjectionNode(
			node.Child(index),
			language,
			source,
			nodeType,
			text,
		); found != nil {
			return found
		}
	}
	return nil
}
