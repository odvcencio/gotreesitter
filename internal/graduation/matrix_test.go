package graduation

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/odvcencio/gotreesitter/internal/sched"
)

// Receipt files stay outside the repository. The CLI invokes VerifySources on
// the supplied file; this test ensures a structurally valid stale receipt fails.
func TestRejectStaleReceiptSources(t *testing.T) {
	if err := passingMatrix().VerifySources("../.."); err == nil {
		t.Fatal("stale synthetic receipt accepted for current engine sources")
	}
}

func TestDefaultsRequireReceipt(t *testing.T) {
	if err := VerifyDefaults(nil); err != nil {
		t.Fatal(err)
	}
	// Changing a returned map must not mutate the production routing config.
	copy := Allowlist()
	copy["fixture"] = true
	if Allowlist()["fixture"] {
		t.Fatal("allowlist copy changed routing config")
	}
}

func passingMatrix() *Matrix {
	hash := fmt.Sprintf("%x", sha256.Sum256([]byte("fixture")))
	passed := func(names ...string) []Gate {
		var gates []Gate
		for _, name := range names {
			gates = append(gates, Gate{ID: name, Status: "passed", Evidence: "synthetic gate test"})
		}
		return gates
	}
	m := &Matrix{Schema: "compact-graduation/v1", CandidateRevision: hash[:40], LegacyRevision: hash[:40], CorpusLockSHA256: hash, RuntimeSHA256: hash, HarnessSHA256: hash, FixturesSHA256: hash,
		Protocol:      Protocol{Benchtime: "750ms", GOMAXPROCS: 1, Count: 1, PairedCycle: "Go-C-C-Go", Warmups: 2},
		Prerequisites: passed("E-A_exit", "M1_frozen_baseline", "M1_C_oracle")}
	for seed := 1; seed <= 20; seed++ {
		m.Protocol.Seeds = append(m.Protocol.Seeds, seed)
	}
	oracle, _ := json.Marshal(map[string]string{"contract": "tree-sitter-c-v1", "language": "fixture", "binding_commit": hash[:40], "runtime_commit": hash[:40], "grammar_commit": hash[:40], "grammar_artifact_sha256": hash})
	language := Language{Grammar: "fixture", FixtureShape: "fixture", OracleIdentity: oracle,
		Gates: passed("locked_real_corpus", "edit_session_72", "edit_sites_16", "cliff", "race", "memory_budget", "highlight_tags")}
	for _, size := range []string{"32k", "137k", "1m"} {
		for _, operation := range []string{"fresh", "byte"} {
			cell := Cell{Size: size, Operation: operation, SourceBytes: 1 << 20, SourceSHA256: hash, EditSHA256: hash,
				Correctness: map[string][]Check{}, Counters: map[string][]Counters{}, Timing: map[string][]Sample{}}
			steps := 1
			if operation == "byte" {
				steps = 4
			}
			for _, engine := range []string{"legacy", "compact"} {
				for step := 0; step < steps; step++ {
					completion, _ := json.Marshal(map[string]any{"StopReason": "accepted", "SourceLen": cell.SourceBytes, "RootEndByte": cell.SourceBytes})
					cell.Correctness[engine] = append(cell.Correctness[engine], Check{Step: step, SourceSHA256: hash, GoDigest: hash, FreshDigest: hash, CDigest: hash, InitialServed: true, RequestedServed: true, FreshCEqual: true, IncrementalEqual: true, HasErrorEqual: true, Clean: true, Runtime: completion})
				}
				for direction := 0; direction < 2; direction++ {
					phase := sched.Work{Attempts: 1, Tokens: 10, Nodes: 10}
					accounting := sched.OperationWork{Total: phase, Initial: phase}
					raw, _ := json.Marshal(map[string]any{"phase_work": accounting})
					cell.Counters[engine] = append(cell.Counters[engine], Counters{Direction: direction, WholeLookups: 10, Tokens: 10, Nodes: 10, MaxVersions: 1, ReusedSubtrees: 10, ReusedBytes: 100, Complete: true, RawWork: raw})
				}
			}
			for _, seed := range m.Protocol.Seeds {
				order := []string{"legacy", "compact", "C", "C", "compact", "legacy"}
				if seed%2 == 0 {
					order = []string{"compact", "legacy", "C", "C", "legacy", "compact"}
				}
				for pass, engine := range order {
					ns := float64(100)
					if engine == "legacy" {
						ns = 101
					}
					sample := Sample{Seed: seed, Pass: pass, NS: ns, Bytes: 10, Allocs: 1}
					if engine == "compact" && operation == "fresh" {
						sample.Served = 1
					}
					cell.Timing[engine] = append(cell.Timing[engine], sample)
				}
			}
			language.Cells = append(language.Cells, cell)
		}
	}
	for _, engine := range []string{"legacy", "compact", "C"} {
		for _, operation := range []string{"fresh", "byte"} {
			language.RSS = append(language.RSS, RSS{Engine: engine, Operation: operation, SourceBytes: 1 << 20, Samples: []float64{1 << 20, 1 << 20, 1 << 20}, ProbeSamples: []float64{1 << 20, 1 << 20, 1 << 20}})
		}
	}
	language.Graduated = true
	m.Languages = []Language{language}
	return m
}

