package grammargen

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/odvcencio/gotreesitter"
)

const (
	realCorpusEnableEnv         = "GTS_GRAMMARGEN_REAL_CORPUS_ENABLE"
	realCorpusRootEnv           = "GTS_GRAMMARGEN_REAL_CORPUS_ROOT"
	realCorpusProfileEnv        = "GTS_GRAMMARGEN_REAL_CORPUS_PROFILE"
	realCorpusMaxCasesEnv       = "GTS_GRAMMARGEN_REAL_CORPUS_MAX_CASES"
	realCorpusMaxSampleBytesEnv = "GTS_GRAMMARGEN_REAL_CORPUS_MAX_SAMPLE_BYTES"
	realCorpusCandidateMultEnv  = "GTS_GRAMMARGEN_REAL_CORPUS_CANDIDATE_MULTIPLIER"
	realCorpusMaxSecsPerGramEnv = "GTS_GRAMMARGEN_REAL_CORPUS_MAX_SECONDS_PER_GRAMMAR"
	realCorpusMaxGrammarsEnv    = "GTS_GRAMMARGEN_REAL_CORPUS_MAX_GRAMMARS"
	realCorpusRequireParityEnv  = "GTS_GRAMMARGEN_REAL_CORPUS_REQUIRE_PARITY"
	realCorpusRatchetUpdateEnv  = "GTS_GRAMMARGEN_REAL_CORPUS_RATCHET_UPDATE"
	realCorpusRatchetRebaseEnv  = "GTS_GRAMMARGEN_REAL_CORPUS_RATCHET_REBASE"
	realCorpusFloorsPathEnv     = "GTS_GRAMMARGEN_REAL_CORPUS_FLOORS_PATH"
	realCorpusAllowPartialEnv   = "GTS_GRAMMARGEN_REAL_CORPUS_ALLOW_PARTIAL"
	realCorpusSkipEnv           = "GTS_GRAMMARGEN_REAL_CORPUS_SKIP"
	realCorpusOnlyEnv           = "GTS_GRAMMARGEN_REAL_CORPUS_ONLY"
	realCorpusFloorsFileVersion = 3
	maxRealCorpusWalkFiles      = 6000
)

type realCorpusMetrics struct {
	Eligible    int `json:"eligible"`
	NoError     int `json:"no_error"`
	SExprParity int `json:"sexpr_parity"`
	DeepParity  int `json:"deep_parity"`
}

type realCorpusFloorFile struct {
	Version       int                          `json:"version"`
	GeneratedAt   string                       `json:"generated_at"`
	CommitSHA     string                       `json:"commit_sha,omitempty"`
	CorpusRoot    string                       `json:"corpus_root"`
	Profile       string                       `json:"profile"`
	MaxCases      int                          `json:"max_cases"`
	MaxSampleB    int                          `json:"max_sample_bytes"`
	GrammarCount  int                          `json:"grammar_count"`
	TotalEligible int                          `json:"total_eligible"`
	TotalNoError  int                          `json:"total_no_error"`
	TotalSExpr    int                          `json:"total_sexpr_parity"`
	TotalDeep     int                          `json:"total_deep_parity"`
	Metrics       map[string]realCorpusMetrics `json:"metrics"`
}

type realCorpusProfile string

const (
	realCorpusProfileSmoke      realCorpusProfile = "smoke"
	realCorpusProfileBalanced   realCorpusProfile = "balanced"
	realCorpusProfileAggressive realCorpusProfile = "aggressive"
)

type realCorpusSampleSource string

const (
	realCorpusSourceCorpusBlock realCorpusSampleSource = "corpus_block"
	realCorpusSourceCorpusRaw   realCorpusSampleSource = "corpus_raw"
	realCorpusSourceRepoRaw     realCorpusSampleSource = "repo_raw"
)

type realCorpusSampleCandidate struct {
	Text   string
	Trim   string
	Size   int
	Source realCorpusSampleSource
	Path   string
}

type realCorpusCollectConfig struct {
	TargetEligible      int
	MaxSampleBytes      int
	CandidateMultiplier int
	Profile             realCorpusProfile
}

