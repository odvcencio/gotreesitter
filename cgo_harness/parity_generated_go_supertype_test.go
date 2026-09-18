//go:build cgo && treesitter_c_parity

package cgoharness

import (
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestGeneratedGoAliasedSupertypeLockedCParity(t *testing.T) {
	lockPath, err := findParityLockPath()
	if err != nil {
		t.Fatal(err)
	}
	lock, err := loadParityLock(lockPath)
	if err != nil {
		t.Fatal(err)
	}
	entry, ok := lock["go"]
	if !ok {
		t.Fatal("Go grammar is absent from languages.lock")
	}
	repoDir, ok := parityLocalRepoDir(entry)
	if !ok {
		t.Skip("set GTS_PARITY_REPO_ROOT to the seeded Go grammar directory")
	}
	if err := verifyPinnedRepo(repoDir, entry.Commit); err != nil {
		t.Fatal(err)
	}
	grammar, err := importGrammargenSource(grammargenCGOGrammar{jsonPath: filepath.Join(repoDir, "src", "grammar.json")})
	if err != nil {
		t.Fatal(err)
	}
	generated, err := grammargenGenerate(grammar, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := cOracleRawLanguage("go")
	if err != nil {
		t.Fatal(err)
	}
	_, cSubtypes := cOracleSupertypeMap(raw)
	_, goSubtypes := goSupertypeMap(generated)
	const supertype = "_simple_type"
	if len(cSubtypes[supertype]) == 0 || len(goSubtypes[supertype]) == 0 {
		t.Fatal("the fixture requires nonempty _simple_type maps")
	}
	order := []string{supertype}
	if differences := supertypeMapDivergence(order, cSubtypes, order, goSubtypes); len(differences) != 0 {
		t.Fatalf("generated Go aliased supertype differs from locked C:\n%s", strings.Join(differences, "\n"))
	}
}
