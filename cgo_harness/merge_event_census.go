//go:build cgo && treesitter_c_parity && gts_merge_census

package cgoharness

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	gotreesitter "github.com/odvcencio/gotreesitter"
)

// Merge-event census (stage M0 of spec.merge-time-election.v1).
//
// The lane's thesis, from finding.late-election-unifying-cause: the reference
// runtime packs duplicate versions at the merge and elects across the packed
// pair at the first spanning pop, while this parser refuses to pack when the
// subtree shapes differ, carries both versions, and elects at the end. The
// direct test of that thesis is a count: how many merges does the reference
// runtime perform, how many does production perform, and which gate refuses
// the rest. This file takes that measurement. It ships ZERO behaviour change.
//
// The census extends the stage-D0 derivation-set differential
// (derivation_set_differential.go, PR #646) rather than building a parallel
// instrument. It reuses D0's corpora (derivationSetCensusLanguages), D0's
// per-source loop shape, and D0's pinned-baseline discipline. D0's baseline
// is 32 set differences over 95 constructed sources; this one's is merge
// counts and refusal counts.
//
// # The reference-runtime counting method, and why the logger is not enough
//
// D0 reconstructs the reference runtime's live version set from
// ts_parser_set_logger output. That method CANNOT reach a merge event.
// parser.c calls ts_stack_merge at four sites (parser.c:1033 after a reduce
// slice, :1114 inside ts_parser__do_all_potential_reductions, :1513 in error
// handling, and :1796/:1805 in ts_parser__condense_stack) and none of them
// logs. The only merge-adjacent log line is "condense" (parser.c:1855), which
// fires once per condense turn when made_changes is set by ANY of removal,
// merge, or version swap, so it cannot be attributed to a merge. The
// "process version:%u, version_count:%u" line (parser.c:2149) reports the
// live version count, but a version count falls for removal and cap
// enforcement as well as for merge, so a difference is not a merge count.
//
// The census therefore reads the instrumented build, which this repository
// already carries and already validates: cgo_harness/work_count/tree_sitter_v0_25_1.patch
// puts merge_attempts_proxy and merge_successes_proxy directly around
// ts_stack_merge and the six GTSLinkUnionOutcome counters around
// stack_node_add_link. No new C instrumentation is invented here.
//
// Two properties keep the instrumented build equal to D0's oracle:
//
//   - The runtime source is the SAME source. The patch is applied to
//     github.com/tree-sitter/go-tree-sitter@v0.25.0/src, resolved from the
//     module cache, which is exactly the runtime D0's sitter.NewParser()
//     compiles. The compile flags are the module's own cgo flags.
//   - The grammar table is the SAME table. The driver dlopens the shared
//     object COracleIdentity reports, which the parity C-reference loader
//     built from grammars/languages.lock.

const (
	mergeCensusRuntimeModule  = "github.com/tree-sitter/go-tree-sitter"
	mergeCensusDriverSchema   = "gts-merge-census-c/v1"
	cTopologyDriverSchema     = "gts-topology-receipt-c/v1"
	cTopologyReceiptSchema    = "gts-topology-receipt/v1"
	cTopologyEventCapacity    = 4096
	mergeCensusParseTimeoutUS = "30000000"
)

