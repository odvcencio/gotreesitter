package gotreesitter_test

import (
	"bytes"
	"testing"

	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

type scalaCheckpointIdentityDriftScanner struct {
	gts.ExternalScanner
	identity gts.ExternalScannerCheckpointIdentity
}

func (s scalaCheckpointIdentityDriftScanner) SupportsIncrementalReuse() bool { return false }

func (s scalaCheckpointIdentityDriftScanner) SupportsIncrementalReuseFromErrorTree() bool {
	return false
}

func (s scalaCheckpointIdentityDriftScanner) UsesExternalScannerCheckpoints() bool { return true }

func (s scalaCheckpointIdentityDriftScanner) CheckpointIdentity() (gts.ExternalScannerCheckpointIdentity, bool) {
	return s.identity, true
}

func TestScalaIncrementalScannerCertification(t *testing.T) {
	source := []byte(`object First:
  def one = 1

object Second:
  def two = 2

object Third:
  def three = 3
`)
	offset := bytes.Index(source, []byte("two = 2")) + len("two = ")
	if offset < len("two = ") {
		t.Fatal("Scala edit site is missing")
	}
	edited := make([]byte, 0, len(source))
	edited = append(edited, source[:offset]...)
	edited = append(edited, '4')
	edited = append(edited, source[offset+1:]...)
	profile := runScalaIncrementalCertification(t, source, edited, gts.InputEdit{
		StartByte:   uint32(offset),
		OldEndByte:  uint32(offset + 1),
		NewEndByte:  uint32(offset + 1),
		StartPoint:  pointForOffset(source, offset),
		OldEndPoint: pointForOffset(source, offset+1),
		NewEndPoint: pointForOffset(edited, offset+1),
	})
	requireReleaseSameWidthReparse(t, profile)
	if !profile.ReuseUnsupported || profile.ReuseUnsupportedReason != "external_scanner_unsupported" ||
		profile.OldTreeReuseRoute || profile.NewNodesAllocated == 0 {
		t.Fatalf("Scala edit did not use its certified fallback: %+v", profile)
	}
}

func TestScalaTokenInvariantLeafRejectsCheckpointIdentityDrift(t *testing.T) {
	for _, field := range []string{"scanner", "grammar"} {
		t.Run(field, func(t *testing.T) {
			lang := grammars.ScalaLanguage()
			originalScanner := lang.ExternalScanner
			t.Cleanup(func() { lang.ExternalScanner = originalScanner })
			provider, ok := lang.ExternalScanner.(gts.ExternalScannerCheckpointIdentityProvider)
			if !ok {
				t.Fatalf("exact Scala scanner has no checkpoint identity: %T", lang.ExternalScanner)
			}
			identity, ok := provider.CheckpointIdentity()
			if !ok {
				t.Fatal("exact Scala scanner returned no checkpoint identity")
			}
			parser := gts.NewParser(lang)
			source := []byte("object A { def value = 1 }\n")
			oldTree, err := parser.Parse(source)
			if err != nil {
				t.Fatalf("parse Scala identity source: %v", err)
			}
			defer oldTree.Release()
			if field == "scanner" {
				identity.Scanner = append([]byte(nil), identity.Scanner...)
				identity.Scanner[0]++
			} else {
				identity.Grammar = append([]byte(nil), identity.Grammar...)
				identity.Grammar[0]++
			}
			lang.ExternalScanner = scalaCheckpointIdentityDriftScanner{
				ExternalScanner: lang.ExternalScanner,
				identity:        identity,
			}
			offset := bytes.IndexByte(source, '1')
			edited := append([]byte(nil), source...)
			edited[offset] = '2'
			oldTree.Edit(gts.InputEdit{
				StartByte:   uint32(offset),
				OldEndByte:  uint32(offset + 1),
				NewEndByte:  uint32(offset + 1),
				StartPoint:  pointForOffset(source, offset),
				OldEndPoint: pointForOffset(source, offset+1),
				NewEndPoint: pointForOffset(edited, offset+1),
			})
			incremental, profile, err := parser.ParseIncrementalProfiled(edited, oldTree)
			if err != nil {
				t.Fatalf("parse Scala identity drift: %v", err)
			}
			defer incremental.Release()
			if !profile.ReuseUnsupported || profile.ReuseUnsupportedReason != "external_scanner_unsupported" ||
				profile.OldTreeReuseRoute || profile.ReusedSubtrees != 0 || profile.ReusedBytes != 0 {
				t.Fatalf("Scala %s identity drift did not fail closed: %+v", field, profile)
			}
			fresh, err := gts.NewParser(lang).Parse(edited)
			if err != nil {
				t.Fatalf("fresh Scala identity-drift parse: %v", err)
			}
			defer fresh.Release()
			requireIncrementalDeepTreeMatchesFresh(t, incremental, fresh, lang)
		})
	}
}

func TestScalaTokenInvariantLeafRejectsCheckpointCapabilityRemoval(t *testing.T) {
	lang := grammars.ScalaLanguage()
	originalScanner := lang.ExternalScanner
	t.Cleanup(func() { lang.ExternalScanner = originalScanner })
	parser := gts.NewParser(lang)
	source := []byte("object A { def value = 1 }\n")
	oldTree, err := parser.Parse(source)
	if err != nil {
		t.Fatalf("parse Scala capability source: %v", err)
	}
	defer oldTree.Release()
	lang.ExternalScanner = (grammars.ScalaExternalScanner{}).ExternalScannerForLanguage(lang)
	edited := bytes.Replace(source, []byte("1"), []byte("2"), 1)
	offset := bytes.Index(source, []byte("1"))
	oldTree.Edit(gts.InputEdit{
		StartByte:   uint32(offset),
		OldEndByte:  uint32(offset + 1),
		NewEndByte:  uint32(offset + 1),
		StartPoint:  pointForOffset(source, offset),
		OldEndPoint: pointForOffset(source, offset+1),
		NewEndPoint: pointForOffset(edited, offset+1),
	})
	incremental, profile, err := parser.ParseIncrementalProfiled(edited, oldTree)
	if err != nil {
		t.Fatalf("parse Scala capability-removal edit: %v", err)
	}
	defer incremental.Release()
	if !profile.ReuseUnsupported || profile.ReuseUnsupportedReason != "external_scanner_unsupported" ||
		profile.ReusedSubtrees != 0 || profile.ReusedBytes != 0 || profile.OldTreeReuseRoute {
		t.Fatalf("checkpoint capability removal did not fail closed: %+v", profile)
	}
	fresh, err := gts.NewParser(lang).Parse(edited)
	if err != nil {
		t.Fatalf("fresh Scala capability-removal parse: %v", err)
	}
	defer fresh.Release()
	requireIncrementalDeepTreeMatchesFresh(t, incremental, fresh, lang)
}

func TestScalaChangedWidthEditUsesGeneralScannerFallback(t *testing.T) {
	source := []byte("object First:\n  def one = 1\n\nobject Second:\n  def two = 2\n")
	offset := bytes.Index(source, []byte("two = 2")) + len("two = ")
	edited := make([]byte, 0, len(source)+1)
	edited = append(edited, source[:offset]...)
	edited = append(edited, '2', '0')
	edited = append(edited, source[offset+1:]...)
	profile := runScalaIncrementalCertification(t, source, edited, gts.InputEdit{
		StartByte:   uint32(offset),
		OldEndByte:  uint32(offset + 1),
		NewEndByte:  uint32(offset + 2),
		StartPoint:  pointForOffset(source, offset),
		OldEndPoint: pointForOffset(source, offset+1),
		NewEndPoint: pointForOffset(edited, offset+2),
	})
	if !profile.ReuseUnsupported ||
		profile.ReuseUnsupportedReason != "external_scanner_unsupported" ||
		profile.OldTreeReuseRoute || profile.ReusedSubtrees != 0 || profile.ReusedBytes != 0 {
		t.Fatalf("changed-width Scala edit did not fail closed: %+v", profile)
	}
}

func TestScalaIncrementalScannerCleanAndRecoveryEdits(t *testing.T) {
	tests := []struct {
		name               string
		before             string
		after              string
		start              int
		oldEnd             int
		newEnd             int
		wantFallbackReason string
	}{
		{name: "delete final parenthesis", before: "((y)->)", after: "((y)->", start: 6, oldEnd: 7, newEnd: 6, wantFallbackReason: "external_scanner_unsupported"},
		{name: "insert final parenthesis", before: "((y)->", after: "((y)->)", start: 6, oldEnd: 6, newEnd: 7, wantFallbackReason: "external_scanner_error_tree_unsupported"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			before := []byte(test.before)
			after := []byte(test.after)
			profile := runScalaIncrementalCertification(t, before, after, gts.InputEdit{
				StartByte:   uint32(test.start),
				OldEndByte:  uint32(test.oldEnd),
				NewEndByte:  uint32(test.newEnd),
				StartPoint:  pointForOffset(before, test.start),
				OldEndPoint: pointForOffset(before, test.oldEnd),
				NewEndPoint: pointForOffset(after, test.newEnd),
			})
			if test.wantFallbackReason != "" {
				if !profile.ReuseUnsupported ||
					profile.ReuseUnsupportedReason != test.wantFallbackReason ||
					profile.OldTreeReuseRoute || profile.ReusedSubtrees != 0 || profile.ReusedBytes != 0 {
					t.Fatalf("Scala incremental tree did not fail closed: %+v", profile)
				}
			} else if profile.ReuseUnsupported || !profile.OldTreeReuseRoute {
				t.Fatalf("clean Scala tree did not enter the reuse route: %+v", profile)
			}
		})
	}
}

