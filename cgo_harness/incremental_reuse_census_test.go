//go:build cgo && treesitter_c_parity

package cgoharness

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	"github.com/odvcencio/gotreesitter/internal/benchfixtures"
	"github.com/odvcencio/gotreesitter/internal/diag/incrcensus"
	sitter "github.com/tree-sitter/go-tree-sitter"
)

type reuseCensusFixture struct {
	Language    string `json:"language"`
	Path        string `json:"path"`
	SHA256      string `json:"sha256"`
	Bytes       int    `json:"bytes"`
	TargetBytes int    `json:"target_bytes"`
}
type reuseCensusRow struct {
	Language                 string                      `json:"language"`
	Path                     string                      `json:"path"`
	Bytes                    int                         `json:"bytes"`
	TargetBytes              int                         `json:"target_bytes"`
	Class                    string                      `json:"class"`
	Site                     int                         `json:"site"`
	Offset                   int                         `json:"offset"`
	SourceSHA256             string                      `json:"source_sha256"`
	EditedSHA256             string                      `json:"edited_sha256"`
	GoNanos                  []int64                     `json:"go_nanos"`
	CNanos                   []int64                     `json:"c_nanos"`
	FreshGoNanos             int64                       `json:"fresh_go_nanos"`
	Profile                  gts.IncrementalParseProfile `json:"profile"`
	Runtime                  gts.ParseRuntime            `json:"runtime"`
	IncrementalEqualsFreshGo bool                        `json:"incremental_equals_fresh_go"`
	FreshGoEqualsC           bool                        `json:"fresh_go_equals_c"`
	IncrementalCEqualsFreshC bool                        `json:"incremental_c_equals_fresh_c"`
	ObservedEqualsUnobserved bool                        `json:"observed_equals_unobserved"`
	GoDigest                 string                      `json:"go_digest"`
	FreshGoDigest            string                      `json:"fresh_go_digest"`
	CDigest                  string                      `json:"c_digest"`
	Census                   incrcensus.Report           `json:"census"`
}

func reuseCensusCases(t testing.TB) []reuseCensusFixture {
	t.Helper()
	lang := os.Getenv("GTS_INCR_CENSUS_LANG")
	if lang == "" {
		t.Skip("set GTS_INCR_CENSUS_LANG and GTS_INCR_CENSUS_ROOT for the one-language census")
	}
	data, err := os.ReadFile("../docs/receipts/incremental-reuse-census/fixtures.json")
	if err != nil {
		t.Fatal(err)
	}
	var manifest struct {
		Files []reuseCensusFixture `json:"files"`
	}
	if err = json.Unmarshal(data, &manifest); err != nil {
		t.Fatal(err)
	}
	var cases []reuseCensusFixture
	for _, v := range manifest.Files {
		if v.Language == lang {
			cases = append(cases, v)
		}
	}
	if len(cases) != 2 {
		t.Fatalf("want exactly two real fixtures for %s", lang)
	}
	return cases
}
func reuseCensusParse(p *gts.Parser, entry grammars.LangEntry, src []byte, old *gts.Tree) (*gts.Tree, error) {
	if entry.TokenSourceFactory != nil {
		factory := func(s []byte) (gts.TokenSource, error) { return entry.TokenSourceFactory(s, entry.Language()), nil }
		if old != nil {
			return p.ParseIncrementalWithTokenSourceFactory(src, old, factory)
		}
		return p.ParseWithTokenSourceFactory(src, factory)
	}
	if old != nil {
		return p.ParseIncremental(src, old)
	}
	return p.Parse(src)
}
func reuseCensusEdit(src []byte, class string, site int, numeric []int) ([]byte, gts.InputEdit) {
	at := len(src) * (site + 1) / 4
	end := at
	var insert []byte
	switch class {
	case "numeric_replace":
		distance := len(src)
		for _, offset := range numeric {
			d := offset - at
			if d < 0 {
				d = -d
			}
			if d < distance {
				at = offset
				distance = d
			}
		}
		end = at + 1
		insert = []byte{'1'}
		if src[at] == '1' {
			insert[0] = '2'
		}
	case "one_byte":
		insert = []byte(" ")
	case "100_byte":
		insert = []byte(strings.Repeat(" ", 100))
	case "splice":
		end = at + 100
		insert = append([]byte(nil), src[at+100:at+132]...)
	}
	edited := make([]byte, 0, len(src)-(end-at)+len(insert))
	edited = append(edited, src[:at]...)
	edited = append(edited, insert...)
	edited = append(edited, src[end:]...)
	return edited, canonicalGoInputEdit(src, edited, at, end, at+len(insert))
}
func reuseCensusDigest(t *testing.T, tree *gts.Tree, lang *gts.Language) string {
	t.Helper()
	if tree == nil {
		t.Fatal("nil Go census tree")
	}
	v, err := benchfixtures.InspectGoTree(tree.RootNode(), lang)
	if err != nil {
		t.Fatal(err)
	}
	return v.SHA256
}