func TestMultiGrammarImportRealCorpusParity(t *testing.T) {
	if !getenvBool(realCorpusEnableEnv) {
		t.Skipf("set %s=1 to enable real-corpus grammargen parity checks", realCorpusEnableEnv)
	}

	root := strings.TrimSpace(os.Getenv(realCorpusRootEnv))
	if root == "" {
		root = "/tmp/grammar_parity"
	}
	if _, err := os.Stat(root); err != nil {
		t.Skipf("real corpus root unavailable: %s (%v)", root, err)
	}

	profile := parseRealCorpusProfile(os.Getenv(realCorpusProfileEnv))
	maxCases := getenvInt(realCorpusMaxCasesEnv, defaultMaxCasesForProfile(profile))
	maxSampleBytes := getenvInt(realCorpusMaxSampleBytesEnv, defaultMaxSampleBytesForProfile(profile))
	candidateMult := getenvInt(realCorpusCandidateMultEnv, defaultCandidateMultiplierForProfile(profile))
	maxSecsPerGrammar := getenvInt(realCorpusMaxSecsPerGramEnv, defaultMaxSecondsPerGrammar(profile))
	maxGrammars := getenvInt(realCorpusMaxGrammarsEnv, 0)
	requireParity := getenvBool(realCorpusRequireParityEnv)
	updateRatchet := getenvBool(realCorpusRatchetUpdateEnv)
	rebaseRatchet := getenvBool(realCorpusRatchetRebaseEnv)
	allowPartial := getenvBool(realCorpusAllowPartialEnv)
	collectCfg := realCorpusCollectConfig{
		TargetEligible:      maxCases,
		MaxSampleBytes:      maxSampleBytes,
		CandidateMultiplier: candidateMult,
		Profile:             profile,
	}

	floorsPath := strings.TrimSpace(os.Getenv(realCorpusFloorsPathEnv))
	if floorsPath == "" {
		floorsPath = defaultRealCorpusFloorsPath()
	}
	floorFile, foundFloors, err := loadRealCorpusFloorFile(floorsPath)
	if err != nil {
		t.Fatalf("load floor file %s: %v", floorsPath, err)
	}
	if floorFile.Metrics == nil {
		floorFile.Metrics = map[string]realCorpusMetrics{}
	}
	if !updateRatchet && foundFloors {
		if floorFile.MaxCases > 0 && maxCases < floorFile.MaxCases {
			t.Fatalf("max cases %d is below ratchet floor file max_cases=%d; increase %s or regenerate floors", maxCases, floorFile.MaxCases, realCorpusMaxCasesEnv)
		}
		if floorFile.MaxSampleB > 0 && maxSampleBytes < floorFile.MaxSampleB {
			t.Fatalf("max sample bytes %d is below ratchet floor file max_sample_bytes=%d; increase %s or regenerate floors", maxSampleBytes, floorFile.MaxSampleB, realCorpusMaxSampleBytesEnv)
		}
		floorProfile := floorProfileOrSmoke(floorFile.Profile)
		if profileStrength(profile) < profileStrength(floorProfile) {
			t.Fatalf("profile %q is weaker than ratchet floor profile %q; set %s=%s (or stronger) or regenerate floors",
				profile, floorProfile, realCorpusProfileEnv, floorProfile)
		}
	}

	skipSet := map[string]bool{}
	if raw := strings.TrimSpace(os.Getenv(realCorpusSkipEnv)); raw != "" {
		for _, s := range strings.Split(raw, ",") {
			if s = strings.TrimSpace(s); s != "" {
				skipSet[s] = true
			}
		}
	}
	onlySet := map[string]bool{}
	if raw := strings.TrimSpace(os.Getenv(realCorpusOnlyEnv)); raw != "" {
		for _, s := range strings.Split(raw, ",") {
			if s = strings.TrimSpace(s); s != "" {
				onlySet[s] = true
			}
		}
	}

	testedGrammars := 0
	totalEligible := 0
	totalNoError := 0
	totalSExprParity := 0
	totalDeepParity := 0
	observed := map[string]realCorpusMetrics{}

	for _, g := range importParityGrammars {
		if skipSet[g.name] {
			continue
		}
		if len(onlySet) > 0 && !onlySet[g.name] {
			continue
		}
		if maxGrammars > 0 && testedGrammars >= maxGrammars {
			break
		}

		repoRoot := parityGrammarRepoRoot(g, root)
		if repoRoot == "" {
			continue
		}
		candidates := collectGrammarCorpusCandidates(t, repoRoot, collectCfg)
		if len(candidates) == 0 {
			continue
		}

		testedGrammars++
		g := g
		t.Run(g.name, func(t *testing.T) {
			gram, err := importParityGrammarSource(g)
			if err != nil {
				t.Fatalf("import failed: %v", err)
			}

			if getenvBool("GTS_GRAMMARGEN_LR_SPLIT") {
				gram.EnableLRSplitting = true
			}

			timeout := g.genTimeout
			if timeout == 0 {
				timeout = 30 * time.Second
			}
			genLang, err := generateWithTimeout(gram, timeout)
			if err != nil {
				t.Fatalf("generate failed: %v", err)
			}
			refLang := g.blobFunc()
			adaptExternalScanner(refLang, genLang)

			genParser := gotreesitter.NewParser(genLang)
			refParser := gotreesitter.NewParser(refLang)

			// Log root symbol inference for diagnostics.
			if genRootSym, genHasRoot := genParser.InferredRootSymbol(); genHasRoot {
				genRootName := ""
				if int(genRootSym) < len(genLang.SymbolNames) {
					genRootName = genLang.SymbolNames[genRootSym]
				}
				t.Logf("root-diag: gen inferredRoot=sym%d(%q) hasRoot=true", genRootSym, genRootName)
			} else {
				t.Logf("root-diag: gen hasRoot=false symCount=%d tokenCount=%d initialState=%d",
					genLang.SymbolCount, genLang.TokenCount, genLang.InitialState)
			}

			metrics := realCorpusMetrics{}
			mismatchLogs := 0
			divCategoryCounts := map[string]int{}
			seen := 0
			grammarDeadline := time.Time{}
			if maxSecsPerGrammar > 0 {
				grammarDeadline = time.Now().Add(time.Duration(maxSecsPerGrammar) * time.Second)
			}

			for i, cand := range candidates {
				if maxSecsPerGrammar > 0 && time.Now().After(grammarDeadline) && metrics.Eligible > 0 {
					t.Logf("real-corpus: stopping early at sample %d due grammar time budget (%ds)", i, maxSecsPerGrammar)
					break
				}
				seen++
				genTree, _ := genParser.Parse([]byte(cand.Text))
				refTree, _ := refParser.Parse([]byte(cand.Text))

				genRoot := genTree.RootNode()
				refRoot := refTree.RootNode()

				// Safety: skip samples with pathologically deep parse trees
				// (e.g. HCL's 189K-deep recursive tree) to prevent goroutine
				// stack overflow during SExpr serialization. Use safeSExpr
				// which limits recursion depth; an empty return signals truncation.
				const maxSafeDepth = 2000
				genSexp := safeSExpr(genRoot, genLang, maxSafeDepth)
				refSexp := safeSExpr(refRoot, refLang, maxSafeDepth)
				if genSexp == "" && refSexp == "" {
					t.Logf("real-corpus: skipping sample %d (%s:%s) — tree too deep to serialize",
						i, cand.Source, cand.Path)
					continue
				}

				refHasError := strings.Contains(refSexp, "ERROR") || strings.Contains(refSexp, "MISSING")
				if refHasError {
					continue
				}
				metrics.Eligible++

				genHasError := strings.Contains(genSexp, "ERROR") || strings.Contains(genSexp, "MISSING")
				if genHasError {
					if mismatchLogs < 25 {
						mismatchLogs++
						t.Logf("sample %d (%s:%s) gen ERROR on clean ref: %s",
							i, cand.Source, cand.Path,
							genSexp[:min(len(genSexp), 200)])
					}
					continue
				}
				metrics.NoError++

				sexprMatch := genSexp == refSexp
				// Normalize unicode escapes: ts2go blobs may have \uXXXX
				// in symbol names from C source, grammargen uses UTF-8.
				if !sexprMatch && strings.Contains(refSexp, `\u`) {
					sexprMatch = unescapeUnicodeInType(genSexp) == unescapeUnicodeInType(refSexp)
				}
				if !sexprMatch {
					refRootType := refRoot.Type(refLang)
					genRootType := genRoot.Type(genLang)
					// When the reference root type is empty or the ref root
					// is error (ts2go extraction issue), the SExprs may differ
					// only in the root node name. Normalize by stripping the
					// root wrapper from both and comparing inner content.
					if (refRootType == "" || refRoot.IsError()) && genRootType != "" {
						genInner := stripSExprRoot(genSexp)
						refInner := stripSExprRoot(refSexp)
						if genInner != "" && genInner == refInner {
							sexprMatch = true
						}
					}
					// When ref root is unnamed (ts2go metadata issue),
					// SExpr returns "" for it. Reconstruct the ref
					// SExpr from its children using the gen root type,
					// matching deep comparison's leniency at root level.
					if !sexprMatch && genRoot.IsNamed() && !refRoot.IsNamed() && genRootType != "" {
						reconstructed := rebuildRootSExpr(refRoot, refLang, genRootType)
						if genSexp == reconstructed {
							sexprMatch = true
						}
					}
				}
				divs := compareTreesDeep(genRoot, genLang, refRoot, refLang, "root", 10)
				if len(divs) == 0 {
					metrics.DeepParity++
					// Deep parity is stricter than SExpr parity (checks
					// types, ranges, childCounts, named at every node).
					// If deep passes, count SExpr as matching even if
					// the SExpr strings differ due to Language metadata
					// differences (visibility, named) between gen/ref.
					sexprMatch = true
				} else {
					// Always record div categories regardless of sexpr status,
					// so the summary captures deep-only failures too.
					divCategoryCounts[divs[0].Category]++
				}
				if sexprMatch {
					metrics.SExprParity++
				}
				if len(divs) > 0 {
					if !sexprMatch && requireParity {
						t.Fatalf("sample %d (%s:%s) deep parity mismatch: %s\nGEN: %s\nREF: %s",
							i, cand.Source, cand.Path, divs[0].String(), genSexp, refSexp)
					} else if mismatchLogs < 25 {
						mismatchLogs++
						t.Logf("sample %d (%s:%s) deep mismatch: %s", i, cand.Source, cand.Path, divs[0].String())
						if divs[0].Category == "type" && (divs[0].GenValue == "" || divs[0].RefValue == "") {
							t.Logf("  root-diag: genRoot.symbol=%d genRoot.Type=%q refRoot.symbol=%d refRoot.Type=%q genSymNames=%d",
								genRoot.Symbol(), genRoot.Type(genLang), refRoot.Symbol(), refRoot.Type(refLang), len(genLang.SymbolNames))
						}
						if divs[0].Category == "childCount" {
							logChildCountDiag(t, divs[0], genRoot, refRoot, genLang, refLang)
						}
					}
				}
				if metrics.Eligible >= maxCases {
					break
				}
			}

			if metrics.Eligible == 0 {
				t.Skip("no clean reference samples in extracted corpus set")
			}

			if !updateRatchet && len(floorFile.Metrics) > 0 {
				floor, ok := floorFile.Metrics[g.name]
				if !ok {
					t.Errorf("missing ratchet floor for grammar %q in %s", g.name, floorsPath)
				} else {
					enforceRealCorpusRatchet(t, floor, metrics)
				}
			}

			observed[g.name] = metrics
			totalEligible += metrics.Eligible
			totalNoError += metrics.NoError
			totalSExprParity += metrics.SExprParity
			totalDeepParity += metrics.DeepParity

			// When childCount divergences are the dominant issue, dump
			// ChildCount comparison of parse actions between gen and ref.
			if ccCount := divCategoryCounts["childCount"]; ccCount > 0 && ccCount >= divCategoryCounts["type"] {
				logReduceActionDiff(t, g.name, genLang, refLang, 10)
			}

			divSummary := ""
			if len(divCategoryCounts) > 0 {
				parts := make([]string, 0, len(divCategoryCounts))
				for cat, cnt := range divCategoryCounts {
					parts = append(parts, fmt.Sprintf("%s=%d", cat, cnt))
				}
				sort.Strings(parts)
				divSummary = " divs=[" + strings.Join(parts, ",") + "]"
			}
			t.Logf("real-corpus[%s]: no-error %d/%d, sexpr parity %d/%d, deep parity %d/%d%s (requireParity=%v, seen=%d/%d)",
				profile,
				metrics.NoError, metrics.Eligible,
				metrics.SExprParity, metrics.Eligible,
				metrics.DeepParity, metrics.Eligible,
				divSummary,
				requireParity, seen, len(candidates))
		})
	}

	if testedGrammars == 0 {
		t.Skipf("no grammar repos with corpus samples found under %s", root)
	}

	if !updateRatchet && len(floorFile.Metrics) > 0 && !allowPartial {
		for grammarName := range floorFile.Metrics {
			if _, ok := observed[grammarName]; !ok {
				t.Errorf("ratchet floor grammar %q not exercised in this run (set %s=1 to allow partial runs)", grammarName, realCorpusAllowPartialEnv)
			}
		}
	}

	if updateRatchet {
		if rebaseRatchet {
			hasStaleFloors := false
			for grammarName := range floorFile.Metrics {
				if _, ok := observed[grammarName]; !ok {
					hasStaleFloors = true
					break
				}
			}
			if len(floorFile.Metrics) > 0 &&
				!hasStaleFloors &&
				profileStrength(profile) <= profileStrength(floorProfileOrSmoke(floorFile.Profile)) &&
				maxCases <= floorFile.MaxCases &&
				(maxSampleBytes <= floorFile.MaxSampleB || floorFile.MaxSampleB == 0) {
				t.Fatalf("ratchet rebase requested but run is not stronger than existing floor config and has no stale floor entries to prune; increase profile/maxCases/maxSampleBytes")
			}
			rebased := make(map[string]realCorpusMetrics, len(observed))
			for grammarName, cur := range observed {
				rebased[grammarName] = cur
			}
			floorFile.Metrics = rebased
		} else {
			for grammarName, cur := range observed {
				if prev, ok := floorFile.Metrics[grammarName]; ok {
					if cur.Eligible < prev.Eligible {
						t.Fatalf("ratchet update would decrease eligible floor for %s: prev=%+v new=%+v", grammarName, prev, cur)
					}
					if cur.NoError*prev.Eligible < prev.NoError*cur.Eligible ||
						cur.SExprParity*prev.Eligible < prev.SExprParity*cur.Eligible ||
						cur.DeepParity*prev.Eligible < prev.DeepParity*cur.Eligible {
						t.Fatalf("ratchet update would regress ratios for %s: prev=%+v new=%+v", grammarName, prev, cur)
					}
				}
				floorFile.Metrics[grammarName] = cur
			}
		}
		floorFile.Version = realCorpusFloorsFileVersion
		floorFile.GeneratedAt = time.Now().UTC().Format(time.RFC3339)
		floorFile.CommitSHA = gitHeadShortSHA(12)
		floorFile.CorpusRoot = root
		floorFile.Profile = string(profile)
		floorFile.MaxCases = maxCases
		floorFile.MaxSampleB = maxSampleBytes
		floorFile.GrammarCount = testedGrammars
		floorFile.TotalEligible = totalEligible
		floorFile.TotalNoError = totalNoError
		floorFile.TotalSExpr = totalSExprParity
		floorFile.TotalDeep = totalDeepParity
		if err := writeRealCorpusFloorFile(floorsPath, floorFile); err != nil {
			t.Fatalf("write floor file %s: %v", floorsPath, err)
		}
		t.Logf("updated ratchet floor file: %s", floorsPath)
	}

	t.Logf("REAL CORPUS SUMMARY: profile=%s grammars=%d eligible=%d no-error=%d sexpr_parity=%d deep_parity=%d requireParity=%v ratchetUpdate=%v ratchetRebase=%v maxCases=%d maxSampleBytes=%d",
		profile, testedGrammars, totalEligible, totalNoError, totalSExprParity, totalDeepParity,
		requireParity, updateRatchet, rebaseRatchet, maxCases, maxSampleBytes)
}

