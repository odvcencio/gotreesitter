// Command go_c_timing_verify verifies one sealed Go/C full-parse board without
// executing either benchmark artifact. It fails closed on schema, identity,
// ledger, sample, static-ELF, schedule, semantic, estimator, and A/A null-gate
// drift.
package main

import (
	"bufio"
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"debug/elf"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"math/big"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"
)

const (
	boardSchema      = "gts-go-c-full-parse-board/v3"
	sampleSchema     = "gts-go-c-full-parse-sample/v2"
	receiptSchema    = "gts-go-c-full-parse-verification/v3"
	modeAA           = "identical_go_aa"
	modeProduction   = "production_go_c"
	zeroHash         = "0000000000000000000000000000000000000000000000000000000000000000"
	minElapsedNS     = uint64(750_000_000)
	maxIterations    = uint64(1_000_000_000)
	openFlagsLiteral = "O_RDONLY|O_CLOEXEC|O_NOFOLLOW"
	executionMethod  = "fexecve"
	sourceTransport  = "inherited_verified_fd"
)

var (
	lanes       = []string{"public_validated", "selected_release", "selected_consumer"}
	fixtures    = []string{"rewrite", "query_compile", "language", "grammargen_lr"}
	positionsAA = []positionRule{
		{Label: "A", Implementation: "go"},
		{Label: "B", Implementation: "go"},
		{Label: "B", Implementation: "go"},
		{Label: "A", Implementation: "go"},
	}
	positionsProduction = []positionRule{
		{Label: "Go", Implementation: "go"},
		{Label: "C", Implementation: "c"},
		{Label: "C", Implementation: "c"},
		{Label: "Go", Implementation: "go"},
	}
)

type positionRule struct {
	Label          string
	Implementation string
}

type boardManifest struct {
	Schema                  string                     `json:"schema"`
	BoardID                 string                     `json:"board_id"`
	Mode                    string                     `json:"mode"`
	Contract                fileBinding                `json:"contract"`
	ArtifactSchema          fileBinding                `json:"artifact_schema"`
	BuildRecipe             fileBinding                `json:"build_recipe"`
	DeepReproductionReceipt fileBinding                `json:"deep_reproduction_receipt"`
	Authority               authorityBinding           `json:"authority"`
	Collector               collectorBinding           `json:"collector"`
	ClockAdmission          fileBinding                `json:"clock_admission"`
	Artifacts               map[string]artifactBinding `json:"artifacts"`
	Fixtures                []fixtureBinding           `json:"fixtures"`
	Schedule                scheduleBinding            `json:"schedule"`
	Ledger                  fileBinding                `json:"ledger"`
	LedgerSeal              fileBinding                `json:"ledger_seal"`
	AAGate                  *aaGateBinding             `json:"aa_gate,omitempty"`
	AAAuthorization         *aaAuthorization           `json:"aa_authorization,omitempty"`
}

type fileBinding struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
}

type authorityBinding struct {
	AuthorityID            string      `json:"authority_id"`
	SubjectID              string      `json:"subject_id"`
	PublicKey              string      `json:"public_key"`
	Status                 string      `json:"status"`
	OperatorRootEquivalent bool        `json:"operator_root_equivalent"`
	BootID                 string      `json:"boot_id"`
	Receipt                fileBinding `json:"receipt"`
}

type collectorBinding struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
	Device uint64 `json:"device"`
	Inode  uint64 `json:"inode"`
	Size   uint64 `json:"size"`
}

type artifactBinding struct {
	Implementation    string            `json:"implementation"`
	SourceSHA256      string            `json:"source_sha256"`
	Path              string            `json:"path"`
	SHA256            string            `json:"sha256"`
	Device            uint64            `json:"device"`
	Inode             uint64            `json:"inode"`
	Size              uint64            `json:"size"`
	CommandTemplate   []string          `json:"command_template"`
	Tags              []string          `json:"tags"`
	EnvironmentSHA256 string            `json:"environment_sha256"`
	BuildManifest     fileBinding       `json:"build_manifest"`
	StaticELF         *staticELFBinding `json:"static_elf,omitempty"`
}

type staticELFBinding struct {
	Class           string      `json:"class"`
	Data            string      `json:"data"`
	OSABI           string      `json:"osabi"`
	Machine         string      `json:"machine"`
	Type            string      `json:"type"`
	Linkage         string      `json:"linkage"`
	RequiredSymbols []string    `json:"required_symbols"`
	RetainedReadelf fileBinding `json:"retained_readelf"`
	RetainedNM      fileBinding `json:"retained_nm"`
	RetainedObjdump fileBinding `json:"retained_objdump"`
	RetainedLinkMap fileBinding `json:"retained_link_map"`
}

type fixtureBinding struct {
	Name         string `json:"name"`
	Path         string `json:"path"`
	SHA256       string `json:"sha256"`
	Bytes        uint64 `json:"bytes"`
	DeepSHA256   string `json:"deep_sha256"`
	DeepBytes    uint64 `json:"deep_bytes"`
	VisitedNodes uint64 `json:"visited_nodes"`
	Checksum     string `json:"traversal_checksum"`
	Device       uint64 `json:"device"`
	Inode        uint64 `json:"inode"`
}

type scheduleBinding struct {
	Lanes                      []string `json:"lanes"`
	Fixtures                   []string `json:"fixtures"`
	Cycles                     []int    `json:"cycles"`
	Positions                  []string `json:"positions"`
	Samples                    int      `json:"samples"`
	ScheduledAttemptsPerWindow int      `json:"scheduled_attempts_per_window"`
	DeferredAttemptsPerWindow  int      `json:"deferred_attempts_per_window"`
	MinimumRetrySpacingNS      uint64   `json:"minimum_retry_spacing_ns"`
}

type aaGateBinding struct {
	PublicBenchstatSameDirectionSignificantFixtures int         `json:"public_benchstat_same_direction_significant_fixtures"`
	BenchstatReceipt                                fileBinding `json:"benchstat_receipt"`
}

type aaAuthorization struct {
	AuthorizationReceipt fileBinding `json:"authorization_receipt"`
	ConsumptionReceipt   fileBinding `json:"consumption_receipt"`
	SealedAABoard        fileBinding `json:"sealed_aa_board"`
	AAVerification       fileBinding `json:"aa_verification_receipt"`
	RegistryPublicKey    string      `json:"registry_public_key"`
}

type ledgerRecord struct {
	Ordinal           uint64             `json:"ordinal"`
	Window            int                `json:"window"`
	Lane              string             `json:"lane"`
	Fixture           string             `json:"fixture"`
	Cycle             int                `json:"cycle"`
	Attempt           int                `json:"attempt"`
	Phase             string             `json:"phase"`
	PredecessorSHA256 string             `json:"predecessor_sha256"`
	StartMonotonicNS  uint64             `json:"start_monotonic_ns"`
	EndMonotonicNS    uint64             `json:"end_monotonic_ns"`
	ExitCode          int                `json:"exit_code"`
	ExitClass         string             `json:"exit_class"`
	RejectionReceipt  *fileBinding       `json:"rejection_receipt,omitempty"`
	Positions         []positionEvidence `json:"positions"`
}

type positionEvidence struct {
	Position             int                 `json:"position"`
	Label                string              `json:"label"`
	Implementation       string              `json:"implementation"`
	Sample               fileBinding         `json:"sample"`
	CollectorReceipt     fileBinding         `json:"collector_receipt"`
	MaxRSSBytes          uint64              `json:"max_rss_bytes"`
	ArtifactOpened       descriptorEvidence  `json:"artifact_opened"`
	ArtifactRetainedPost descriptorEvidence  `json:"artifact_retained_post"`
	ArtifactReopenedPost descriptorEvidence  `json:"artifact_reopened_post"`
	FixtureOpened        descriptorEvidence  `json:"fixture_opened"`
	FixtureRetainedPost  descriptorEvidence  `json:"fixture_retained_post"`
	FixtureReopenedPost  descriptorEvidence  `json:"fixture_reopened_post"`
	ExecutionMethod      string              `json:"execution_method"`
	SourceTransport      string              `json:"source_transport"`
	InheritedSourceFD    int                 `json:"inherited_source_fd"`
	StdoutFrame          stdoutFrameEvidence `json:"stdout_frame"`
	Argv                 []string            `json:"argv"`
	Tags                 []string            `json:"tags"`
	EnvironmentSHA256    string              `json:"environment_sha256"`
}

type descriptorEvidence struct {
	FD        int              `json:"fd"`
	OpenFlags string           `json:"open_flags"`
	Identity  identityEvidence `json:"identity"`
}

type stdoutFrameEvidence struct {
	CompleteEOF bool   `json:"complete_eof"`
	AtomicWrite bool   `json:"atomic_write"`
	Bytes       uint64 `json:"bytes"`
	SHA256      string `json:"sha256"`
}

type collectorPositionReceipt struct {
	Schema               string             `json:"schema"`
	ExecutionMethod      string             `json:"execution_method"`
	SourceTransport      string             `json:"source_transport"`
	InheritedSourceFD    int                `json:"inherited_source_fd"`
	Argv                 []string           `json:"argv"`
	ArtifactOpened       descriptorEvidence `json:"artifact_opened"`
	ArtifactRetainedPost descriptorEvidence `json:"artifact_retained_post"`
	ArtifactReopenedPost descriptorEvidence `json:"artifact_reopened_post"`
	FixtureOpened        descriptorEvidence `json:"fixture_opened"`
	FixtureRetainedPost  descriptorEvidence `json:"fixture_retained_post"`
	FixtureReopenedPost  descriptorEvidence `json:"fixture_reopened_post"`
	StdoutCompleteEOF    bool               `json:"stdout_complete_eof"`
	StdoutAtomicPacket   bool               `json:"stdout_atomic_packet"`
	StdoutBytes          uint64             `json:"stdout_bytes"`
	StdoutSHA256         string             `json:"stdout_sha256"`
	ExitCode             int                `json:"exit_code"`
	EnvironmentSHA256    string             `json:"environment_sha256"`
}

type identityEvidence struct {
	CanonicalPath string `json:"canonical_path"`
	Device        uint64 `json:"device"`
	Inode         uint64 `json:"inode"`
	Size          uint64 `json:"size"`
	SHA256        string `json:"sha256"`
}

type sample struct {
	Implementation string
	Lane           string
	Fixture        string
	SourceSHA256   string
	SourceBytes    uint64
	Clock          string
	Iterations     uint64
	ElapsedNS      uint64
	VisitedNodes   uint64
	Checksum       string
}

type rationalJSON struct {
	Numerator   string `json:"numerator"`
	Denominator string `json:"denominator"`
}

type cellResult struct {
	Lane            string       `json:"lane"`
	Fixture         string       `json:"fixture"`
	Q               rationalJSON `json:"q_product"`
	Root            string       `json:"display_root"`
	AAMedianRSSPass *bool        `json:"aa_median_rss_pass,omitempty"`
	AAMaxRSSPass    *bool        `json:"aa_max_rss_pass,omitempty"`
}

type laneResult struct {
	Lane string       `json:"lane"`
	Q    rationalJSON `json:"q_product"`
	Root string       `json:"display_root"`
}

type verificationReceipt struct {
	Schema            string       `json:"schema"`
	Status            string       `json:"status"`
	BoardID           string       `json:"board_id"`
	Mode              string       `json:"mode"`
	ManifestSHA256    string       `json:"manifest_sha256"`
	LedgerSHA256      string       `json:"ledger_sha256"`
	LedgerTailSHA256  string       `json:"ledger_tail_sha256"`
	Samples           int          `json:"samples"`
	Cells             []cellResult `json:"cells"`
	Lanes             []laneResult `json:"lanes"`
	AANullGatesPassed *bool        `json:"aa_null_gates_passed,omitempty"`
}

