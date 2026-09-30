//go:build cgo && treesitter_c_parity

package cgoharness

import (
	"fmt"
	"os"
	"testing"

	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

func TestAWKIncrementalInsertLockedCFresh(t *testing.T) {
	reported, err := os.ReadFile("../internal/benchfixtures/testdata/real/awk")
	if err != nil {
		t.Fatal(err)
	}
	cLanguage, err := COracleLanguage("awk")
	if err != nil {
		t.Fatal(err)
	}
	identity, err := COracleIdentity("awk")
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("C runtime=%s@%s grammar=%s@%s artifact=%s", identity.RuntimeVersion, identity.RuntimeCommit, identity.GrammarRepo, identity.GrammarCommit, identity.GrammarArtifactSHA256)
	for _, fixture := range []struct {
		name   string
		source []byte
		offset int
	}{
		{"shrunk", []byte(`{ a[(1),(2),""]=value }`), 18},
		{"reported", reported, 438},
	} {
		for _, candidate := range []bool{false, true} {
			for _, profiled := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/candidate=%t/profiled=%t", fixture.name, candidate, profiled), func(t *testing.T) {
					language := grammars.AwkLanguage()
					parser := gts.NewParser(language)
					parser.SetAdmissionCandidateRoute(candidate)
					old, err := parser.Parse(fixture.source)
					if err != nil {
						t.Fatal(err)
					}
					defer old.Release()
					initialC := awkCTree(t, cLanguage, fixture.source)
					defer initialC.Close()
					assertLockedCTreeExact(t, "initial", old, language, initialC)
					edited := append([]byte(nil), fixture.source[:fixture.offset]...)
					edited = append(edited, 'x')
					edited = append(edited, fixture.source[fixture.offset:]...)
					old.Edit(gts.InputEdit{
						StartByte: uint32(fixture.offset), OldEndByte: uint32(fixture.offset), NewEndByte: uint32(fixture.offset + 1),
						StartPoint: pointAtOffset(fixture.source, fixture.offset), OldEndPoint: pointAtOffset(fixture.source, fixture.offset), NewEndPoint: pointAtOffset(edited, fixture.offset+1),
					})
					var incremental *gts.Tree
					if profiled {
						incremental, _, err = parser.ParseIncrementalProfiled(edited, old)
					} else {
						incremental, err = parser.ParseIncremental(edited, old)
					}
					if err != nil {
						t.Fatal(err)
					}
					defer incremental.Release()
					fresh, err := parser.Parse(edited)
					if err != nil {
						t.Fatal(err)
					}
					defer fresh.Release()
					freshC := awkCTree(t, cLanguage, edited)
					defer freshC.Close()
					assertLockedCTreeExact(t, "Go fresh versus C fresh", fresh, language, freshC)
					assertLockedCTreeExact(t, "Go incremental versus C fresh", incremental, language, freshC)
				})
			}
		}
	}
}