func enforceRealCorpusRatchet(t *testing.T, floor, cur realCorpusMetrics) {
	t.Helper()
	if cur.Eligible < floor.Eligible {
		t.Errorf("ratchet regression eligible: %d < floor %d", cur.Eligible, floor.Eligible)
	}
	if cur.NoError < floor.NoError {
		t.Errorf("ratchet regression no-error: %d < floor %d", cur.NoError, floor.NoError)
	}
	if cur.SExprParity < floor.SExprParity {
		t.Errorf("ratchet regression sexpr parity: %d < floor %d", cur.SExprParity, floor.SExprParity)
	}
	if cur.DeepParity < floor.DeepParity {
		t.Errorf("ratchet regression deep parity: %d < floor %d", cur.DeepParity, floor.DeepParity)
	}
	if floor.Eligible > 0 && cur.Eligible > 0 {
		if cur.NoError*floor.Eligible < floor.NoError*cur.Eligible {
			t.Errorf("ratchet regression no-error ratio: %d/%d < floor %d/%d", cur.NoError, cur.Eligible, floor.NoError, floor.Eligible)
		}
		if cur.SExprParity*floor.Eligible < floor.SExprParity*cur.Eligible {
			t.Errorf("ratchet regression sexpr parity ratio: %d/%d < floor %d/%d", cur.SExprParity, cur.Eligible, floor.SExprParity, floor.Eligible)
		}
		if cur.DeepParity*floor.Eligible < floor.DeepParity*cur.Eligible {
			t.Errorf("ratchet regression deep parity ratio: %d/%d < floor %d/%d", cur.DeepParity, cur.Eligible, floor.DeepParity, floor.Eligible)
		}
	}
}