type acceptedWindow struct {
	Record  ledgerRecord
	Samples [4]sample
}

type signedEnvelope struct {
	Schema    string          `json:"schema"`
	Payload   json.RawMessage `json:"payload"`
	PublicKey string          `json:"public_key"`
	Signature string          `json:"signature"`
}

type authorityReceiptPayload struct {
	Schema                    string `json:"schema"`
	AuthorityID               string `json:"authority_id"`
	SubjectID                 string `json:"subject_id"`
	BootID                    string `json:"boot_id"`
	CollectorSHA256           string `json:"collector_sha256"`
	Status                    string `json:"status"`
	OperatorRootEquivalent    bool   `json:"operator_root_equivalent"`
	OperatorUnrestrictedSudo  bool   `json:"operator_unrestricted_sudo"`
	OperatorRootCapabilities  bool   `json:"operator_root_capabilities"`
	OperatorPtraceAccess      bool   `json:"operator_ptrace_access"`
	OperatorBPFAccess         bool   `json:"operator_bpf_access"`
	OperatorRawTimingRead     bool   `json:"operator_raw_timing_read"`
	OperatorCollectorIdentity bool   `json:"operator_collector_identity"`
	Nonce                     string `json:"nonce"`
}

type clockAdmissionPayload struct {
	Schema                   string `json:"schema"`
	BoardID                  string `json:"board_id"`
	AuthorityID              string `json:"authority_id"`
	BootID                   string `json:"boot_id"`
	CollectorSHA256          string `json:"collector_sha256"`
	Decision                 string `json:"decision"`
	RawTimingReadable        bool   `json:"raw_timing_readable"`
	Mode                     string `json:"mode"`
	ContractSHA256           string `json:"contract_sha256"`
	ArtifactSchemaSHA256     string `json:"artifact_schema_sha256"`
	BuildRecipeSHA256        string `json:"build_recipe_sha256"`
	GoArtifactSHA256         string `json:"go_artifact_sha256"`
	CArtifactSHA256          string `json:"c_artifact_sha256"`
	GoArtifactIdentitySHA256 string `json:"go_artifact_identity_sha256"`
	CArtifactIdentitySHA256  string `json:"c_artifact_identity_sha256"`
	CollectorIdentitySHA256  string `json:"collector_identity_sha256"`
	FixtureSetSHA256         string `json:"fixture_set_sha256"`
	ScheduleSHA256           string `json:"schedule_sha256"`
}

type rejectionPayload struct {
	Schema          string `json:"schema"`
	BoardID         string `json:"board_id"`
	AuthorityID     string `json:"authority_id"`
	BootID          string `json:"boot_id"`
	CollectorSHA256 string `json:"collector_sha256"`
	Ordinal         uint64 `json:"ordinal"`
	Window          int    `json:"window"`
	Attempt         int    `json:"attempt"`
	Reason          string `json:"reason"`
	TimingObserved  bool   `json:"timing_observed"`
}

type ledgerSealPayload struct {
	Schema           string `json:"schema"`
	BoardID          string `json:"board_id"`
	AuthorityID      string `json:"authority_id"`
	BootID           string `json:"boot_id"`
	CollectorSHA256  string `json:"collector_sha256"`
	LedgerSHA256     string `json:"ledger_sha256"`
	LedgerTailSHA256 string `json:"ledger_tail_sha256"`
	Records          uint64 `json:"records"`
}

type deepReproductionPayload struct {
	Schema            string            `json:"schema"`
	Status            string            `json:"status"`
	ReviewerSubjectID string            `json:"reviewer_subject_id"`
	GoSourceSHA256    string            `json:"go_source_sha256"`
	CSourceSHA256     string            `json:"c_source_sha256"`
	FixtureSetSHA256  string            `json:"fixture_set_sha256"`
	GoCommand         []string          `json:"go_command"`
	CCommand          []string          `json:"c_command"`
	GoStdoutSHA256    map[string]string `json:"go_stdout_sha256"`
	CStdoutSHA256     map[string]string `json:"c_stdout_sha256"`
	ToolRuntimeSHA256 string            `json:"tool_runtime_sha256"`
	ConstantsMatched  bool              `json:"constants_matched"`
}

type benchstatPayload struct {
	Schema                           string `json:"schema"`
	BoardID                          string `json:"board_id"`
	Status                           string `json:"status"`
	SameDirectionSignificantFixtures int    `json:"same_direction_significant_fixtures"`
}

type aaVerificationReceipt struct {
	Schema            string `json:"schema"`
	Status            string `json:"status"`
	BoardID           string `json:"board_id"`
	Mode              string `json:"mode"`
	ManifestSHA256    string `json:"manifest_sha256"`
	AANullGatesPassed bool   `json:"aa_null_gates_passed"`
}

type aaAuthorizationPayload struct {
	Schema                   string `json:"schema"`
	Status                   string `json:"status"`
	ProductionBoardID        string `json:"production_board_id"`
	AABoardID                string `json:"aa_board_id"`
	AABoardSHA256            string `json:"aa_board_sha256"`
	AAVerificationSHA256     string `json:"aa_verification_sha256"`
	AuthorityID              string `json:"authority_id"`
	BootID                   string `json:"boot_id"`
	CollectorSHA256          string `json:"collector_sha256"`
	GoArtifactSHA256         string `json:"go_artifact_sha256"`
	CArtifactSHA256          string `json:"c_artifact_sha256"`
	GoArtifactIdentitySHA256 string `json:"go_artifact_identity_sha256"`
	CArtifactIdentitySHA256  string `json:"c_artifact_identity_sha256"`
	CollectorIdentitySHA256  string `json:"collector_identity_sha256"`
	FixtureSetSHA256         string `json:"fixture_set_sha256"`
	AuthorizationNonce       string `json:"authorization_nonce"`
	MaximumProductionBoards  int    `json:"maximum_production_boards"`
	RegistryPublicKey        string `json:"registry_public_key"`
}

type aaConsumptionPayload struct {
	Schema                 string `json:"schema"`
	Status                 string `json:"status"`
	RegistryID             string `json:"registry_id"`
	Sequence               uint64 `json:"sequence"`
	PreviousRegistrySHA256 string `json:"previous_registry_sha256"`
	AuthorizationSHA256    string `json:"authorization_sha256"`
	AuthorizationNonce     string `json:"authorization_nonce"`
	ProductionBoardID      string `json:"production_board_id"`
	UseCount               int    `json:"use_count"`
}

type artifactSchemaMetadata struct {
	ID string `json:"$id"`
}

type staticBuildRecipe struct {
	Schema            string                  `json:"schema"`
	Status            string                  `json:"status"`
	VariantNote       string                  `json:"variant_note,omitempty"`
	Target            string                  `json:"target"`
	Driver            recipeDriver            `json:"driver"`
	Runtime           recipeRuntime           `json:"runtime"`
	Grammar           recipeGrammar           `json:"grammar"`
	Toolchain         recipeToolchain         `json:"toolchain"`
	EnvironmentPolicy recipeEnvironmentPolicy `json:"environment_policy"`
	Network           string                  `json:"network"`
	InputMount        string                  `json:"input_mount"`
	Commands          [][]string              `json:"commands"`
	Forbidden         []string                `json:"forbidden"`
	ArtifactFreeze    string                  `json:"artifact_freeze"`
}

type recipeDriver struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
}
type recipeRuntime struct {
	Commit        string `json:"commit"`
	SourceTreeOID string `json:"source_tree_oid"`
	LibCSHA256    string `json:"lib_c_sha256"`
}
type recipeGrammar struct {
	Commit        string `json:"commit"`
	SourceTreeOID string `json:"source_tree_oid"`
	ParserCSHA256 string `json:"parser_c_sha256"`
}
type recipeToolchain struct {
	CCPath          string `json:"cc_path"`
	GCCVersion      string `json:"gcc_version"`
	CCSHA256        string `json:"cc_sha256"`
	CC1SHA256       string `json:"cc1_sha256"`
	Collect2SHA256  string `json:"collect2_sha256"`
	AsSHA256        string `json:"as_sha256"`
	LDSHA256        string `json:"ld_sha256"`
	NMSHA256        string `json:"nm_sha256"`
	ReadelfSHA256   string `json:"readelf_sha256"`
	LibcASHA256     string `json:"libc_a_sha256"`
	LibgccASHA256   string `json:"libgcc_a_sha256"`
	LibgccEHASHA256 string `json:"libgcc_eh_a_sha256"`
}
type recipeEnvironmentPolicy struct {
	Inherit string            `json:"inherit"`
	Set     map[string]string `json:"set"`
}

type artifactBuildManifest struct {
	Schema                string          `json:"schema"`
	ArtifactSchemaSHA256  string          `json:"artifact_schema_sha256"`
	RecipeSHA256          string          `json:"recipe_sha256"`
	Epoch                 buildEpoch      `json:"epoch"`
	Inputs                buildInputs     `json:"inputs"`
	Commands              [][]string      `json:"commands"`
	EnvironmentSHA256     string          `json:"environment_sha256"`
	Toolchain             recipeToolchain `json:"toolchain"`
	Artifact              buildArtifact   `json:"artifact"`
	ELF                   buildELF        `json:"elf"`
	RetainedEvidence      buildEvidence   `json:"retained_evidence"`
	FixtureManifestSHA256 string          `json:"fixture_manifest_sha256"`
}
type buildEpoch struct {
	OCIArchiveSHA256        string `json:"oci_archive_sha256"`
	ManifestDigest          string `json:"manifest_digest"`
	ConfigSHA256            string `json:"config_sha256"`
	RecipeSHA256            string `json:"recipe_sha256"`
	ToolchainManifestSHA256 string `json:"toolchain_manifest_sha256"`
	SysrootManifestSHA256   string `json:"sysroot_manifest_sha256"`
}
type buildInputs struct {
	NormalizedSnapshotSHA256 string `json:"normalized_snapshot_sha256"`
	DriverSHA256             string `json:"driver_sha256"`
	RuntimeTreeOID           string `json:"runtime_tree_oid"`
	RuntimeSHA256            string `json:"runtime_sha256"`
	GrammarTreeOID           string `json:"grammar_tree_oid"`
	GrammarSHA256            string `json:"grammar_sha256"`
}
type buildArtifact struct {
	SHA256  string `json:"sha256"`
	Size    uint64 `json:"size"`
	BuildID string `json:"build_id"`
}
type buildELF struct {
	Class   string `json:"class"`
	Data    string `json:"data"`
	OSABI   string `json:"osabi"`
	Machine string `json:"machine"`
	Type    string `json:"type"`
	Linkage string `json:"linkage"`
}
type buildEvidence struct {
	CompilerStdout fileBinding `json:"compiler_stdout"`
	CompilerStderr fileBinding `json:"compiler_stderr"`
	GCCTrace       fileBinding `json:"gcc_trace"`
	LinkMap        fileBinding `json:"link_map"`
	Readelf        fileBinding `json:"readelf"`
	NM             fileBinding `json:"nm"`
	Objdump        fileBinding `json:"objdump"`
}