type cTopologyEvent struct {
	EventID           uint64 `json:"event_id"`
	Kind              uint64 `json:"kind"`
	ActionID          uint64 `json:"action_id"`
	ActionOrdinal     int64  `json:"action_ordinal"`
	ActionType        uint64 `json:"action_type"`
	State             uint64 `json:"state"`
	LookaheadSymbol   uint64 `json:"lookahead_symbol"`
	ByteOffset        uint64 `json:"byte_offset"`
	VersionID         uint64 `json:"version_id"`
	VersionIndex      uint64 `json:"version_index"`
	SourceVersionID   uint64 `json:"source_version_id"`
	SourceIndex       uint64 `json:"source_index"`
	TargetVersionID   uint64 `json:"target_version_id"`
	TargetIndex       uint64 `json:"target_index"`
	SurvivorVersionID uint64 `json:"survivor_version_id"`
	RemovedVersionID  uint64 `json:"removed_version_id"`
	NodeID            uint64 `json:"node_id"`
	PredecessorNodeID uint64 `json:"predecessor_node_id"`
	LinkID            uint64 `json:"link_id"`
	LinkOrdinal       uint64 `json:"link_ordinal"`
	PopID             uint64 `json:"pop_id"`
	PathOrdinal       uint64 `json:"path_ordinal"`
	PopToNodeID       uint64 `json:"pop_to_node_id"`
	ElectionID        uint64 `json:"election_id"`
	IncumbentID       uint64 `json:"incumbent_id"`
	CandidateID       uint64 `json:"candidate_id"`
	SelectedID        uint64 `json:"selected_id"`
	PayloadCount      uint64 `json:"payload_count"`
	Flags             uint64 `json:"flags"`
}

type cTopologyReceipt struct {
	Schema             string           `json:"schema"`
	Capacity           uint32           `json:"capacity"`
	EventsSeen         uint64           `json:"events_seen"`
	EventsRetained     uint32           `json:"events_retained"`
	EventsDropped      uint64           `json:"events_dropped"`
	Truncated          bool             `json:"truncated"`
	ArithmeticOverflow bool             `json:"arithmetic_overflow"`
	IdentityCollision  bool             `json:"identity_collision"`
	IdentityIncomplete bool             `json:"identity_incomplete"`
	Events             []cTopologyEvent `json:"events"`
}

type cTopologyRow struct {
	Schema         string           `json:"schema"`
	Path           string           `json:"path"`
	Status         string           `json:"status"`
	SourceBytes    uint32           `json:"source_bytes"`
	RootEndByte    uint32           `json:"root_end_byte"`
	RootChildCount uint32           `json:"root_child_count"`
	RootHasError   bool             `json:"root_has_error"`
	Receipt        cTopologyReceipt `json:"receipt"`
}

// mergeCensusCRow is one reference-runtime measurement, decoded from the
// driver's per-source JSON line.
type mergeCensusCRow struct {
	Schema        string `json:"schema"`
	Path          string `json:"path"`
	Status        string `json:"status"`
	SourceBytes   uint32 `json:"source_bytes"`
	RootEndByte   uint32 `json:"root_end_byte"`
	RootHasError  bool   `json:"root_has_error"`
	MergeAttempts uint64 `json:"merge_attempts_proxy"`
	// MergeSuccesses is M_c: the number of ts_stack_merge calls that
	// collapsed two versions into one.
	MergeSuccesses         uint64 `json:"merge_successes_proxy"`
	VersionCreations       uint64 `json:"stack_version_creations_proxy"`
	Shifts                 uint64 `json:"shifts"`
	Reductions             uint64 `json:"reductions"`
	Accepts                uint64 `json:"accept_actions"`
	ExplicitRecovers       uint64 `json:"explicit_recover_actions"`
	ReductionPopRequests   uint64 `json:"reduction_pop_requests"`
	EmittedPopPaths        uint64 `json:"emitted_pop_paths"`
	LinkUnionAttempts      uint64 `json:"predecessor_link_union_attempts"`
	LinkUnionDuplicate     uint64 `json:"predecessor_link_union_duplicate_noop"`
	LinkUnionPrecedence    uint64 `json:"predecessor_link_union_precedence_replaced"`
	LinkUnionRecursive     uint64 `json:"predecessor_link_union_recursive_changed"`
	LinkUnionAppended      uint64 `json:"predecessor_link_union_alternate_appended"`
	LinkUnionRejected      uint64 `json:"predecessor_link_union_rejected"`
	AlternateLinksAppended uint64 `json:"alternate_predecessor_links_appended"`
	GraphLinkAdditions     uint64 `json:"graph_link_additions_proxy"`
	Overflow               bool   `json:"overflow"`
}