func parseRealCorpusProfile(raw string) realCorpusProfile {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", string(realCorpusProfileAggressive):
		return realCorpusProfileAggressive
	case string(realCorpusProfileSmoke):
		return realCorpusProfileSmoke
	case string(realCorpusProfileBalanced):
		return realCorpusProfileBalanced
	default:
		return realCorpusProfileAggressive
	}
}

func floorProfileOrSmoke(raw string) realCorpusProfile {
	if strings.TrimSpace(raw) == "" {
		return realCorpusProfileSmoke
	}
	return parseRealCorpusProfile(raw)
}

func profileStrength(p realCorpusProfile) int {
	switch p {
	case realCorpusProfileSmoke:
		return 1
	case realCorpusProfileBalanced:
		return 2
	default:
		return 3
	}
}

func defaultMaxCasesForProfile(p realCorpusProfile) int {
	switch p {
	case realCorpusProfileSmoke:
		return 8
	case realCorpusProfileBalanced:
		return 16
	default:
		return 24
	}
}

func defaultMaxSampleBytesForProfile(p realCorpusProfile) int {
	switch p {
	case realCorpusProfileSmoke:
		return 64 * 1024
	case realCorpusProfileBalanced:
		return 256 * 1024
	default:
		return 512 * 1024
	}
}