func main() {
	var manifestPath, outputPath, trustedAuthorityPublicKey string
	flag.StringVar(&manifestPath, "manifest", "", "sealed board manifest")
	flag.StringVar(&outputPath, "out", "", "new verification receipt path")
	flag.StringVar(&trustedAuthorityPublicKey, "authority-public-key", "", "externally pinned Ed25519 authority public key (base64)")
	flag.Parse()
	if manifestPath == "" || outputPath == "" || trustedAuthorityPublicKey == "" || flag.NArg() != 0 {
		fatalf("usage: go_c_timing_verify --manifest BOARD.json --out RECEIPT.json --authority-public-key BASE64")
	}
	manifestBytes, _, err := readNoFollow(manifestPath)
	if err != nil {
		fatalf("read manifest: %v", err)
	}
	var manifest boardManifest
	if err := decodeStrict(manifestBytes, &manifest); err != nil {
		fatalf("decode manifest: %v", err)
	}
	manifestDir := filepath.Dir(manifestPath)
	if err := verifyManifest(&manifest, manifestDir, trustedAuthorityPublicKey); err != nil {
		fatalf("manifest admission: %v", err)
	}
	ledgerBytes, err := readBoundFile(manifestDir, manifest.Ledger)
	if err != nil {
		fatalf("ledger identity: %v", err)
	}
	windows, tail, err := verifyLedger(&manifest, manifestDir, ledgerBytes)
	if err != nil {
		fatalf("ledger admission: %v", err)
	}
	receipt, err := analyze(&manifest, windows)
	if err != nil {
		fatalf("board analysis: %v", err)
	}
	receipt.ManifestSHA256 = hashBytes(manifestBytes)
	receipt.LedgerSHA256 = hashBytes(ledgerBytes)
	receipt.LedgerTailSHA256 = tail
	encoded, err := json.MarshalIndent(receipt, "", "  ")
	if err != nil {
		fatalf("encode receipt: %v", err)
	}
	encoded = append(encoded, '\n')
	if err := writeNewFile(outputPath, encoded); err != nil {
		fatalf("write receipt: %v", err)
	}
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "go_c_timing_verify: "+format+"\n", args...)
	os.Exit(1)
}

func decodeStrict(data []byte, target any) error {
	if err := rejectDuplicateJSONKeys(data); err != nil {
		return err
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("multiple JSON values")
		}
		return err
	}
	return nil
}

func rejectDuplicateJSONKeys(data []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	var walk func() error
	walk = func() error {
		token, err := decoder.Token()
		if err != nil {
			return err
		}
		delim, ok := token.(json.Delim)
		if !ok {
			return nil
		}
		switch delim {
		case '{':
			seen := make(map[string]struct{})
			for decoder.More() {
				keyToken, err := decoder.Token()
				if err != nil {
					return err
				}
				key, ok := keyToken.(string)
				if !ok {
					return errors.New("JSON object key is not a string")
				}
				if _, duplicate := seen[key]; duplicate {
					return fmt.Errorf("duplicate JSON object key %q", key)
				}
				seen[key] = struct{}{}
				if err := walk(); err != nil {
					return err
				}
			}
			_, err = decoder.Token()
			return err
		case '[':
			for decoder.More() {
				if err := walk(); err != nil {
					return err
				}
			}
			_, err = decoder.Token()
			return err
		default:
			return fmt.Errorf("unexpected JSON delimiter %q", delim)
		}
	}
	if err := walk(); err != nil {
		return err
	}
	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("multiple JSON values")
		}
		return err
	}
	return nil
}

func writeNewFile(path string, data []byte) error {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	if _, err := file.Write(data); err != nil {
		_ = file.Close()
		return err
	}
	return file.Close()
}

func verifyManifest(manifest *boardManifest, base, trustedAuthorityPublicKey string) error {
	if manifest.Schema != boardSchema {
		return fmt.Errorf("schema=%q, want %q", manifest.Schema, boardSchema)
	}
	if manifest.BoardID == "" || strings.TrimSpace(manifest.BoardID) != manifest.BoardID {
		return errors.New("board_id must be non-empty and unpadded")
	}
	if manifest.Mode != modeAA && manifest.Mode != modeProduction {
		return fmt.Errorf("unsupported mode %q", manifest.Mode)
	}
	trustedKeyBytes, err := base64.StdEncoding.DecodeString(trustedAuthorityPublicKey)
	if err != nil || len(trustedKeyBytes) != ed25519.PublicKeySize || manifest.Authority.PublicKey != trustedAuthorityPublicKey {
		return errors.New("board authority key does not match the external trust anchor")
	}
	if _, err := readBoundFile(base, manifest.Contract); err != nil {
		return fmt.Errorf("contract: %w", err)
	}
	schemaBytes, err := readBoundFile(base, manifest.ArtifactSchema)
	if err != nil {
		return fmt.Errorf("artifact schema: %w", err)
	}
	if err := rejectDuplicateJSONKeys(schemaBytes); err != nil {
		return fmt.Errorf("artifact schema: %w", err)
	}
	var schemaMeta artifactSchemaMetadata
	if err := json.Unmarshal(schemaBytes, &schemaMeta); err != nil || schemaMeta.ID != "gts-locked-static-c-timing-artifact/v3" {
		return errors.New("artifact schema identity is invalid")
	}
	recipeBytes, err := readBoundFile(base, manifest.BuildRecipe)
	if err != nil {
		return fmt.Errorf("build recipe: %w", err)
	}
	var recipe staticBuildRecipe
	if err := decodeStrict(recipeBytes, &recipe); err != nil {
		return fmt.Errorf("build recipe: %w", err)
	}
	if err := verifyRecipe(&recipe); err != nil {
		return err
	}
	if manifest.Authority.Status != "restricted" ||
		manifest.Authority.OperatorRootEquivalent ||
		manifest.Authority.AuthorityID == "" || manifest.Authority.BootID == "" {
		return errors.New("timing requires a non-root-equivalent restricted authority and boot binding")
	}
	if err := verifyAuthorityReceipt(manifest, base); err != nil {
		return err
	}
	if err := verifyCurrentIdentity(base, manifest.Collector.Path,
		manifest.Collector.SHA256, manifest.Collector.Device,
		manifest.Collector.Inode, manifest.Collector.Size); err != nil {
		return fmt.Errorf("collector: %w", err)
	}
	if err := verifyClockAdmission(manifest, base); err != nil {
		return err
	}
	if err := verifySchedule(manifest); err != nil {
		return err
	}
	if err := requireHash(manifest.Ledger.SHA256, "ledger sha256"); err != nil {
		return err
	}
	if len(manifest.Artifacts) != requiredArtifactCount(manifest.Mode) {
		return fmt.Errorf("artifact count=%d, want %d", len(manifest.Artifacts), requiredArtifactCount(manifest.Mode))
	}
	goArtifact, ok := manifest.Artifacts["go"]
	if !ok {
		return errors.New("go artifact is required")
	}
	if err := verifyArtifact(base, "go", goArtifact, &recipe, manifest); err != nil {
		return err
	}
	cArtifact, ok := manifest.Artifacts["c"]
	if !ok {
		return errors.New("sealed board requires the C artifact even when A/A executes only Go")
	}
	if err := verifyArtifact(base, "c", cArtifact, &recipe, manifest); err != nil {
		return err
	}
	if manifest.Mode == modeProduction {
		if manifest.AAAuthorization == nil {
			return errors.New("production board lacks one sealed A/A authorization")
		}
		if err := verifyAAAuthorization(manifest, base); err != nil {
			return err
		}
		if manifest.AAGate != nil {
			return errors.New("production board must not redefine A/A null gates")
		}
	} else {
		if manifest.AAAuthorization != nil {
			return errors.New("A/A board must not consume an A/A authorization")
		}
		if manifest.AAGate == nil || manifest.AAGate.PublicBenchstatSameDirectionSignificantFixtures < 0 ||
			manifest.AAGate.PublicBenchstatSameDirectionSignificantFixtures > 1 {
			return errors.New("A/A benchstat public same-direction significance gate failed")
		}
		if err := verifyBenchstatReceipt(manifest, base); err != nil {
			return err
		}
	}
	if err := verifyFixtures(manifest.Fixtures, base); err != nil {
		return err
	}
	if err := verifyDeepReproduction(manifest, base); err != nil {
		return err
	}
	return nil
}

func requiredArtifactCount(mode string) int {
	return 2
}

func verifySignedPayload(base string, binding fileBinding, trustedPublicKey string, target any) (string, error) {
	raw, err := readBoundFile(base, binding)
	if err != nil {
		return "", err
	}
	var envelope signedEnvelope
	if err := decodeStrict(raw, &envelope); err != nil {
		return "", err
	}
	if envelope.Schema != "gts-signed-receipt/v1" || envelope.PublicKey != trustedPublicKey || len(envelope.Payload) == 0 {
		return "", errors.New("signed receipt envelope identity mismatch")
	}
	publicKey, err := base64.StdEncoding.DecodeString(envelope.PublicKey)
	if err != nil || len(publicKey) != ed25519.PublicKeySize {
		return "", errors.New("invalid receipt public key")
	}
	signature, err := base64.StdEncoding.DecodeString(envelope.Signature)
	if err != nil || len(signature) != ed25519.SignatureSize || !ed25519.Verify(ed25519.PublicKey(publicKey), envelope.Payload, signature) {
		return "", errors.New("receipt Ed25519 signature verification failed")
	}
	if err := decodeStrict(envelope.Payload, target); err != nil {
		return "", fmt.Errorf("signed payload: %w", err)
	}
	return hashBytes(raw), nil
}

func verifyAuthorityReceipt(manifest *boardManifest, base string) error {
	authority := manifest.Authority
	if authority.Status != "restricted" || authority.OperatorRootEquivalent || authority.AuthorityID == "" ||
		authority.SubjectID == "" || authority.BootID == "" || authority.PublicKey == "" {
		return errors.New("timing requires a named non-root-equivalent restricted authority, subject, boot, and trust key")
	}
	var payload authorityReceiptPayload
	if _, err := verifySignedPayload(base, authority.Receipt, authority.PublicKey, &payload); err != nil {
		return fmt.Errorf("authority receipt: %w", err)
	}
	if payload.Schema != "gts-restricted-timing-authority/v3" || payload.Status != "restricted" ||
		payload.AuthorityID != authority.AuthorityID || payload.SubjectID != authority.SubjectID ||
		payload.BootID != authority.BootID || payload.CollectorSHA256 != manifest.Collector.SHA256 || payload.Nonce == "" ||
		payload.OperatorRootEquivalent || payload.OperatorUnrestrictedSudo || payload.OperatorRootCapabilities ||
		payload.OperatorPtraceAccess || payload.OperatorBPFAccess || payload.OperatorRawTimingRead || payload.OperatorCollectorIdentity {
		return errors.New("authority receipt privilege/subject/boot/collector claims do not admit timing")
	}
	return nil
}

func verifyClockAdmission(manifest *boardManifest, base string) error {
	var payload clockAdmissionPayload
	if _, err := verifySignedPayload(base, manifest.ClockAdmission, manifest.Authority.PublicKey, &payload); err != nil {
		return fmt.Errorf("clock admission: %w", err)
	}
	if payload.Schema != "gts-clock-admission/v3" || payload.Decision != "admitted" || payload.RawTimingReadable ||
		payload.BoardID != manifest.BoardID || payload.AuthorityID != manifest.Authority.AuthorityID ||
		payload.BootID != manifest.Authority.BootID || payload.CollectorSHA256 != manifest.Collector.SHA256 ||
		payload.Mode != manifest.Mode || payload.ContractSHA256 != manifest.Contract.SHA256 ||
		payload.ArtifactSchemaSHA256 != manifest.ArtifactSchema.SHA256 || payload.BuildRecipeSHA256 != manifest.BuildRecipe.SHA256 ||
		payload.GoArtifactSHA256 != manifest.Artifacts["go"].SHA256 || payload.CArtifactSHA256 != manifest.Artifacts["c"].SHA256 ||
		payload.GoArtifactIdentitySHA256 != artifactBindingHash(manifest.Artifacts["go"]) ||
		payload.CArtifactIdentitySHA256 != artifactBindingHash(manifest.Artifacts["c"]) ||
		payload.CollectorIdentitySHA256 != collectorBindingHash(manifest.Collector) ||
		payload.FixtureSetSHA256 != fixtureSetHash(manifest.Fixtures) || payload.ScheduleSHA256 != scheduleHash(manifest.Schedule) {
		return errors.New("clock admission payload does not bind this restricted board")
	}
	return nil
}