// mergeCensusCOracle is a built driver: the patched runtime plus the census
// driver, linked into one executable.
type mergeCensusCOracle struct {
	Binary     string
	RuntimeDir string
	RuntimeSHA string
	PatchSHA   string
	DriverSHA  string
	root       string
}

// Cleanup removes the private build tree.
func (o *mergeCensusCOracle) Cleanup() {
	if o == nil || o.root == "" {
		return
	}
	_ = os.RemoveAll(o.root)
}

// mergeCensusBuildCOracle snapshots the pinned runtime source, applies the
// work-count patch, and links the census driver. The build is hermetic: no
// network, no repository checkout, and no write to the module cache.
func mergeCensusBuildCOracle(repoRoot string) (*mergeCensusCOracle, error) {
	runtimeDir, err := mergeCensusRuntimeDir(repoRoot)
	if err != nil {
		return nil, err
	}
	patchPath := filepath.Join(repoRoot, "cgo_harness", "work_count", "tree_sitter_v0_25_1.patch")
	driverPath := filepath.Join(repoRoot, "cgo_harness", "pure_c", "merge_census_oracle.c")
	patchSHA, err := mergeCensusFileSHA(patchPath)
	if err != nil {
		return nil, err
	}
	driverSHA, err := mergeCensusFileSHA(driverPath)
	if err != nil {
		return nil, err
	}

	root, err := os.MkdirTemp("", "gts-merge-census-c-*")
	if err != nil {
		return nil, fmt.Errorf("create merge-census build root: %w", err)
	}
	oracle := &mergeCensusCOracle{
		RuntimeDir: runtimeDir, PatchSHA: patchSHA, DriverSHA: driverSHA, root: root,
	}

	libRoot := filepath.Join(root, "lib")
	if err := mergeCensusCopyTree(filepath.Join(runtimeDir, "src"), filepath.Join(libRoot, "src")); err != nil {
		oracle.Cleanup()
		return nil, err
	}
	if err := mergeCensusCopyTree(filepath.Join(runtimeDir, "include"), filepath.Join(libRoot, "include")); err != nil {
		oracle.Cleanup()
		return nil, err
	}
	runtimeSHA, err := mergeCensusTreeSHA(libRoot)
	if err != nil {
		oracle.Cleanup()
		return nil, err
	}
	oracle.RuntimeSHA = runtimeSHA

	// Make the snapshot its own repository before patching. git apply resolves
	// paths against the enclosing repository when there is one, so a temporary
	// directory that happens to sit inside a checkout would otherwise send the
	// hunks to the wrong tree.
	initialize := exec.Command("git", "init", "-q", ".")
	initialize.Dir = root
	if out, err := initialize.CombinedOutput(); err != nil {
		oracle.Cleanup()
		return nil, fmt.Errorf("initialize the runtime snapshot repository: %w: %s", err, bytes.TrimSpace(out))
	}

	apply := exec.Command("git", "apply", "-p1", patchPath)
	apply.Dir = root
	if out, err := apply.CombinedOutput(); err != nil {
		oracle.Cleanup()
		return nil, fmt.Errorf(
			"apply work-count patch to the pinned runtime snapshot: %w: %s\n"+
				"the patch is the repository's own C instrument; a failure here means the pinned runtime moved and the census must be re-adjudicated, not re-fitted",
			err, bytes.TrimSpace(out),
		)
	}

	binary := filepath.Join(root, "merge_census_oracle")
	compile := exec.Command(
		mergeCensusCompiler(),
		"-O1", "-std=c11", "-D_POSIX_C_SOURCE=200112L", "-D_DEFAULT_SOURCE",
		"-I"+filepath.Join(libRoot, "include", "tree_sitter"),
		"-I"+filepath.Join(libRoot, "include"),
		"-I"+filepath.Join(libRoot, "src"),
		driverPath,
		filepath.Join(libRoot, "src", "lib.c"),
		"-ldl", "-o", binary,
	)
	if out, err := compile.CombinedOutput(); err != nil {
		oracle.Cleanup()
		return nil, fmt.Errorf("compile merge-census C oracle: %w: %s", err, bytes.TrimSpace(out))
	}
	oracle.Binary = binary

	// Run the patch's own three counter models before trusting any count.
	check := exec.Command(binary, "--exact-model")
	out, err := check.Output()
	if err != nil {
		oracle.Cleanup()
		return nil, fmt.Errorf("merge-census C oracle self-check: %w", err)
	}
	var model struct {
		Schema string `json:"schema"`
		Passed bool   `json:"passed"`
	}
	if err := json.Unmarshal(bytes.TrimSpace(out), &model); err != nil {
		oracle.Cleanup()
		return nil, fmt.Errorf("decode merge-census C oracle self-check: %w", err)
	}
	if model.Schema != "gts-merge-census-c-exact-model/v1" || !model.Passed {
		oracle.Cleanup()
		return nil, fmt.Errorf("merge-census C oracle self-check failed: %+v", model)
	}
	return oracle, nil
}

