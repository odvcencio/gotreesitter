package graduation

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"sort"
	"strings"

	"github.com/odvcencio/gotreesitter/internal/sched"
)

// Matrix is the measured receipt, rather than a second set of thresholds.
// Required limits below come from docs/v1-design.md. Missing evidence blocks
// graduation; it never supplies a zero counter or a passing measurement.
type Matrix struct {
	Schema            string     `json:"schema"`
	CandidateRevision string     `json:"candidate_revision"`
	LegacyRevision    string     `json:"legacy_revision"`
	CorpusLockSHA256  string     `json:"corpus_lock_sha256"`
	RuntimeSHA256     string     `json:"runtime_source_sha256"`
	HarnessSHA256     string     `json:"harness_source_sha256"`
	FixturesSHA256    string     `json:"fixture_manifest_sha256"`
	Protocol          Protocol   `json:"protocol"`
	Prerequisites     []Gate     `json:"prerequisites"`
	Languages         []Language `json:"languages"`
}

type Protocol struct {
	Seeds       []int  `json:"seeds"`
	Benchtime   string `json:"benchtime"`
	GOMAXPROCS  int    `json:"gomaxprocs"`
	Count       int    `json:"count_per_process"`
	PairedCycle string `json:"paired_cycle"`
	Warmups     int    `json:"warmup_operations"`
	Notes       string `json:"notes"`
}

type Gate struct {
	ID       string `json:"name"`
	Status   string `json:"status"`
	Evidence string `json:"evidence"`
}

type Language struct {
	Grammar        string          `json:"language"`
	FixtureShape   string          `json:"fixture_shape"`
	OracleIdentity json.RawMessage `json:"oracle_identity"`
	Cells          []Cell          `json:"cells"`
	RSS            []RSS           `json:"rss"`
	Gates          []Gate          `json:"gates"`
	Graduated      bool            `json:"graduated"`
	Blockers       []string        `json:"blockers"`
}

type Cell struct {
	Size         string                `json:"size"`
	Operation    string                `json:"operation"`
	SourceBytes  int                   `json:"source_bytes"`
	SourceSHA256 string                `json:"source_sha256"`
	EditSHA256   string                `json:"edited_source_sha256"`
	Correctness  map[string][]Check    `json:"correctness"`
	Counters     map[string][]Counters `json:"counters"`
	Timing       map[string][]Sample   `json:"timing"`
	Summary      json.RawMessage       `json:"summary"`
}

type Check struct {
	Step                 int             `json:"step"`
	SourceSHA256         string          `json:"source_sha256"`
	GoDigest             string          `json:"go_digest"`
	FreshDigest          string          `json:"fresh_digest"`
	CDigest              string          `json:"C_digest"`
	InitialServed        bool            `json:"initial_served"`
	InitialDeclineReason string          `json:"initial_decline_reason"`
	RequestedServed      bool            `json:"requested_served"`
	FreshCEqual          bool            `json:"fresh_C_equal"`
	IncrementalEqual     bool            `json:"incremental_fresh_equal"`
	HasErrorEqual        bool            `json:"has_error_equal"`
	Clean                bool            `json:"clean"`
	NoEditAllocations    float64         `json:"no_edit_allocations"`
	Runtime              json.RawMessage `json:"runtime"`
	Profile              json.RawMessage `json:"profile"`
}

type Counters struct {
	Direction      int             `json:"direction"`
	WholeLookups   uint64          `json:"whole_table_lookups_proxy"`
	Tokens         uint64          `json:"tokens"`
	Nodes          uint64          `json:"nodes"`
	MaxVersions    uint64          `json:"max_live_versions"`
	ReusedSubtrees uint64          `json:"reused_subtrees"`
	ReusedBytes    uint64          `json:"reused_bytes"`
	Complete       bool            `json:"whole_counters_complete"`
	RawWork        json.RawMessage `json:"raw_work"`
}

type Sample struct {
	Seed     int     `json:"seed"`
	Pass     int     `json:"pass"`
	NS       float64 `json:"ns_per_op"`
	Bytes    float64 `json:"bytes_per_op"`
	Allocs   float64 `json:"allocs_per_op"`
	Served   float64 `json:"compact_served_per_op"`
	Declined float64 `json:"compact_declined_per_op"`
}

