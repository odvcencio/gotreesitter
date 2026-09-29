// Package grammarreceipt defines the versioned JSON record used to track
// grammar graduation evidence.
package grammarreceipt

import (
	"encoding/hex"
	"fmt"
	"strings"
	"time"
)

const SchemaV1 = "gts-grammar-receipt/v1"

type RouteStatus string

const (
	RouteAccepted RouteStatus = "accepted"
	RouteDeclined RouteStatus = "declined"
	RouteForest   RouteStatus = "forest_route"
)

type ResultStatus string

const (
	ResultPass        ResultStatus = "pass"
	ResultFail        ResultStatus = "fail"
	ResultUnavailable ResultStatus = "unavailable"
	ResultNotRun      ResultStatus = "not_run"
	ResultTimeout     ResultStatus = "timeout"
)

// Receipt is one grammar's complete, point-in-time graduation evidence.
// Receipts are generated artifacts and are not checked into the repository.
type Receipt struct {
	Schema             string           `json:"schema"`
	GeneratedAt        time.Time        `json:"generated_at"`
	Grammar            GrammarIdentity  `json:"grammar"`
	GotreesitterCommit string           `json:"gotreesitter_commit"`
	GitTreeDirty       bool             `json:"git_tree_dirty"`
	COracle            COracleIdentity  `json:"c_oracle"`
	Cohort             string           `json:"cohort"`
	Route              CompactRoute     `json:"compact_route"`
	Corpus             CorpusIdentity   `json:"corpus"`
	Execution          *ExecutionResult `json:"execution,omitempty"`
	FreshParity        ParityResult     `json:"fresh_parity"`
	IncrementalParity  ParityResult     `json:"incremental_parity"`
	InvariantGate      InvariantResult  `json:"invariant_gate"`
}

type ExecutionResult struct {
	Status    ResultStatus `json:"status"`
	TimeLimit string       `json:"time_limit,omitempty"`
	Details   string       `json:"details,omitempty"`
}

type GrammarIdentity struct {
	Name       string `json:"name"`
	BlobSHA256 string `json:"blob_sha256"`
}

type COracleIdentity struct {
	RuntimeVersion  string           `json:"runtime_version"`
	RuntimeCommit   string           `json:"runtime_commit"`
	Grammar         CGrammarArtifact `json:"grammar_artifact"`
	BindingModule   string           `json:"binding_module,omitempty"`
	BindingVersion  string           `json:"binding_version,omitempty"`
	BindingCommit   string           `json:"binding_commit,omitempty"`
	Transport       string           `json:"transport,omitempty"`
	CompilerPath    string           `json:"compiler_path,omitempty"`
	CompilerVersion string           `json:"compiler_version,omitempty"`
	CompileFlags    string           `json:"compile_flags,omitempty"`
	RuntimeLinkage  string           `json:"runtime_linkage,omitempty"`
	GrammarLinkage  string           `json:"grammar_linkage,omitempty"`
}

type CGrammarArtifact struct {
	Repository string `json:"repository"`
	Commit     string `json:"commit"`
	SHA256     string `json:"sha256"`
}

type CompactRoute struct {
	Status         RouteStatus `json:"status"`
	DeclineReason  string      `json:"decline_reason,omitempty"`
	DeclineReasons []string    `json:"decline_reasons,omitempty"`
	SampledFiles   int         `json:"sampled_files"`
	AcceptedFiles  int         `json:"accepted_files"`
	DeclinedFiles  int         `json:"declined_files"`
	ForestFiles    int         `json:"forest_files"`
	Probe          string      `json:"probe"`
	ProbeSHA256    string      `json:"probe_source_sha256"`
}

type CorpusIdentity struct {
	LockPath       string          `json:"lock_path"`
	LockSHA256     string          `json:"lock_sha256"`
	ManifestSHA256 string          `json:"manifest_sha256"`
	Selection      CorpusSelection `json:"selection"`
	Files          []CorpusFile    `json:"files"`
}

type CorpusSelection struct {
	Order        string `json:"order"`
	MaxFiles     int    `json:"max_files"`
	MaxFileBytes int64  `json:"max_file_bytes"`
}

