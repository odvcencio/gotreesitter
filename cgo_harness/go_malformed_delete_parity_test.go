//go:build cgo && treesitter_c_parity && gts_parsercorephase0 && !gts_no_parsercorephase0

package cgoharness

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"testing"

	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	"github.com/odvcencio/gotreesitter/internal/benchfixtures"
	sitter "github.com/tree-sitter/go-tree-sitter"
)

// TestGoCognociente137KiBTopEditLockedC keeps the public issue-454 witness
// independent of the parser's internal fresh-result verifier. Each leaf builds
// its own old tree and an independent fresh Go parser. The malformed delete is
// compared just as strictly as the clean insert and replacement.
func TestGoCognociente137KiBTopEditLockedC(t *testing.T) {
	source := cognocienteGo137KiBSource()
	const sourceSHA256 = "5434575bb249338937331c3756e10dcec34237d107f523ea315996fb6de0ef8f"
	if len(source) != 140368 || fmt.Sprintf("%x", sha256.Sum256(source)) != sourceSHA256 {
		t.Fatal("Cognociente Go witness changed")
	}
	site := bytes.Index(source, []byte("x := a + b"))
	if site != 57 {
		t.Fatalf("top edit starts at %d, want 57", site)
	}
	inserted := make([]byte, 0, len(source)+1)
	inserted = append(inserted, source[:site]...)
	inserted = append(inserted, source[site])
	inserted = append(inserted, source[site:]...)
	replaced := append([]byte(nil), source...)
	replaced[site]++ // "y := a + b", exactly as in the report.
	deleted := make([]byte, 0, len(source)-1)
	deleted = append(deleted, source[:site]...)
	deleted = append(deleted, source[site+1:]...)
	for _, route := range []struct {
		name      string
		candidate bool
	}{
		{name: "production", candidate: false},
		{name: "candidate", candidate: true},
	} {
		t.Run(route.name, func(t *testing.T) {
			for _, profiled := range []bool{false, true} {
				api := "plain"
				if profiled {
					api = "profiled"
				}
				t.Run(api, func(t *testing.T) {
					for _, change := range []struct {
						name         string
						source       []byte
						oldEnd       int
						newEnd       int
						wantHasError bool
					}{
						{name: "insert", source: inserted, oldEnd: site, newEnd: site + 1},
						{name: "replace", source: replaced, oldEnd: site + 1, newEnd: site + 1},
						{name: "delete", source: deleted, oldEnd: site + 1, newEnd: site, wantHasError: true},
					} {
						t.Run(change.name, func(t *testing.T) {
							language := grammars.GoLanguage()
							parser := gts.NewParser(language)
							parser.SetAdmissionCandidateRoute(route.candidate)
							old, err := parser.Parse(source)
							if err != nil {
								t.Fatalf("initial Go parse: %v", err)
							}
							if old == nil {
								t.Fatal("initial Go tree is nil")
							}
							defer func() {
								if old != nil {
									old.Release()
								}
							}()
							cLanguage, err := ParityCLanguage("go")
							if err != nil {
								t.Fatal(err)
							}
							cParser := sitter.NewParser()
							defer cParser.Close()
							if err := cParser.SetLanguage(cLanguage); err != nil {
								t.Fatal(err)
							}
							cOld := cParser.Parse(source, nil)
							if cOld == nil {
								t.Fatal("initial locked C tree is nil")
							}
							defer func() {
								if cOld != nil {
									cOld.Close()
								}
							}()
							assertLockedCTreeExact(t, "initial witness", old, language, cOld)
							edit := gts.InputEdit{
								StartByte: uint32(site), OldEndByte: uint32(change.oldEnd), NewEndByte: uint32(change.newEnd),
								StartPoint: pointAtOffset(source, site), OldEndPoint: pointAtOffset(source, change.oldEnd), NewEndPoint: pointAtOffset(change.source, change.newEnd),
							}
							old.Edit(edit)
							var next *gts.Tree
							var profile gts.IncrementalParseProfile
							if profiled {
								next, profile, err = parser.ParseIncrementalProfiled(change.source, old)
							} else {
								next, err = parser.ParseIncremental(change.source, old)
							}
							if err != nil {
								t.Fatalf("incremental Go parse: %v", err)
							}
							if next == nil {
								t.Fatal("incremental Go tree is nil")
							}
							// A successful parse returns an owned handle even when
							// the result and producer share the same tree pointer.
							old.Release()
							old = nil
							defer next.Release()
							cEdit := realCorpusCInputEdit(edit)
							cOld.Edit(&cEdit)
							cNext := cParser.Parse(change.source, cOld)
							if cNext == nil {
								t.Fatal("incremental locked C tree is nil")
							}
							defer cNext.Close()
							cOld.Close()
							cOld = nil
							cFresh := cParser.Parse(change.source, nil)
							if cFresh == nil {
								t.Fatal("fresh locked C tree is nil")
							}
							defer cFresh.Close()
							freshParser := gts.NewParser(language)
							freshParser.SetAdmissionCandidateRoute(route.candidate)
							fresh, err := freshParser.Parse(change.source)
							if err != nil {
								t.Fatalf("independent fresh Go parse: %v", err)
							}
							if fresh == nil {
								t.Fatal("independent fresh Go tree is nil")
							}
							defer fresh.Release()
							for name, tree := range map[string]*gts.Tree{"incremental Go": next, "independent fresh Go": fresh} {
								root := tree.RootNode()
								if root == nil || root.StartByte() != 0 || root.EndByte() != uint32(len(change.source)) {
									t.Fatalf("%s root does not cover all %d bytes", name, len(change.source))
								}
								if root.HasError() != change.wantHasError || (root.IsError() && !root.HasError()) {
									t.Fatalf("%s error state: HasError=%v IsError=%v, want HasError=%v", name, root.HasError(), root.IsError(), change.wantHasError)
								}
							}
							for name, tree := range map[string]*sitter.Tree{"incremental C": cNext, "fresh C": cFresh} {
								root := tree.RootNode()
								if root == nil || root.StartByte() != 0 || root.EndByte() != uint(len(change.source)) || root.HasError() != change.wantHasError {
									t.Fatalf("%s root coverage or error state differs from the exact witness", name)
								}
							}
							compare := assertLockedCTreeExact
							if change.wantHasError {
								// This helper includes every error and missing node in the
								// same structural, flag, and deep-digest comparison.
								compare = assertLockedCTreeExactWithErrors
							}
							gotDigest, err := benchfixtures.InspectGoTree(next.RootNode(), language)
							if err != nil {
								t.Fatal(err)
							}
							freshDigest, err := benchfixtures.InspectGoTree(fresh.RootNode(), language)
							if err != nil {
								t.Fatal(err)
							}
							if gotDigest.SHA256 != freshDigest.SHA256 {
								t.Fatalf("incremental/fresh Go deep digest: %s versus %s", gotDigest.SHA256, freshDigest.SHA256)
							}
							cNextDigest, err := COracleDeepDigest(cNext)
							if err != nil {
								t.Fatal(err)
							}
							cFreshDigest, err := COracleDeepDigest(cFresh)
							if err != nil {
								t.Fatal(err)
							}
							if cNextDigest != cFreshDigest {
								t.Fatalf("incremental/fresh C deep digest: %s versus %s", cNextDigest, cFreshDigest)
							}
							t.Logf("D8 incremental/fresh Go and incremental/fresh C agree independently: deep_digest=%s", gotDigest.SHA256)
							if profiled {
								// These public counters describe the selected parse;
								// they are not a total-work or throughput assertion.
								t.Logf("selected_parse reused_subtrees=%d new_nodes=%d reparse_ns=%d", profile.ReusedSubtrees, profile.NewNodesAllocated, profile.ReparseNanos)
							}
							compare(t, "independent fresh Go versus fresh C", fresh, language, cFresh)
							compare(t, "incremental Go versus fresh C", next, language, cFresh)
							compare(t, "incremental Go versus incremental C", next, language, cNext)
							t.Logf("source_bytes=%d edited_bytes=%d edit_byte=%d deep_digest=%s profiled=%v", len(source), len(change.source), site, gotDigest.SHA256, profiled)
						})
					}
				})
			}
		})
	}
}