func defaultCandidateMultiplierForProfile(p realCorpusProfile) int {
	switch p {
	case realCorpusProfileSmoke:
		return 6
	case realCorpusProfileBalanced:
		return 10
	default:
		return 14
	}
}

func defaultMaxSecondsPerGrammar(p realCorpusProfile) int {
	// Unlimited by default for deterministic ratchet behavior. Set
	// GTS_GRAMMARGEN_REAL_CORPUS_MAX_SECONDS_PER_GRAMMAR to cap runtime in
	// exploratory runs.
	_ = p
	return 0
}

func defaultRealCorpusFloorsPath() string {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		return "grammargen/testdata/real_corpus_parity_floors.json"
	}
	return filepath.Join(filepath.Dir(file), "testdata", "real_corpus_parity_floors.json")
}

func loadRealCorpusFloorFile(path string) (realCorpusFloorFile, bool, error) {
	var out realCorpusFloorFile
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			out.Version = realCorpusFloorsFileVersion
			out.Metrics = map[string]realCorpusMetrics{}
			return out, false, nil
		}
		return out, false, err
	}
	if err := json.Unmarshal(data, &out); err != nil {
		return out, false, err
	}
	if out.Metrics == nil {
		out.Metrics = map[string]realCorpusMetrics{}
	}
	if out.Version == 0 {
		out.Version = realCorpusFloorsFileVersion
	}
	return out, true, nil
}

func writeRealCorpusFloorFile(path string, f realCorpusFloorFile) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, 0o644)
}