type RSS struct {
	Engine       string    `json:"engine"`
	Operation    string    `json:"operation"`
	SourceBytes  int       `json:"source_bytes"`
	Samples      []float64 `json:"max_rss_bytes"`
	ProbeSamples []float64 `json:"probe_max_rss_bytes"`
}

func Read(r io.Reader) (*Matrix, error) {
	var matrix Matrix
	d := json.NewDecoder(r)
	d.DisallowUnknownFields()
	if err := d.Decode(&matrix); err != nil {
		return nil, err
	}
	var extra any
	if err := d.Decode(&extra); err != io.EOF {
		return nil, fmt.Errorf("graduation matrix has trailing data")
	}
	return &matrix, nil
}

func digest(value string, bytes int) bool {
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == bytes
}

// Validate checks the receipt's shape and each recorded decision. A blocked
// language is a valid matrix row. Marking it graduated without its evidence
// is a gate failure, as is removing a required size, route, or observation.
func (m *Matrix) Validate() error {
	if m.Schema != "compact-graduation/v1" || !digest(m.CandidateRevision, 20) || !digest(m.LegacyRevision, 20) || !digest(m.CorpusLockSHA256, 32) || !digest(m.RuntimeSHA256, 32) || !digest(m.HarnessSHA256, 32) || !digest(m.FixturesSHA256, 32) {
		return fmt.Errorf("graduation matrix identity is incomplete")
	}
	p := m.Protocol
	if len(p.Seeds) != 20 || p.Benchtime != "750ms" || p.GOMAXPROCS != 1 || p.Count != 1 || p.PairedCycle != "Go-C-C-Go" || p.Warmups != 2 {
		return fmt.Errorf("graduation matrix does not use the required randomized protocol")
	}
	seenSeeds := map[int]bool{}
	for _, seed := range p.Seeds {
		if seed < 0 || seenSeeds[seed] {
			return fmt.Errorf("duplicate or invalid shuffle seed %d", seed)
		}
		seenSeeds[seed] = true
	}
	if err := gatesValid(m.Prerequisites, []string{"E-A_exit", "M1_frozen_baseline", "M1_C_oracle"}); err != nil {
		return err
	}
	seen := map[string]bool{}
	for _, language := range m.Languages {
		if language.Grammar == "" || strings.ToLower(language.Grammar) != language.Grammar || seen[language.Grammar] || len(language.OracleIdentity) == 0 {
			return fmt.Errorf("invalid or duplicate grammar identity %q", language.Grammar)
		}
		seen[language.Grammar] = true
		var oracle struct {
			Contract       string `json:"contract"`
			Grammar        string `json:"language"`
			BindingCommit  string `json:"binding_commit"`
			RuntimeCommit  string `json:"runtime_commit"`
			GrammarCommit  string `json:"grammar_commit"`
			ArtifactSHA256 string `json:"grammar_artifact_sha256"`
		}
		if err := json.Unmarshal(language.OracleIdentity, &oracle); err != nil || oracle.Contract != "tree-sitter-c-v1" || oracle.Grammar != language.Grammar || !digest(oracle.BindingCommit, 20) || !digest(oracle.RuntimeCommit, 20) || !digest(oracle.GrammarCommit, 20) || !digest(oracle.ArtifactSHA256, 32) {
			return fmt.Errorf("%s: locked C oracle identity is incomplete", language.Grammar)
		}
		if err := gatesValid(language.Gates, []string{"locked_real_corpus", "edit_session_72", "edit_sites_16", "cliff", "race", "memory_budget", "highlight_tags"}); err != nil {
			return fmt.Errorf("%s: %w", language.Grammar, err)
		}
		cells := map[string]bool{}
		for _, cell := range language.Cells {
			key := cell.Size + "/" + cell.Operation
			if cells[key] || cell.SourceBytes <= 0 || !digest(cell.SourceSHA256, 32) || !digest(cell.EditSHA256, 32) {
				return fmt.Errorf("%s: invalid or duplicate fixture %s", language.Grammar, key)
			}
			cells[key] = true
			steps := 1
			if cell.Operation == "byte" {
				steps = 4
			}
			for _, engine := range []string{"legacy", "compact"} {
				checks := cell.Correctness[engine]
				if len(checks) != steps || len(cell.Counters[engine]) != 2 {
					return fmt.Errorf("%s/%s/%s: incomplete correctness or counters", language.Grammar, key, engine)
				}
				for step, check := range checks {
					if check.Step != step || !digest(check.SourceSHA256, 32) || !digest(check.GoDigest, 32) || !digest(check.FreshDigest, 32) || !digest(check.CDigest, 32) || check.FreshCEqual != (check.FreshDigest == check.CDigest) || check.IncrementalEqual != (check.GoDigest == check.FreshDigest) {
						return fmt.Errorf("%s/%s/%s: unauthenticated correctness step %d", language.Grammar, key, engine, step)
					}
					wantSource := cell.SourceSHA256
					if cell.Operation == "byte" && step%2 == 0 {
						wantSource = cell.EditSHA256
					}
					if check.SourceSHA256 != wantSource {
						return fmt.Errorf("%s/%s/%s: correctness source differs from timed fixture", language.Grammar, key, engine)
					}
					var completion struct {
						StopReason          string
						SourceLen           int
						RootEndByte         int
						Truncated           bool
						TokenSourceEOFEarly bool
					}
					if err := json.Unmarshal(check.Runtime, &completion); err != nil || completion.StopReason != "accepted" || completion.SourceLen != cell.SourceBytes || completion.RootEndByte != cell.SourceBytes || completion.Truncated || completion.TokenSourceEOFEarly {
						return fmt.Errorf("%s/%s/%s: incomplete parse observation", language.Grammar, key, engine)
					}
				}
				for direction, counter := range cell.Counters[engine] {
					if counter.Direction != direction {
						return fmt.Errorf("%s/%s/%s: counter direction missing", language.Grammar, key, engine)
					}
					if counter.Complete {
						var raw struct {
							PhaseWork *sched.OperationWork `json:"phase_work"`
						}
						if err := json.Unmarshal(counter.RawWork, &raw); err != nil || raw.PhaseWork == nil ||
							!CompleteFrontierPeak(*raw.PhaseWork, counter.MaxVersions) ||
							raw.PhaseWork.Total.Tokens != counter.Tokens || raw.PhaseWork.Total.Nodes != counter.Nodes {
							return fmt.Errorf("%s/%s/%s: counter peak lacks complete phase accounting", language.Grammar, key, engine)
						}
					}
				}
			}
			for _, engine := range []string{"legacy", "compact", "C"} {
				passes := map[int]map[int]bool{}
				for _, sample := range cell.Timing[engine] {
					if !seenSeeds[sample.Seed] || sample.Pass < 0 || sample.Pass >= 6 || !finite(sample.NS) || sample.NS <= 0 || !finite(sample.Bytes) || sample.Bytes < 0 || !finite(sample.Allocs) || sample.Allocs < 0 || !finite(sample.Served) || sample.Served < 0 || sample.Served > 1 || !finite(sample.Declined) || sample.Declined < 0 || sample.Declined > 1 {
						return fmt.Errorf("%s/%s/%s: invalid timing sample", language.Grammar, key, engine)
					}
					if engine == "compact" && cell.Operation == "fresh" && sample.Served+sample.Declined != 1 {
						return fmt.Errorf("%s/%s: missing timed compact admission", language.Grammar, key)
					}
					order := []string{"legacy", "compact", "C", "C", "compact", "legacy"}
					if sample.Seed%2 == 0 {
						order = []string{"compact", "legacy", "C", "C", "legacy", "compact"}
					}
					if order[sample.Pass] != engine {
						return fmt.Errorf("%s/%s/%s: timing pass breaks Go-C-C-Go order", language.Grammar, key, engine)
					}
					if passes[sample.Seed] == nil {
						passes[sample.Seed] = map[int]bool{}
					}
					if passes[sample.Seed][sample.Pass] {
						return fmt.Errorf("%s/%s/%s: duplicate timing pass", language.Grammar, key, engine)
					}
					passes[sample.Seed][sample.Pass] = true
				}
				if len(passes) != 20 {
					return fmt.Errorf("%s/%s/%s: requires 20 completed seeds", language.Grammar, key, engine)
				}
				for seed, observed := range passes {
					if len(observed) != 2 {
						return fmt.Errorf("%s/%s/%s: seed %d is not paired", language.Grammar, key, engine, seed)
					}
				}
			}
		}
		for _, size := range []string{"32k", "137k", "1m"} {
			for _, operation := range []string{"fresh", "byte"} {
				if !cells[size+"/"+operation] {
					return fmt.Errorf("%s: required fixture %s/%s missing", language.Grammar, size, operation)
				}
			}
		}
		if len(cells) != 6 {
			return fmt.Errorf("%s: unexpected fixture cells", language.Grammar)
		}
		rssCells := map[string]bool{}
		for _, rss := range language.RSS {
			key := rss.Engine + "/" + rss.Operation
			if rssCells[key] || (rss.Engine != "legacy" && rss.Engine != "compact" && rss.Engine != "C") || (rss.Operation != "fresh" && rss.Operation != "byte") || len(rss.Samples) != 3 || len(rss.ProbeSamples) != 3 || rss.SourceBytes < 1<<20 {
				return fmt.Errorf("%s: invalid or missing repeated RSS cell %s", language.Grammar, key)
			}
			for _, cell := range language.Cells {
				if cell.Size == "1m" && cell.Operation == rss.Operation && cell.SourceBytes != rss.SourceBytes {
					return fmt.Errorf("%s/%s: RSS source differs from the timed fixture", language.Grammar, key)
				}
			}
			for i, sample := range rss.Samples {
				if !finite(sample) || sample <= 0 || !finite(rss.ProbeSamples[i]) || rss.ProbeSamples[i] <= 0 || rss.ProbeSamples[i] > sample {
					return fmt.Errorf("%s/%s: invalid RSS sample", language.Grammar, key)
				}
			}
			rssCells[key] = true
		}
		if len(rssCells) != 6 {
			return fmt.Errorf("%s: missing RSS route or operation", language.Grammar)
		}
		blockers := m.Blockers(language)
		if language.Graduated != (len(blockers) == 0) || strings.Join(language.Blockers, "\n") != strings.Join(blockers, "\n") {
			return fmt.Errorf("%s: graduation decision or blocker list disagrees with measurements", language.Grammar)
		}
	}
	return nil
}