// mergeCensusRepoRoot resolves the repository root from the parity lock the
// C-reference loader already locates, so the census needs no new path
// convention.
func mergeCensusRepoRoot() (string, error) {
	lockPath, err := findParityLockPath()
	if err != nil {
		return "", err
	}
	absolute, err := filepath.Abs(lockPath)
	if err != nil {
		return "", err
	}
	return filepath.Dir(filepath.Dir(absolute)), nil
}

func mergeCensusCompiler() string {
	if value := strings.TrimSpace(os.Getenv("CC")); value != "" {
		return value
	}
	return "cc"
}

// mergeCensusRuntimeDir resolves the pinned runtime source directory from the
// module cache. It never downloads and never writes.
func mergeCensusRuntimeDir(repoRoot string) (string, error) {
	command := exec.Command("go", "list", "-m", "-f", "{{.Dir}}", mergeCensusRuntimeModule)
	command.Dir = filepath.Join(repoRoot, "cgo_harness")
	out, err := command.Output()
	if err != nil {
		return "", fmt.Errorf("resolve %s module directory: %w", mergeCensusRuntimeModule, err)
	}
	dir := strings.TrimSpace(string(out))
	if dir == "" {
		return "", fmt.Errorf("resolve %s module directory: empty result", mergeCensusRuntimeModule)
	}
	if _, err := os.Stat(filepath.Join(dir, "src", "lib.c")); err != nil {
		return "", fmt.Errorf("pinned runtime source is not at %s: %w", dir, err)
	}
	return dir, nil
}

func mergeCensusFileSHA(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

func mergeCensusTreeSHA(root string) (string, error) {
	digest := sha256.New()
	var paths []string
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		paths = append(paths, path)
		return nil
	})
	if err != nil {
		return "", err
	}
	sort.Strings(paths)
	for _, path := range paths {
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return "", err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return "", err
		}
		fmt.Fprintf(digest, "%s\n%x\n", filepath.ToSlash(relative), sha256.Sum256(data))
	}
	return hex.EncodeToString(digest.Sum(nil)), nil
}

func mergeCensusCopyTree(source, destination string) error {
	return filepath.Walk(source, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		target := filepath.Join(destination, relative)
		if info.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		return os.WriteFile(target, data, 0o644)
	})
}

// mergeCensusCSymbols returns the grammar entry-point candidates for a lock
// row, in the same order the parity loader tries them.
func mergeCensusCSymbols(name string) ([]string, error) {
	lockPath, err := findParityLockPath()
	if err != nil {
		return nil, err
	}
	lock, err := loadParityLock(lockPath)
	if err != nil {
		return nil, err
	}
	entry, ok := lock[name]
	if !ok {
		return nil, fmt.Errorf("parity lock has no entry for %q", name)
	}
	return parityLanguageSymbols(entry), nil
}