func TestGraduationRejectsForgedOrIncompleteReceipts(t *testing.T) {
	if err := passingMatrix().Validate(); err != nil {
		t.Fatal(err)
	}
	cases := map[string]func(*Matrix){
		"forged complete counter":     func(m *Matrix) { m.Languages[0].Cells[0].Counters["compact"][0].RawWork = nil },
		"counter differs from phases": func(m *Matrix) { m.Languages[0].Cells[0].Counters["compact"][0].Tokens++ },
		"missing size":                func(m *Matrix) { m.Languages[0].Cells = m.Languages[0].Cells[:5] },
		"missing seed":                func(m *Matrix) { m.Languages[0].Cells[0].Timing["C"] = m.Languages[0].Cells[0].Timing["C"][:38] },
		"duplicate pass":              func(m *Matrix) { m.Languages[0].Cells[0].Timing["C"][1] = m.Languages[0].Cells[0].Timing["C"][0] },
		"wrong C cycle":               func(m *Matrix) { m.Languages[0].Cells[0].Timing["C"][0].Pass = 0 },
		"missing prerequisite":        func(m *Matrix) { m.Prerequisites = m.Prerequisites[:2] },
		"blocked prerequisite":        func(m *Matrix) { m.Prerequisites[0].Status = "not_established" },
		"missing query gate":          func(m *Matrix) { m.Languages[0].Gates = m.Languages[0].Gates[:6] },
		"compact fallback":            func(m *Matrix) { m.Languages[0].Cells[1].Correctness["compact"][0].RequestedServed = false },
		"missing timed admission":     func(m *Matrix) { m.Languages[0].Cells[0].Timing["compact"][0].Served = 0 },
		"timed compact fallback": func(m *Matrix) {
			m.Languages[0].Cells[0].Timing["compact"][0].Served = 0
			m.Languages[0].Cells[0].Timing["compact"][0].Declined = 1
		},
		"changed source": func(m *Matrix) {
			m.Languages[0].Cells[0].Correctness["compact"][0].SourceSHA256 = fmt.Sprintf("%x", sha256.Sum256([]byte("other")))
		},
		"forged C match": func(m *Matrix) {
			m.Languages[0].Cells[0].Correctness["compact"][0].CDigest = fmt.Sprintf("%x", sha256.Sum256([]byte("other")))
		},
		"no edit allocation":         func(m *Matrix) { m.Languages[0].Cells[0].Correctness["compact"][0].NoEditAllocations = 1 },
		"counter regression":         func(m *Matrix) { m.Languages[0].Cells[0].Counters["compact"][0].WholeLookups = 11 },
		"unknown counters":           func(m *Matrix) { m.Languages[0].Cells[0].Counters["compact"][0].Complete = false },
		"reuse regression":           func(m *Matrix) { m.Languages[0].Cells[1].Counters["compact"][0].ReusedBytes = 97 },
		"RSS limit":                  func(m *Matrix) { m.Languages[0].RSS[2].Samples[0] = 401 << 20 },
		"missing RSS":                func(m *Matrix) { m.Languages[0].RSS = m.Languages[0].RSS[:5] },
		"RSS source mismatch":        func(m *Matrix) { m.Languages[0].RSS[0].SourceBytes = 2 << 20 },
		"RSS probe above final peak": func(m *Matrix) { m.Languages[0].RSS[0].ProbeSamples[0]++ },
		"false completion": func(m *Matrix) {
			m.Languages[0].Cells[0].Correctness["compact"][0].Runtime = json.RawMessage(`{"StopReason":"accepted","SourceLen":1048576,"RootEndByte":1}`)
		},
		"allocation ratchet": func(m *Matrix) {
			for i := range m.Languages[0].Cells[0].Timing["compact"] {
				m.Languages[0].Cells[0].Timing["compact"][i].Allocs = 2
			}
		},
		"loses whole operation": func(m *Matrix) {
			for i := range m.Languages[0].Cells[0].Timing["compact"] {
				m.Languages[0].Cells[0].Timing["compact"][i].NS = 102
			}
		},
		"ties legacy everywhere": func(m *Matrix) {
			for c := range m.Languages[0].Cells {
				for i := range m.Languages[0].Cells[c].Timing["compact"] {
					m.Languages[0].Cells[c].Timing["compact"][i].NS = 101
				}
			}
		},
		"above C limit": func(m *Matrix) {
			for i := range m.Languages[0].Cells[0].Timing["C"] {
				m.Languages[0].Cells[0].Timing["C"][i].NS = 9
			}
		},
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			matrix := passingMatrix()
			mutate(matrix)
			if err := matrix.Validate(); err == nil {
				t.Fatal("invalid graduation receipt accepted")
			}
		})
	}
}