func verifyBenchstatReceipt(manifest *boardManifest, base string) error {
	var payload benchstatPayload
	if _, err := verifySignedPayload(base, manifest.AAGate.BenchstatReceipt, manifest.Authority.PublicKey, &payload); err != nil {
		return fmt.Errorf("A/A benchstat receipt: %w", err)
	}
	if payload.Schema != "gts-aa-benchstat-gate/v3" || payload.Status != "passed" || payload.BoardID != manifest.BoardID ||
		payload.SameDirectionSignificantFixtures != manifest.AAGate.PublicBenchstatSameDirectionSignificantFixtures ||
		payload.SameDirectionSignificantFixtures < 0 || payload.SameDirectionSignificantFixtures > 1 {
		return errors.New("A/A benchstat signed gate mismatch")
	}
	return nil
}

func verifyDeepReproduction(manifest *boardManifest, base string) error {
	var payload deepReproductionPayload
	if _, err := verifySignedPayload(base, manifest.DeepReproductionReceipt, manifest.Authority.PublicKey, &payload); err != nil {
		return fmt.Errorf("deep reproduction receipt: %w", err)
	}
	if payload.Schema != "gts-go-c-deep-reproduction/v3" || payload.Status != "passed" || !payload.ConstantsMatched ||
		payload.ReviewerSubjectID == "" || payload.ReviewerSubjectID == manifest.Authority.SubjectID ||
		payload.GoSourceSHA256 != manifest.Artifacts["go"].SourceSHA256 || payload.CSourceSHA256 != manifest.Artifacts["c"].SourceSHA256 ||
		payload.FixtureSetSHA256 != fixtureSetHash(manifest.Fixtures) || payload.ToolRuntimeSHA256 == "" ||
		len(payload.GoCommand) == 0 || len(payload.CCommand) == 0 || len(payload.GoStdoutSHA256) != len(fixtures) || len(payload.CStdoutSHA256) != len(fixtures) {
		return errors.New("deep reproduction receipt does not bind both implementations, fixtures, commands, runtime, and successful constants")
	}
	if err := requireHash(payload.ToolRuntimeSHA256, "deep reproduction tool/runtime"); err != nil {
		return err
	}
	for _, fixture := range fixtures {
		goHash, goOK := payload.GoStdoutSHA256[fixture]
		cHash, cOK := payload.CStdoutSHA256[fixture]
		if !goOK || !cOK || requireHash(goHash, "Go deep stdout") != nil || requireHash(cHash, "C deep stdout") != nil || goHash != cHash {
			return fmt.Errorf("deep reproduction output mismatch for %s", fixture)
		}
	}
	return nil
}

func fixtureSetHash(entries []fixtureBinding) string {
	encoded, err := json.Marshal(entries)
	if err != nil {
		return ""
	}
	return hashBytes(encoded)
}

func artifactBindingHash(artifact artifactBinding) string {
	encoded, err := json.Marshal(artifact)
	if err != nil {
		return ""
	}
	return hashBytes(encoded)
}

func collectorBindingHash(collector collectorBinding) string {
	encoded, err := json.Marshal(collector)
	if err != nil {
		return ""
	}
	return hashBytes(encoded)
}

func scheduleHash(schedule scheduleBinding) string {
	encoded, err := json.Marshal(schedule)
	if err != nil {
		return ""
	}
	return hashBytes(encoded)
}

func verifySchedule(manifest *boardManifest) error {
	schedule := manifest.Schedule
	if !equalStrings(schedule.Lanes, lanes) || !equalStrings(schedule.Fixtures, fixtures) ||
		!equalInts(schedule.Cycles, []int{1, 2, 3, 4, 5}) || schedule.Samples != 240 ||
		schedule.ScheduledAttemptsPerWindow != 5 || schedule.DeferredAttemptsPerWindow != 5 ||
		schedule.MinimumRetrySpacingNS < 61_000_000_000 {
		return errors.New("schedule must be the frozen 3x4x5x4 board with 5+5 attempts and >=61s retry spacing")
	}
	wantPositions := []string{"A", "B", "B", "A"}
	if manifest.Mode == modeProduction {
		wantPositions = []string{"Go", "C", "C", "Go"}
	}
	if !equalStrings(schedule.Positions, wantPositions) {
		return fmt.Errorf("positions=%v, want %v", schedule.Positions, wantPositions)
	}
	return nil
}

func verifyArtifact(base, key string, artifact artifactBinding, recipe *staticBuildRecipe, manifest *boardManifest) error {
	wantTemplate := []string{"go_timing_oracle_" + key, "--bench", "--lane", "{lane}", "--fixture", "{fixture}", "--source-fd", "{source_fd}"}
	wantTags := []string{}
	if key == "go" {
		wantTags = []string{"gts_parsercorephase0"}
	}
	if artifact.Implementation != key || !equalStrings(artifact.CommandTemplate, wantTemplate) ||
		!equalStrings(artifact.Tags, wantTags) ||
		artifact.EnvironmentSHA256 == "" {
		return fmt.Errorf("artifact %s identity fields are incomplete", key)
	}
	if err := requireHash(artifact.EnvironmentSHA256, key+" environment sha256"); err != nil {
		return err
	}
	if err := requireHash(artifact.SourceSHA256, key+" source sha256"); err != nil {
		return err
	}
	if err := verifyCurrentIdentity(base, artifact.Path, artifact.SHA256,
		artifact.Device, artifact.Inode, artifact.Size); err != nil {
		return fmt.Errorf("artifact %s: %w", key, err)
	}
	buildBytes, err := readBoundFile(base, artifact.BuildManifest)
	if err != nil {
		return fmt.Errorf("artifact %s build manifest: %w", key, err)
	}
	if key == "c" {
		if artifact.StaticELF == nil {
			return errors.New("C artifact lacks static ELF admission")
		}
		if err := verifyStaticELF(base, resolve(base, artifact.Path), artifact, artifact.StaticELF); err != nil {
			return fmt.Errorf("C static ELF: %w", err)
		}
		if err := verifyCBuildManifest(buildBytes, base, artifact, recipe, manifest); err != nil {
			return fmt.Errorf("C build manifest: %w", err)
		}
	} else if artifact.StaticELF != nil {
		return errors.New("Go artifact must not claim the C static ELF record")
	}
	return nil
}

func verifyRecipe(recipe *staticBuildRecipe) error {
	if recipe.Schema != "gts-locked-static-c-timing-build-recipe/v3" ||
		recipe.Status != "source_only_unbuilt" || recipe.Target != "linux/amd64" ||
		recipe.Network != "none" || recipe.InputMount != "read_only_normalized_snapshot" ||
		recipe.EnvironmentPolicy.Inherit != "none" {
		return errors.New("build recipe lifecycle/target/isolation policy drifted")
	}
	wantEnvironment := map[string]string{
		"LANG": "C", "LC_ALL": "C", "TZ": "UTC", "SOURCE_DATE_EPOCH": "0",
		"GIT_CONFIG_GLOBAL": "/dev/null", "GIT_CONFIG_NOSYSTEM": "1",
		"PATH": "/usr/bin:/bin", "HOME": "/nonexistent",
	}
	if !equalStringMap(recipe.EnvironmentPolicy.Set, wantEnvironment) {
		return errors.New("recipe environment must be an exact empty-inheritance allowlist")
	}
	if len(recipe.Commands) != 4 {
		return errors.New("recipe must contain exactly four commands")
	}
	for index := 0; index < 3; index++ {
		command := recipe.Commands[index]
		for _, required := range []string{"-O2", "-DNDEBUG", "-std=c11", "-fno-pie", "-c"} {
			if !contains(command, required) {
				return fmt.Errorf("recipe compile command %d lacks %s", index+1, required)
			}
		}
	}
	link := recipe.Commands[3]
	for _, required := range []string{"-static", "-no-pie", "-O2", "-Wl,-Map=go_timing_oracle.map"} {
		if !contains(link, required) {
			return fmt.Errorf("recipe link command lacks %s", required)
		}
	}
	for _, command := range recipe.Commands {
		if len(command) == 0 || command[0] != recipe.Toolchain.CCPath {
			return errors.New("recipe command/toolchain path mismatch")
		}
		for _, argument := range command {
			lower := strings.ToLower(argument)
			for _, forbidden := range []string{"-fpic", "-fpie", "-pie", "-flto", "-pg", "--coverage", "sanitize", "profile-generate", "profile-use"} {
				if strings.Contains(lower, forbidden) && argument != "-fno-pie" && argument != "-no-pie" {
					return fmt.Errorf("recipe contains forbidden build argument %q", argument)
				}
			}
		}
	}
	for name, hash := range map[string]string{
		"driver": recipe.Driver.SHA256, "runtime lib.c": recipe.Runtime.LibCSHA256,
		"grammar parser.c": recipe.Grammar.ParserCSHA256, "cc": recipe.Toolchain.CCSHA256,
		"cc1": recipe.Toolchain.CC1SHA256, "collect2": recipe.Toolchain.Collect2SHA256,
		"as": recipe.Toolchain.AsSHA256, "ld": recipe.Toolchain.LDSHA256,
		"nm": recipe.Toolchain.NMSHA256, "readelf": recipe.Toolchain.ReadelfSHA256,
		"libc.a": recipe.Toolchain.LibcASHA256, "libgcc.a": recipe.Toolchain.LibgccASHA256,
		"libgcc_eh.a": recipe.Toolchain.LibgccEHASHA256,
	} {
		if err := requireHash(hash, "recipe "+name); err != nil {
			return err
		}
	}
	for name, oid := range map[string]string{
		"runtime commit": recipe.Runtime.Commit, "runtime tree": recipe.Runtime.SourceTreeOID,
		"grammar commit": recipe.Grammar.Commit, "grammar tree": recipe.Grammar.SourceTreeOID,
	} {
		if !isLowerHex(oid, 40) {
			return fmt.Errorf("recipe %s must be a lowercase 40-byte Git OID", name)
		}
	}
	return nil
}

