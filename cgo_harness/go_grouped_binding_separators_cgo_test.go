//go:build cgo && treesitter_c_parity

package cgoharness

import (
	"crypto/sha256"
	"fmt"
	"strings"
	"testing"

	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	sitter "github.com/tree-sitter/go-tree-sitter"
)

func TestGoGroupedBindingSeparatorsLockedC(t *testing.T) {
	// The pinned C grammar requires a final separator in grouped declarations.
	// Keep the omitted-separator witness and its clean control independent.
	const source = `package p
var ( a = 1; b, c int; _, d = 2, 3 )
const ( A = iota; B; left, right = 1, 2; _, C = 3, 4 )
var x = 1; var y, z int
const solo = 9; const other = 10
`
	lang := grammars.GoLanguage()
	cl := loadCanonicalGoCLanguage(t)
	for _, tc := range []struct {
		name, source, sha, digest string
		size                      int
		hasError                  bool
	}{
		{"original", source, "76320bca05dd9fbb88d7b6a9e876afb67ebf9e7fa81eee62e1736022d8d9b351", "722ef52b67924bac9609b7a2fd1e0995565f93f6399d5c6dd783950cb68055f4", 159, true},
		{"explicit_separators", strings.ReplaceAll(source, " )", "; )"), "8bcda758c990f30d5cff4c1401a9369aa7e81eed6f22e98754fdcd04d64b3a67", "7f91b26958d63ad8ae555ca637c8cbc71677f26ff3db34bb5bbe97e9b74dfbae", 161, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if len(tc.source) != tc.size || fmt.Sprintf("%x", sha256.Sum256([]byte(tc.source))) != tc.sha {
				t.Fatal("grouped fixture identity changed")
			}
			cp := sitter.NewParser()
			defer cp.Close()
			if err := cp.SetLanguage(cl); err != nil {
				t.Fatal(err)
			}
			ct := cp.Parse([]byte(tc.source), nil)
			if ct == nil {
				t.Fatal("no C tree")
			}
			defer ct.Close()
			d, err := COracleDeepDigest(ct)
			if err != nil {
				t.Fatal(err)
			}
			if d != tc.digest || ct.RootNode().HasError() != tc.hasError {
				t.Fatalf("locked C changed: digest=%s error=%t", d, ct.RootNode().HasError())
			}
			t.Logf("source_bytes=%d sha256=%x C_HasError=%t C_digest=%s", len(tc.source), sha256.Sum256([]byte(tc.source)), ct.RootNode().HasError(), d)
			for _, route := range []bool{false, true} {
				t.Run(fmt.Sprintf("candidate=%t", route), func(t *testing.T) {
					p := gts.NewParser(lang)
					p.SetAdmissionCandidateRoute(route)
					gt, err := p.Parse([]byte(tc.source))
					if err != nil {
						t.Fatal(err)
					}
					if gt == nil {
						t.Fatal("no Go tree")
					}
					defer gt.Release()
					t.Logf("Go_HasError=%t tree=%s", gt.RootNode().HasError(), gt.RootNode().SExpr(lang))
					assertLockedCTreeExactWithErrors(t, "grouped bindings", gt, lang, ct)
				})
			}
		})
	}
}

