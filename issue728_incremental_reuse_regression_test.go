package gotreesitter_test

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"strings"
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

type issue728ReuseCase struct {
	name      string
	lang      func() *gotreesitter.Language
	source    func(int) []byte
	marker    []byte
	insert    byte
	wantReuse bool
}

func TestIssue728IncrementalReuseMeasurement(t *testing.T) {
	cases := []issue728ReuseCase{
		{name: "css", lang: grammars.CssLanguage, source: issue728CSSSource, marker: []byte(".c"), insert: 'c', wantReuse: true},
		{name: "scss", lang: grammars.ScssLanguage, source: issue728SCSSSource, marker: []byte(".c"), insert: 'c', wantReuse: true},
		{name: "typescript", lang: grammars.TypescriptLanguage, source: issue728TypeScriptSource, marker: []byte("export function f"), insert: 'f', wantReuse: true},
		{name: "tsx", lang: grammars.TsxLanguage, source: issue728TypeScriptSource, marker: []byte("export function f"), insert: 'f', wantReuse: true},
		{name: "toml", lang: grammars.TomlLanguage, source: issue728TOMLSource, marker: []byte("key"), insert: 'k', wantReuse: true},
		{name: "cmake", lang: grammars.CmakeLanguage, source: issue728CMakeSource, marker: []byte("VAR"), insert: 'X', wantReuse: true},
		{name: "sql", lang: grammars.SqlLanguage, source: issue728SQLSource, marker: []byte("AS value_"), insert: 'x', wantReuse: true},
		{name: "ini", lang: grammars.IniLanguage, source: issue728INISource, marker: []byte("key"), insert: 'x', wantReuse: true},
		{name: "go", lang: grammars.GoLanguage, source: issue728GoSource, marker: []byte("func f"), insert: 'f', wantReuse: true},
	}
	for _, tc := range cases {
		for _, size := range []int{63 << 10, 65 << 10, 137 << 10} {
			t.Run(fmt.Sprintf("%s/%dKiB", tc.name, size>>10), func(t *testing.T) {
				src := tc.source(size)
				if len(src) < size {
					t.Fatalf("source length = %d, want at least %d bytes", len(src), size)
				}
				if size < 64<<10 && len(src) >= 64<<10 {
					t.Fatalf("source length = %d, want below 64 KiB", len(src))
				}
				if size >= 64<<10 && len(src) < 64<<10 {
					t.Fatalf("source length = %d, want at least 64 KiB", len(src))
				}
				offset := bytes.Index(src, tc.marker)
				if offset < 0 {
					t.Fatalf("source does not contain marker %q", tc.marker)
				}
				editOffset := offset + len(tc.marker)
				edited := make([]byte, 0, len(src)+1)
				edited = append(edited, src[:editOffset]...)
				edited = append(edited, tc.insert)
				edited = append(edited, src[editOffset:]...)
				point := issue728PointAt(src, editOffset)

				gotreesitter.ResetAdmissionCandidateCountersForTest()
				parser := gotreesitter.NewParser(tc.lang())
				parser.SetAdmissionCandidateRoute(true)
				oldTree, err := parser.Parse(src)
				if err != nil {
					t.Fatalf("initial parse: %v", err)
				}
				defer oldTree.Release()
				oldRouted, oldFallbacks := gotreesitter.AdmissionCandidateCounters()
				if oldRouted != 1 || oldFallbacks != 0 {
					t.Fatalf("candidate route counters = %d/%d, want 1/0 (reason=%q)", oldRouted, oldFallbacks, gotreesitter.AdmissionCandidateLastFallbackReason())
				}
				if oldTree.UsedForestFastPath() || oldTree.ParseRuntime().ForestFastPath {
					t.Fatal("candidate fresh tree unexpectedly used the forest route")
				}
				oldTree.Edit(gotreesitter.InputEdit{
					StartByte: uint32(editOffset), OldEndByte: uint32(editOffset), NewEndByte: uint32(editOffset + 1),
					StartPoint: point, OldEndPoint: point, NewEndPoint: issue728PointAt(edited, editOffset+1),
				})
				newTree, profile, err := parser.ParseIncrementalProfiled(edited, oldTree)
				if err != nil {
					t.Fatalf("incremental parse: %v", err)
				}
				defer newTree.Release()
				newRouted, newFallbacks := gotreesitter.AdmissionCandidateCounters()
				if newRouted != oldRouted || newFallbacks != oldFallbacks {
					t.Fatalf("incremental parse changed candidate route counters: before=%d/%d after=%d/%d", oldRouted, oldFallbacks, newRouted, newFallbacks)
				}
				if tc.wantReuse {
					if profile.ReuseUnsupported || profile.ReuseUnsupportedReason != "" {
						t.Fatalf("incremental reuse remained unsupported: %+v", profile)
					}
					if profile.ReusedBytes == 0 || profile.ReusedSubtrees == 0 || profile.NewNodesAllocated == 0 {
						t.Fatalf("incremental reuse counters are empty: %+v", profile)
					}
				} else if profile.ReuseUnsupported || profile.ReuseUnsupportedReason != "" || profile.ReusedBytes != 0 || profile.ReusedSubtrees != 0 || profile.ReuseRejectScannerUnquiescent == 0 {
					t.Fatalf("checkpoint-refuted scanner was not rejected at reuse boundaries: %+v", profile)
				}
				if !profile.OldTreeReuseRoute || !newTree.ParseRuntime().IncrementalOldTreeReuseRoute {
					t.Fatalf("incremental route tag is missing: profile=%+v runtime=%s", profile, newTree.ParseRuntime().Summary())
				}
				if newTree.UsedForestFastPath() || newTree.ParseRuntime().ForestFastPath {
					t.Fatal("incremental result unexpectedly used the forest route")
				}

				freshParser := gotreesitter.NewParser(tc.lang())
				freshParser.SetAdmissionCandidateRoute(false)
				freshTree, err := freshParser.Parse(edited)
				if err != nil {
					t.Fatalf("fresh parse: %v", err)
				}
				defer freshTree.Release()
				freshDigest := issue728TreeDigest(freshTree, tc.lang())
				requireIncrementalDeepTreeMatchesFresh(t, newTree, freshTree, tc.lang())
				if newTree.RootNode().HasError() != freshTree.RootNode().HasError() ||
					newTree.RootNode().Type(tc.lang()) != freshTree.RootNode().Type(tc.lang()) ||
					issue728TreeDigest(newTree, tc.lang()) != freshDigest {
					t.Fatalf("incremental tree differs from fresh tree: incremental=%s fresh=%s", issue728TreeDigest(newTree, tc.lang()), freshDigest)
				}
				t.Logf("source=%d oldRoute={forest=%v runtimeForest=%v usedForest=%v routed=%d fallbacks=%d} profile={reusedBytes=%d reusedSubtrees=%d newNodes=%d unsupported=%v reason=%q oldTreeReuse=%v scannerRejects=%d reparseNanos=%d} newRoute={forest=%v runtimeForest=%v incrementalReuse=%v routed=%d fallbacks=%d} freshDigest=%s",
					len(src),
					oldTree.UsedForestFastPath(), oldTree.ParseRuntime().ForestFastPath, oldTree.UsedForestFastPath(), oldRouted, oldFallbacks,
					profile.ReusedBytes, profile.ReusedSubtrees, profile.NewNodesAllocated, profile.ReuseUnsupported, profile.ReuseUnsupportedReason, profile.OldTreeReuseRoute, profile.ReuseRejectScannerUnquiescent, profile.ReparseNanos,
					newTree.UsedForestFastPath(), newTree.ParseRuntime().ForestFastPath, newTree.ParseRuntime().IncrementalOldTreeReuseRoute, newRouted, newFallbacks,
					freshDigest,
				)
			})
		}
	}
}