type CorpusFile struct {
	Repository string `json:"repository"`
	Revision   string `json:"revision"`
	Path       string `json:"path"`
	Bytes      int64  `json:"bytes"`
	SHA256     string `json:"sha256"`
}

type ParityResult struct {
	Status       ResultStatus `json:"status"`
	Cases        int          `json:"cases"`
	Matched      int          `json:"matched"`
	Mismatched   int          `json:"mismatched"`
	Errors       int          `json:"errors"`
	FirstFailure *Failure     `json:"first_failure,omitempty"`
	Files        []FileResult `json:"files,omitempty"`
	Steps        []StepResult `json:"steps,omitempty"`
}

type FileResult struct {
	Path              string      `json:"path"`
	SourceSHA256      string      `json:"source_sha256"`
	Bytes             int         `json:"bytes"`
	GoTreeSHA256      string      `json:"go_tree_sha256,omitempty"`
	CTreeSHA256       string      `json:"c_tree_sha256,omitempty"`
	Pass              bool        `json:"pass"`
	Route             RouteStatus `json:"route,omitempty"`
	DeclineReason     string      `json:"decline_reason,omitempty"`
	GoParseError      string      `json:"go_parse_error,omitempty"`
	GoStopReason      string      `json:"go_stop_reason,omitempty"`
	CParseError       string      `json:"c_parse_error,omitempty"`
	RootCoversInput   bool        `json:"root_covers_input"`
	ErrorRootHasError bool        `json:"error_root_has_error"`
	Divergence        *Failure    `json:"first_divergence,omitempty"`
}

type StepResult struct {
	Step                     int      `json:"step"`
	EditClass                string   `json:"edit_class"`
	Site                     string   `json:"site"`
	SourceSHA256             string   `json:"source_sha256"`
	GoIncrementalSHA256      string   `json:"go_incremental_tree_sha256,omitempty"`
	GoFreshSHA256            string   `json:"go_fresh_tree_sha256,omitempty"`
	CIncrementalSHA256       string   `json:"c_incremental_tree_sha256,omitempty"`
	CFreshSHA256             string   `json:"c_fresh_tree_sha256,omitempty"`
	GoIncrementalEqualsFresh bool     `json:"go_incremental_equals_fresh"`
	CIncrementalEqualsFresh  bool     `json:"c_incremental_equals_fresh"`
	LockedCParity            bool     `json:"locked_c_parity"`
	LockedCIncrementalParity bool     `json:"locked_c_incremental_parity"`
	InvariantPass            bool     `json:"invariant_pass"`
	Pass                     bool     `json:"pass"`
	Failure                  *Failure `json:"first_failure,omitempty"`
}

type Failure struct {
	Category string `json:"category"`
	Path     string `json:"path,omitempty"`
	GoValue  string `json:"go_value,omitempty"`
	CValue   string `json:"c_value,omitempty"`
	Error    string `json:"error,omitempty"`
}

type InvariantResult struct {
	Status                    ResultStatus   `json:"status"`
	SessionSteps              int            `json:"session_steps"`
	StepsPassed               int            `json:"steps_passed"`
	UniqueSitesByEditClass    map[string]int `json:"unique_sites_by_edit_class,omitempty"`
	RootCoverageFailures      int            `json:"root_coverage_failures"`
	ErrorRootFailures         int            `json:"error_root_failures"`
	IncrementalFreshFailures  int            `json:"incremental_fresh_failures"`
	NoEditReparseAllocsPerRun float64        `json:"no_edit_reparse_allocs_per_run"`
	NoEditReparsePass         bool           `json:"no_edit_reparse_pass"`
	Failures                  []Failure      `json:"failures,omitempty"`
}

