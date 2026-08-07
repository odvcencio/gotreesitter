package gotreesitter_test

import (
	"bytes"
	"fmt"
	"os"
	"strings"
	"testing"

	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

type unavailableForestCheckpointScanner struct {
	gts.ExternalScanner
}

func (unavailableForestCheckpointScanner) Serialize(any, []byte) int { return 0 }

func (unavailableForestCheckpointScanner) SupportsIncrementalReuse() bool { return true }

func (unavailableForestCheckpointScanner) UsesExternalScannerCheckpoints() bool { return true }

func TestForestIncrementalStatelessAdmission(t *testing.T) {
	cases := []struct {
		name       string
		lang       func() *gts.Language
		wantForest bool
		prefix     string
		suffix     string
		line       func(int) string
	}{
		{name: "awk", lang: grammars.AwkLanguage, line: func(i int) string {
			return fmt.Sprintf("BEGIN { value_%06d = %d; print value_%06d }\n", i, i, i)
		}},
		{name: "kdl", lang: grammars.KdlLanguage, line: func(i int) string {
			return fmt.Sprintf("node_%06d value=%d\n", i, i)
		}},
		{name: "nix", lang: grammars.NixLanguage, prefix: "{\n", suffix: "}\n", line: func(i int) string {
			return fmt.Sprintf("value_%06d = %d;\n", i, i)
		}},
		{name: "squirrel", lang: grammars.SquirrelLanguage, line: func(i int) string {
			return fmt.Sprintf("local value_%06d = %d;\n", i, i)
		}},
		{name: "uxntal", lang: grammars.UxntalLanguage, line: func(i int) string {
			return fmt.Sprintf("@value_%06d BRK\n", i)
		}},
		{name: "toml", lang: grammars.TomlLanguage, wantForest: true, line: func(i int) string {
			return fmt.Sprintf("value_%06d = %d\n", i, i)
		}},
		{name: "css", lang: grammars.CssLanguage, wantForest: true, line: func(i int) string {
			return fmt.Sprintf(".value_%06d { width: %dpx; }\n", i, i)
		}},
		{name: "tsx", lang: grammars.TsxLanguage, wantForest: true, line: func(i int) string {
			return fmt.Sprintf("const value_%06d = %d;\n", i, i)
		}},
		{name: "ini", lang: grammars.IniLanguage, wantForest: true, line: func(i int) string {
			return fmt.Sprintf("[value_%06d]\nkey = %d\n", i, i)
		}},
		{name: "scss", lang: grammars.ScssLanguage, wantForest: true, line: func(i int) string {
			return fmt.Sprintf(".value_%06d { width: %dpx; }\n", i, i)
		}},
		{name: "typescript", lang: grammars.TypescriptLanguage, wantForest: true, line: func(i int) string {
			return fmt.Sprintf("const value_%06d = %d;\n", i, i)
		}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			forestLang := tc.lang()
			if tc.wantForest {
				previous := forestLang.WantsForest
				forestLang.WantsForest = true
				defer func() { forestLang.WantsForest = previous }()
			}
			plans := []struct {
				size      int
				positions []string
				classes   []statelessScannerEditClass
			}{
				{size: 4 << 10, positions: []string{"start", "middle", "end"}, classes: []statelessScannerEditClass{statelessScannerInsert, statelessScannerDelete, statelessScannerReplace}},
				{size: 137 << 10, positions: []string{"middle"}, classes: []statelessScannerEditClass{statelessScannerReplace}},
			}
			for _, plan := range plans {
				source, sites := makeStatelessScannerCertificationSource(plan.size, tc.prefix, tc.suffix, tc.line)
				for _, position := range plan.positions {
					var site int
					switch position {
					case "start":
						site = sites[1]
					case "middle":
						site = sites[len(sites)/2]
					case "end":
						site = sites[len(sites)-2]
					}
					for _, class := range plan.classes {
						edited, edit := applyStatelessScannerCertificationEdit(source, site, class)
						runLang := tc.lang()
						if tc.wantForest {
							runLang = forestLang
						}
						runForestIncrementalStatelessSample(t, runLang, source, edited, edit)
					}
				}
			}
		})
	}
}