func cognocienteGo137KiBSource() []byte {
	var b bytes.Buffer
	b.WriteString("package main\n\nimport \"fmt\"\n\n")
	for i := 0; b.Len() < 137*1024; i++ {
		fmt.Fprintf(&b, "func f%d(a int, b int) int {\n\tx := a + b\n\tfmt.Println(\"f%d\", x)\n\treturn x\n}\n\n", i, i)
	}
	return b.Bytes()
}

// These small witnesses pin the lexer boundary separately from the full edit.
func TestGoMalformedDeclarationPrefixLockedC(t *testing.T) {
	language := grammars.GoLanguage()
	cLanguage := loadCanonicalGoCLanguage(t)
	for _, body := range []string{":= a + b", ":= a + b\nfmt.Println(\"f0\", x)\nreturn x", ":= 1", ": a + b", "= a + b"} {
		for _, candidate := range []bool{false, true} {
			t.Run(fmt.Sprintf("%q/candidate=%t", body, candidate), func(t *testing.T) {
				source := []byte("package main\nfunc f(a int,b int) int {\n" + body + "\n}\n")
				cp := sitter.NewParser()
				defer cp.Close()
				if err := cp.SetLanguage(cLanguage); err != nil {
					t.Fatal(err)
				}
				ct := cp.Parse(source, nil)
				if ct == nil {
					t.Fatal("C returned no tree")
				}
				defer ct.Close()
				gp := gts.NewParser(language)
				gp.SetAdmissionCandidateRoute(candidate)
				gt, err := gp.Parse(source)
				if err != nil {
					t.Fatal(err)
				}
				defer gt.Release()
				assertLockedCTreeExactWithErrors(t, "malformed declaration prefix", gt, language, ct)
			})
		}
	}
}