// Validate rejects receipts that omit identities or use values outside v1's
// route and result vocabularies.
func (r Receipt) Validate() error {
	if r.Schema != SchemaV1 {
		return fmt.Errorf("schema %q, want %q", r.Schema, SchemaV1)
	}
	if r.GeneratedAt.IsZero() {
		return fmt.Errorf("generated_at is required")
	}
	if err := validateGrammarIdentity(r.Grammar); err != nil {
		return err
	}
	if err := requireSHA256("grammar.blob_sha256", r.Grammar.BlobSHA256); err != nil {
		return err
	}
	if !isCommit(r.GotreesitterCommit) {
		return fmt.Errorf("gotreesitter_commit must be a full Git commit")
	}
	if !isCommit(r.COracle.RuntimeCommit) {
		return fmt.Errorf("c_oracle.runtime_commit must be a full Git commit")
	}
	if strings.TrimSpace(r.COracle.RuntimeVersion) == "" {
		return fmt.Errorf("c_oracle.runtime_version is required")
	}
	if strings.TrimSpace(r.COracle.Grammar.Repository) == "" {
		return fmt.Errorf("c_oracle.grammar_artifact.repository is required")
	}
	if !isCommit(r.COracle.Grammar.Commit) {
		return fmt.Errorf("c_oracle.grammar_artifact.commit must be a full Git commit")
	}
	if err := requireSHA256("c_oracle.grammar_artifact.sha256", r.COracle.Grammar.SHA256); err != nil {
		return err
	}
	if !validCohort(r.Cohort) {
		return fmt.Errorf("unknown cohort %q", r.Cohort)
	}
	if r.Route.Status != RouteAccepted && r.Route.Status != RouteDeclined && r.Route.Status != RouteForest {
		return fmt.Errorf("unknown compact route %q", r.Route.Status)
	}
	if r.Route.Status == RouteDeclined && strings.TrimSpace(r.Route.DeclineReason) == "" {
		return fmt.Errorf("compact decline requires a decline_reason")
	}
	if strings.TrimSpace(r.Route.Probe) == "" {
		return fmt.Errorf("compact route probe is required")
	}
	if err := requireSHA256("compact_route.probe_source_sha256", r.Route.ProbeSHA256); err != nil {
		return err
	}
	if err := requireSHA256("corpus.lock_sha256", r.Corpus.LockSHA256); err != nil {
		return err
	}
	if err := requireSHA256("corpus.manifest_sha256", r.Corpus.ManifestSHA256); err != nil {
		return err
	}
	if r.Execution != nil {
		if r.Execution.Status != ResultPass && r.Execution.Status != ResultTimeout && r.Execution.Status != ResultFail {
			return fmt.Errorf("unknown execution status %q", r.Execution.Status)
		}
		if r.Execution.Status == ResultTimeout {
			if strings.TrimSpace(r.Execution.TimeLimit) == "" {
				return fmt.Errorf("timeout execution requires a time_limit")
			}
			if r.FreshParity.Status != ResultTimeout || r.IncrementalParity.Status != ResultTimeout || r.InvariantGate.Status != ResultTimeout {
				return fmt.Errorf("timeout execution requires timeout parity and invariant results")
			}
		}
	}
	if err := validResult("fresh_parity.status", r.FreshParity.Status); err != nil {
		return err
	}
	if err := validResult("incremental_parity.status", r.IncrementalParity.Status); err != nil {
		return err
	}
	if err := validResult("invariant_gate.status", r.InvariantGate.Status); err != nil {
		return err
	}
	return nil
}

func validateGrammarIdentity(identity GrammarIdentity) error {
	return requireNonBlank("grammar.name", identity.Name)
}

func requireNonBlank(field, value string) error {
	if len(strings.TrimSpace(value)) == 0 {
		return fmt.Errorf("%s is required", field)
	}
	return nil
}

func validCohort(value string) bool {
	switch value {
	case "1a", "1b", "2", "3", "4-A", "4-B", "4-C", "4-D", "4-E", "4-F", "Lean 4":
		return true
	default:
		return false
	}
}

func validResult(field string, value ResultStatus) error {
	switch value {
	case ResultPass, ResultFail, ResultUnavailable, ResultNotRun, ResultTimeout:
		return nil
	default:
		return fmt.Errorf("%s has unknown status %q", field, value)
	}
}

func isCommit(value string) bool {
	if len(value) != 40 && len(value) != 64 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func requireSHA256(field, value string) error {
	if len(value) != 64 {
		return fmt.Errorf("%s must be a SHA-256 digest", field)
	}
	if _, err := hex.DecodeString(value); err != nil {
		return fmt.Errorf("%s must be a SHA-256 digest", field)
	}
	return nil
}