func TestIssue728RefutedExternalScannersFailClosed(t *testing.T) {
	cases := []issue728ReuseCase{
		{name: "python", lang: grammars.PythonLanguage, source: issue728PythonSource, marker: []byte("value"), insert: 'x'},
		{name: "starlark", lang: grammars.StarlarkLanguage, source: issue728PythonSource, marker: []byte("value"), insert: 'x'},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			src := tc.source(8 << 10)
			offset := bytes.Index(src, tc.marker)
			if offset < 0 {
				t.Fatalf("source does not contain marker %q", tc.marker)
			}
			edited := make([]byte, 0, len(src)+1)
			edited = append(edited, src[:offset+len(tc.marker)]...)
			edited = append(edited, tc.insert)
			edited = append(edited, src[offset+len(tc.marker):]...)

			gotreesitter.ResetAdmissionCandidateCountersForTest()
			parser := gotreesitter.NewParser(tc.lang())
			parser.SetAdmissionCandidateRoute(true)
			oldTree, err := parser.Parse(src)
			if err != nil {
				t.Fatalf("initial parse: %v", err)
			}
			defer oldTree.Release()
			if oldTree.UsedForestFastPath() || oldTree.ParseRuntime().ForestFastPath {
				t.Fatal("refuted scanner fixture unexpectedly used the forest route")
			}
			oldTree.Edit(gotreesitter.InputEdit{
				StartByte: uint32(offset + len(tc.marker)), OldEndByte: uint32(offset + len(tc.marker)), NewEndByte: uint32(offset + len(tc.marker) + 1),
				StartPoint: issue728PointAt(src, offset+len(tc.marker)), OldEndPoint: issue728PointAt(src, offset+len(tc.marker)), NewEndPoint: issue728PointAt(edited, offset+len(tc.marker)+1),
			})
			incremental, profile, err := parser.ParseIncrementalProfiled(edited, oldTree)
			if err != nil {
				t.Fatalf("incremental parse: %v", err)
			}
			defer incremental.Release()
			if !profile.ReuseUnsupported || profile.ReuseUnsupportedReason == "" || profile.OldTreeReuseRoute || profile.ReusedSubtrees != 0 || profile.ReusedBytes != 0 {
				t.Fatalf("refuted scanner entered reuse: %+v", profile)
			}

			freshParser := gotreesitter.NewParser(tc.lang())
			freshParser.SetAdmissionCandidateRoute(false)
			freshTree, err := freshParser.Parse(edited)
			if err != nil {
				t.Fatalf("fresh parse: %v", err)
			}
			defer freshTree.Release()
			requireIncrementalDeepTreeMatchesFresh(t, incremental, freshTree, tc.lang())
		})
	}
}

