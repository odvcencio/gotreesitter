package grammarruntime

import (
	"crypto/sha256"
	"testing"

	ts "github.com/odvcencio/gotreesitter"
)

func TestPerlDeclaredShiftProfileRequiresExactBlob(t *testing.T) {
	PurgeEmbeddedLanguageCache()
	t.Cleanup(func() { PurgeEmbeddedLanguageCache() })
	lang := Language("perl")
	policies := builtinLanguageRuntimeProfiles["perl"].conflictPolicies
	if len(policies) != 2 {
		t.Fatalf("declared policies=%d, want 2", len(policies))
	}
	for _, expected := range policies {
		found := false
		for _, attached := range lang.ConflictPolicies {
			if attached.State == expected.State && attached.Lookahead == expected.Lookahead && attached.Kind == expected.Kind && len(attached.ReduceSymbols) == 1 && attached.ReduceSymbols[0] == expected.ReduceSymbols[0] {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("declared policy not attached: %+v", expected)
		}
	}
	for i, p := range policies {
		if p.Kind != ts.ConflictPolicyShift || len(p.ReduceSymbols) != 1 {
			t.Fatalf("policy %d=%+v", i, p)
		}
	}
	p, q := policies[0], policies[1]
	if p.State != 1545 || p.Lookahead != 16 || lang.SymbolNames[p.ReduceSymbols[0]] != "_listexpr" {
		t.Fatalf("list profile=%+v", p)
	}
	if q.State != ts.ConflictPolicyAnyState || q.Lookahead != 23 || lang.SymbolNames[q.ReduceSymbols[0]] != "function" {
		t.Fatalf("call profile=%+v", q)
	}
	uncertified := &ts.Language{}
	if attachBuiltinLanguageRuntimeProfile("perl", sha256.Sum256([]byte("uncertified Perl")), uncertified) || len(uncertified.ConflictPolicies) != 0 {
		t.Fatal("uncertified blob received conflict rules")
	}
}