// mergeCensusRunC measures every source in one batch. Sources are written to
// a private directory in manifest order, so the returned rows line up with
// the input slice index for index.
func mergeCensusRunC(oracle *mergeCensusCOracle, language string, sources []a3CertificationSweepSource) ([]mergeCensusCRow, error) {
	if len(sources) == 0 {
		return nil, nil
	}
	identity, err := COracleIdentity(language)
	if err != nil {
		return nil, fmt.Errorf("resolve %s C oracle identity: %w", language, err)
	}
	symbols, err := mergeCensusCSymbols(language)
	if err != nil {
		return nil, err
	}

	batchRoot, err := os.MkdirTemp("", "gts-merge-census-batch-*")
	if err != nil {
		return nil, fmt.Errorf("create merge-census batch root: %w", err)
	}
	defer os.RemoveAll(batchRoot)

	var manifest strings.Builder
	paths := make([]string, len(sources))
	for index, source := range sources {
		path := filepath.Join(batchRoot, fmt.Sprintf("%06d.src", index))
		if err := os.WriteFile(path, source.Source, 0o644); err != nil {
			return nil, fmt.Errorf("write merge-census source: %w", err)
		}
		paths[index] = path
		manifest.WriteString(path)
		manifest.WriteString("\n")
	}
	manifestPath := filepath.Join(batchRoot, "manifest.txt")
	if err := os.WriteFile(manifestPath, []byte(manifest.String()), 0o644); err != nil {
		return nil, fmt.Errorf("write merge-census manifest: %w", err)
	}

	command := exec.Command(
		oracle.Binary, identity.GrammarArtifactPath,
		strings.Join(symbols, ","), manifestPath, mergeCensusParseTimeoutUS,
	)
	var stdout, stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	if err := command.Run(); err != nil {
		return nil, fmt.Errorf("run merge-census C oracle for %s: %w: %s", language, err, bytes.TrimSpace(stderr.Bytes()))
	}

	byPath := make(map[string]mergeCensusCRow, len(sources))
	scanner := bufio.NewScanner(&stdout)
	scanner.Buffer(make([]byte, 0, 1<<20), 1<<22)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var row mergeCensusCRow
		if err := json.Unmarshal([]byte(line), &row); err != nil {
			return nil, fmt.Errorf("decode merge-census C row: %w: %s", err, line)
		}
		if row.Schema != mergeCensusDriverSchema {
			return nil, fmt.Errorf("merge-census C row schema %q, want %q", row.Schema, mergeCensusDriverSchema)
		}
		byPath[row.Path] = row
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read merge-census C rows: %w", err)
	}

	rows := make([]mergeCensusCRow, len(sources))
	for index, path := range paths {
		row, ok := byPath[path]
		if !ok {
			return nil, fmt.Errorf("merge-census C oracle produced no row for %s/%s", language, sources[index].Name)
		}
		rows[index] = row
	}
	return rows, nil
}