// Sample the complete document, rather than repeating the first edit site.
// Deleting the assignment target makes the missing-token and skipped-token
// versions compete; C keeps the cheaper skipped-token recovery.
func TestGoAssignmentTargetEdits16SitesLockedC(t *testing.T) {
	source := bytes.ReplaceAll(cognocienteGo137KiBSource(), []byte("x := a + b"), []byte("x = a + b"))
	if len(source) != 138608 || fmt.Sprintf("%x", sha256.Sum256(source)) != "8dc8aad35490cffcb042bf3a79dc75d3fdd63be87fc92c638ea36351c57fce4d" {
		t.Fatal("assignment-target witness changed")
	}
	var sites []int
	for offset := 0; offset < len(source); {
		index := bytes.Index(source[offset:], []byte("x = a + b"))
		if index < 0 {
			break
		}
		sites = append(sites, offset+index)
		offset += index + len("x = a + b")
	}
	if len(sites) < 16 {
		t.Fatal("assignment witness has fewer than 16 distinct sites")
	}
	language := grammars.GoLanguage()
	cLanguage := loadCanonicalGoCLanguage(t)
	for _, candidate := range []bool{false, true} {
		for _, kind := range []string{"insert", "replace", "delete"} {
			for site := 0; site < 16; site++ {
				t.Run(fmt.Sprintf("candidate=%t/%s/site=%02d", candidate, kind, site), func(t *testing.T) {
					start := sites[site*(len(sites)-1)/15]
					end := start + 1
					replacement := []byte("y")
					switch kind {
					case "insert":
						end = start
					case "delete":
						replacement = nil
					}
					edited := append([]byte(nil), source[:start]...)
					edited = append(edited, replacement...)
					edited = append(edited, source[end:]...)
					edit := gts.InputEdit{
						StartByte: uint32(start), OldEndByte: uint32(end), NewEndByte: uint32(start + len(replacement)),
						StartPoint: pointAtOffset(source, start), OldEndPoint: pointAtOffset(source, end), NewEndPoint: pointAtOffset(edited, start+len(replacement)),
					}
					gp := gts.NewParser(language)
					gp.SetAdmissionCandidateRoute(candidate)
					old, err := gp.Parse(source)
					if err != nil || old == nil {
						t.Fatalf("old Go parse: %v", err)
					}
					old.Edit(edit)
					next, err := gp.ParseIncremental(edited, old)
					old.Release()
					if err != nil || next == nil {
						t.Fatalf("incremental Go parse: %v", err)
					}
					defer next.Release()
					freshParser := gts.NewParser(language)
					freshParser.SetAdmissionCandidateRoute(candidate)
					routedBefore, fallbackBefore := gts.AdmissionCandidateCounters()
					fresh, err := freshParser.Parse(edited)
					routedAfter, fallbackAfter := gts.AdmissionCandidateCounters()
					decline := ""
					if fallbackAfter > fallbackBefore {
						decline = gts.AdmissionCandidateLastFallbackReason()
					}
					if err != nil || fresh == nil {
						t.Fatalf("fresh Go parse: %v", err)
					}
					defer fresh.Release()
					cp := sitter.NewParser()
					defer cp.Close()
					if err := cp.SetLanguage(cLanguage); err != nil {
						t.Fatal(err)
					}
					cOld := cp.Parse(source, nil)
					if cOld == nil {
						t.Fatal("nil old C tree")
					}
					cEdit := realCorpusCInputEdit(edit)
					cOld.Edit(&cEdit)
					cNext := cp.Parse(edited, cOld)
					cOld.Close()
					if cNext == nil {
						t.Fatal("nil incremental C tree")
					}
					defer cNext.Close()
					cFresh := cp.Parse(edited, nil)
					if cFresh == nil {
						t.Fatal("nil fresh C tree")
					}
					defer cFresh.Close()
					assertLockedCTreeExactWithErrors(t, "fresh Go/fresh C", fresh, language, cFresh)
					assertLockedCTreeExactWithErrors(t, "incremental Go/fresh C", next, language, cFresh)
					assertLockedCTreeExactWithErrors(t, "incremental Go/incremental C", next, language, cNext)
					t.Logf("assignment_target bytes=%d site=%d edit_byte=%d source_sha256=%x edited_sha256=%x fresh_routed=%d fresh_fallback=%d decline=%q", len(source), site, start, sha256.Sum256(source), sha256.Sum256(edited), routedAfter-routedBefore, fallbackAfter-fallbackBefore, decline)
				})
			}
		}
	}
}