// Only the const group's final separator changes. The var group stays clean,
// so a root error cannot substitute for the const declaration's own error bit.
func TestGoGroupedConstSeparatorEditsMatchLockedC(t *testing.T) {
	const original = `package p
var ( a = 1; b, c int; _, d = 2, 3 )
const ( A = iota; B; left, right = 1, 2; _, C = 3, 4 )
var x = 1; var y, z int
const solo = 9; const other = 10
`
	clean := []byte(strings.ReplaceAll(original, " )", "; )"))
	site := strings.Index(string(clean), "_, C = 3, 4; )") + len("_, C = 3, 4")
	if site != 100 || clean[site] != ';' {
		t.Fatalf("const separator byte=%d, want 100", site)
	}
	malformed := append([]byte(nil), clean[:site]...)
	malformed = append(malformed, clean[site+1:]...)
	if len(clean) != 161 || fmt.Sprintf("%x", sha256.Sum256(clean)) != "8bcda758c990f30d5cff4c1401a9369aa7e81eed6f22e98754fdcd04d64b3a67" ||
		len(malformed) != 160 || fmt.Sprintf("%x", sha256.Sum256(malformed)) != "cc0ecfd316388946bfd86af2841e22af8bbc6058e97216394ee51458e1074b42" {
		t.Fatal("grouped const delete/restore witnesses changed")
	}
	const constStart = 48
	language := grammars.GoLanguage()
	if language == nil {
		t.Fatal("Go grammar unavailable")
	}
	cLanguage := loadCanonicalGoCLanguage(t)
	for _, route := range []struct {
		name      string
		candidate bool
	}{{"production", false}, {"candidate", true}} {
		t.Run(route.name, func(t *testing.T) {
			for _, profiled := range []bool{false, true} {
				api := "plain"
				if profiled {
					api = "profiled"
				}
				t.Run(api, func(t *testing.T) {
					gp := gts.NewParser(language)
					gp.SetAdmissionCandidateRoute(route.candidate)
					incremental := func(source []byte, old *gts.Tree) (*gts.Tree, error) {
						if profiled {
							next, _, err := gp.ParseIncrementalProfiled(source, old)
							return next, err
						}
						return gp.ParseIncremental(source, old)
					}
					cp := sitter.NewParser()
					defer cp.Close()
					if err := cp.SetLanguage(cLanguage); err != nil {
						t.Fatal(err)
					}
					source := clean
					beforeRouted, beforeFallback := gts.AdmissionCandidateCounters()
					current, err := gp.Parse(source)
					if err != nil || current == nil {
						t.Fatalf("initial Go parse: %v", err)
					}
					defer func() {
						if current != nil {
							current.Release()
						}
					}()
					currentC := cp.Parse(source, nil)
					if currentC == nil || currentC.RootNode() == nil {
						t.Fatal("initial C parse returned no tree")
					}
					defer func() {
						if currentC != nil {
							currentC.Close()
						}
					}()
					logGoHiddenMissingRoute(t, "initial", current, beforeRouted, beforeFallback)
					check := func(label string, wantError bool) {
						t.Helper()
						requireGoHiddenMissingAccepted(t, label+" incremental Go", current, source, wantError)
						freshParser := gts.NewParser(language)
						freshParser.SetAdmissionCandidateRoute(route.candidate)
						routed, fallback := gts.AdmissionCandidateCounters()
						fresh, err := freshParser.Parse(source)
						if err != nil || fresh == nil {
							t.Fatalf("%s fresh Go: %v", label, err)
						}
						defer fresh.Release()
						logGoHiddenMissingRoute(t, label+" fresh", fresh, routed, fallback)
						requireGoHiddenMissingAccepted(t, label+" fresh Go", fresh, source, wantError)
						for _, tree := range []*gts.Tree{current, fresh} {
							decl := findGoNodeByTypeAndStart(tree.RootNode(), language, "const_declaration", constStart)
							if decl == nil || decl.HasError() != wantError {
								t.Fatalf("%s Go const error=%t present=%t want=%t", label, decl != nil && decl.HasError(), decl != nil, wantError)
							}
						}
						freshCParser := sitter.NewParser()
						defer freshCParser.Close()
						if err := freshCParser.SetLanguage(cLanguage); err != nil {
							t.Fatal(err)
						}
						freshC := freshCParser.Parse(source, nil)
						if freshC == nil || freshC.RootNode() == nil {
							t.Fatal("fresh C returned no tree")
						}
						defer freshC.Close()
						for _, tree := range []*sitter.Tree{currentC, freshC} {
							root := tree.RootNode()
							decl := findCNodeByKindAndStart(root, "const_declaration", constStart)
							if root.HasError() != wantError || root.StartByte() != 0 || root.EndByte() != uint(len(source)) || decl == nil || decl.HasError() != wantError {
								t.Fatalf("%s C lost expected root/const coverage or error state", label)
							}
						}
						freshDigest, err := COracleDeepDigest(freshC)
						if err != nil {
							t.Fatal(err)
						}
						incrementalDigest, err := COracleDeepDigest(currentC)
						if err != nil || incrementalDigest != freshDigest {
							t.Fatalf("%s C fresh/incremental digest=%s/%s err=%v", label, freshDigest, incrementalDigest, err)
						}
						if !wantError && freshDigest != "7f91b26958d63ad8ae555ca637c8cbc71677f26ff3db34bb5bbe97e9b74dfbae" {
							t.Fatalf("%s clean locked-C digest changed: %s", label, freshDigest)
						}
						assertLockedCTreeExactWithErrors(t, label+" fresh Go/fresh C", fresh, language, freshC)
						assertLockedCTreeExactWithErrors(t, label+" fresh Go/incremental C", fresh, language, currentC)
						assertLockedCTreeExactWithErrors(t, label+" incremental Go/fresh C", current, language, freshC)
						assertLockedCTreeExactWithErrors(t, label+" incremental Go/incremental C", current, language, currentC)
						var noEditErr error
						noEditNil := false
						allocations := testing.AllocsPerRun(5, func() {
							same, err := incremental(source, current)
							if err != nil {
								noEditErr = err
								return
							}
							if same == nil {
								noEditNil = true
								return
							}
							same.Release()
						})
						if noEditErr != nil || noEditNil || allocations != 0 {
							t.Fatalf("%s no-edit: allocations=%g nil=%t error=%v", label, allocations, noEditNil, noEditErr)
						}
						requireGoHiddenMissingAccepted(t, label+" after no-edit", current, source, wantError)
						decl := findGoNodeByTypeAndStart(current.RootNode(), language, "const_declaration", constStart)
						if decl == nil || decl.HasError() != wantError {
							t.Fatalf("%s no-edit changed nested const error", label)
						}
						assertLockedCTreeExactWithErrors(t, label+" after no-edit/C", current, language, freshC)
						t.Logf("state=%s bytes=%d source_sha256=%x C_digest=%s const_error=%t no_edit_allocs=%g", label, len(source), sha256.Sum256(source), freshDigest, wantError, allocations)
					}
					check("initial-clean", false)
					for cycle := 0; cycle < 2; cycle++ {
						for _, deleted := range []bool{true, false} {
							after := clean
							oldEnd, newEnd := site, site+1
							label := fmt.Sprintf("cycle-%d/restore-const-separator", cycle+1)
							if deleted {
								after = malformed
								oldEnd, newEnd = site+1, site
								label = fmt.Sprintf("cycle-%d/delete-const-separator", cycle+1)
							}
							edit := gts.InputEdit{
								StartByte: uint32(site), OldEndByte: uint32(oldEnd), NewEndByte: uint32(newEnd),
								StartPoint: pointAtOffset(source, site), OldEndPoint: pointAtOffset(source, oldEnd), NewEndPoint: pointAtOffset(after, newEnd),
							}
							beforeRouted, beforeFallback = gts.AdmissionCandidateCounters()
							previous := current
							previous.Edit(edit)
							current, err = incremental(after, previous)
							previous.Release()
							if err != nil || current == nil {
								t.Fatalf("%s incremental Go: %v", label, err)
							}
							logGoHiddenMissingRoute(t, label, current, beforeRouted, beforeFallback)
							previousC := currentC
							cEdit := realCorpusCInputEdit(edit)
							previousC.Edit(&cEdit)
							currentC = cp.Parse(after, previousC)
							previousC.Close()
							if currentC == nil || currentC.RootNode() == nil {
								t.Fatalf("%s incremental C returned no tree", label)
							}
							source = after
							check(label, deleted)
						}
					}
				})
			}
		})
	}
}