func mergeCensusRunCTopology(
	oracle *mergeCensusCOracle,
	language string,
	source a3CertificationSweepSource,
) (cTopologyRow, error) {
	identity, err := COracleIdentity(language)
	if err != nil {
		return cTopologyRow{}, fmt.Errorf("resolve %s C oracle identity: %w", language, err)
	}
	symbols, err := mergeCensusCSymbols(language)
	if err != nil {
		return cTopologyRow{}, err
	}

	batchRoot, err := os.MkdirTemp("", "gts-c-topology-receipt-*")
	if err != nil {
		return cTopologyRow{}, fmt.Errorf("create C topology receipt root: %w", err)
	}
	defer os.RemoveAll(batchRoot)
	sourcePath := filepath.Join(batchRoot, "000000.src")
	if err := os.WriteFile(sourcePath, source.Source, 0o644); err != nil {
		return cTopologyRow{}, fmt.Errorf("write C topology source: %w", err)
	}
	manifestPath := filepath.Join(batchRoot, "manifest.txt")
	if err := os.WriteFile(manifestPath, []byte(sourcePath+"\n"), 0o644); err != nil {
		return cTopologyRow{}, fmt.Errorf("write C topology manifest: %w", err)
	}

	command := exec.Command(
		oracle.Binary, "--topology", identity.GrammarArtifactPath,
		strings.Join(symbols, ","), manifestPath, mergeCensusParseTimeoutUS,
	)
	var stdout, stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	if err := command.Run(); err != nil {
		return cTopologyRow{}, fmt.Errorf(
			"run C topology receipt for %s/%s: %w: %s",
			language, source.Name, err, bytes.TrimSpace(stderr.Bytes()),
		)
	}

	decoder := json.NewDecoder(bytes.NewReader(stdout.Bytes()))
	decoder.DisallowUnknownFields()
	var row cTopologyRow
	if err := decoder.Decode(&row); err != nil {
		return cTopologyRow{}, fmt.Errorf("decode C topology receipt: %w", err)
	}
	var trailing json.RawMessage
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return cTopologyRow{}, fmt.Errorf("decode C topology receipt: trailing JSON value")
		}
		return cTopologyRow{}, fmt.Errorf("decode C topology receipt trailer: %w", err)
	}
	if row.Schema != cTopologyDriverSchema {
		return cTopologyRow{}, fmt.Errorf("C topology row schema %q, want %q", row.Schema, cTopologyDriverSchema)
	}
	if row.Receipt.Schema != cTopologyReceiptSchema {
		return cTopologyRow{}, fmt.Errorf(
			"C topology receipt schema %q, want %q",
			row.Receipt.Schema, cTopologyReceiptSchema,
		)
	}
	if row.Path != sourcePath {
		return cTopologyRow{}, fmt.Errorf("C topology row path %q, want %q", row.Path, sourcePath)
	}
	return row, nil
}

// mergeCensusRow is one source's full accounting: the reference runtime's
// merge counts, production's merge counts and refusal breakdown, and the
// compact core's link-union counts.
type mergeCensusRow struct {
	Language string
	Name     string

	C  mergeCensusCRow
	Go gotreesitter.MergeEventCensusCounts

	// CompactAccepted reports whether the compact route reached an accept, so
	// the compact link-union numbers are readable.
	CompactAccepted bool
	// GoParseError records a production parse failure. The row still carries
	// the reference-runtime numbers.
	GoParseError string
}

// MergeRatio is M_p/M_c for this source: production merge successes divided
// by the reference runtime's merge successes. It is defined only when the
// reference runtime merged at least once.
func (r mergeCensusRow) MergeRatio() (float64, bool) {
	if r.C.MergeSuccesses == 0 {
		return 0, false
	}
	return float64(r.Go.Successes) / float64(r.C.MergeSuccesses), true
}

// runMergeEventCensusRow measures one source on all three engines.
//
// Production runs on the DEFAULT route, because the refusal gates under
// measurement (the score-equality gate, the distinct-materializing-shapes
// refusal, and the deep link-payload test) are production GSS code. The
// compact route runs second, only to read the compact core's own link-union
// counters at its acceptance.
func runMergeEventCensusRow(
	language string, lang *gotreesitter.Language, name string, source []byte, cRow mergeCensusCRow,
) mergeCensusRow {
	row := mergeCensusRow{Language: language, Name: name, C: cRow}

	gotreesitter.MergeEventCensusReset()
	parser := gotreesitter.NewParser(lang)
	parser.SetAdmissionCandidateRoute(false)
	tree, err := parser.Parse(source)
	if err != nil {
		row.GoParseError = err.Error()
	} else if tree != nil {
		tree.Release()
	}
	row.Go = gotreesitter.MergeEventCensusSnapshot()

	compactParser := gotreesitter.NewParser(lang)
	compactParser.SetAdmissionCandidateRoute(true)
	compactTree, compactErr := compactParser.Parse(source)
	if compactErr == nil && compactTree != nil {
		compactTree.Release()
	}
	compact := gotreesitter.MergeEventCensusSnapshot()
	if compact.CompactAcceptancesObserved > row.Go.CompactAcceptancesObserved {
		row.CompactAccepted = true
		row.Go.CompactLinkUnionAttempts = compact.CompactLinkUnionAttempts
		row.Go.CompactLinkUnionDuplicateNoop = compact.CompactLinkUnionDuplicateNoop
		row.Go.CompactLinkUnionPrecedenceReplaced = compact.CompactLinkUnionPrecedenceReplaced
		row.Go.CompactLinkUnionRecursiveChanged = compact.CompactLinkUnionRecursiveChanged
		row.Go.CompactLinkUnionAlternateAppended = compact.CompactLinkUnionAlternateAppended
		row.Go.CompactLinkUnionRejected = compact.CompactLinkUnionRejected
		row.Go.CompactAcceptancesObserved = compact.CompactAcceptancesObserved
	}
	return row
}

