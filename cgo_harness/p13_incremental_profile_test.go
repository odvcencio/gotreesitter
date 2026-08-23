//go:build cgo && treesitter_c_parity

package cgoharness

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"runtime/pprof"
	"strconv"
	"strings"
	"testing"
	"time"

	gotreesitter "github.com/odvcencio/gotreesitter"
)

type p13IncrementalParse func() (*gotreesitter.Tree, gotreesitter.IncrementalParseProfile, error)

// p13IncrementalProfileHook is set only by the evidence benchmark. The
// production benchmark keeps its original call shape when the hook is nil.
var p13IncrementalProfileHook func(string, string, p13IncrementalParse) (*gotreesitter.Tree, gotreesitter.IncrementalParseProfile, error)

var p13ProfileSkipValidation bool

type p13ProfileRow struct {
	Case      string                               `json:"case"`
	Direction string                               `json:"direction"`
	Iteration int                                  `json:"iteration"`
	Profile   string                               `json:"profile"`
	ElapsedNS int64                                `json:"elapsed_ns"`
	Parse     gotreesitter.IncrementalParseProfile `json:"parse_profile"`
	Runtime   gotreesitter.ParseRuntime            `json:"runtime"`
}

type p13ProfileManifest struct {
	Schema             string          `json:"schema"`
	Base               string          `json:"base"`
	Case               string          `json:"case"`
	ExpectedIterations int             `json:"expected_iterations"`
	ObservedIterations int             `json:"observed_iterations"`
	Rows               []p13ProfileRow `json:"rows"`
	AllocBefore        string          `json:"alloc_before"`
	AllocAfterTimed    string          `json:"alloc_after_timed"`
	HeapBefore         string          `json:"heap_before"`
	HeapAfterForcedGC  string          `json:"heap_after_forced_gc"`
	Chain              []string        `json:"chain"`
}

const p13ProfileManifestSchema = "gts-p13-incremental-attribution/v1"

var p13ProfileChain = []string{
	"benchmarkCanonicalGoIncremental",
	"ParseIncrementalProfiled",
	"parseInternal",
	"applyActionWithReduceChain",
	"applyReduceActionDispatch",
	"applyReduceActionFromGSS",
	"captureRawShape",
	"rawShapeComputeContentHash",
	"nodeArena.rawShapeHash",
	"recursive hash",
	"rawShapeChild.entry",
	"stackEntryNodeParseState",
}

// BenchmarkP13TimerIsolated captures one representative case per process.
// Admission, the initial parse, and each edit happen outside the CPU timer.
func BenchmarkP13TimerIsolated(b *testing.B) {
	wanted := strings.TrimSpace(os.Getenv("P13_PROFILE_CASE"))
	if wanted == "" {
		b.Skip("set P13_PROFILE_CASE")
	}
	outDir := strings.TrimSpace(os.Getenv("P13_PROFILE_OUT"))
	if outDir == "" {
		b.Fatal("set P13_PROFILE_OUT")
	}
	expected, err := strconv.Atoi(strings.TrimSpace(os.Getenv("P13_PROFILE_EXPECTED")))
	if err != nil || expected <= 0 {
		b.Fatalf("P13_PROFILE_EXPECTED must be positive: %q", os.Getenv("P13_PROFILE_EXPECTED"))
	}

	for _, tc := range loadCanonicalGoIncrementalCases(b) {
		if tc.spec.Name != wanted || tc.spec.Role != "representative" || tc.spec.Language != "go" {
			continue
		}
		tc := tc
		b.Run(tc.spec.Name, func(b *testing.B) {
			goLang := canonicalIncrementalGoLanguage(b, tc.spec.Language)
			cLang := canonicalIncrementalCLanguage(b, tc.spec.Language)
			// This is the semantic admission gate. It performs fresh and
			// incremental Go/C parses before the first CPU timer starts.
			admitCanonicalGoIncrementalCase(b, tc, goLang, cLang)

			capture := &p13ProfileCapture{b: b, outDir: outDir, expected: expected}
			// Nested benchmarks can receive one calibration iteration even with
			// an iteration-form benchtime. Pin the evidence loop explicitly.
			b.N = expected
			p13ProfileSkipValidation = strings.TrimSpace(os.Getenv("P13_PROFILE_ALLOC_ONLY")) == "1"
			p13IncrementalProfileHook = capture.hook
			benchmarkCanonicalGoIncremental(b, tc, goLang)
			p13IncrementalProfileHook = nil
			p13ProfileSkipValidation = false
			capture.finish(tc.spec.Name)
		})
		return
	}
	b.Fatalf("representative canonical case %q was not found", wanted)
}

type p13ProfileCapture struct {
	b        *testing.B
	outDir   string
	expected int
	rows     []p13ProfileRow
	base     string
}