func gatesValid(gates []Gate, required []string) error {
	seen := map[string]bool{}
	for _, gate := range gates {
		if gate.ID == "" || seen[gate.ID] || gate.Evidence == "" || (gate.Status != "passed" && gate.Status != "failed" && gate.Status != "not_established") {
			return fmt.Errorf("invalid or duplicate graduation gate %q", gate.ID)
		}
		seen[gate.ID] = true
	}
	for _, name := range required {
		if !seen[name] {
			return fmt.Errorf("required graduation gate %s missing", name)
		}
	}
	return nil
}

func finite(value float64) bool { return !math.IsNaN(value) && !math.IsInf(value, 0) }

func median(values []float64) float64 {
	if len(values) == 0 {
		return math.Inf(1)
	}
	values = append([]float64(nil), values...)
	sort.Float64s(values)
	n := len(values)
	if n%2 == 0 {
		return (values[n/2-1] + values[n/2]) / 2
	}
	return values[n/2]
}

func ratios(a, b []Sample) []float64 {
	left, right := map[int][]float64{}, map[int][]float64{}
	for _, sample := range a {
		left[sample.Seed] = append(left[sample.Seed], sample.NS)
	}
	for _, sample := range b {
		right[sample.Seed] = append(right[sample.Seed], sample.NS)
	}
	var result []float64
	for seed, samples := range left {
		if len(samples) == 2 && len(right[seed]) == 2 {
			result = append(result, median(samples)/median(right[seed]))
		}
	}
	return result
}