// mergeCensusTotals aggregates rows for one corpus.
type mergeCensusTotals struct {
	Sources int

	CMergeAttempts  uint64
	CMergeSuccesses uint64
	CLinkUnionAttempts,
	CLinkUnionDuplicate,
	CLinkUnionPrecedence,
	CLinkUnionRecursive,
	CLinkUnionAppended,
	CLinkUnionRejected uint64

	GoAttempts  uint64
	GoSuccesses uint64

	RefuseNoGSSHead      uint64
	RefuseNoGSSHeadBoth  uint64
	RefuseNoGSSHeadOne   uint64
	RefuseStatus         uint64
	RefuseScoreOrShifted uint64
	RefuseScoreOnly      uint64
	RefuseShiftedOnly    uint64
	RefuseStateOrOffset  uint64
	RefuseCleanZero      uint64
	RefuseErrorCost      uint64
	RefuseDistinctShapes uint64
	RefuseMergeFailed    uint64

	LinkPayloadTests              uint64
	LinkPayloadDeepAccepts        uint64
	LinkPayloadDeepRefusals       uint64
	LinkPayloadShallowWouldAccept uint64
	LinkPayloadDeepWouldAccept    uint64
	LinkPayloadPending            uint64

	CompactAccepted     int
	CompactUnionAttempt uint64
	CompactUnionAppend  uint64
	CompactUnionDup     uint64
	CompactUnionReject  uint64

	// SourcesWhereGoOverMerges is the stop-the-line class of gate G6: a source
	// where production collapses MORE versions than the reference runtime.
	SourcesWhereGoOverMerges int
	// SourcesWhereCMergesAndGoDoesNot is the lane's headline class: the
	// reference runtime collapsed at least one pair and production collapsed
	// none.
	SourcesWhereCMergesAndGoDoesNot int
}