func (c *p13ProfileCapture) hook(caseName, direction string, parse p13IncrementalParse) (*gotreesitter.Tree, gotreesitter.IncrementalParseProfile, error) {
	c.b.Helper()
	if len(c.rows) == 0 {
		c.base = filepath.Join(c.outDir, "base.txt")
		if err := os.WriteFile(c.base, []byte(canonicalP13Base()+"\n"), 0o444); err != nil {
			return nil, gotreesitter.IncrementalParseProfile{}, err
		}
		if err := c.writeProfile(filepath.Join(c.outDir, "alloc_before.pb"), "allocs"); err != nil {
			return nil, gotreesitter.IncrementalParseProfile{}, err
		}
		if err := c.writeProfile(filepath.Join(c.outDir, "heap_before.pb"), "heap"); err != nil {
			return nil, gotreesitter.IncrementalParseProfile{}, err
		}
	}
	iteration := len(c.rows) + 1
	profilePath := filepath.Join(c.outDir, fmt.Sprintf("%s_%s_%04d.pprof", caseName, direction, iteration))
	if strings.TrimSpace(os.Getenv("P13_PROFILE_ALLOC_ONLY")) == "1" {
		start := time.Now()
		tree, profile, parseErr := parse()
		elapsed := time.Since(start)
		if parseErr != nil {
			return tree, profile, parseErr
		}
		runtimeProfile := gotreesitter.ParseRuntime{}
		if tree != nil {
			runtimeProfile = tree.ParseRuntime()
		}
		c.rows = append(c.rows, p13ProfileRow{
			Case: caseName, Direction: direction, Iteration: iteration,
			ElapsedNS: elapsed.Nanoseconds(), Parse: profile, Runtime: runtimeProfile,
		})
		if iteration == c.expected {
			if err := c.writeProfile(filepath.Join(c.outDir, "alloc_after_timed.pb"), "allocs"); err != nil {
				return tree, profile, err
			}
		}
		return tree, profile, nil
	}
	file, err := os.Create(profilePath)
	if err != nil {
		return nil, gotreesitter.IncrementalParseProfile{}, err
	}
	if err := pprof.StartCPUProfile(file); err != nil {
		_ = file.Close()
		return nil, gotreesitter.IncrementalParseProfile{}, err
	}
	start := time.Now()
	tree, profile, parseErr := parse()
	elapsed := time.Since(start)
	pprof.StopCPUProfile()
	closeErr := file.Close()
	if parseErr == nil && closeErr != nil {
		parseErr = closeErr
	}
	if parseErr != nil {
		return tree, profile, parseErr
	}
	runtimeProfile := gotreesitter.ParseRuntime{}
	if tree != nil {
		runtimeProfile = tree.ParseRuntime()
	}
	c.rows = append(c.rows, p13ProfileRow{
		Case: caseName, Direction: direction, Iteration: iteration,
		Profile: profilePath, ElapsedNS: elapsed.Nanoseconds(),
		Parse: profile, Runtime: runtimeProfile,
	})
	// Capture the allocation profile before the benchmark's validation call.
	if iteration == c.expected {
		if err := c.writeProfile(filepath.Join(c.outDir, "alloc_after_timed.pb"), "allocs"); err != nil {
			return tree, profile, err
		}
	}
	return tree, profile, nil
}

func (c *p13ProfileCapture) finish(caseName string) {
	c.b.Helper()
	if len(c.rows) != c.expected {
		c.b.Fatalf("profile rows=%d want=%d", len(c.rows), c.expected)
	}
	runtime.GC()
	if err := c.writeProfile(filepath.Join(c.outDir, "heap_after_forced_gc.pb"), "heap"); err != nil {
		c.b.Fatal(err)
	}
	manifest := p13ProfileManifest{
		Schema: p13ProfileManifestSchema, Base: canonicalP13Base(), Case: caseName,
		ExpectedIterations: c.expected, ObservedIterations: len(c.rows), Rows: c.rows,
		AllocBefore:       filepath.Join(c.outDir, "alloc_before.pb"),
		AllocAfterTimed:   filepath.Join(c.outDir, "alloc_after_timed.pb"),
		HeapBefore:        filepath.Join(c.outDir, "heap_before.pb"),
		HeapAfterForcedGC: filepath.Join(c.outDir, "heap_after_forced_gc.pb"),
		Chain:             append([]string(nil), p13ProfileChain...),
	}
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		c.b.Fatal(err)
	}
	data = append(data, '\n')
	path := filepath.Join(c.outDir, "manifest.json")
	if err := os.WriteFile(path, data, 0o444); err != nil {
		c.b.Fatal(err)
	}
}

func (c *p13ProfileCapture) writeProfile(path, name string) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	profile := pprof.Lookup(name)
	if profile == nil {
		_ = file.Close()
		return fmt.Errorf("pprof profile %q is unavailable", name)
	}
	err = profile.WriteTo(file, 0)
	closeErr := file.Close()
	if err == nil {
		err = closeErr
	}
	return err
}

func canonicalP13Base() string {
	return "de4fe455d3a9778b8a9b09347908e860a1f6b7f8"
}