func gitHeadShortSHA(n int) string {
	if n <= 0 {
		n = 12
	}
	out, err := exec.Command("git", "rev-parse", "--short="+strconv.Itoa(n), "HEAD").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func parityGrammarRepoRoot(g importParityGrammar, root string) string {
	for _, p := range []string{g.jsonPath, g.path} {
		if p == "" {
			continue
		}
		rel, err := filepath.Rel(root, p)
		if err != nil {
			continue
		}
		rel = filepath.ToSlash(rel)
		if strings.HasPrefix(rel, "../") || rel == ".." {
			continue
		}
		parts := strings.Split(rel, "/")
		if len(parts) == 0 || parts[0] == "." || parts[0] == "" {
			continue
		}
		repoRoot := filepath.Join(root, parts[0])
		if info, statErr := os.Stat(repoRoot); statErr == nil && info.IsDir() {
			return repoRoot
		}
	}
	return ""
}

func importParityGrammarSource(g importParityGrammar) (*Grammar, error) {
	if g.jsonPath != "" {
		source, err := os.ReadFile(g.jsonPath)
		if err != nil {
			return nil, fmt.Errorf("read grammar.json: %w", err)
		}
		return ImportGrammarJSON(source)
	}
	source, err := os.ReadFile(g.path)
	if err != nil {
		return nil, fmt.Errorf("read grammar.js: %w", err)
	}
	return ImportGrammarJS(source)
}

func collectGrammarCorpusCandidates(t *testing.T, repoRoot string, cfg realCorpusCollectConfig) []realCorpusSampleCandidate {
	t.Helper()
	if cfg.TargetEligible <= 0 {
		cfg.TargetEligible = defaultMaxCasesForProfile(cfg.Profile)
	}
	if cfg.MaxSampleBytes <= 0 {
		cfg.MaxSampleBytes = defaultMaxSampleBytesForProfile(cfg.Profile)
	}
	if cfg.CandidateMultiplier <= 0 {
		cfg.CandidateMultiplier = defaultCandidateMultiplierForProfile(cfg.Profile)
	}

	corpusDirs := existingCorpusDirs(repoRoot)
	rawDirs := existingGrammarSampleDirs(repoRoot, corpusDirs)

	corpusFiles := walkSampleFiles(corpusDirs)
	rawFiles := walkSampleFiles(rawDirs)

	seen := map[string]struct{}{}
	cands := make([]realCorpusSampleCandidate, 0, cfg.TargetEligible*cfg.CandidateMultiplier)
	for _, path := range corpusFiles {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		extracted := extractTreeSitterCorpusInputs(data)
		for _, sample := range extracted {
			if c, ok := newCandidate(sample, path, realCorpusSourceCorpusBlock, cfg.MaxSampleBytes, seen); ok {
				cands = append(cands, c)
			}
		}
		// Only treat corpus file as raw input when no corpus block could be extracted.
		if len(extracted) == 0 {
			if c, ok := newCandidate(string(data), path, realCorpusSourceCorpusRaw, cfg.MaxSampleBytes, seen); ok {
				cands = append(cands, c)
			}
		}
	}
	for _, path := range rawFiles {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		if c, ok := newCandidate(string(data), path, realCorpusSourceRepoRaw, cfg.MaxSampleBytes, seen); ok {
			cands = append(cands, c)
		}
	}
	if len(cands) == 0 {
		return nil
	}

	limit := cfg.TargetEligible * cfg.CandidateMultiplier
	sortRealCorpusCandidates(cands, cfg.Profile)
	if len(cands) > limit {
		cands = cands[:limit]
	}
	return cands
}

func existingCorpusDirs(repoRoot string) []string {
	return existingDirs([]string{
		filepath.Join(repoRoot, "test", "corpus"),
		filepath.Join(repoRoot, "tests", "corpus"),
		filepath.Join(repoRoot, "corpus"),
	})
}

func existingGrammarSampleDirs(repoRoot string, corpusDirs []string) []string {
	candidates := []string{
		filepath.Join(repoRoot, "examples"),
		filepath.Join(repoRoot, "example"),
		filepath.Join(repoRoot, "samples"),
		filepath.Join(repoRoot, "sample"),
		filepath.Join(repoRoot, "fixtures"),
		filepath.Join(repoRoot, "test", "highlight"),
		filepath.Join(repoRoot, "tests", "highlight"),
		filepath.Join(repoRoot, "test", "highlights"),
		filepath.Join(repoRoot, "tests", "highlights"),
		filepath.Join(repoRoot, "test", "fixtures"),
		filepath.Join(repoRoot, "tests", "fixtures"),
	}
	return existingDirs(candidates)
}

func existingDirs(dirs []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(dirs))
	for _, dir := range dirs {
		if _, ok := seen[dir]; ok {
			continue
		}
		seen[dir] = struct{}{}
		info, err := os.Stat(dir)
		if err != nil || !info.IsDir() {
			continue
		}
		out = append(out, dir)
	}
	return out
}

func walkSampleFiles(dirs []string) []string {
	out := make([]string, 0, 256)
	for _, dir := range dirs {
		scanned := 0
		_ = filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			if d.IsDir() {
				name := d.Name()
				if name == ".git" || name == "node_modules" || name == "target" || name == "vendor" {
					return filepath.SkipDir
				}
				return nil
			}
			scanned++
			if scanned > maxRealCorpusWalkFiles {
				return fs.SkipAll
			}
			out = append(out, path)
			return nil
		})
	}
	sort.Strings(out)
	return out
}

func newCandidate(text, path string, source realCorpusSampleSource, maxSampleBytes int, seen map[string]struct{}) (realCorpusSampleCandidate, bool) {
	// Some vendored fixture bundles flatten nested paths into '%' encoded names
	// and can produce pathological parse costs with little additional signal.
	if source == realCorpusSourceRepoRaw && strings.Contains(path, "%") {
		return realCorpusSampleCandidate{}, false
	}
	normalized := strings.ReplaceAll(text, "\r\n", "\n")
	trimmed := strings.TrimSpace(normalized)
	if trimmed == "" || len(trimmed) > maxSampleBytes || !utf8.ValidString(normalized) || strings.ContainsRune(normalized, '\x00') {
		return realCorpusSampleCandidate{}, false
	}
	if _, ok := seen[trimmed]; ok {
		return realCorpusSampleCandidate{}, false
	}
	seen[trimmed] = struct{}{}
	// Ensure trailing newline — most grammars expect EOF after final \n,
	// and real source files almost universally end with one. Corpus block
	// extraction strips trailing newlines, causing parse failures in
	// grammars that require them (gomod, make, etc.).
	if !strings.HasSuffix(normalized, "\n") {
		normalized += "\n"
	}
	return realCorpusSampleCandidate{
		Text:   normalized,
		Trim:   trimmed,
		Size:   len(trimmed),
		Source: source,
		Path:   path,
	}, true
}

func sortRealCorpusCandidates(cands []realCorpusSampleCandidate, profile realCorpusProfile) {
	sort.Slice(cands, func(i, j int) bool {
		a, b := cands[i], cands[j]
		as := sourceRank(profile, a.Source)
		bs := sourceRank(profile, b.Source)
		if as != bs {
			return as < bs
		}
		switch profile {
		case realCorpusProfileSmoke:
			if a.Size != b.Size {
				return a.Size < b.Size
			}
		case realCorpusProfileBalanced:
			// Balanced: prioritize corpus block tests first, then larger files.
			if a.Size != b.Size {
				return a.Size > b.Size
			}
		default:
			// Aggressive: maximize stress by preferring larger inputs.
			if a.Size != b.Size {
				return a.Size > b.Size
			}
		}
		if a.Path != b.Path {
			return a.Path < b.Path
		}
		return a.Trim < b.Trim
	})
}