func runForestIncrementalStatelessSample(t *testing.T, lang *gts.Language, source, edited []byte, edit gts.InputEdit) {
	t.Helper()
	oldParser := gts.NewParser(lang)
	oldParser.SetAdmissionCandidateRoute(false)
	oldTree, err := oldParser.Parse(source)
	if err != nil {
		t.Fatalf("initial parse: %v", err)
	}
	defer oldTree.Release()
	requireCompleteParse(t, oldTree, source, lang, "initial forest admission")
	if !oldTree.ParseRuntime().ForestFastPath {
		t.Fatal("initial tree did not use the automatic forest route")
	}
	oldTree.Edit(edit)

	incrementalParser := gts.NewParser(lang)
	incrementalParser.SetAdmissionCandidateRoute(false)
	incremental, profile, err := incrementalParser.ParseIncrementalProfiled(edited, oldTree)
	if err != nil {
		t.Fatalf("incremental parse: %v", err)
	}
	defer incremental.Release()
	requireCompleteParse(t, incremental, edited, lang, "incremental forest admission")
	if profile.ReuseUnsupported || !profile.OldTreeReuseRoute || profile.ReusedSubtrees == 0 || profile.ReusedBytes == 0 {
		t.Fatalf("forest tree did not enter useful reuse: %+v", profile)
	}

	freshParser := gts.NewParser(lang)
	freshParser.SetAdmissionCandidateRoute(false)
	fresh, err := freshParser.Parse(edited)
	if err != nil {
		t.Fatalf("fresh parse: %v", err)
	}
	defer fresh.Release()
	requireCompleteParse(t, fresh, edited, lang, "fresh forest admission")
	requireIncrementalDeepTreeMatchesFresh(t, incremental, fresh, lang)
}

// TestForestIncrementalCheckpointAdmissionCMake keeps CMake on the
// checkpoint-certified route. A changed-length edit must reuse the old tree.
func TestForestIncrementalCheckpointAdmissionCMake(t *testing.T) {
	source, sites := makeStatelessScannerCertificationSource(32<<10, "", "", func(i int) string {
		return fmt.Sprintf("set(VALUE_%06d %d)\n", i, i)
	})
	if len(sites) == 0 {
		t.Fatal("CMake fixture has no edit site")
	}
	edited, edit := applyStatelessScannerCertificationEdit(source, sites[len(sites)/2], statelessScannerReplace)
	lang := grammars.CmakeLanguage()
	previous := lang.WantsForest
	lang.WantsForest = true
	defer func() { lang.WantsForest = previous }()
	runForestIncrementalStatelessSample(t, lang, source, edited, edit)
}