// TestIncrementalReuseCensus never searches for a passing edit. All three fixed
// sites and classes are emitted, including pre-existing parity failures. The
// hard assertion owns observer transparency; the receipt retains oracle and
// incremental invariant outcomes without silently excluding malformed edits.
func TestIncrementalReuseCensus(t *testing.T) {
	cases := reuseCensusCases(t)
	root := os.Getenv("GTS_INCR_CENSUS_ROOT")
	out := os.Getenv("GTS_INCR_CENSUS_OUT")
	if root == "" || out == "" {
		t.Fatal("census root and output are required")
	}
	output, err := os.Create(out)
	if err != nil {
		t.Fatal(err)
	}
	defer output.Close()
	enc := json.NewEncoder(output)
	entry, ok := parityEntriesByName[cases[0].Language]
	if !ok {
		t.Fatal("unregistered language")
	}
	lang := entry.Language()
	cLang, err := COracleLanguage(entry.Name)
	if err != nil {
		t.Fatal(err)
	}
	identity, err := COracleIdentity(entry.Name)
	if err != nil {
		t.Fatal(err)
	}
	identity.GrammarArtifactPath = ""
	identity.CompilerPath = filepath.Base(identity.CompilerPath)
	if err = enc.Encode(struct {
		Schema    string               `json:"schema"`
		Oracle    COracleBuildIdentity `json:"oracle"`
		GoVersion string               `json:"go_version"`
	}{"gts-incremental-reuse-census-run/v1", identity, runtime.Version()}); err != nil {
		t.Fatal(err)
	}
	cp := sitter.NewParser()
	defer cp.Close()
	if err = cp.SetLanguage(cLang); err != nil {
		t.Fatal(err)
	}
	for _, fixture := range cases {
		src, err := os.ReadFile(filepath.Join(root, fixture.Language, fixture.Path))
		if err != nil {
			t.Fatal(err)
		}
		if len(src) != fixture.Bytes || fmt.Sprintf("%x", sha256.Sum256(src)) != fixture.SHA256 {
			t.Fatal("fixture bytes differ from authenticated manifest")
		}
		numberTree, parseErr := reuseCensusParse(gts.NewParser(lang), entry, src, nil)
		if parseErr != nil {
			t.Fatal(parseErr)
		}
		var numeric []int
		stack := []*gts.Node{numberTree.RootNode()}
		for len(stack) > 0 {
			node := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			if node.ChildCount() == 0 {
				switch node.Type(lang) {
				case "int_literal", "float_literal", "number", "integer_literal", "real_literal", "integer", "float", "decimal_integer_literal", "decimal_floating_point_literal":
					for offset := int(node.StartByte()); offset < int(node.EndByte()); offset++ {
						if src[offset] >= '0' && src[offset] <= '9' {
							numeric = append(numeric, offset)
							break
						}
					}
				}
			}
			for i := node.ChildCount() - 1; i >= 0; i-- {
				stack = append(stack, node.Child(i))
			}
		}
		numberTree.Release()
		if len(numeric) == 0 {
			t.Fatal("fixture has no numeric edits")
		}
		for _, class := range []string{"one_byte", "100_byte", "splice", "numeric_replace"} {
			for site := 0; site < 3; site++ {
				edited, edit := reuseCensusEdit(src, class, site, numeric)
				row := reuseCensusRow{Language: fixture.Language, Path: fixture.Path, Bytes: len(src), TargetBytes: fixture.TargetBytes, Class: class, Site: site, Offset: int(edit.StartByte), SourceSHA256: fixture.SHA256, EditedSHA256: fmt.Sprintf("%x", sha256.Sum256(edited))}
				p := gts.NewParser(lang)
				p.SetTimeoutMicros(30_000_000)
				started := time.Now()
				fresh, err := reuseCensusParse(p, entry, edited, nil)
				row.FreshGoNanos = time.Since(started).Nanoseconds()
				if err != nil {
					t.Fatal(err)
				}
				row.FreshGoDigest = reuseCensusDigest(t, fresh, lang)
				fresh.Release()
				cfresh := cp.Parse(edited, nil)
				if cfresh == nil {
					t.Fatal("nil C fresh tree")
				}
				row.CDigest, err = COracleDeepDigest(cfresh)
				if err != nil {
					t.Fatal(err)
				}
				cfresh.Close()
				// Go-C-C-Go cycles; setup, source loading, snapshots, and digests are untimed.
				for cycle := 0; cycle < 3; cycle++ {
					for _, engine := range []string{"go", "c", "c", "go"} {
						if engine == "go" {
							old, err := reuseCensusParse(p, entry, src, nil)
							if err != nil {
								t.Fatal(err)
							}
							started = time.Now()
							old.Edit(edit)
							inc, err := reuseCensusParse(p, entry, edited, old)
							row.GoNanos = append(row.GoNanos, time.Since(started).Nanoseconds())
							if err != nil {
								t.Fatal(err)
							}
							row.GoDigest = reuseCensusDigest(t, inc, lang)
							row.Runtime = inc.ParseRuntime()
							inc.Release()
							old.Release()
						} else {
							old := cp.Parse(src, nil)
							if old == nil {
								t.Fatal("nil C old tree")
							}
							started = time.Now()
							old.Edit(&sitter.InputEdit{StartByte: uint(edit.StartByte), OldEndByte: uint(edit.OldEndByte), NewEndByte: uint(edit.NewEndByte), StartPosition: sitter.Point{Row: uint(edit.StartPoint.Row), Column: uint(edit.StartPoint.Column)}, OldEndPosition: sitter.Point{Row: uint(edit.OldEndPoint.Row), Column: uint(edit.OldEndPoint.Column)}, NewEndPosition: sitter.Point{Row: uint(edit.NewEndPoint.Row), Column: uint(edit.NewEndPoint.Column)}})
							inc := cp.Parse(edited, old)
							row.CNanos = append(row.CNanos, time.Since(started).Nanoseconds())
							if inc == nil {
								t.Fatal("nil C incremental tree")
							}
							digest, err := COracleDeepDigest(inc)
							if err != nil {
								t.Fatal(err)
							}
							if len(row.CNanos) == 1 {
								row.IncrementalCEqualsFreshC = true
							}
							row.IncrementalCEqualsFreshC = row.IncrementalCEqualsFreshC && digest == row.CDigest
							inc.Close()
							old.Close()
						}
					}
				}
				old, err := reuseCensusParse(p, entry, src, nil)
				if err != nil {
					t.Fatal(err)
				}
				inc, census, err := reuseCensusObserve(old, func() (*gts.Tree, error) { old.Edit(edit); return reuseCensusParse(p, entry, edited, old) })
				if err != nil {
					t.Fatal(err)
				}
				row.Census = census
				observedDigest := reuseCensusDigest(t, inc, lang)
				row.ObservedEqualsUnobserved = observedDigest == row.GoDigest
				row.IncrementalEqualsFreshGo = row.GoDigest == row.FreshGoDigest
				row.FreshGoEqualsC = row.FreshGoDigest == row.CDigest
				if !row.ObservedEqualsUnobserved {
					t.Errorf("observer changed tree %s/%d/%s/%d", entry.Name, fixture.TargetBytes, class, site)
				}
				observedRuntime := inc.ParseRuntime()
				if observedRuntime.TokensConsumed != row.Runtime.TokensConsumed || observedRuntime.NodesAllocated != row.Runtime.NodesAllocated || observedRuntime.Iterations != row.Runtime.Iterations {
					t.Errorf("observer changed deterministic work %s/%d/%s/%d", entry.Name, fixture.TargetBytes, class, site)
				}
				if inc.RootNode().IsError() && !inc.RootNode().HasError() {
					t.Errorf("ERROR root lacks HasError")
				}
				if inc.RootNode().EndByte() != uint32(len(edited)) && inc.ParseRuntime().StopReason == gts.ParseStopAccepted {
					t.Errorf("accepted result lacks full input coverage")
				}
				inc.Release()
				old.Release()
				if err = enc.Encode(row); err != nil {
					t.Fatal(err)
				}
				t.Logf("%s/%d/%s/%d fresh_go=%t c=%t retained=%d/%d", entry.Name, fixture.TargetBytes, class, site, row.IncrementalEqualsFreshGo, row.FreshGoEqualsC, row.Census.ReusedNodes, row.Census.OldNodes)
			}
		}
	}
}