func TestScalaIncrementalRejectsForeignLanguageTree(t *testing.T) {
	first := grammars.ScalaLanguage()
	second, err := grammars.LoadLanguage("scala", grammars.BlobByName("scala"))
	if err != nil {
		t.Fatalf("load second Scala language: %v", err)
	}
	if first == second {
		t.Fatal("Scala language mismatch witness reused one pointer")
	}
	source := []byte("object A { def value = 1 }\n")
	edited := bytes.Replace(source, []byte("1"), []byte("20"), 1)
	oldTree, err := gts.NewParser(first).Parse(source)
	if err != nil {
		t.Fatalf("parse foreign old tree: %v", err)
	}
	defer oldTree.Release()
	offset := bytes.Index(source, []byte("1"))
	oldTree.Edit(gts.InputEdit{
		StartByte:   uint32(offset),
		OldEndByte:  uint32(offset + 1),
		NewEndByte:  uint32(offset + 2),
		StartPoint:  pointForOffset(source, offset),
		OldEndPoint: pointForOffset(source, offset+1),
		NewEndPoint: pointForOffset(edited, offset+2),
	})
	incremental, profile, err := gts.NewParser(second).ParseIncrementalProfiled(edited, oldTree)
	if err != nil {
		t.Fatalf("parse with foreign old tree: %v", err)
	}
	defer incremental.Release()
	if !profile.ReuseUnsupported || profile.ReuseUnsupportedReason != "old_tree_language_mismatch" ||
		profile.OldTreeReuseRoute || profile.ReusedSubtrees != 0 || profile.ReusedBytes != 0 {
		t.Fatalf("foreign old tree did not fail closed: %+v", profile)
	}
	fresh, err := gts.NewParser(second).Parse(edited)
	if err != nil {
		t.Fatalf("fresh second-language parse: %v", err)
	}
	defer fresh.Release()
	requireIncrementalDeepTreeMatchesFresh(t, incremental, fresh, second)
}