func sourceRank(profile realCorpusProfile, source realCorpusSampleSource) int {
	switch profile {
	case realCorpusProfileSmoke:
		switch source {
		case realCorpusSourceCorpusBlock:
			return 0
		case realCorpusSourceCorpusRaw:
			return 1
		default:
			return 2
		}
	case realCorpusProfileBalanced:
		switch source {
		case realCorpusSourceCorpusBlock:
			return 0
		case realCorpusSourceRepoRaw:
			return 1
		default:
			return 2
		}
	default:
		switch source {
		case realCorpusSourceRepoRaw:
			return 0
		case realCorpusSourceCorpusRaw:
			return 1
		default:
			return 2
		}
	}
}

func extractTreeSitterCorpusInputs(data []byte) []string {
	lines := strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n")
	out := make([]string, 0, 8)

	for i := 0; i < len(lines); {
		if !isEqualsFence(lines[i]) {
			i++
			continue
		}
		// Skip title block.
		i++
		for i < len(lines) && !isEqualsFence(lines[i]) {
			i++
		}
		if i >= len(lines) {
			break
		}
		// After second fence, parse source until --- separator.
		i++
		for i < len(lines) && strings.TrimSpace(lines[i]) == "" {
			i++
		}
		start := i
		for i < len(lines) && !isDashFence(lines[i]) {
			i++
		}
		if i > start {
			src := strings.Trim(strings.Join(lines[start:i], "\n"), "\n")
			if strings.TrimSpace(src) != "" {
				out = append(out, src)
			}
		}
		if i < len(lines) {
			i++
		}
	}
	return out
}

func isEqualsFence(line string) bool {
	s := strings.TrimSpace(line)
	if len(s) < 3 {
		return false
	}
	for _, r := range s {
		if r != '=' {
			return false
		}
	}
	return true
}

func isDashFence(line string) bool {
	s := strings.TrimSpace(line)
	if len(s) < 3 {
		return false
	}
	for _, r := range s {
		if r != '-' {
			return false
		}
	}
	return true
}

func getenvInt(key string, def int) int {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return def
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n <= 0 {
		return def
	}
	return n
}

// safeSExpr calls n.SExpr with a recursion depth guard. If the tree is deeper
// than maxDepth named nodes, it returns "" to signal "too deep to serialize".
// This prevents goroutine stack overflow on pathologically deep trees (e.g.
// HCL's 189K-deep recursive parse tree).
func safeSExpr(n *gotreesitter.Node, lang *gotreesitter.Language, maxDepth int) string {
	if n == nil || lang == nil {
		return ""
	}
	var rec func(node *gotreesitter.Node, depth int) string
	rec = func(node *gotreesitter.Node, depth int) string {
		if node == nil || !node.IsNamed() {
			return ""
		}
		if depth > maxDepth {
			return "\x00" // sentinel: truncated
		}
		name := node.Type(lang)
		cc := node.ChildCount()
		if cc == 0 {
			return "(" + name + ")"
		}
		parts := make([]string, 0, cc)
		for i := 0; i < cc; i++ {
			s := rec(node.Child(i), depth+1)
			if s == "\x00" {
				return "\x00"
			}
			if s != "" {
				parts = append(parts, s)
			}
		}
		if len(parts) == 0 {
			return "(" + name + ")"
		}
		return "(" + name + " " + strings.Join(parts, " ") + ")"
	}
	result := rec(n, 0)
	if result == "\x00" {
		return "" // signal: too deep
	}
	return result
}

// stripSExprRoot removes the outermost S-expression wrapper, returning only
// the inner content. For example, "(program (x) (y))" → "(x) (y)" and
// "( (x) (y))" → "(x) (y)".
func stripSExprRoot(s string) string {
	s = strings.TrimSpace(s)
	if len(s) < 2 || s[0] != '(' || s[len(s)-1] != ')' {
		return s
	}
	inner := s[1 : len(s)-1]
	// Skip the root type name (if any).
	inner = strings.TrimLeft(inner, " ")
	// Find first '(' which starts the first child.
	idx := strings.IndexByte(inner, '(')
	if idx < 0 {
		return strings.TrimSpace(inner)
	}
	// Everything before '(' is the root name + space. Skip it.
	return strings.TrimSpace(inner[idx:])
}

// logChildCountDiag logs diagnostic info for a childCount divergence,
// walking the tree path to the divergent node and listing its children.
func logChildCountDiag(t *testing.T, div parityDivergence, genRoot, refRoot *gotreesitter.Node, genLang, refLang *gotreesitter.Language) {
	t.Helper()
	// Walk down to the divergent node using the path.
	genNode := walkToPath(genRoot, genLang, div.Path)
	refNode := walkToPath(refRoot, refLang, div.Path)
	if genNode == nil || refNode == nil {
		return
	}
	genChildren := make([]string, 0, genNode.ChildCount())
	for i := 0; i < genNode.ChildCount(); i++ {
		c := genNode.Child(i)
		if c != nil {
			genChildren = append(genChildren, fmt.Sprintf("%s[%d:%d]", c.Type(genLang), c.StartByte(), c.EndByte()))
		}
	}
	refChildren := make([]string, 0, refNode.ChildCount())
	for i := 0; i < refNode.ChildCount(); i++ {
		c := refNode.Child(i)
		if c != nil {
			refChildren = append(refChildren, fmt.Sprintf("%s[%d:%d]", c.Type(refLang), c.StartByte(), c.EndByte()))
		}
	}
	t.Logf("  cc-diag: path=%s genCC=%d refCC=%d", div.Path, genNode.ChildCount(), refNode.ChildCount())
	t.Logf("    gen-children: %v", genChildren)
	t.Logf("    ref-children: %v", refChildren)
}

