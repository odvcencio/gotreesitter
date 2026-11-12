package parse_test

import (
	"fmt"
	"strings"
	"testing"

	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

func TestPowerShellIncrementalScannerCertification(t *testing.T) {
	for _, plan := range []struct {
		name      string
		size      int
		positions []string
		classes   []powerShellCertificationEditClass
	}{
		{
			name:      "4KiB",
			size:      4 << 10,
			positions: []string{"start", "middle", "end"},
			classes:   []powerShellCertificationEditClass{powerShellCertificationInsert, powerShellCertificationDelete, powerShellCertificationReplace},
		},
		{
			name:      "137KiB",
			size:      137 << 10,
			positions: []string{"start"},
			classes:   []powerShellCertificationEditClass{powerShellCertificationInsert},
		},
	} {
		source, sites := makePowerShellCertificationSource(plan.size)
		for _, position := range plan.positions {
			var site int
			switch position {
			case "start":
				site = sites[1]
			case "middle":
				site = sites[len(sites)/2]
			case "end":
				site = sites[len(sites)-2]
			default:
				t.Fatalf("unknown PowerShell certification position %q", position)
			}
			for _, class := range plan.classes {
				t.Run(fmt.Sprintf("%s/%s/%s", plan.name, class, position), func(t *testing.T) {
					edited, edit := applyPowerShellCertificationEdit(source, site, class)
					profile := runPowerShellCertificationSample(t, source, edited, edit)
					if plan.size >= 137<<10 && profile.ReusedBytes*100 < uint64(len(edited))*5 {
						t.Fatalf("PowerShell certification reused only %d/%d bytes (<5%%): %+v", profile.ReusedBytes, len(edited), profile)
					}
				})
			}
		}
	}
}

type powerShellCertificationEditClass uint8

const (
	powerShellCertificationInsert powerShellCertificationEditClass = iota
	powerShellCertificationDelete
	powerShellCertificationReplace
)

func (c powerShellCertificationEditClass) String() string {
	switch c {
	case powerShellCertificationInsert:
		return "insert"
	case powerShellCertificationDelete:
		return "delete"
	case powerShellCertificationReplace:
		return "replace"
	default:
		return "unknown"
	}
}

func makePowerShellCertificationSource(target int) ([]byte, []int) {
	var source strings.Builder
	source.Grow(target + 128)
	sites := make([]int, 0, target/48)
	for i := 0; source.Len() < target || len(sites) < 4; i++ {
		fmt.Fprintf(&source, "$value%06d = ", i)
		sites = append(sites, source.Len())
		source.WriteString("100\n")
		fmt.Fprintf(&source, "Write-Output $value%06d\n", i)
	}
	return []byte(source.String()), sites
}

func applyPowerShellCertificationEdit(source []byte, offset int, class powerShellCertificationEditClass) ([]byte, gts.InputEdit) {
	oldEnd := offset
	replacement := []byte{'9'}
	switch class {
	case powerShellCertificationDelete:
		oldEnd = offset + 1
		replacement = nil
	case powerShellCertificationReplace:
		oldEnd = offset + 1
		replacement = []byte("88")
	}
	edited := make([]byte, 0, len(source)-(oldEnd-offset)+len(replacement))
	edited = append(edited, source[:offset]...)
	edited = append(edited, replacement...)
	edited = append(edited, source[oldEnd:]...)
	return edited, gts.InputEdit{
		StartByte:   uint32(offset),
		OldEndByte:  uint32(oldEnd),
		NewEndByte:  uint32(offset + len(replacement)),
		StartPoint:  pointForOffset(source, offset),
		OldEndPoint: pointForOffset(source, oldEnd),
		NewEndPoint: pointForOffset(edited, offset+len(replacement)),
	}
}

func runPowerShellCertificationSample(t *testing.T, source, edited []byte, edit gts.InputEdit) gts.IncrementalParseProfile {
	t.Helper()
	lang := grammars.PowershellLanguage()
	parser := gts.NewParser(lang)
	parser.SetAdmissionCandidateRoute(false)
	oldTree, err := parser.Parse(source)
	if err != nil {
		t.Fatalf("initial PowerShell parse: %v", err)
	}
	defer oldTree.Release()
	initialRoot := requireCompleteParse(t, oldTree, source, lang, "initial PowerShell certification")
	if initialRoot.HasError() {
		t.Fatalf("initial PowerShell parse produced ERROR: %s", initialRoot.SExpr(lang))
	}

	oldTree.Edit(edit)
	incremental, profile, err := parser.ParseIncrementalProfiled(edited, oldTree)
	if err != nil {
		t.Fatalf("incremental PowerShell parse: %v", err)
	}
	defer incremental.Release()
	incrementalRoot := requireCompleteParse(t, incremental, edited, lang, "incremental PowerShell certification")
	if profile.ReuseUnsupported {
		t.Fatalf("certified PowerShell scanner fell back: reason=%q", profile.ReuseUnsupportedReason)
	}
	if !profile.OldTreeReuseRoute || profile.ReusedSubtrees == 0 || profile.ReusedBytes == 0 {
		t.Fatalf("certified PowerShell scanner did not reuse old syntax: %+v", profile)
	}
	if incrementalRoot.HasError() {
		t.Fatalf("incremental PowerShell parse produced ERROR: %s", incrementalRoot.SExpr(lang))
	}

	freshParser := gts.NewParser(lang)
	freshParser.SetAdmissionCandidateRoute(false)
	fresh, err := freshParser.Parse(edited)
	if err != nil {
		t.Fatalf("fresh PowerShell parse: %v", err)
	}
	defer fresh.Release()
	freshRoot := requireCompleteParse(t, fresh, edited, lang, "fresh PowerShell certification")
	if freshRoot.HasError() {
		t.Fatalf("fresh PowerShell parse produced ERROR: %s", freshRoot.SExpr(lang))
	}
	requireIncrementalDeepTreeMatchesFresh(t, incremental, fresh, lang)
	return profile
}