func runScalaIncrementalCertification(
	t *testing.T,
	source []byte,
	edited []byte,
	edit gts.InputEdit,
) gts.IncrementalParseProfile {
	t.Helper()
	lang := grammars.ScalaLanguage()
	parser := gts.NewParser(lang)
	parser.SetAdmissionCandidateRoute(false)
	oldTree, err := parser.Parse(source)
	if err != nil {
		t.Fatalf("initial Scala parse: %v", err)
	}
	defer oldTree.Release()
	oldRuntime := oldTree.ParseRuntime()
	if oldRuntime.ExternalScannerCheckpointRecords == 0 ||
		oldRuntime.ExternalScannerCheckpointBytesAllocated == 0 ||
		oldRuntime.ExternalScannerSnapshotBytesAllocated == 0 {
		t.Fatalf("initial Scala tree has no scanner checkpoint proof: %s", oldRuntime.Summary())
	}
	oldTree.Edit(edit)
	incremental, profile, err := parser.ParseIncrementalProfiled(edited, oldTree)
	if err != nil {
		t.Fatalf("incremental Scala parse: %v", err)
	}
	defer incremental.Release()
	freshParser := gts.NewParser(lang)
	freshParser.SetAdmissionCandidateRoute(false)
	fresh, err := freshParser.Parse(edited)
	if err != nil {
		t.Fatalf("fresh Scala parse: %v", err)
	}
	defer fresh.Release()
	if incremental.RootNode().SExpr(lang) != fresh.RootNode().SExpr(lang) {
		t.Fatalf("incremental Scala tree = %s\nfresh Scala tree = %s",
			incremental.RootNode().SExpr(lang), fresh.RootNode().SExpr(lang))
	}
	requireIncrementalDeepTreeMatchesFresh(t, incremental, fresh, lang)
	return profile
}