func (t *mergeCensusTotals) add(row mergeCensusRow) {
	t.Sources++

	t.CMergeAttempts += row.C.MergeAttempts
	t.CMergeSuccesses += row.C.MergeSuccesses
	t.CLinkUnionAttempts += row.C.LinkUnionAttempts
	t.CLinkUnionDuplicate += row.C.LinkUnionDuplicate
	t.CLinkUnionPrecedence += row.C.LinkUnionPrecedence
	t.CLinkUnionRecursive += row.C.LinkUnionRecursive
	t.CLinkUnionAppended += row.C.LinkUnionAppended
	t.CLinkUnionRejected += row.C.LinkUnionRejected

	t.GoAttempts += row.Go.Attempts
	t.GoSuccesses += row.Go.Successes

	t.RefuseNoGSSHead += row.Go.RefuseNoGSSHead
	t.RefuseNoGSSHeadBoth += row.Go.RefuseNoGSSHeadBoth
	t.RefuseNoGSSHeadOne += row.Go.RefuseNoGSSHeadOne
	t.RefuseStatus += row.Go.RefuseStatus
	t.RefuseScoreOrShifted += row.Go.RefuseScoreOrShifted
	t.RefuseScoreOnly += row.Go.RefuseScoreOnly
	t.RefuseShiftedOnly += row.Go.RefuseShiftedOnly
	t.RefuseStateOrOffset += row.Go.RefuseStateOrOffset
	t.RefuseCleanZero += row.Go.RefuseCleanZero
	t.RefuseErrorCost += row.Go.RefuseErrorCost
	t.RefuseDistinctShapes += row.Go.RefuseDistinctShapes
	t.RefuseMergeFailed += row.Go.RefuseMergeFailed

	t.LinkPayloadTests += row.Go.LinkPayloadTests
	t.LinkPayloadDeepAccepts += row.Go.LinkPayloadDeepAccepts
	t.LinkPayloadDeepRefusals += row.Go.LinkPayloadDeepRefusals
	t.LinkPayloadShallowWouldAccept += row.Go.LinkPayloadShallowWouldAccept
	t.LinkPayloadDeepWouldAccept += row.Go.LinkPayloadDeepWouldAccept
	t.LinkPayloadPending += row.Go.LinkPayloadPending

	if row.CompactAccepted {
		t.CompactAccepted++
		t.CompactUnionAttempt += row.Go.CompactLinkUnionAttempts
		t.CompactUnionAppend += row.Go.CompactLinkUnionAlternateAppended
		t.CompactUnionDup += row.Go.CompactLinkUnionDuplicateNoop
		t.CompactUnionReject += row.Go.CompactLinkUnionRejected
	}

	if row.Go.Successes > row.C.MergeSuccesses {
		t.SourcesWhereGoOverMerges++
	}
	if row.C.MergeSuccesses > 0 && row.Go.Successes == 0 {
		t.SourcesWhereCMergesAndGoDoesNot++
	}
}

// Ratio is the corpus-level M_p/M_c: production merge successes over the
// reference runtime's merge successes. The lane drives it toward 1.
func (t *mergeCensusTotals) Ratio() (float64, bool) {
	if t.CMergeSuccesses == 0 {
		return 0, false
	}
	return float64(t.GoSuccesses) / float64(t.CMergeSuccesses), true
}

func (t *mergeCensusTotals) ratioText() string {
	ratio, ok := t.Ratio()
	if !ok {
		return "undefined"
	}
	return fmt.Sprintf("%.4f", ratio)
}

// refusalLine renders the refusal breakdown, highest class first, so the
// dominant gate is readable without arithmetic.
func (t *mergeCensusTotals) refusalLine() string {
	type entry struct {
		Name  string
		Count uint64
	}
	entries := []entry{
		{"score-or-shifted", t.RefuseScoreOrShifted},
		{"state-or-offset", t.RefuseStateOrOffset},
		{"clean-zero", t.RefuseCleanZero},
		{"error-cost", t.RefuseErrorCost},
		{"distinct-shapes", t.RefuseDistinctShapes},
		{"status", t.RefuseStatus},
		{"no-gss-head-both-flat", t.RefuseNoGSSHeadBoth},
		{"no-gss-head-one-flat", t.RefuseNoGSSHeadOne},
		{"merge-failed", t.RefuseMergeFailed},
	}
	sort.SliceStable(entries, func(i, j int) bool { return entries[i].Count > entries[j].Count })
	parts := make([]string, 0, len(entries))
	for _, item := range entries {
		parts = append(parts, fmt.Sprintf("%s=%d", item.Name, item.Count))
	}
	return strings.Join(parts, " ")
}

// linkPayloadLine renders the Tier-2 accounting, whose headline number is
// shallow-would-accept: the count of reference-runtime collapses production's
// deep test turns into appends.
func (t *mergeCensusTotals) linkPayloadLine() string {
	return fmt.Sprintf(
		"tests=%d deep-accept=%d deep-refuse=%d shallow-would-accept=%d deep-would-accept=%d pending=%d",
		t.LinkPayloadTests, t.LinkPayloadDeepAccepts, t.LinkPayloadDeepRefusals,
		t.LinkPayloadShallowWouldAccept, t.LinkPayloadDeepWouldAccept, t.LinkPayloadPending,
	)
}
