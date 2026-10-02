package graduation

import "testing"

func TestScopedReceiptStillRequiresEveryEnabledDefault(t *testing.T) {
	old := graduatedGrammars
	defer func() { graduatedGrammars = old }()
	matrix := &Matrix{Languages: []Language{{Grammar: "python"}}}
	graduatedGrammars = nil
	if err := VerifyCoverage(matrix, []string{"python"}); err != nil {
		t.Fatal(err)
	}
	if err := VerifyCoverage(matrix, RequestedGrammars); err == nil {
		t.Fatal("partial receipt passed the fleet gate")
	}
	graduatedGrammars = []string{"go"}
	if err := VerifyCoverage(matrix, []string{"python"}); err == nil {
		t.Fatal("scope omitted the enabled Go default")
	}
	graduatedGrammars = nil
	for _, scope := range [][]string{nil, {"python", "python"}, {"unknown"}} {
		if err := VerifyCoverage(matrix, scope); err == nil {
			t.Fatalf("invalid scope passed: %v", scope)
		}
	}
}