func TestForestIncrementalCheckpointAdmission(t *testing.T) {
	source := []byte(strings.Repeat("<section><custom-element>value</custom-element></section>\n", 96))
	middle := len(source) / 2
	valueOffset := bytes.Index(source[middle:], []byte("value"))
	if valueOffset < 0 {
		t.Fatal("checkpoint fixture has no middle edit site")
	}
	edit := insertEdit(source, middle+valueOffset)
	lang := grammars.HtmlLanguage()

	oldParser := gts.NewParser(lang)
	oldParser.SetAdmissionCandidateRoute(false)
	oldTree, ok := oldParser.ParseForestExperimental(source)
	if !ok {
		_, _, reason, states := oldParser.ForestDeclineInfo()
		t.Fatalf("checkpoint forest parse declined: reason=%s states=%v", reason, states)
	}
	defer oldTree.Release()
	requireCompleteParse(t, oldTree, source, lang, "initial checkpoint forest admission")
	oldTree.Edit(edit.inEdit)

	incrementalParser := gts.NewParser(lang)
	incrementalParser.SetAdmissionCandidateRoute(false)
	incremental, profile, err := incrementalParser.ParseIncrementalProfiled(edit.edited, oldTree)
	if err != nil {
		t.Fatalf("checkpoint incremental parse: %v", err)
	}
	defer incremental.Release()
	requireCompleteParse(t, incremental, edit.edited, lang, "incremental checkpoint forest admission")
	if profile.ReuseUnsupported || !profile.OldTreeReuseRoute || profile.ReusedSubtrees == 0 || profile.ReusedBytes == 0 {
		t.Fatalf("checkpoint forest tree did not enter useful reuse: %+v", profile)
	}

	freshParser := gts.NewParser(lang)
	freshParser.SetAdmissionCandidateRoute(false)
	fresh, err := freshParser.Parse(edit.edited)
	if err != nil {
		t.Fatalf("checkpoint fresh parse: %v", err)
	}
	defer fresh.Release()
	requireCompleteParse(t, fresh, edit.edited, lang, "fresh checkpoint forest admission")
	requireIncrementalDeepTreeMatchesFresh(t, incremental, fresh, lang)
}

func TestForestCheckpointAdmissionFailsClosedWithoutExactSnapshot(t *testing.T) {
	lang := *grammars.HtmlLanguage()
	lang.Name = "synthetic-unavailable-checkpoint"
	lang.ExternalScanner = unavailableForestCheckpointScanner{ExternalScanner: lang.ExternalScanner}

	parser := gts.NewParser(&lang)
	tree, ok := parser.ParseForestExperimental([]byte("<div>value</div>"))
	if tree != nil {
		tree.Release()
	}
	if ok {
		t.Fatal("forest admitted a scanner boundary without an exact checkpoint")
	}
	_, _, reason, _ := parser.ForestDeclineInfo()
	if reason != "scanner_checkpoint_unavailable" {
		t.Fatalf("forest decline reason = %q, want scanner_checkpoint_unavailable", reason)
	}
}

func TestForestIncrementalOwnershipRegressionGo(t *testing.T) {
	source, err := os.ReadFile("testdata/incremental_gate/go_print.go")
	if err != nil {
		t.Fatal(err)
	}
	lang := grammars.GoLanguage()
	const sites = 10
	var edits []forestEdit
	for i := 1; i <= sites; i++ {
		at := i * len(source) / (sites + 2)
		edits = append(edits, replaceEdit(source, at), insertEdit(source, at), deleteEdit(source, at))
	}
	valid := 0
	for i, candidate := range edits {
		oldParser := gts.NewParser(lang)
		oldParser.SetAdmissionCandidateRoute(false)
		oldTree, ok := oldParser.ParseForestExperimental(source)
		if !ok {
			_, _, reason, states := oldParser.ForestDeclineInfo()
			t.Fatalf("edit %d: forest declined: reason=%s states=%v", i, reason, states)
		}
		oldTree.Edit(candidate.inEdit)
		incrementalParser := gts.NewParser(lang)
		incrementalParser.SetAdmissionCandidateRoute(false)
		incremental, _, err := incrementalParser.ParseIncrementalProfiled(candidate.edited, oldTree)
		oldTree.Release()
		if err != nil {
			t.Fatalf("edit %d: incremental: %v", i, err)
		}
		freshParser := gts.NewParser(lang)
		freshParser.SetAdmissionCandidateRoute(false)
		fresh, err := freshParser.Parse(candidate.edited)
		if err != nil {
			incremental.Release()
			t.Fatalf("edit %d: fresh: %v", i, err)
		}
		if fresh.RootNode().HasError() || incremental.RootNode().HasError() {
			incremental.Release()
			fresh.Release()
			continue
		}
		valid++
		requireIncrementalDeepTreeMatchesFresh(t, incremental, fresh, lang)
		incremental.Release()
		fresh.Release()
	}
	if valid < 3 {
		t.Fatalf("only %d valid edits", valid)
	}
}