// walkToPath walks a parse tree following a breadcrumb path like "root/object[0]/pair[1]".
func walkToPath(root *gotreesitter.Node, lang *gotreesitter.Language, path string) *gotreesitter.Node {
	if path == "root" {
		return root
	}
	// Best-effort: walk named children by type.
	parts := strings.Split(path, "/")
	node := root
	for _, part := range parts[1:] { // skip "root"
		// Strip index suffix like "[0]", "#1"
		name := part
		if idx := strings.IndexByte(name, '#'); idx >= 0 {
			name = name[:idx]
		}
		if idx := strings.IndexByte(name, '['); idx >= 0 {
			name = name[:idx]
		}
		found := false
		for i := 0; i < node.ChildCount(); i++ {
			c := node.Child(i)
			if c != nil && c.Type(lang) == name {
				node = c
				found = true
				break
			}
		}
		if !found {
			return nil
		}
	}
	return node
}

// logReduceActionDiff compares reduce actions between gen and ref languages,
// logging symbol+ChildCount pairs that differ. This helps diagnose whether
// childCount divergences stem from production generation or parser runtime.
func logReduceActionDiff(t *testing.T, grammarName string, genLang, refLang *gotreesitter.Language, maxLog int) {
	t.Helper()

	type reduceKey struct {
		symName string
		cc      uint8
	}

	// Collect gen reduce actions by symbol name.
	genReduces := map[string][]uint8{} // symName → list of ChildCounts
	for _, pa := range genLang.ParseActions {
		for _, a := range pa.Actions {
			if a.Type == gotreesitter.ParseActionReduce {
				name := ""
				if int(a.Symbol) < len(genLang.SymbolNames) {
					name = genLang.SymbolNames[a.Symbol]
				}
				genReduces[name] = appendUnique8(genReduces[name], a.ChildCount)
			}
		}
	}

	// Collect ref reduce actions by symbol name.
	refReduces := map[string][]uint8{}
	for _, pa := range refLang.ParseActions {
		for _, a := range pa.Actions {
			if a.Type == gotreesitter.ParseActionReduce {
				name := ""
				if int(a.Symbol) < len(refLang.SymbolNames) {
					name = refLang.SymbolNames[a.Symbol]
				}
				refReduces[name] = appendUnique8(refReduces[name], a.ChildCount)
			}
		}
	}

	// Compare and log differences.
	logged := 0
	for name, genCCs := range genReduces {
		if logged >= maxLog {
			break
		}
		refCCs, ok := refReduces[name]
		if !ok {
			continue // symbol only in gen
		}
		// Check if the CC sets differ.
		genSet := make(map[uint8]bool)
		for _, cc := range genCCs {
			genSet[cc] = true
		}
		refSet := make(map[uint8]bool)
		for _, cc := range refCCs {
			refSet[cc] = true
		}
		// Find CCs in ref but not gen.
		var missing []uint8
		for cc := range refSet {
			if !genSet[cc] {
				missing = append(missing, cc)
			}
		}
		// Find CCs in gen but not ref.
		var extra []uint8
		for cc := range genSet {
			if !refSet[cc] {
				extra = append(extra, cc)
			}
		}
		if len(missing) > 0 || len(extra) > 0 {
			vis := ""
			if int(0) < len(genLang.SymbolMetadata) {
				// Check visibility of this symbol in both.
				genVis := "?"
				refVis := "?"
				for i, sn := range genLang.SymbolNames {
					if sn == name && i < len(genLang.SymbolMetadata) {
						if genLang.SymbolMetadata[i].Visible {
							genVis = "V"
						} else {
							genVis = "-"
						}
						break
					}
				}
				for i, sn := range refLang.SymbolNames {
					if sn == name && i < len(refLang.SymbolMetadata) {
						if refLang.SymbolMetadata[i].Visible {
							refVis = "V"
						} else {
							refVis = "-"
						}
						break
					}
				}
				vis = fmt.Sprintf(" vis=gen:%s/ref:%s", genVis, refVis)
			}
			t.Logf("  reduce-diff[%s]: sym=%q gen-cc=%v ref-cc=%v missing=%v extra=%v%s",
				grammarName, name, genCCs, refCCs, missing, extra, vis)
			logged++
		}
	}
}

func appendUnique8(s []uint8, v uint8) []uint8 {
	for _, x := range s {
		if x == v {
			return s
		}
	}
	return append(s, v)
}

// rebuildRootSExpr constructs an SExpr for a node that is unnamed (so
// SExpr() returns "") by treating it as a named root with the given name.
// This lets the SExpr comparison match deep comparison's leniency when the
// ts2go reference blob has incorrect Named=false metadata on the root.
func rebuildRootSExpr(root *gotreesitter.Node, lang *gotreesitter.Language, rootName string) string {
	parts := make([]string, 0, root.ChildCount())
	for i := 0; i < root.ChildCount(); i++ {
		s := root.Child(i).SExpr(lang)
		if s != "" {
			parts = append(parts, s)
		}
	}
	if len(parts) == 0 {
		return "(" + rootName + ")"
	}
	return "(" + rootName + " " + strings.Join(parts, " ") + ")"
}

func getenvBool(key string) bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(key))) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}