func issue728TreeDigest(tree *gotreesitter.Tree, lang *gotreesitter.Language) string {
	return fmt.Sprintf("%x", sha256.Sum256([]byte(tree.RootNode().SExpr(lang))))
}

func issue728PointAt(src []byte, offset int) gotreesitter.Point {
	var point gotreesitter.Point
	for _, b := range src[:offset] {
		if b == '\n' {
			point.Row++
			point.Column = 0
		} else {
			point.Column++
		}
	}
	return point
}

func issue728FillSource(target int, line func(int) string) []byte {
	var builder strings.Builder
	for i := 0; builder.Len() < target; i++ {
		builder.WriteString(line(i))
	}
	return []byte(builder.String())
}

func issue728CSSSource(target int) []byte {
	return issue728FillSource(target, func(i int) string {
		return fmt.Sprintf(".c%d { color: #012345; margin: 0px; padding: 1px; }\n", i)
	})
}

func issue728SCSSSource(target int) []byte {
	return issue728FillSource(target, func(i int) string {
		return fmt.Sprintf("$color%d: #012345;\n.c%d { color: $color%d; margin: 0px; padding: 1px; }\n", i, i, i)
	})
}

func issue728TypeScriptSource(target int) []byte {
	return issue728FillSource(target, func(i int) string {
		return fmt.Sprintf("export function f%d(): number { const v = %d; return v }\n", i, i)
	})
}

func issue728TOMLSource(target int) []byte {
	return issue728FillSource(target, func(i int) string {
		return fmt.Sprintf("key%d = \"value-%d\"\n", i, i)
	})
}

func issue728CMakeSource(target int) []byte {
	return issue728FillSource(target, func(i int) string {
		return fmt.Sprintf("set(VAR%d value-%d)\n", i, i)
	})
}

func issue728SQLSource(target int) []byte {
	return issue728FillSource(target, func(i int) string {
		return fmt.Sprintf("SELECT $q$payload_%06d$q$ AS value_%06d;\n", i, i)
	})
}

func issue728INISource(target int) []byte {
	return issue728FillSource(target, func(i int) string {
		return fmt.Sprintf("[section%d]\nkey%d = \"value-%d\"\n", i, i, i)
	})
}

func issue728PythonSource(target int) []byte {
	return issue728FillSource(target, func(i int) string {
		return fmt.Sprintf("def f%d():\n    value = %d\n    return value\n\n", i, i)
	})
}

func issue728GoSource(target int) []byte {
	var builder strings.Builder
	builder.WriteString("package main\n\n")
	for i := 0; builder.Len() < target; i++ {
		fmt.Fprintf(&builder, "func f%d() int { v := %d; return v }\n", i, i)
	}
	return []byte(builder.String())
}
