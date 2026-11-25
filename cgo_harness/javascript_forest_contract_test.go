//go:build cgo && treesitter_c_parity && gts_ccore_test_certify

package cgoharness

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/cgo_harness/internal/corecords"
	"github.com/odvcencio/gotreesitter/internal/ccore/bind"
	"github.com/odvcencio/gotreesitter/internal/ccore/bind/bindtest"
	"github.com/odvcencio/gotreesitter/internal/ccore/certified"
	sitter "github.com/tree-sitter/go-tree-sitter"
)

type javascriptForestCase struct {
	ForestRecordsEqual  bool
	CoreRecordsEqual    bool
	ExplicitNative      bool
	ExplicitAbstains    uint64
	DefaultNative       bool
	DefaultAbstains     uint64
	DefaultCompleteDiff string
	DefaultForest       bool
	Input               string
	SHA256              string
	Bytes               int
	ForestAccept        bool
	ForestParity        bool
	ForestIdentityDiff  string
	CoreService         bool
	CoreParity          bool
	OracleError         bool
	Decline             string
	Offset              uint32
	Lookahead           gts.Symbol
	States              []gts.StateID
}

func grantJavaScriptForestForTest(t *testing.T) {
	t.Helper()
	modes := certified.Fresh | certified.Recover | certified.Scan | certified.Incr | certified.Forest
	restore, err := bindtest.CertifyForTests(modes, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(restore)

	// Enable the process candidate switch without forcing any parser off
	// its implied forest route.
	previous := gts.AdmissionCandidateRouteDefault()
	gts.SetAdmissionCandidateRouteDefault(true)
	t.Cleanup(func() { gts.SetAdmissionCandidateRouteDefault(previous) })
}

func TestJavaScriptForestContract(t *testing.T) {
	evidenceDir := os.Getenv("GTS_JS_FOREST_CONTRACT")
	if evidenceDir == "" {
		t.Skip("set a directory for the JavaScript forest contract evidence")
	}
	grantJavaScriptForestForTest(t)
	lang, oracle := ccoreLanguages(t, "javascript")
	c := sitter.NewParser()
	defer c.Close()
	if err := c.SetLanguage(oracle); err != nil {
		t.Fatal(err)
	}
	t.Run("transport lifetime", func(t *testing.T) {
		source, _ := ccoreRouteSource(t, "javascript")
		chain := gradChain(source)
		step := chain[1]
		// The default fresh parse runs on the ordinary core when the certificate
		// carries the Forest and Incr modes; the explicit entry point keeps the
		// forest election whose transfer this subtest checks.
		original, ok := gts.NewParser(lang).ParseForestExperimental(source)
		if !ok || original == nil || !original.UsedForestFastPath() {
			t.Fatal("failed native explicit forest input")
		}
		copied := original.Copy()
		original.Release()
		defer copied.Release()
		copied.Edit(*step.edit)
		_, bf, bi := bind.RouteCounters()
		actual, err := gts.NewParser(lang).ParseIncremental(step.src, copied)
		_, af, ai := bind.RouteCounters()
		if err != nil || actual == nil {
			t.Fatalf("reparse %v", err)
		}
		defer actual.Release()
		ct := c.Parse(source, nil)
		ct.Edit(gradSitterEdit(*step.edit))
		want := c.Parse(step.src, ct)
		ct.Close()
		defer want.Close()
		var exported bytes.Buffer
		gh, gn := bind.CompactRecords(actual, &exported)
		oraclePath := filepath.Join(t.TempDir(), "incremental-c.records")
		ch, cn, rc := corecords.Hash(want, oraclePath)
		oracleBytes, readErr := os.ReadFile(oraclePath)
		exactBytes := readErr == nil && bytes.Equal(exported.Bytes(), oracleBytes)
		rt := actual.ParseRuntime()
		diff := completeForestCIdentityDiff(actual.RootNode(), lang, want.RootNode())
		t.Logf("FIRST_EDIT offset=%d fresh=%d incremental=%d forest=%t reusedSubtrees=%d reusedBytes=%d records=%x/%d C=%x/%d bytesIdentical=%t diff=%q", step.edit.StartByte, af-bf, ai-bi, actual.UsedForestFastPath(), rt.CompactIncrementalReusedSubtrees, rt.CompactIncrementalReusedBytes, gh, gn, ch, cn, exactBytes, diff)
		if af != bf || ai-bi != 1 || actual.UsedForestFastPath() || rt.CompactIncrementalReusedSubtrees != 67 || rt.CompactIncrementalReusedBytes != 1803 || !exactBytes || gh != ch || gn != cn || rc != 0 || diff != "" {
			t.Fail()
		}
	})
	t.Run("route scopes", func(t *testing.T) {
		src := bytes.Repeat([]byte("let x=1;\n"), 30000)
		for _, c := range []struct {
			name     string
			setup    func(*gts.Parser)
			wantStop string
		}{
			{"cancel", func(p *gts.Parser) { var flag uint32 = 1; p.SetCancellationFlag(&flag) }, string(gts.ParseStopCancelled)},
			{"timeout", func(p *gts.Parser) { p.SetTimeoutMicros(1) }, string(gts.ParseStopTimeout)},
			{"memory", func(p *gts.Parser) { p.SetMemoryBudgetBytes(1) }, "budget"},
			{"included", func(p *gts.Parser) {
				p.SetIncludedRanges([]gts.Range{{StartByte: 0, EndByte: 8, EndPoint: gts.Point{Column: 8}}})
			}, ""},
			{"work-limit", func(p *gts.Parser) { p.SetParseWorkLimits(gts.ParseWorkLimits{IterationLimit: 1}) }, ""},
		} {
			t.Run(c.name, func(t *testing.T) {
				p := gts.NewParser(lang)
				c.setup(p)
				input := src
				if c.wantStop == "" {
					input = []byte("let x=1;")
				}
				before, _, _ := bind.ForestCounters()
				tree, ok := p.ParseForestExperimental(input)
				after, _, _ := bind.ForestCounters()
				if tree != nil {
					defer tree.Release()
				}
				_, _, reason, _ := p.ForestDeclineInfo()
				if after != before || (c.wantStop != "" && (ok || tree != nil || reason != c.wantStop)) {
					t.Fatalf("native accepted=%d forest=%t decline=%q want=%q", after-before, ok, reason, c.wantStop)
				}
				t.Logf("SCOPE %s native_accepted=%d forest=%t reason=%q", c.name, after-before, ok, reason)
			})
		}
	})
	for _, s := range []string{"let preserved = 1; let untouched = 10; let edited = 2;", "let preserved = 1; let untouched = 10; let edited = 3;"} {
		want := c.Parse([]byte(s), nil)
		got, ok := gts.NewParser(lang).ParseForestExperimental([]byte(s))
		if !ok || got == nil {
			t.Fatal("missing forest witness")
		}
		ch, cn, rc := corecords.Hash(want, "")
		gh, gn := bind.CompactRecords(got, nil)
		diff := completeForestCIdentityDiff(got.RootNode(), lang, want.RootNode())
		t.Logf("SEEK_ORACLE raw=%x/%d pinned=%x/%d identityDiff=%q", gh, gn, ch, cn, diff)
		if rc != 0 || diff != "" || gh != ch || gn != cn {
			t.Fail()
		}
		got.Release()
		want.Close()
	}
	sample, _ := ccoreRouteSource(t, "javascript")
	inputs := gradInputs(t, "javascript")
	for _, step := range gradChain(sample) {
		inputs = append(inputs, ccoreInput{ID: step.id, Src: step.src})
	}
	for _, input := range inputs {
		inspectJavaScriptForestCase(t, lang, c, evidenceDir, input)
	}
}

func inspectJavaScriptForestCase(t *testing.T, lang *gts.Language, c *sitter.Parser, evidenceDir string, input ccoreInput) {
	t.Helper()
	source := input.Src
	cTree := c.Parse(source, nil)
	if cTree == nil {
		t.Fatalf("%s: pinned C returned no tree", input.ID)
	}
	defer cTree.Close()
	want := ccoreCDigest(cTree)
	row := javascriptForestCase{
		Input: input.ID, SHA256: gradSHA(source), Bytes: len(source),
		OracleError: cTree.RootNode().HasError(),
	}
	forest := gts.NewParser(lang)
	priorExplicit, _, _ := bind.RouteCounters()
	exA, exD, exU := bind.ForestCounters()
	forestTree, found := forest.ParseForestExperimental(source)
	a, d, u := bind.ForestCounters()
	row.ExplicitNative = (a-exA == 1 && found) || (d-exD == 1 && !found)
	row.ExplicitAbstains = u - exU
	afterExplicit, _, _ := bind.RouteCounters()
	if row.ExplicitNative && afterExplicit["core_stop"] != priorExplicit["core_stop"] {
		t.Fatalf("%s: native explicit outcome counted a core stop", input.ID)
	}
	row.ForestAccept = found
	if found {
		if forestTree == nil || !forestTree.UsedForestFastPath() {
			t.Fatalf("%s: accepted forest lost tree provenance", input.ID)
		}
		row.ForestParity = ccoreLegacyDigest(forestTree, lang) == want
		row.ForestIdentityDiff = completeForestCIdentityDiff(forestTree.RootNode(), lang, cTree.RootNode())
		ch, cn, rc := corecords.Hash(cTree, "")
		h, n := bind.CompactRecords(forestTree, nil)
		row.ForestRecordsEqual = rc == 0 && n == cn && h == ch
		if !row.ForestRecordsEqual {
			path := filepath.Join(t.TempDir(), "oracle.records")
			corecords.Hash(cTree, path)
			fd, err := os.Open(path)
			if err != nil {
				t.Fatal(err)
			}
			var raw bytes.Buffer
			bind.CompactRecords(forestTree, &raw)
			a, b := bufio.NewScanner(&raw), bufio.NewScanner(fd)
			fields := map[string]int{}
			rawShape := 0
			first := ""
			for a.Scan() && b.Scan() {
				x, y := strings.Fields(a.Text()), strings.Fields(b.Text())
				if len(x) != len(y) {
					fields["field-count"]++
				}
				for i := 0; i < len(x) && i < len(y); i++ {
					key, _, _ := strings.Cut(x[i], "=")
					yKey, _, _ := strings.Cut(y[i], "=")
					if x[i] != y[i] {
						if first == "" {
							first = x[i] + "/" + y[i]
						}
						if key == "sym" || key == "n" || key != yKey {
							rawShape++
						}
						fields[key]++
					}
				}
			}
			if err := a.Err(); err != nil {
				t.Fatal(err)
			}
			if err := b.Err(); err != nil {
				t.Fatal(err)
			}
			fd.Close()
			js, _ := json.Marshal(fields)
			t.Logf("RECORD_FIELDS %s nativeRows=%d pinnedRows=%d shapeFieldMismatches=%d fields=%s first=%s", input.ID, n, cn, rawShape, js, first)
		}
		if !row.ForestRecordsEqual {
			old := forestTree.Copy()
			next := append(bytes.Clone(source), '\n')
			m := uint32(len(source))
			pt := gradPoint(source, len(source))
			end := gradPoint(next, len(next))
			old.Edit(gts.InputEdit{StartByte: m, OldEndByte: m, NewEndByte: m + 1, StartPoint: pt, OldEndPoint: pt, NewEndPoint: end})
			_, beforeFresh, beforeIncr := bind.RouteCounters()
			actual, err := gts.NewParser(lang).ParseIncremental(next, old)
			_, afterFresh, afterIncr := bind.RouteCounters()
			old.Release()
			if err != nil || actual == nil {
				t.Fatalf("negative control failed to return a tree %v", err)
			}
			nc := c.Parse(next, nil)
			diff := completeForestCIdentityDiff(actual.RootNode(), lang, nc.RootNode())
			var exported bytes.Buffer
			gh, gn := bind.CompactRecords(actual, &exported)
			oraclePath := filepath.Join(t.TempDir(), "negative-c.records")
			ch, cn, rc := corecords.Hash(nc, oraclePath)
			cb, err := os.ReadFile(oraclePath)
			exactBytes := err == nil && bytes.Equal(exported.Bytes(), cb)
			t.Logf("NEGATIVE_TRANSFER %s fresh=%d incr=%d C_identity=%t C_records=%t", input.ID, afterFresh-beforeFresh, afterIncr-beforeIncr, diff == "", rc == 0 && gh == ch && gn == cn && exactBytes)
			if afterFresh-beforeFresh != 1 || afterIncr != beforeIncr || diff != "" || rc != 0 || !exactBytes || gh != ch || gn != cn {
				t.Fail()
			}
			nc.Close()
			actual.Release()
		}
		forestTree.Release()
	} else {
		if forestTree != nil {
			forestTree.Release()
			t.Fatalf("%s: declined forest returned a tree", input.ID)
		}
		row.Offset, row.Lookahead, row.Decline, row.States = forest.ForestDeclineInfo()
	}

	defA, defD, defU := bind.ForestCounters()
	priorDefault, beforeDefault, _ := bind.RouteCounters()
	defaultTree, err := gts.NewParser(lang).Parse(source)
	nextDefault, afterDefault, _ := bind.RouteCounters()
	endA, endD, endU := bind.ForestCounters()
	row.DefaultNative = (endA-defA == 1 || endD-defD == 1) && afterDefault-beforeDefault == 1
	row.DefaultAbstains = endU - defU
	if row.DefaultNative && nextDefault["core_stop"] != priorDefault["core_stop"] {
		t.Fatalf("%s: completed default route counted a core stop", input.ID)
	}
	if err != nil || defaultTree == nil {
		t.Fatalf("default %s: %v", input.ID, err)
	}
	row.DefaultForest = defaultTree.UsedForestFastPath()
	row.DefaultCompleteDiff = completeForestCIdentityDiff(defaultTree.RootNode(), lang, cTree.RootNode())
	defaultTree.Release()
	core := gts.NewParser(lang)
	core.SetAdmissionCandidateRoute(true)
	_, before, _ := bind.RouteCounters()
	coreTree, err := core.ParseStrict(source)
	_, after, _ := bind.RouteCounters()
	row.CoreService = after-before == 1
	if err != nil || coreTree == nil {
		t.Errorf("%s: ccore candidate could not produce a strict tree: %v", input.ID, err)
	}
	if coreTree != nil {
		row.CoreParity = row.CoreService && ccoreLegacyDigest(coreTree, lang) == want
		if row.ForestAccept {
			ch, cn, rc := corecords.Hash(cTree, "")
			h, n := bind.CompactRecords(coreTree, nil)
			row.CoreRecordsEqual = rc == 0 && n == cn && h == ch
		}
		coreTree.Release()
	}
	if !row.ForestAccept || !row.ForestParity || !row.CoreParity {
		witness := filepath.Join(evidenceDir, row.SHA256+".js")
		if err := os.WriteFile(witness, source, 0644); err != nil {
			t.Fatal(err)
		}
	}
	raw, err := json.Marshal(row)
	if err != nil {
		t.Fatal(err)
	}
	fmt.Printf("JS_FOREST_CONTRACT %s\n", raw)
}