func verifyCBuildManifest(data []byte, base string, artifact artifactBinding, recipe *staticBuildRecipe, board *boardManifest) error {
	var build artifactBuildManifest
	if err := decodeStrict(data, &build); err != nil {
		return err
	}
	if build.Schema != "gts-locked-static-c-timing-artifact/v3" ||
		build.ArtifactSchemaSHA256 != board.ArtifactSchema.SHA256 || build.RecipeSHA256 != board.BuildRecipe.SHA256 || build.Epoch.RecipeSHA256 != board.BuildRecipe.SHA256 ||
		build.EnvironmentSHA256 != artifact.EnvironmentSHA256 || build.Artifact.SHA256 != artifact.SHA256 ||
		build.Artifact.Size != artifact.Size || build.Inputs.DriverSHA256 != recipe.Driver.SHA256 ||
		build.Inputs.DriverSHA256 != artifact.SourceSHA256 || build.Inputs.RuntimeTreeOID != recipe.Runtime.SourceTreeOID ||
		build.Inputs.GrammarTreeOID != recipe.Grammar.SourceTreeOID || build.FixtureManifestSHA256 != fixtureSetHash(board.Fixtures) ||
		!equalCommandLists(build.Commands, recipe.Commands) || build.Toolchain != recipe.Toolchain {
		return errors.New("artifact manifest does not cross-bind recipe, epoch, sources, toolchain, environment, fixture set, commands, or artifact")
	}
	if !isLowerHexString(build.Artifact.BuildID) || build.Artifact.BuildID == strings.Repeat("0", len(build.Artifact.BuildID)) {
		return errors.New("artifact build ID must be nonzero lowercase hexadecimal")
	}
	for name, hash := range map[string]string{
		"OCI archive": build.Epoch.OCIArchiveSHA256, "OCI manifest": build.Epoch.ManifestDigest,
		"OCI config": build.Epoch.ConfigSHA256, "toolchain manifest": build.Epoch.ToolchainManifestSHA256,
		"sysroot manifest": build.Epoch.SysrootManifestSHA256, "normalized snapshot": build.Inputs.NormalizedSnapshotSHA256,
		"runtime source": build.Inputs.RuntimeSHA256, "grammar source": build.Inputs.GrammarSHA256,
	} {
		if err := requireHash(hash, "build "+name); err != nil {
			return err
		}
	}
	static := artifact.StaticELF
	if build.ELF != (buildELF{Class: static.Class, Data: static.Data, OSABI: static.OSABI, Machine: static.Machine, Type: static.Type, Linkage: static.Linkage}) {
		return errors.New("artifact ELF manifest and board admission differ")
	}
	evidence := map[string]fileBinding{
		"compiler stdout": build.RetainedEvidence.CompilerStdout, "compiler stderr": build.RetainedEvidence.CompilerStderr,
		"gcc trace": build.RetainedEvidence.GCCTrace, "link map": build.RetainedEvidence.LinkMap,
		"readelf": build.RetainedEvidence.Readelf, "nm": build.RetainedEvidence.NM, "objdump": build.RetainedEvidence.Objdump,
	}
	for name, binding := range evidence {
		if _, err := readBoundFile(base, binding); err != nil {
			return fmt.Errorf("%s evidence: %w", name, err)
		}
	}
	if build.RetainedEvidence.LinkMap != static.RetainedLinkMap || build.RetainedEvidence.Readelf != static.RetainedReadelf ||
		build.RetainedEvidence.NM != static.RetainedNM || build.RetainedEvidence.Objdump != static.RetainedObjdump {
		return errors.New("build and static-admission evidence paths/hashes differ")
	}
	return nil
}

func equalCommandLists(a, b [][]string) bool {
	if len(a) != len(b) {
		return false
	}
	for index := range a {
		if !equalStrings(a[index], b[index]) {
			return false
		}
	}
	return true
}

func equalStringMap(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for key, value := range a {
		if b[key] != value {
			return false
		}
	}
	return true
}

func verifyStaticELF(base, path string, artifact artifactBinding, binding *staticELFBinding) error {
	if binding.Class != "ELFCLASS64" || binding.Data != "ELFDATA2LSB" ||
		binding.OSABI != "ELFOSABI_SYSV:0" || binding.Machine != "EM_X86_64" ||
		binding.Type != "ET_EXEC:2" || binding.Linkage != "elf:no_interp,no_dynamic,no_needed" {
		return errors.New("manifest ELF literals do not match the v3 contract")
	}
	for name, evidence := range map[string]fileBinding{
		"readelf receipt":  binding.RetainedReadelf,
		"nm receipt":       binding.RetainedNM,
		"objdump receipt":  binding.RetainedObjdump,
		"link-map receipt": binding.RetainedLinkMap,
	} {
		if _, err := readBoundFile(base, evidence); err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
	}
	artifactBytes, opened, err := readNoFollow(path)
	if err != nil {
		return err
	}
	if opened.SHA256 != artifact.SHA256 || opened.Device != artifact.Device || opened.Inode != artifact.Inode || opened.Size != artifact.Size {
		return errors.New("ELF inspection descriptor differs from the sealed artifact identity")
	}
	file, err := elf.NewFile(bytes.NewReader(artifactBytes))
	if err != nil {
		return err
	}
	defer file.Close()
	if file.Class != elf.ELFCLASS64 || file.Data != elf.ELFDATA2LSB ||
		file.OSABI != elf.ELFOSABI_NONE || file.Machine != elf.EM_X86_64 ||
		file.Type != elf.ET_EXEC {
		return fmt.Errorf("ELF header class=%s data=%s osabi=%s machine=%s type=%s",
			file.Class, file.Data, file.OSABI, file.Machine, file.Type)
	}
	for _, program := range file.Progs {
		if program.Type == elf.PT_INTERP || program.Type == elf.PT_DYNAMIC {
			return fmt.Errorf("forbidden program header %s", program.Type)
		}
	}
	if file.Section(".dynamic") != nil || file.Section(".interp") != nil {
		return errors.New("dynamic/interpreter section present")
	}
	if needed, err := file.DynString(elf.DT_NEEDED); err == nil && len(needed) != 0 {
		return fmt.Errorf("DT_NEEDED=%v", needed)
	}
	symbols, err := file.Symbols()
	if err != nil {
		return fmt.Errorf("read static symbols: %w", err)
	}
	defined := make(map[string]bool, len(symbols))
	for _, symbol := range symbols {
		name := symbol.Name
		lower := strings.ToLower(name)
		for _, forbidden := range []string{
			"mcount", "__fentry__", "__cyg_profile", "gcov", "llvm_profile",
			"sanitizer", "asan", "lsan", "msan", "ubsan", "tsan",
			"coverage", "profil", "perf_event", "bpf", "uprobe", "work_count",
			"observer", "tracepoint", "got_trace", "gts_trace",
		} {
			if strings.Contains(lower, forbidden) {
				return fmt.Errorf("forbidden symbol %q", name)
			}
		}
		if symbol.Section == elf.SHN_UNDEF {
			if name == "" {
				continue
			}
			return fmt.Errorf("undefined static symbol %q", name)
		}
		defined[name] = true
	}
	required := []string{
		"main", "tree_sitter_go", "ts_parser_new", "ts_parser_set_language",
		"ts_parser_parse", "ts_parser_parse_string", "ts_tree_cursor_new",
		"ts_tree_cursor_goto_first_child", "ts_tree_cursor_goto_next_sibling",
		"ts_tree_cursor_goto_parent", "ts_tree_cursor_delete", "ts_tree_delete",
	}
	if !equalStringSets(binding.RequiredSymbols, required) {
		return errors.New("manifest required-symbol set drifted")
	}
	for _, name := range required {
		if !defined[name] {
			return fmt.Errorf("required symbol %q is not defined", name)
		}
	}
	return nil
}

func verifyFixtures(got []fixtureBinding, base string) error {
	if len(got) != len(fixtures) {
		return fmt.Errorf("fixture count=%d, want %d", len(got), len(fixtures))
	}
	want := map[string]fixtureBinding{
		"rewrite":       {Name: "rewrite", SHA256: "74c0705f8729670559492fb5460a01b2a1a2a109928e1aeb52736e485e8ff097", Bytes: 5116, DeepSHA256: "b3f9814b65763642d4eac58b9065018048ea13e6f10d56afb28a0479bf5a68a1", DeepBytes: 74152, VisitedNodes: 1524, Checksum: "c71076528146b3f8"},
		"query_compile": {Name: "query_compile", SHA256: "b788ee19b0075f0b9b567a9f93ea657e715bc8a6a40a99d3ca5c761404e71894", Bytes: 20168, DeepSHA256: "ecc090a83a4343a1c7c2afbad63277f5b4d60c42d8d94a2af2a9b16e46f2ccb5", DeepBytes: 370051, VisitedNodes: 7524, Checksum: "9c30f7f940ebadf3"},
		"language":      {Name: "language", SHA256: "009aa9fd5352c712f3839670c7df8a9b00ae878ee20dc88131a438b2d5edfd9a", Bytes: 41387, DeepSHA256: "583df223904fe414c33bba3b474c6557ecdb20e7f47e304b9a09bfcc2da44539", DeepBytes: 347920, VisitedNodes: 7082, Checksum: "41c2eb88ff43be57"},
		"grammargen_lr": {Name: "grammargen_lr", SHA256: "a7e4a1a64b25a60aea36183b9d6d53dcd9240942cdb10e67a3cf9e6ce30f95b2", Bytes: 235626, DeepSHA256: "1472cfd9a014d4034dbc1456afd12c282630ef787c3543cf0cecb73619883ad2", DeepBytes: 3502352, VisitedNodes: 71768, Checksum: "4c3be1f65cf3aed4"},
	}
	for index, name := range fixtures {
		entry := got[index]
		expected := want[name]
		if entry.Name != expected.Name || entry.SHA256 != expected.SHA256 ||
			entry.Bytes != expected.Bytes || entry.DeepSHA256 != expected.DeepSHA256 ||
			entry.DeepBytes != expected.DeepBytes || entry.VisitedNodes != expected.VisitedNodes ||
			entry.Checksum != expected.Checksum {
			return fmt.Errorf("fixture %d (%s) does not match frozen identity/constants", index, name)
		}
		if err := verifyCurrentIdentity(base, entry.Path, entry.SHA256, entry.Device, entry.Inode, entry.Bytes); err != nil {
			return fmt.Errorf("fixture %s: %w", name, err)
		}
	}
	return nil
}

func verifyAAAuthorization(manifest *boardManifest, base string) error {
	auth := manifest.AAAuthorization
	aaBoardRaw, err := readBoundFile(base, auth.SealedAABoard)
	if err != nil {
		return fmt.Errorf("sealed A/A board: %w", err)
	}
	var aaBoard boardManifest
	if err := decodeStrict(aaBoardRaw, &aaBoard); err != nil {
		return fmt.Errorf("sealed A/A board: %w", err)
	}
	if aaBoard.Schema != boardSchema || aaBoard.Mode != modeAA || aaBoard.BoardID == "" ||
		aaBoard.Authority != manifest.Authority ||
		aaBoard.Collector != manifest.Collector || artifactBindingHash(aaBoard.Artifacts["go"]) != artifactBindingHash(manifest.Artifacts["go"]) ||
		artifactBindingHash(aaBoard.Artifacts["c"]) != artifactBindingHash(manifest.Artifacts["c"]) || fixtureSetHash(aaBoard.Fixtures) != fixtureSetHash(manifest.Fixtures) {
		return errors.New("sealed A/A board mode or identities differ from production")
	}
	aaVerificationRaw, err := readBoundFile(base, auth.AAVerification)
	if err != nil {
		return fmt.Errorf("A/A verification receipt: %w", err)
	}
	var aaVerification verificationReceipt
	if err := decodeStrict(aaVerificationRaw, &aaVerification); err != nil {
		return err
	}
	if aaVerification.Schema != receiptSchema || aaVerification.Status != "passed" || aaVerification.Mode != modeAA ||
		aaVerification.AANullGatesPassed == nil || !*aaVerification.AANullGatesPassed || aaVerification.BoardID != aaBoard.BoardID || aaVerification.ManifestSHA256 != auth.SealedAABoard.SHA256 {
		return errors.New("A/A verification receipt does not prove passed A/A null gates")
	}
	var authorization aaAuthorizationPayload
	authorizationHash, err := verifySignedPayload(base, auth.AuthorizationReceipt, manifest.Authority.PublicKey, &authorization)
	if err != nil {
		return fmt.Errorf("A/A authorization receipt: %w", err)
	}
	if authorization.Schema != "gts-aa-production-authorization/v3" || authorization.Status != "authorized_once" ||
		authorization.ProductionBoardID != manifest.BoardID || authorization.AABoardID != aaBoard.BoardID ||
		authorization.AABoardSHA256 != auth.SealedAABoard.SHA256 || authorization.AAVerificationSHA256 != auth.AAVerification.SHA256 ||
		authorization.AuthorityID != manifest.Authority.AuthorityID || authorization.BootID != manifest.Authority.BootID ||
		authorization.CollectorSHA256 != manifest.Collector.SHA256 || authorization.GoArtifactSHA256 != manifest.Artifacts["go"].SHA256 ||
		authorization.CArtifactSHA256 != manifest.Artifacts["c"].SHA256 || authorization.FixtureSetSHA256 != fixtureSetHash(manifest.Fixtures) ||
		authorization.GoArtifactIdentitySHA256 != artifactBindingHash(manifest.Artifacts["go"]) ||
		authorization.CArtifactIdentitySHA256 != artifactBindingHash(manifest.Artifacts["c"]) ||
		authorization.CollectorIdentitySHA256 != collectorBindingHash(manifest.Collector) ||
		authorization.AuthorizationNonce == "" || authorization.MaximumProductionBoards != 1 ||
		authorization.RegistryPublicKey != auth.RegistryPublicKey || auth.RegistryPublicKey == manifest.Authority.PublicKey {
		return errors.New("signed A/A authorization does not authorize exactly this one production board")
	}
	var consumption aaConsumptionPayload
	if _, err := verifySignedPayload(base, auth.ConsumptionReceipt, auth.RegistryPublicKey, &consumption); err != nil {
		return fmt.Errorf("A/A consumption registry receipt: %w", err)
	}
	if consumption.Schema != "gts-aa-authorization-consumption/v3" || consumption.Status != "consumed_once" ||
		consumption.RegistryID == "" || consumption.Sequence == 0 || consumption.AuthorizationSHA256 != authorizationHash ||
		consumption.AuthorizationNonce != authorization.AuthorizationNonce || consumption.ProductionBoardID != manifest.BoardID ||
		consumption.UseCount != 1 || (consumption.PreviousRegistrySHA256 != zeroHash && requireHash(consumption.PreviousRegistrySHA256, "previous registry receipt") != nil) {
		return errors.New("independent registry did not prove single-use authorization consumption")
	}
	return nil
}