func metricMedian(samples []Sample, allocations bool) float64 {
	seeds := map[int][]float64{}
	for _, sample := range samples {
		value := sample.Bytes
		if allocations {
			value = sample.Allocs
		}
		seeds[sample.Seed] = append(seeds[sample.Seed], value)
	}
	var values []float64
	for _, seed := range seeds {
		values = append(values, median(seed))
	}
	return median(values)
}

// Blockers derives decisions from raw samples, not the display summaries.
// The owner's correctness alternative needs an actual C witness that compact
// fixes and legacy fails; an asserted exception string cannot qualify it.
func (m *Matrix) Blockers(language Language) []string {
	blocked := map[string]bool{}
	for _, gate := range append(append([]Gate(nil), m.Prerequisites...), language.Gates...) {
		if gate.Status != "passed" {
			blocked[gate.ID+": "+gate.Status] = true
		}
	}
	beatsLegacy, improvesLegacy, fixesLegacy := true, false, false
	for _, cell := range language.Cells {
		label := cell.Size + "/" + cell.Operation
		for i, check := range cell.Correctness["compact"] {
			if !check.FreshCEqual || !check.IncrementalEqual || !check.HasErrorEqual || !check.Clean || check.NoEditAllocations != 0 {
				blocked[label+": correctness"] = true
			}
			if !check.InitialServed || !check.RequestedServed {
				blocked[label+": compact decline"] = true
			}
			legacy := cell.Correctness["legacy"]
			if i < len(legacy) && check.RequestedServed && check.GoDigest == check.CDigest && legacy[i].GoDigest != legacy[i].CDigest {
				fixesLegacy = true
			}
		}
		legacyRatio := median(ratios(cell.Timing["compact"], cell.Timing["legacy"]))
		if legacyRatio > 1 {
			beatsLegacy = false
		}
		if legacyRatio < 1 {
			improvesLegacy = true
		}
		if median(ratios(cell.Timing["compact"], cell.Timing["C"])) > 10 {
			blocked[label+": Go/C above 10x"] = true
		}
		if cell.Operation == "fresh" {
			for _, sample := range cell.Timing["compact"] {
				if sample.Served != 1 || sample.Declined != 0 {
					blocked[label+": timed compact decline"] = true
				}
			}
		}
		for _, allocations := range []bool{false, true} {
			if metricMedian(cell.Timing["compact"], allocations) > metricMedian(cell.Timing["legacy"], allocations)*1.1 {
				blocked[label+": allocation ratchet above 10%"] = true
			}
		}
		for direction, compact := range cell.Counters["compact"] {
			legacy := cell.Counters["legacy"]
			if direction >= len(legacy) {
				blocked[label+": missing legacy counters"] = true
				continue
			}
			old := legacy[direction]
			if !compact.Complete || !old.Complete {
				blocked[label+": complete work counters not established"] = true
			}
			for _, work := range [][2]uint64{{old.WholeLookups, compact.WholeLookups}, {old.Tokens, compact.Tokens}, {old.Nodes, compact.Nodes}, {old.MaxVersions, compact.MaxVersions}} {
				if float64(work[1]) > float64(work[0])*1.02 {
					blocked[label+": work ledger above 2%"] = true
				}
			}
			for _, reuse := range [][2]uint64{{old.ReusedSubtrees, compact.ReusedSubtrees}, {old.ReusedBytes, compact.ReusedBytes}} {
				if float64(reuse[1]) < float64(reuse[0])*0.98 {
					blocked[label+": reuse ledger below -2%"] = true
				}
			}
			if cell.Operation == "byte" && (compact.ReusedSubtrees == 0 || compact.ReusedBytes == 0) {
				blocked[label+": no positive compact reuse"] = true
			}
		}
	}
	if (!beatsLegacy || !improvesLegacy) && !fixesLegacy {
		blocked["owner policy: no whole-operation win or C correctness fix in measured cells"] = true
	}
	for _, operation := range []string{"fresh", "byte"} {
		var compact, legacy []float64
		for _, row := range language.RSS {
			if row.Operation != operation {
				continue
			}
			if row.Engine == "compact" {
				compact = row.Samples
				for _, sample := range row.Samples {
					if row.SourceBytes <= 0 || sample > float64(row.SourceBytes)*400 {
						blocked["1m/"+operation+": RSS above 400 bytes/source byte"] = true
					}
				}
			}
			if row.Engine == "legacy" {
				legacy = row.Samples
			}
		}
		if len(compact) < 3 || len(legacy) < 3 {
			blocked["1m/"+operation+": repeated RSS not established"] = true
		} else if median(compact) > median(legacy) {
			blocked["1m/"+operation+": RSS above legacy"] = true
		}
	}
	var result []string
	for reason := range blocked {
		result = append(result, reason)
	}
	sort.Strings(result)
	return result
}

// Graduated returns the only names eligible for implicit compact routing.
func (m *Matrix) Graduated() []string {
	names := []string{}
	for _, language := range m.Languages {
		if len(m.Blockers(language)) == 0 {
			names = append(names, language.Grammar)
		}
	}
	sort.Strings(names)
	return names
}
