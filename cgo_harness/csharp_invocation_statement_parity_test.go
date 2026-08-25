//go:build cgo && treesitter_c_parity

package cgoharness

import "testing"

func TestCSharpInvocationStatementParity(t *testing.T) {
	src := []byte("class C { void F(){ newLines.Add(line); } }\n")
	tc := parityCase{name: "c_sharp", source: string(src)}
	runParityCase(t, tc, "invocation-statement-vs-local-declaration", src)
}

func TestCSharpReadToEndMemberAccessWithTopLevelVarParity(t *testing.T) {
	src := []byte(`using System.Diagnostics;

var filePath = "";

string GetOutput()
{
    var process = new Process
    {
        StartInfo = new ProcessStartInfo
        {
            Arguments = $"test --filter skip-all-corpus-tests",

        }
    };
    var output = process.StandardOutput.ReadToEnd();
    process.WaitForExit();
    return output;
}
`)
	tc := parityCase{name: "c_sharp", source: string(src)}
	runParityCase(t, tc, "read-to-end-member-access", src)
}

func TestCSharpSwitchTupleCaseParity(t *testing.T) {
	src := []byte("class C { void M(){ switch (a, a) { case (1, 1): break; } } }\n")
	tc := parityCase{name: "c_sharp", source: string(src)}
	runParityCase(t, tc, "switch-tuple-case", src)
}

func TestCSharpInterpolatedStringArgumentParity(t *testing.T) {
	src := []byte("class C { void M(){ newLines.Add($\"{leadingWhitespaces}// <- {variable}\"); } }\n")
	tc := parityCase{name: "c_sharp", source: string(src)}
	runParityCase(t, tc, "interpolated-string-argument", src)
}

func TestCSharpNestedMemberAccessQualifiedLeftParity(t *testing.T) {
	src := []byte("class C { void M(){ if (match.Success && match.Groups.Count == 3) { } } }\n")
	tc := parityCase{name: "c_sharp", source: string(src)}
	runParityCase(t, tc, "nested-member-access-qualified-left", src)
}

func TestCSharpImplicitObjectCreationArgumentParity(t *testing.T) {
	src := []byte(`class C { void M(System.Action<object, string> onUpdate){ onUpdate(new(null, "Deployment failed")); } }
`)
	tc := parityCase{name: "c_sharp", source: string(src)}
	runParityCase(t, tc, "implicit-object-creation-argument", src)
}

func TestCSharpIsPatternConditionalExpressionParity(t *testing.T) {
	src := []byte(`class C { string M(string outputDir, string jsonPath, object context){ var path = outputDir is null ? PathHelper.GetBicepparamOutputPath(jsonPath) : FileHelper.GetResultFilePath(context, outputDir); return path; } }
`)
	tc := parityCase{name: "c_sharp", source: string(src)}
	runParityCase(t, tc, "is-pattern-conditional-expression", src)
}

func TestCSharpGenericBaseListParity(t *testing.T) {
	src := []byte(`public class CliResultAssertions : ReferenceTypeAssertions<CliResult, CliResultAssertions>
{
}
`)
	tc := parityCase{name: "c_sharp", source: string(src)}
	runParityCase(t, tc, "generic-base-list", src)
}

func TestCSharpGenericObjectCreationInCollectionInitializerParity(t *testing.T) {
	src := []byte("class C { object x = new D() { { typeof(T?), new F<T>(G.I) }, }; }\n")
	tc := parityCase{name: "c_sharp", source: string(src)}
	runParityCase(t, tc, "generic-object-creation-in-collection-initializer", src)
}