func verifyLedger(manifest *boardManifest, base string, data []byte) ([]acceptedWindow, string, error) {
	scanner := bufio.NewScanner(bytes.NewReader(data))
	scanner.Buffer(make([]byte, 64*1024), 8*1024*1024)
	previous := zeroHash
	var records []ledgerRecord
	for scanner.Scan() {
		raw := append([]byte(nil), scanner.Bytes()...)
		if len(raw) == 0 || bytes.TrimSpace(raw) == nil || !bytes.Equal(raw, bytes.TrimSpace(raw)) {
			return nil, "", fmt.Errorf("ledger line %d is blank or padded", len(records)+1)
		}
		var record ledgerRecord
		if err := decodeStrict(raw, &record); err != nil {
			return nil, "", fmt.Errorf("ledger line %d: %w", len(records)+1, err)
		}
		if record.Ordinal != uint64(len(records)+1) || record.PredecessorSHA256 != previous {
			return nil, "", fmt.Errorf("ledger line %d ordinal/predecessor mismatch", len(records)+1)
		}
		previous = hashBytes(raw)
		records = append(records, record)
	}
	if err := scanner.Err(); err != nil {
		return nil, "", err
	}
	if len(records) == 0 {
		return nil, "", errors.New("ledger is empty")
	}

	type attemptState struct {
		accepted    bool
		lastEnd     uint64
		nextAttempt int
	}
	states := make([]attemptState, 60)
	for index := range states {
		states[index].nextAttempt = 1
	}
	accepted := make([]*acceptedWindow, 60)
	deferredStarted := false
	lastScheduledWindow := -1
	lastDeferredWindow := -1
	rules := positionsAA
	if manifest.Mode == modeProduction {
		rules = positionsProduction
	}

	for line, record := range records {
		window, lane, fixture, cycle, ok := windowIdentity(record.Window)
		if !ok || record.Window != window || record.Lane != lane ||
			record.Fixture != fixture || record.Cycle != cycle {
			return nil, "", fmt.Errorf("ledger line %d window identity mismatch", line+1)
		}
		state := &states[record.Window-1]
		if state.accepted {
			return nil, "", fmt.Errorf("ledger line %d retries accepted window %d", line+1, record.Window)
		}
		if record.Attempt != state.nextAttempt || record.StartMonotonicNS == 0 ||
			record.EndMonotonicNS < record.StartMonotonicNS {
			return nil, "", fmt.Errorf("ledger line %d attempt/time sequence mismatch", line+1)
		}
		if state.lastEnd != 0 && (record.StartMonotonicNS < state.lastEnd ||
			record.StartMonotonicNS-state.lastEnd < manifest.Schedule.MinimumRetrySpacingNS) {
			return nil, "", fmt.Errorf("ledger line %d retry spacing is below the frozen minimum", line+1)
		}
		state.lastEnd = record.EndMonotonicNS
		state.nextAttempt++

		switch record.Phase {
		case "scheduled":
			if deferredStarted || record.Attempt > manifest.Schedule.ScheduledAttemptsPerWindow ||
				record.Window < lastScheduledWindow {
				return nil, "", fmt.Errorf("ledger line %d invalid scheduled phase ordering", line+1)
			}
			if lastScheduledWindow > 0 && record.Window != lastScheduledWindow {
				prior := states[lastScheduledWindow-1]
				if !prior.accepted && prior.nextAttempt != manifest.Schedule.ScheduledAttemptsPerWindow+1 {
					return nil, "", fmt.Errorf("ledger line %d left scheduled window %d before five attempts", line+1, lastScheduledWindow)
				}
			}
			lastScheduledWindow = record.Window
		case "deferred":
			if !deferredStarted {
				for index, prior := range states {
					if !prior.accepted && prior.nextAttempt != manifest.Schedule.ScheduledAttemptsPerWindow+1 {
						return nil, "", fmt.Errorf("ledger line %d began deferred phase before window %d completed scheduled attempts", line+1, index+1)
					}
				}
				deferredStarted = true
			}
			if record.Attempt <= manifest.Schedule.ScheduledAttemptsPerWindow ||
				record.Attempt > manifest.Schedule.ScheduledAttemptsPerWindow+manifest.Schedule.DeferredAttemptsPerWindow ||
				record.Window < lastDeferredWindow {
				return nil, "", fmt.Errorf("ledger line %d invalid deferred phase ordering", line+1)
			}
			if lastDeferredWindow > 0 && record.Window != lastDeferredWindow {
				prior := states[lastDeferredWindow-1]
				if !prior.accepted && prior.nextAttempt != manifest.Schedule.ScheduledAttemptsPerWindow+manifest.Schedule.DeferredAttemptsPerWindow+1 {
					return nil, "", fmt.Errorf("ledger line %d left deferred window %d before acceptance/exhaustion", line+1, lastDeferredWindow)
				}
			}
			lastDeferredWindow = record.Window
		default:
			return nil, "", fmt.Errorf("ledger line %d unknown phase %q", line+1, record.Phase)
		}

		switch {
		case record.ExitCode == 75 && record.ExitClass == "retryable_environment":
			if len(record.Positions) != 0 || record.RejectionReceipt == nil {
				return nil, "", fmt.Errorf("ledger line %d retryable rejection exposes sample positions", line+1)
			}
			if err := verifyRejectionReceipt(manifest, base, record); err != nil {
				return nil, "", fmt.Errorf("ledger line %d rejection receipt: %w", line+1, err)
			}
		case record.ExitCode == 0 && record.ExitClass == "accepted":
			if len(record.Positions) != 4 || record.RejectionReceipt != nil {
				return nil, "", fmt.Errorf("ledger line %d accepted window has %d positions", line+1, len(record.Positions))
			}
			windowResult := &acceptedWindow{Record: record}
			for position := 0; position < 4; position++ {
				evidence := record.Positions[position]
				rule := rules[position]
				if evidence.Position != position+1 || evidence.Label != rule.Label ||
					evidence.Implementation != rule.Implementation {
					return nil, "", fmt.Errorf("ledger line %d position %d mode mapping mismatch", line+1, position+1)
				}
				parsed, err := verifyPosition(manifest, base, record, evidence)
				if err != nil {
					return nil, "", fmt.Errorf("ledger line %d position %d: %w", line+1, position+1, err)
				}
				windowResult.Samples[position] = parsed
			}
			accepted[record.Window-1] = windowResult
			state.accepted = true
		default:
			return nil, "", fmt.Errorf("ledger line %d terminal/invalid exit code=%d class=%q", line+1, record.ExitCode, record.ExitClass)
		}
	}

	result := make([]acceptedWindow, 60)
	for index, window := range accepted {
		if window == nil {
			return nil, "", fmt.Errorf("window %d is incomplete after attempt exhaustion", index+1)
		}
		result[index] = *window
	}
	if err := verifyLedgerSeal(manifest, base, data, previous, uint64(len(records))); err != nil {
		return nil, "", err
	}
	return result, previous, nil
}

func verifyRejectionReceipt(manifest *boardManifest, base string, record ledgerRecord) error {
	var payload rejectionPayload
	if _, err := verifySignedPayload(base, *record.RejectionReceipt, manifest.Authority.PublicKey, &payload); err != nil {
		return err
	}
	allowed := map[string]bool{"thermal_outside_envelope": true, "cpu_offline": true, "frequency_policy_drift": true, "collector_io_failure_before_clock": true}
	if payload.Schema != "gts-timing-blind-environment-rejection/v3" || payload.BoardID != manifest.BoardID ||
		payload.AuthorityID != manifest.Authority.AuthorityID || payload.BootID != manifest.Authority.BootID ||
		payload.CollectorSHA256 != manifest.Collector.SHA256 || payload.Ordinal != record.Ordinal ||
		payload.Window != record.Window || payload.Attempt != record.Attempt || !allowed[payload.Reason] || payload.TimingObserved {
		return errors.New("signed retry rejection is not timing-blind or record-bound")
	}
	return nil
}

func verifyLedgerSeal(manifest *boardManifest, base string, ledger []byte, tail string, records uint64) error {
	var payload ledgerSealPayload
	if _, err := verifySignedPayload(base, manifest.LedgerSeal, manifest.Authority.PublicKey, &payload); err != nil {
		return fmt.Errorf("ledger seal: %w", err)
	}
	if payload.Schema != "gts-ledger-seal/v3" || payload.BoardID != manifest.BoardID ||
		payload.AuthorityID != manifest.Authority.AuthorityID || payload.BootID != manifest.Authority.BootID ||
		payload.CollectorSHA256 != manifest.Collector.SHA256 || payload.LedgerSHA256 != hashBytes(ledger) ||
		payload.LedgerTailSHA256 != tail || payload.Records != records {
		return errors.New("independently signed ledger seal mismatch")
	}
	return nil
}

func windowIdentity(window int) (int, string, string, int, bool) {
	if window < 1 || window > 60 {
		return 0, "", "", 0, false
	}
	index := window - 1
	lane := lanes[index/(len(fixtures)*5)]
	withinLane := index % (len(fixtures) * 5)
	fixture := fixtures[withinLane/5]
	cycle := withinLane%5 + 1
	return window, lane, fixture, cycle, true
}