func TestOwnerCorrectnessAlternativeRequiresCWitness(t *testing.T) {
	matrix := passingMatrix()
	cell := &matrix.Languages[0].Cells[0]
	for i := range cell.Timing["compact"] {
		cell.Timing["compact"][i].NS = 102
	}
	legacy := &cell.Correctness["legacy"][0]
	legacy.GoDigest = fmt.Sprintf("%x", sha256.Sum256([]byte("legacy C mismatch")))
	legacy.FreshDigest = legacy.GoDigest
	legacy.FreshCEqual = false
	if err := matrix.Validate(); err != nil {
		t.Fatalf("proved compact C fix must satisfy the owner's alternative: %v", err)
	}
}

func TestOwnerPolicyUsesTwentySeedTimeMedians(t *testing.T) {
	matrix := passingMatrix()
	cell := &matrix.Languages[0].Cells[0]
	for _, engine := range []string{"legacy", "compact"} {
		for i := range cell.Timing[engine] {
			sample := &cell.Timing[engine][i]
			legacy, compact := 110.0, 100.0
			if sample.Seed >= 10 && sample.Seed <= 11 {
				legacy, compact = 1100, 1000
			} else if sample.Seed >= 12 {
				legacy, compact = 900, 10000
			}
			sample.NS = legacy
			if engine == "compact" {
				sample.NS = compact
			}
		}
	}
	// Contention can make paired ratios disagree with the time medians.
	// The design requires the twenty-seed time median to beat legacy.
	if median(ratios(cell.Timing["compact"], cell.Timing["legacy"])) >= 1 ||
		timingMedian(cell.Timing["compact"]) != 1000 || timingMedian(cell.Timing["legacy"]) != 900 {
		t.Fatal("fixture must distinguish paired ratios from time medians")
	}
	if err := matrix.Validate(); err == nil {
		t.Fatal("a slower twenty-seed median cannot graduate through paired ratios")
	}
}