func verifyPosition(manifest *boardManifest, base string, record ledgerRecord, evidence positionEvidence) (sample, error) {
	artifact, ok := manifest.Artifacts[evidence.Implementation]
	if !ok {
		return sample{}, fmt.Errorf("missing %s artifact", evidence.Implementation)
	}
	fixture, ok := fixtureByName(manifest.Fixtures, record.Fixture)
	if !ok {
		return sample{}, fmt.Errorf("missing fixture %s", record.Fixture)
	}
	wantArgv := expandCommandTemplate(artifact.CommandTemplate, record.Lane, record.Fixture, evidence.InheritedSourceFD)
	if evidence.MaxRSSBytes == 0 || evidence.InheritedSourceFD < 3 || !equalStrings(evidence.Argv, wantArgv) ||
		!equalStrings(evidence.Tags, artifact.Tags) ||
		evidence.EnvironmentSHA256 != artifact.EnvironmentSHA256 || evidence.ExecutionMethod != executionMethod ||
		evidence.SourceTransport != sourceTransport {
		return sample{}, errors.New("argv/tags/environment/RSS evidence mismatch")
	}
	artifactWant := identityEvidence{
		CanonicalPath: canonical(resolve(base, artifact.Path)), Device: artifact.Device,
		Inode: artifact.Inode, Size: artifact.Size, SHA256: artifact.SHA256,
	}
	fixtureWant := identityEvidence{
		CanonicalPath: canonical(resolve(base, fixture.Path)), Device: fixture.Device,
		Inode: fixture.Inode, Size: fixture.Bytes, SHA256: fixture.SHA256,
	}
	if err := verifyDescriptorTriplet(evidence.ArtifactOpened, evidence.ArtifactRetainedPost, evidence.ArtifactReopenedPost, artifactWant); err != nil {
		return sample{}, fmt.Errorf("artifact descriptor contract: %w", err)
	}
	if err := verifyDescriptorTriplet(evidence.FixtureOpened, evidence.FixtureRetainedPost, evidence.FixtureReopenedPost, fixtureWant); err != nil {
		return sample{}, fmt.Errorf("fixture descriptor contract: %w", err)
	}
	if evidence.ArtifactOpened.FD == evidence.FixtureOpened.FD {
		return sample{}, errors.New("artifact and fixture retained descriptors alias")
	}
	collectorRaw, err := readBoundFile(base, evidence.CollectorReceipt)
	if err != nil {
		return sample{}, fmt.Errorf("collector receipt: %w", err)
	}
	var collector collectorPositionReceipt
	if err := decodeStrict(collectorRaw, &collector); err != nil {
		return sample{}, fmt.Errorf("collector receipt: %w", err)
	}
	if collector.Schema != "gts-collector-position/v3" || collector.ExitCode != 0 ||
		collector.ExecutionMethod != evidence.ExecutionMethod || collector.SourceTransport != evidence.SourceTransport ||
		collector.InheritedSourceFD != evidence.InheritedSourceFD || !equalStrings(collector.Argv, evidence.Argv) ||
		collector.ArtifactOpened != evidence.ArtifactOpened || collector.ArtifactRetainedPost != evidence.ArtifactRetainedPost ||
		collector.ArtifactReopenedPost != evidence.ArtifactReopenedPost || collector.FixtureOpened != evidence.FixtureOpened ||
		collector.FixtureRetainedPost != evidence.FixtureRetainedPost || collector.FixtureReopenedPost != evidence.FixtureReopenedPost ||
		collector.StdoutCompleteEOF != evidence.StdoutFrame.CompleteEOF || collector.StdoutAtomicPacket != evidence.StdoutFrame.AtomicWrite ||
		collector.StdoutBytes != evidence.StdoutFrame.Bytes || collector.StdoutSHA256 != evidence.StdoutFrame.SHA256 ||
		collector.EnvironmentSHA256 != evidence.EnvironmentSHA256 {
		return sample{}, errors.New("collector receipt and sealed position evidence differ")
	}
	raw, err := readBoundFile(base, evidence.Sample)
	if err != nil {
		return sample{}, err
	}
	if !evidence.StdoutFrame.CompleteEOF || !evidence.StdoutFrame.AtomicWrite || evidence.StdoutFrame.Bytes != uint64(len(raw)) ||
		evidence.StdoutFrame.SHA256 != evidence.Sample.SHA256 || hashBytes(raw) != evidence.StdoutFrame.SHA256 {
		return sample{}, errors.New("collector stdout frame is not complete, atomic, and sample-bound")
	}
	parsed, err := parseSample(raw)
	if err != nil {
		return sample{}, err
	}
	if parsed.Implementation != evidence.Implementation || parsed.Lane != record.Lane ||
		parsed.Fixture != record.Fixture || parsed.SourceSHA256 != fixture.SHA256 ||
		parsed.SourceBytes != fixture.Bytes || parsed.ElapsedNS < minElapsedNS {
		return sample{}, errors.New("sample metadata, fixture identity, or duration mismatch")
	}
	wantClock := "go-time-monotonic"
	if parsed.Implementation == "c" {
		wantClock = "clock-monotonic"
	}
	if parsed.Clock != wantClock {
		return sample{}, fmt.Errorf("clock=%q, want %q", parsed.Clock, wantClock)
	}
	if parsed.Lane == "selected_consumer" {
		if parsed.VisitedNodes != fixture.VisitedNodes || parsed.Checksum != fixture.Checksum {
			return sample{}, errors.New("consumer census/checksum mismatch")
		}
	} else if parsed.VisitedNodes != 0 || parsed.Checksum != "0000000000000000" {
		return sample{}, errors.New("non-consumer lane reported traversal data")
	}
	return parsed, nil
}

func expandCommandTemplate(template []string, lane, fixture string, sourceFD int) []string {
	result := make([]string, len(template))
	for index, value := range template {
		switch value {
		case "{lane}":
			result[index] = lane
		case "{fixture}":
			result[index] = fixture
		case "{source_fd}":
			result[index] = strconv.Itoa(sourceFD)
		default:
			result[index] = value
		}
	}
	return result
}

func verifyDescriptorTriplet(opened, retained, reopened descriptorEvidence, want identityEvidence) error {
	if opened.FD < 3 || retained.FD != opened.FD || reopened.FD < 3 ||
		opened.OpenFlags != openFlagsLiteral || retained.OpenFlags != openFlagsLiteral || reopened.OpenFlags != openFlagsLiteral ||
		opened.Identity != want || retained.Identity != want || reopened.Identity != want {
		return errors.New("O_NOFOLLOW opened/retained/reopened descriptor identity mismatch")
	}
	return nil
}

func parseSample(data []byte) (sample, error) {
	if len(data) == 0 || data[len(data)-1] != '\n' || bytes.ContainsRune(data, '\r') ||
		!bytes.Equal(data, bytes.ToValidUTF8(data, nil)) {
		return sample{}, errors.New("sample must be newline-terminated UTF-8 with LF only")
	}
	lines := strings.Split(string(data[:len(data)-1]), "\n")
	keys := []string{
		"schema", "status", "implementation", "lane", "fixture", "source_sha256",
		"source_bytes", "clock", "warmups", "iterations", "elapsed_ns",
		"visited_nodes", "traversal_checksum",
	}
	if len(lines) != len(keys) {
		return sample{}, fmt.Errorf("sample line count=%d, want %d", len(lines), len(keys))
	}
	values := make(map[string]string, len(keys))
	for index, key := range keys {
		prefix := key + "="
		if !strings.HasPrefix(lines[index], prefix) || strings.TrimSpace(lines[index]) != lines[index] {
			return sample{}, fmt.Errorf("sample line %d must be ordered key %s", index+1, key)
		}
		value := strings.TrimPrefix(lines[index], prefix)
		if value == "" {
			return sample{}, fmt.Errorf("sample key %s is empty", key)
		}
		values[key] = value
	}
	if values["schema"] != sampleSchema || values["status"] != "ok" || values["warmups"] != "1" {
		return sample{}, errors.New("sample schema/status/warmups mismatch")
	}
	if values["implementation"] != "go" && values["implementation"] != "c" {
		return sample{}, errors.New("sample implementation is invalid")
	}
	if !contains(lanes, values["lane"]) || !contains(fixtures, values["fixture"]) ||
		requireHash(values["source_sha256"], "sample source sha256") != nil ||
		!isLowerHex(values["traversal_checksum"], 16) {
		return sample{}, errors.New("sample enum/hash field is invalid")
	}
	sourceBytes, err := parseUint(values["source_bytes"], false)
	if err != nil {
		return sample{}, fmt.Errorf("source_bytes: %w", err)
	}
	iterations, err := parseUint(values["iterations"], true)
	if err != nil {
		return sample{}, fmt.Errorf("iterations: %w", err)
	}
	if iterations > maxIterations {
		return sample{}, errors.New("iterations exceeds frozen 1e9 cap")
	}
	elapsed, err := parseUint(values["elapsed_ns"], true)
	if err != nil {
		return sample{}, fmt.Errorf("elapsed_ns: %w", err)
	}
	visited, err := parseUint(values["visited_nodes"], false)
	if err != nil {
		return sample{}, fmt.Errorf("visited_nodes: %w", err)
	}
	return sample{
		Implementation: values["implementation"], Lane: values["lane"],
		Fixture: values["fixture"], SourceSHA256: values["source_sha256"],
		SourceBytes: sourceBytes, Clock: values["clock"], Iterations: iterations,
		ElapsedNS: elapsed, VisitedNodes: visited, Checksum: values["traversal_checksum"],
	}, nil
}

func parseUint(raw string, positive bool) (uint64, error) {
	if raw == "" || (len(raw) > 1 && raw[0] == '0') || raw[0] == '+' || raw[0] == '-' {
		return 0, errors.New("must be canonical base-10 unsigned integer")
	}
	value, err := strconv.ParseUint(raw, 10, 64)
	if err != nil || (positive && value == 0) {
		return 0, errors.New("must be in range and positive where required")
	}
	return value, nil
}

func analyze(manifest *boardManifest, windows []acceptedWindow) (verificationReceipt, error) {
	receipt := verificationReceipt{
		Schema: receiptSchema, Status: "passed", BoardID: manifest.BoardID,
		Mode: manifest.Mode, Samples: len(windows) * 4,
	}
	laneProducts := make(map[string]*big.Rat, len(lanes))
	allAAGates := true
	for _, lane := range lanes {
		laneProducts[lane] = new(big.Rat).SetInt64(1)
		for _, fixture := range fixtures {
			cellProduct := new(big.Rat).SetInt64(1)
			var rssA, rssB []uint64
			for cycle := 1; cycle <= 5; cycle++ {
				window := windows[windowIndex(lane, fixture, cycle)]
				q, err := cycleQ(manifest.Mode, window.Samples)
				if err != nil {
					return verificationReceipt{}, fmt.Errorf("%s/%s/cycle%d: %w", lane, fixture, cycle, err)
				}
				cellProduct.Mul(cellProduct, q)
				if manifest.Mode == modeAA {
					rssA = append(rssA, window.Record.Positions[0].MaxRSSBytes, window.Record.Positions[3].MaxRSSBytes)
					rssB = append(rssB, window.Record.Positions[1].MaxRSSBytes, window.Record.Positions[2].MaxRSSBytes)
				}
			}
			cell := cellResult{
				Lane: lane, Fixture: fixture, Q: ratJSON(cellProduct),
				Root: displayRoot(cellProduct, 10),
			}
			if manifest.Mode == modeAA {
				cellRatioPass := ratioWithinPower(cellProduct, 98, 100, 102, 100, 10, false)
				medianPass := rssRatioPass(rssA, rssB, 99, 100, 101, 100, true)
				maxPass := rssMaxRatioPass(rssA, rssB, 98, 100, 102, 100)
				cell.AAMedianRSSPass = &medianPass
				cell.AAMaxRSSPass = &maxPass
				allAAGates = allAAGates && cellRatioPass && medianPass && maxPass
			}
			receipt.Cells = append(receipt.Cells, cell)
			laneProducts[lane].Mul(laneProducts[lane], cellProduct)
		}
		laneQ := laneProducts[lane]
		receipt.Lanes = append(receipt.Lanes, laneResult{
			Lane: lane, Q: ratJSON(laneQ), Root: displayRoot(laneQ, 40),
		})
		if manifest.Mode == modeAA {
			allAAGates = allAAGates && ratioWithinPower(laneQ, 99, 100, 101, 100, 40, false)
		}
	}
	if manifest.Mode == modeAA {
		allAAGates = allAAGates && manifest.AAGate != nil &&
			manifest.AAGate.PublicBenchstatSameDirectionSignificantFixtures <= 1
		receipt.AANullGatesPassed = &allAAGates
		if !allAAGates {
			return verificationReceipt{}, errors.New("identical-Go A/A null or RSS gate failed")
		}
	}
	return receipt, nil
}

func windowIndex(lane, fixture string, cycle int) int {
	laneIndex := indexOf(lanes, lane)
	fixtureIndex := indexOf(fixtures, fixture)
	return laneIndex*len(fixtures)*5 + fixtureIndex*5 + cycle - 1
}

func cycleQ(mode string, samples [4]sample) (*big.Rat, error) {
	for index, sample := range samples {
		if sample.Iterations == 0 || sample.ElapsedNS == 0 {
			return nil, fmt.Errorf("position %d has zero timing rational", index+1)
		}
	}
	var numerator, denominator *big.Int
	if mode == modeAA {
		// Q_AA=(p_B2*p_B3)/(p_A1*p_A4).
		numerator = new(big.Int).SetUint64(samples[1].ElapsedNS)
		numerator.Mul(numerator, new(big.Int).SetUint64(samples[2].ElapsedNS))
		numerator.Mul(numerator, new(big.Int).SetUint64(samples[0].Iterations))
		numerator.Mul(numerator, new(big.Int).SetUint64(samples[3].Iterations))
		denominator = new(big.Int).SetUint64(samples[1].Iterations)
		denominator.Mul(denominator, new(big.Int).SetUint64(samples[2].Iterations))
		denominator.Mul(denominator, new(big.Int).SetUint64(samples[0].ElapsedNS))
		denominator.Mul(denominator, new(big.Int).SetUint64(samples[3].ElapsedNS))
	} else if mode == modeProduction {
		// Q_GC=(p_G1*p_G4)/(p_C2*p_C3).
		numerator = new(big.Int).SetUint64(samples[0].ElapsedNS)
		numerator.Mul(numerator, new(big.Int).SetUint64(samples[3].ElapsedNS))
		numerator.Mul(numerator, new(big.Int).SetUint64(samples[1].Iterations))
		numerator.Mul(numerator, new(big.Int).SetUint64(samples[2].Iterations))
		denominator = new(big.Int).SetUint64(samples[0].Iterations)
		denominator.Mul(denominator, new(big.Int).SetUint64(samples[3].Iterations))
		denominator.Mul(denominator, new(big.Int).SetUint64(samples[1].ElapsedNS))
		denominator.Mul(denominator, new(big.Int).SetUint64(samples[2].ElapsedNS))
	} else {
		return nil, fmt.Errorf("unknown mode %q", mode)
	}
	return new(big.Rat).SetFrac(numerator, denominator), nil
}

func ratJSON(value *big.Rat) rationalJSON {
	return rationalJSON{Numerator: value.Num().String(), Denominator: value.Denom().String()}
}

func displayRoot(value *big.Rat, degree int) string {
	// Display only. Gates never use this approximation.
	ratio := new(big.Float).SetPrec(256).Quo(
		new(big.Float).SetPrec(256).SetInt(value.Num()),
		new(big.Float).SetPrec(256).SetInt(value.Denom()),
	)
	number, _ := ratio.Float64()
	root := new(big.Float).SetPrec(128)
	if number > 0 {
		root.SetFloat64(powApprox(number, degree))
	}
	return root.Text('g', 17)
}

func powApprox(value float64, degree int) float64 {
	if value <= 0 || degree <= 0 {
		return 0
	}
	// Newton iteration avoids importing a floating transcendental package into
	// the exact gate path. This value is receipt display only.
	x := 1.0
	if value > 1 {
		x = value
	}
	for i := 0; i < 100; i++ {
		power := 1.0
		for j := 1; j < degree; j++ {
			power *= x
		}
		if power == 0 {
			break
		}
		next := (float64(degree-1)*x + value/power) / float64(degree)
		if next == x {
			break
		}
		x = next
	}
	return x
}

func ratioWithinPower(value *big.Rat, lowNum, lowDen, highNum, highDen int64, degree int, inclusive bool) bool {
	lowN := new(big.Int).Exp(big.NewInt(lowNum), big.NewInt(int64(degree)), nil)
	lowD := new(big.Int).Exp(big.NewInt(lowDen), big.NewInt(int64(degree)), nil)
	highN := new(big.Int).Exp(big.NewInt(highNum), big.NewInt(int64(degree)), nil)
	highD := new(big.Int).Exp(big.NewInt(highDen), big.NewInt(int64(degree)), nil)
	leftLow := new(big.Int).Mul(new(big.Int).Set(value.Num()), lowD)
	rightLow := new(big.Int).Mul(new(big.Int).Set(value.Denom()), lowN)
	leftHigh := new(big.Int).Mul(new(big.Int).Set(value.Num()), highD)
	rightHigh := new(big.Int).Mul(new(big.Int).Set(value.Denom()), highN)
	if inclusive {
		return leftLow.Cmp(rightLow) >= 0 && leftHigh.Cmp(rightHigh) <= 0
	}
	return leftLow.Cmp(rightLow) > 0 && leftHigh.Cmp(rightHigh) < 0
}

func rssRatioPass(a, b []uint64, lowNum, lowDen, highNum, highDen int64, inclusive bool) bool {
	if len(a) != 10 || len(b) != 10 {
		return false
	}
	sort.Slice(a, func(i, j int) bool { return a[i] < a[j] })
	sort.Slice(b, func(i, j int) bool { return b[i] < b[j] })
	// Twice each even-count median; the factor of two cancels.
	medianA := new(big.Int).SetUint64(a[4])
	medianA.Add(medianA, new(big.Int).SetUint64(a[5]))
	medianB := new(big.Int).SetUint64(b[4])
	medianB.Add(medianB, new(big.Int).SetUint64(b[5]))
	return integerRatioWithin(medianB, medianA, lowNum, lowDen, highNum, highDen, inclusive)
}

func rssMaxRatioPass(a, b []uint64, lowNum, lowDen, highNum, highDen int64) bool {
	if len(a) == 0 || len(b) == 0 {
		return false
	}
	maxA, maxB := a[0], b[0]
	for _, value := range a[1:] {
		if value > maxA {
			maxA = value
		}
	}
	for _, value := range b[1:] {
		if value > maxB {
			maxB = value
		}
	}
	return integerRatioWithin(new(big.Int).SetUint64(maxB), new(big.Int).SetUint64(maxA),
		lowNum, lowDen, highNum, highDen, true)
}

func integerRatioWithin(numerator, denominator *big.Int, lowNum, lowDen, highNum, highDen int64, inclusive bool) bool {
	if denominator.Sign() <= 0 {
		return false
	}
	lowLeft := new(big.Int).Mul(new(big.Int).Set(numerator), big.NewInt(lowDen))
	lowRight := new(big.Int).Mul(new(big.Int).Set(denominator), big.NewInt(lowNum))
	highLeft := new(big.Int).Mul(new(big.Int).Set(numerator), big.NewInt(highDen))
	highRight := new(big.Int).Mul(new(big.Int).Set(denominator), big.NewInt(highNum))
	if inclusive {
		return lowLeft.Cmp(lowRight) >= 0 && highLeft.Cmp(highRight) <= 0
	}
	return lowLeft.Cmp(lowRight) > 0 && highLeft.Cmp(highRight) < 0
}

func readBoundFile(base string, binding fileBinding) ([]byte, error) {
	if err := requireHash(binding.SHA256, "bound file sha256"); err != nil {
		return nil, err
	}
	data, _, err := readNoFollow(resolve(base, binding.Path))
	if err != nil {
		return nil, err
	}
	if got := hashBytes(data); got != binding.SHA256 {
		return nil, fmt.Errorf("sha256=%s, want %s", got, binding.SHA256)
	}
	return data, nil
}

func verifyCurrentIdentity(base, path, wantHash string, wantDevice, wantInode, wantSize uint64) error {
	if err := requireHash(wantHash, "identity sha256"); err != nil {
		return err
	}
	data, identity, err := readNoFollow(resolve(base, path))
	if err != nil {
		return err
	}
	if identity.Size != wantSize || identity.Device != wantDevice || identity.Inode != wantInode {
		return fmt.Errorf("device/inode/size=%d/%d/%d, want %d/%d/%d",
			identity.Device, identity.Inode, identity.Size, wantDevice, wantInode, wantSize)
	}
	if got := hashBytes(data); got != wantHash {
		return fmt.Errorf("sha256=%s, want %s", got, wantHash)
	}
	return nil
}

func readNoFollow(path string) ([]byte, identityEvidence, error) {
	fd, err := syscall.Open(path, syscall.O_RDONLY|syscall.O_CLOEXEC|syscall.O_NOFOLLOW, 0)
	if err != nil {
		return nil, identityEvidence{}, err
	}
	file := os.NewFile(uintptr(fd), path)
	if file == nil {
		_ = syscall.Close(fd)
		return nil, identityEvidence{}, errors.New("create file from verified descriptor")
	}
	defer file.Close()
	var stat syscall.Stat_t
	if err := syscall.Fstat(fd, &stat); err != nil {
		return nil, identityEvidence{}, err
	}
	if stat.Mode&syscall.S_IFMT != syscall.S_IFREG || stat.Size < 0 {
		return nil, identityEvidence{}, errors.New("bound descriptor is not a regular file")
	}
	data, err := io.ReadAll(file)
	if err != nil {
		return nil, identityEvidence{}, err
	}
	identity := identityEvidence{
		CanonicalPath: canonical(path), Device: uint64(stat.Dev), Inode: stat.Ino,
		Size: uint64(stat.Size), SHA256: hashBytes(data),
	}
	if identity.Size != uint64(len(data)) {
		return nil, identityEvidence{}, errors.New("descriptor size changed during retained read")
	}
	return data, identity, nil
}

func resolve(base, path string) string {
	if filepath.IsAbs(path) {
		return filepath.Clean(path)
	}
	return filepath.Clean(filepath.Join(base, path))
}

func canonical(path string) string {
	abs, err := filepath.Abs(path)
	if err != nil {
		return ""
	}
	return filepath.Clean(abs)
}

func hashBytes(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func requireHash(value, name string) error {
	if !isLowerHex(value, 64) || value == zeroHash {
		return fmt.Errorf("%s must be a nonzero lowercase sha256", name)
	}
	return nil
}

func isLowerHex(value string, length int) bool {
	if len(value) != length {
		return false
	}
	for _, char := range value {
		if !((char >= '0' && char <= '9') || (char >= 'a' && char <= 'f')) {
			return false
		}
	}
	return true
}

func isLowerHexString(value string) bool {
	if value == "" {
		return false
	}
	for _, char := range value {
		if !((char >= '0' && char <= '9') || (char >= 'a' && char <= 'f')) {
			return false
		}
	}
	return true
}

func fixtureByName(entries []fixtureBinding, name string) (fixtureBinding, bool) {
	for _, entry := range entries {
		if entry.Name == name {
			return entry, true
		}
	}
	return fixtureBinding{}, false
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for index := range a {
		if a[index] != b[index] {
			return false
		}
	}
	return true
}

func equalInts(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for index := range a {
		if a[index] != b[index] {
			return false
		}
	}
	return true
}

func equalStringSets(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	aCopy, bCopy := append([]string(nil), a...), append([]string(nil), b...)
	sort.Strings(aCopy)
	sort.Strings(bCopy)
	return equalStrings(aCopy, bCopy)
}

func contains(values []string, target string) bool {
	return indexOf(values, target) >= 0
}

func indexOf(values []string, target string) int {
	for index, value := range values {
		if value == target {
			return index
		}
	}
	return -1
}
