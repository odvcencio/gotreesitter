//go:build cgo && treesitter_c_parity && treesitter_c_perfscan

package cgoharness

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	sitter "github.com/tree-sitter/go-tree-sitter"
)

const (
	staticCPerfSchema              = "gts-static-c-perf/v2"
	staticCArtifactManifestSchema  = "gts-static-c-perf-artifact/v1"
	staticCPerfTransport           = "static_executable"
	staticCPerfLinkage             = "fully_static_executable"
	staticCPerfLinkageProof        = "elf:no_interp,no_needed"
	staticCPerfRuntimeRepo         = "https://github.com/tree-sitter/tree-sitter.git"
	staticCPerfOptimizationFlags   = "-O2 -DNDEBUG"
	staticCPerfRuntimeCFlags       = "-O2 -DNDEBUG -std=c11 -D_POSIX_C_SOURCE=200112L -D_DEFAULT_SOURCE"
	staticCPerfGrammarCFlags       = "-O2 -DNDEBUG -std=c11"
	staticCPerfGrammarCAssertFlags = "-O2 -UNDEBUG -std=c11"
	staticCPerfGrammarCXXFlags     = "-O2 -DNDEBUG -std=c++17"
	staticCPerfDriverCFlags        = "-O2 -DNDEBUG -std=c11 -D_POSIX_C_SOURCE=200112L"
	staticCPerfLinkFlags           = "-static -O2"
	staticCPerfSourceFlagProtocol  = "default_ndebug_with_exact_source_exceptions/v1"
	staticCPerfTimingRegion        = "ts_parser_parse_string_only;process,source,parser_setup,tree_delete_excluded"
	staticCPerfOrderProtocol       = "whole_file_blocks_alternate_by_language_sha256_low_bit_xor_selected_file_ordinal"
	staticCPerfFailureVocabulary   = "c_oracle_status/v3:admission_error,parser_error,parser_timeout,transport_error,transport_timeout,digest_error,digest_timeout,resource_limit,measurement_error,protocol_error,incomplete,mismatch"
	staticCPerfCacheEnv            = "GTS_C_ORACLE_CACHE"
	staticCPerfCanaryEnv           = "GTS_STATIC_C_PERF_CANARY"
	staticCPerfWallGrace           = 2 * time.Second
	staticCSourceLocked            = "locked_commit"
	staticCSourceGenerated         = "generated_from_locked_grammar"
	staticCStatusAdmissionError    = "c_oracle_admission_error"
	staticCStatusParserError       = "c_oracle_parser_error"
	staticCStatusParserTimeout     = "c_oracle_parser_timeout"
	staticCStatusTransportError    = "c_oracle_transport_error"
	staticCStatusTransportTimeout  = "c_oracle_transport_timeout"
	staticCStatusDigestError       = "c_oracle_digest_error"
	staticCStatusDigestTimeout     = "c_oracle_digest_timeout"
	staticCStatusResourceLimit     = "c_oracle_resource_limit"
	staticCStatusMeasurementError  = "c_oracle_measurement_error"
	staticCStatusProtocolError     = "c_oracle_protocol_error"
	staticCStatusIncomplete        = "c_oracle_incomplete"
	staticCStatusMismatch          = "c_oracle_mismatch"
)

type perfScanOracleCommonIdentity struct {
	Contract             string `json:"contract"`
	Transport            string `json:"transport"`
	Linkage              string `json:"linkage"`
	BindingModule        string `json:"binding_module"`
	BindingVersion       string `json:"binding_version"`
	BindingCommit        string `json:"binding_commit"`
	RuntimeRepo          string `json:"runtime_repo"`
	RuntimeVersion       string `json:"runtime_version"`
	RuntimeCommit        string `json:"runtime_commit"`
	RuntimeSourceTreeOID string `json:"runtime_source_tree_oid"`
	RuntimeSourceSHA256  string `json:"runtime_source_sha256"`
	OptimizationFlags    string `json:"optimization_flags"`
	RuntimeCFlags        string `json:"runtime_c_flags"`
	GrammarCFlags        string `json:"grammar_c_flags"`
	GrammarCXXFlags      string `json:"grammar_cxx_flags"`
	SourceFlagProtocol   string `json:"source_flag_protocol"`
	DriverCFlags         string `json:"driver_c_flags"`
	LinkFlags            string `json:"link_flags"`
	TimingRegion         string `json:"timing_region"`
	OrderProtocol        string `json:"measurement_order_protocol"`
	FailureVocabulary    string `json:"failure_vocabulary"`
	TargetOS             string `json:"target_os"`
	TargetArch           string `json:"target_arch"`
	CompilerPath         string `json:"compiler_path"`
	CompilerVersion      string `json:"compiler_version"`
	CompilerSHA256       string `json:"compiler_sha256"`
	DriverSHA256         string `json:"driver_sha256"`
	DeepDigestFormat     string `json:"deep_digest_format"`
}

type perfScanOracleSourceIdentity struct {
	Path         string `json:"path"`
	SHA256       string `json:"sha256"`
	CompileFlags string `json:"compile_flags"`
}

type staticCSourceCompileProfileKey struct {
	Language      string
	GrammarRepo   string
	GrammarCommit string
	Path          string
	SHA256        string
}

var staticCSourceCompileProfiles = map[staticCSourceCompileProfileKey]string{
	{
		Language:      "just",
		GrammarRepo:   "https://github.com/IndianBoy42/tree-sitter-just",
		GrammarCommit: "60df3d5b3fda2a22fdb3621226cafab50b763663",
		Path:          "scanner.c",
		SHA256:        "99a4ae2ff08e90a18d21fc42d324411aa0df57ebf486e06e70c0b5dd26d489e8",
	}: staticCPerfGrammarCAssertFlags,
}

type perfScanOracleLanguageIdentity struct {
	Language          string                         `json:"language"`
	GrammarRepo       string                         `json:"grammar_repo"`
	GrammarCommit     string                         `json:"grammar_commit"`
	GrammarSourceTree string                         `json:"grammar_source_tree_oid"`
	GrammarSourceMode string                         `json:"grammar_source_mode"`
	LanguageSymbol    string                         `json:"language_symbol"`
	Parser            perfScanOracleSourceIdentity   `json:"parser"`
	Scanners          []perfScanOracleSourceIdentity `json:"scanners,omitempty"`
	LinkerPath        string                         `json:"linker_path"`
	LinkerVersion     string                         `json:"linker_version"`
	LinkerSHA256      string                         `json:"linker_sha256"`
	BuildKeySHA256    string                         `json:"build_key_sha256"`
	ArtifactSHA256    string                         `json:"artifact_sha256"`
	ArtifactStatic    bool                           `json:"artifact_static"`
	ArtifactLinkage   string                         `json:"artifact_linkage_proof"`
}

type perfScanOracleIdentity struct {
	Common   perfScanOracleCommonIdentity   `json:"common"`
	Language perfScanOracleLanguageIdentity `json:"language"`
}

type perfScanOracleBoardIdentity struct {
	Common    perfScanOracleCommonIdentity     `json:"common"`
	Languages []perfScanOracleLanguageIdentity `json:"languages"`
}

type perfScanOracleAdmission struct {
	DigestFormat     string        `json:"digest_format"`
	SourceSHA256     string        `json:"source_sha256"`
	StaticDeepSHA256 string        `json:"static_deep_sha256"`
	ParityDeepSHA256 string        `json:"parity_deep_sha256"`
	CgoGrammarSHA256 string        `json:"cgo_grammar_sha256,omitempty"`
	CgoPeakRSSBytes  int64         `json:"cgo_peak_rss_bytes,omitempty"`
	Stop             *perfScanStop `json:"stop,omitempty"`
	Admitted         bool          `json:"admitted"`
	Status           string        `json:"status,omitempty"`
	Detail           string        `json:"detail,omitempty"`
}

type staticCPerfOracle struct {
	artifact  string
	identity  perfScanOracleIdentity
	cleanup   func()
	wallGrace time.Duration
}

func (oracle *staticCPerfOracle) Close() {
	if oracle != nil && oracle.cleanup != nil {
		oracle.cleanup()
		oracle.cleanup = nil
	}
}

func (oracle *staticCPerfOracle) transportGrace() time.Duration {
	if oracle != nil && oracle.wallGrace > 0 {
		return oracle.wallGrace
	}
	return staticCPerfWallGrace
}

func staticCAdmissionFailureStatus(status string) bool {
	switch status {
	case staticCStatusAdmissionError,
		staticCStatusParserError, staticCStatusParserTimeout,
		staticCStatusTransportError, staticCStatusTransportTimeout,
		staticCStatusDigestError, staticCStatusDigestTimeout,
		staticCStatusResourceLimit,
		staticCStatusProtocolError,
		staticCStatusIncomplete, staticCStatusMismatch:
		return true
	default:
		return false
	}
}

func staticCMeasurementFailureStatus(status string) bool {
	switch status {
	case staticCStatusParserError, staticCStatusParserTimeout,
		staticCStatusTransportError, staticCStatusTransportTimeout,
		staticCStatusMeasurementError, staticCStatusProtocolError,
		staticCStatusIncomplete:
		return true
	default:
		return false
	}
}

func staticCTimeoutStatus(status string) bool {
	return status == staticCStatusParserTimeout || status == staticCStatusTransportTimeout || status == staticCStatusDigestTimeout
}

type staticCArtifactManifest struct {
	Schema         string `json:"schema"`
	BuildKeySHA256 string `json:"build_key_sha256"`
	ArtifactSHA256 string `json:"artifact_sha256"`
}

type staticCBuildInputSnapshot struct {
	Common          perfScanOracleCommonIdentity
	Language        perfScanOracleLanguageIdentity
	RuntimeRepoDir  string
	RuntimeSource   string
	GrammarRepoDir  string
	GrammarTreePath string
	SourceRoot      string
	ParserPath      string
	ScannerPaths    []string
	DriverPath      string
}

var errStaticCBuildInputDrift = errors.New("static C build input drift")

type staticCPerfMeasurement struct {
	Status  string
	Samples []int64
	Detail  string
}

type staticCOracleError struct {
	Status string
	Detail string
	Stop   *perfScanStop
}

func (err *staticCOracleError) Error() string { return err.Detail }

func staticCOracleErrorStatus(err error) string {
	var oracleErr *staticCOracleError
	if errors.As(err, &oracleErr) && oracleErr.Status != "" {
		return oracleErr.Status
	}
	return staticCStatusAdmissionError
}

func staticCOracleErrorStop(err error) *perfScanStop {
	var oracleErr *staticCOracleError
	if errors.As(err, &oracleErr) && oracleErr.Stop != nil {
		stop := *oracleErr.Stop
		return &stop
	}
	return nil
}

func newStaticCOracleError(status, detail string) error {
	return &staticCOracleError{Status: status, Detail: detail}
}

func newStaticCOracleStoppedError(status, detail string, stop *perfScanStop) error {
	return &staticCOracleError{Status: status, Detail: detail, Stop: stop}
}

func TestStaticCVerifyArtifactRejectsDynamicExecutable(t *testing.T) {
	source := filepath.Join(t.TempDir(), "dynamic.c")
	artifact := filepath.Join(t.TempDir(), "dynamic")
	program := []byte("void ts_parser_parse(void) {}\nvoid tree_sitter_canary(void) {}\nint main(void) { return 0; }\n")
	if err := os.WriteFile(source, program, 0o600); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command("cc", "-O2", source, "-o", artifact).CombinedOutput(); err != nil {
		t.Fatalf("compile dynamic canary: %v\n%s", err, out)
	}
	if proof, err := staticCVerifyArtifact(artifact, "tree_sitter_canary"); err == nil || proof != "" {
		t.Fatalf("dynamic executable accepted: proof=%q err=%v", proof, err)
	}
}

func TestStaticCValidateCachedArtifactRejectsAndRebuildsTampering(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "cache")
	artifact := filepath.Join(dir, "perf_oracle")
	source := filepath.Join(root, "static.c")
	program := []byte("void ts_parser_parse(void) {}\nvoid tree_sitter_canary(void) {}\nint main(void) { return 0; }\n")
	if err := os.WriteFile(source, program, 0o600); err != nil {
		t.Fatal(err)
	}
	buildKey := strings.Repeat("a", 64)
	build := func() error {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
		if out, err := exec.Command("cc", "-static", "-O2", source, "-o", artifact).CombinedOutput(); err != nil {
			return fmt.Errorf("compile static cache canary: %w: %s", err, out)
		}
		return staticCWriteArtifactManifest(dir, artifact, buildKey)
	}
	if err := build(); err != nil {
		t.Fatal(err)
	}
	if _, proof, err := staticCValidateCachedArtifact(dir, artifact, buildKey, "tree_sitter_canary"); err != nil || proof != staticCPerfLinkageProof {
		t.Fatalf("fresh cache validation: proof=%q err=%v", proof, err)
	}
	buildsOnDrift := 0
	if _, _, err := staticCEnsureCachedArtifactValidated(dir, artifact, buildKey, "tree_sitter_canary", func() error {
		return fmt.Errorf("%w: compiler replaced after capture", errStaticCBuildInputDrift)
	}, func() error {
		buildsOnDrift++
		return nil
	}); !errors.Is(err, errStaticCBuildInputDrift) || buildsOnDrift != 0 {
		t.Fatalf("cache-hit input drift err=%v builds=%d", err, buildsOnDrift)
	}
	if _, proof, err := staticCValidateCachedArtifact(dir, artifact, buildKey, "tree_sitter_canary"); err != nil || proof != staticCPerfLinkageProof {
		t.Fatalf("valid cache was quarantined after current-input drift: proof=%q err=%v", proof, err)
	}

	file, err := os.OpenFile(artifact, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.Write([]byte("tampered")); err != nil {
		_ = file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	if _, _, err := staticCValidateCachedArtifact(dir, artifact, buildKey, "tree_sitter_canary"); err == nil || !strings.Contains(err.Error(), "artifact SHA") {
		t.Fatalf("tampered cached executable was accepted: %v", err)
	}
	rebuilds := 0
	if _, proof, err := staticCEnsureCachedArtifact(dir, artifact, buildKey, "tree_sitter_canary", func() error {
		rebuilds++
		return build()
	}); err != nil || proof != staticCPerfLinkageProof || rebuilds != 1 {
		t.Fatalf("tampered cache rebuild: proof=%q rebuilds=%d err=%v", proof, rebuilds, err)
	}

	if err := os.Remove(staticCArtifactManifestPath(dir)); err != nil {
		t.Fatal(err)
	}
	if _, _, err := staticCValidateCachedArtifact(dir, artifact, buildKey, "tree_sitter_canary"); err == nil || !strings.Contains(err.Error(), "manifest") {
		t.Fatalf("manifest-free cached executable was accepted: %v", err)
	}
	if _, proof, err := staticCEnsureCachedArtifact(dir, artifact, buildKey, "tree_sitter_canary", func() error {
		rebuilds++
		return build()
	}); err != nil || proof != staticCPerfLinkageProof || rebuilds != 2 {
		t.Fatalf("manifest-free cache rebuild: proof=%q rebuilds=%d err=%v", proof, rebuilds, err)
	}
	artifactSHA, _, err := staticCValidateCachedArtifact(dir, artifact, buildKey, "tree_sitter_canary")
	if err != nil {
		t.Fatal(err)
	}
	privateArtifact, cleanup, err := staticCPrivateExecutableSnapshot(artifact, artifactSHA, "tree_sitter_canary")
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	if err := os.WriteFile(artifact, []byte("cache path replaced after validation"), 0o755); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command(privateArtifact).CombinedOutput(); err != nil {
		t.Fatalf("verified private artifact did not survive cache replacement: %v: %s", err, out)
	}
}

func TestStaticCInstallBuiltArtifactDoesNotPublishDriftedInputs(t *testing.T) {
	parent := t.TempDir()
	buildDir := filepath.Join(parent, "shared-cache-entry")
	artifact := filepath.Join(buildDir, "perf_oracle")
	tmp := filepath.Join(parent, "private-build")
	if err := os.MkdirAll(tmp, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tmp, "perf_oracle"), []byte("unpublished fixture"), 0o500); err != nil {
		t.Fatal(err)
	}
	want := fmt.Errorf("%w: deterministic mutation", errStaticCBuildInputDrift)
	err := staticCInstallBuiltArtifact(tmp, buildDir, artifact, strings.Repeat("a", 64), "tree_sitter_canary", func() error { return want })
	if !errors.Is(err, errStaticCBuildInputDrift) {
		t.Fatalf("drifted install error=%v", err)
	}
	if _, statErr := os.Stat(buildDir); !os.IsNotExist(statErr) {
		t.Fatalf("drifted artifact became visible in shared cache: %v", statErr)
	}
	if _, statErr := os.Stat(staticCArtifactManifestPath(tmp)); !os.IsNotExist(statErr) {
		t.Fatalf("drifted private build received a publishable manifest: %v", statErr)
	}
}

func TestStaticCRevalidateBuildInputsRejectsDeterministicMutations(t *testing.T) {
	dir := t.TempDir()
	write := func(name, content string, mode os.FileMode) string {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte(content), mode); err != nil {
			t.Fatal(err)
		}
		return path
	}
	compiler := write("compiler", "#!/bin/sh\necho fixture-compiler\n", 0o700)
	linker := write("linker", "#!/bin/sh\necho fixture-linker\n", 0o700)
	runtimeSource := write("runtime.c", "runtime-v1", 0o600)
	driver := write("driver.c", "driver-v1", 0o600)
	parser := write("parser.c", "parser-v1", 0o600)
	scanner := write("scanner.c", "scanner-v1", 0o600)
	compilerPath, compilerVersion, compilerSHA, err := staticCToolIdentityAtPath(compiler)
	if err != nil {
		t.Fatal(err)
	}
	linkerPath, linkerVersion, linkerSHA, err := staticCToolIdentityAtPath(linker)
	if err != nil {
		t.Fatal(err)
	}
	sha := func(path string) string {
		value, err := fileSHA256(path)
		if err != nil {
			t.Fatal(err)
		}
		return value
	}
	inputs := staticCBuildInputSnapshot{
		Common: perfScanOracleCommonIdentity{
			CompilerPath: compilerPath, CompilerVersion: compilerVersion, CompilerSHA256: compilerSHA,
			RuntimeSourceSHA256: sha(runtimeSource), DriverSHA256: sha(driver),
		},
		Language: perfScanOracleLanguageIdentity{
			LinkerPath: linkerPath, LinkerVersion: linkerVersion, LinkerSHA256: linkerSHA,
			Parser:   perfScanOracleSourceIdentity{Path: "parser.c", SHA256: sha(parser)},
			Scanners: []perfScanOracleSourceIdentity{{Path: "scanner.c", SHA256: sha(scanner)}},
		},
		RuntimeSource: runtimeSource,
		DriverPath:    driver,
		SourceRoot:    dir,
		ParserPath:    parser,
		ScannerPaths:  []string{scanner},
	}
	if err := staticCRevalidateBuildInputs(inputs); err != nil {
		t.Fatalf("stable inputs rejected: %v", err)
	}
	for _, tc := range []struct {
		label   string
		path    string
		content string
		mode    os.FileMode
	}{
		{label: "compiler", path: compiler, content: "#!/bin/sh\necho changed-compiler\n", mode: 0o700},
		{label: "linker", path: linker, content: "#!/bin/sh\necho changed-linker\n", mode: 0o700},
		{label: "runtime", path: runtimeSource, content: "runtime-v2", mode: 0o600},
		{label: "driver", path: driver, content: "driver-v2", mode: 0o600},
		{label: "parser", path: parser, content: "parser-v2", mode: 0o600},
		{label: "scanner", path: scanner, content: "scanner-v2", mode: 0o600},
	} {
		t.Run(tc.label, func(t *testing.T) {
			original, err := os.ReadFile(tc.path)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(tc.path, []byte(tc.content), tc.mode); err != nil {
				t.Fatal(err)
			}
			if err := staticCRevalidateBuildInputs(inputs); err == nil || !strings.Contains(err.Error(), tc.label) {
				t.Fatalf("%s mutation was not rejected: %v", tc.label, err)
			}
			if err := os.WriteFile(tc.path, original, tc.mode); err != nil {
				t.Fatal(err)
			}
			if err := staticCRevalidateBuildInputs(inputs); err != nil {
				t.Fatalf("restored %s input rejected: %v", tc.label, err)
			}
		})
	}
}

func TestStaticCSourceCompileFlagsLocksAssertionException(t *testing.T) {
	entry := parityLockEntry{
		Name:    "just",
		RepoURL: "https://github.com/IndianBoy42/tree-sitter-just",
		Commit:  "60df3d5b3fda2a22fdb3621226cafab50b763663",
	}
	const scannerSHA = "99a4ae2ff08e90a18d21fc42d324411aa0df57ebf486e06e70c0b5dd26d489e8"
	flags, err := staticCSourceCompileFlags(entry, "scanner.c", scannerSHA)
	if err != nil {
		t.Fatal(err)
	}
	if flags != staticCPerfGrammarCAssertFlags {
		t.Fatalf("exact assertion profile flags=%q, want %q", flags, staticCPerfGrammarCAssertFlags)
	}

	tests := []struct {
		name   string
		entry  parityLockEntry
		path   string
		sha    string
		want   string
		failed bool
	}{
		{name: "wrong language", entry: parityLockEntry{Name: "other", RepoURL: entry.RepoURL, Commit: entry.Commit}, path: "scanner.c", sha: scannerSHA, want: staticCPerfGrammarCFlags},
		{name: "wrong repository", entry: parityLockEntry{Name: entry.Name, RepoURL: "https://example.invalid/tree-sitter-just", Commit: entry.Commit}, path: "scanner.c", sha: scannerSHA, want: staticCPerfGrammarCFlags},
		{name: "wrong commit", entry: parityLockEntry{Name: entry.Name, RepoURL: entry.RepoURL, Commit: strings.Repeat("0", 40)}, path: "scanner.c", sha: scannerSHA, want: staticCPerfGrammarCFlags},
		{name: "wrong path", entry: entry, path: "other.c", sha: scannerSHA, want: staticCPerfGrammarCFlags},
		{name: "wrong source", entry: entry, path: "scanner.c", sha: strings.Repeat("0", 64), want: staticCPerfGrammarCFlags},
		{name: "ordinary C++", entry: entry, path: "scanner.cc", sha: scannerSHA, want: staticCPerfGrammarCXXFlags},
		{name: "unsupported", entry: entry, path: "scanner.rs", sha: scannerSHA, failed: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := staticCSourceCompileFlags(tc.entry, tc.path, tc.sha)
			if tc.failed {
				if err == nil {
					t.Fatalf("unsupported source returned flags %q", got)
				}
				return
			}
			if err != nil || got != tc.want {
				t.Fatalf("flags=%q want=%q err=%v", got, tc.want, err)
			}
		})
	}
}

func TestStaticCBuildKeyAuthenticatesScannerCompileFlags(t *testing.T) {
	common := perfScanOracleCommonIdentity{SourceFlagProtocol: staticCPerfSourceFlagProtocol}
	language := perfScanOracleLanguageIdentity{
		Scanners: []perfScanOracleSourceIdentity{{
			Path: "scanner.c", SHA256: strings.Repeat("1", 64), CompileFlags: staticCPerfGrammarCFlags,
		}},
	}
	defaultKey := staticCBuildKey(common, language)
	language.Scanners[0].CompileFlags = staticCPerfGrammarCAssertFlags
	assertKey := staticCBuildKey(common, language)
	if defaultKey == assertKey {
		t.Fatal("scanner compile flags did not change the static C build key")
	}
}

func TestStaticCPerfOracleCanaries(t *testing.T) {
	if os.Getenv(staticCPerfCanaryEnv) != "1" {
		t.Skip("set " + staticCPerfCanaryEnv + "=1 to build locked static C oracle canaries")
	}
	tests := []struct {
		name         string
		language     string
		source       string
		scannerKind  string
		scannerFlags string
		nonzeroRoot  bool
	}{
		{name: "no_scanner", language: "go", source: "package main\n\nfunc main() {}\n", scannerKind: "none"},
		{name: "leading_trivia", language: "go", source: " \n\t", scannerKind: "none", nonzeroRoot: true},
		{name: "c_scanner", language: "python", source: "def f(x):\n    return x + 1\n", scannerKind: "c"},
		{name: "c_scanner_assertions", language: "just", source: "name := \"value\"\n", scannerKind: "c", scannerFlags: staticCPerfGrammarCAssertFlags},
		{name: "cxx_scanner", language: "norg", source: "* Heading\n\nBody\n", scannerKind: "cxx"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			oracle, err := buildStaticCPerfOracle(tc.language)
			if err != nil {
				t.Fatal(err)
			}
			defer oracle.Close()
			if oracle.identity.Language.ArtifactLinkage != staticCPerfLinkageProof || !oracle.identity.Language.ArtifactStatic {
				t.Fatalf("static linkage identity = %+v", oracle.identity.Language)
			}
			gotScannerKind := "none"
			for _, scanner := range oracle.identity.Language.Scanners {
				switch strings.ToLower(filepath.Ext(scanner.Path)) {
				case ".c":
					if gotScannerKind == "none" {
						gotScannerKind = "c"
					}
				case ".cc", ".cpp", ".cxx":
					gotScannerKind = "cxx"
				}
			}
			if gotScannerKind != tc.scannerKind {
				t.Fatalf("scanner kind=%q want %q; sources=%+v", gotScannerKind, tc.scannerKind, oracle.identity.Language.Scanners)
			}
			if tc.scannerFlags != "" {
				if len(oracle.identity.Language.Scanners) != 1 || oracle.identity.Language.Scanners[0].CompileFlags != tc.scannerFlags {
					t.Fatalf("scanner flags=%+v, want %q", oracle.identity.Language.Scanners, tc.scannerFlags)
				}
			}

			cLang, err := ParityCLanguage(tc.language)
			if err != nil {
				t.Fatal(err)
			}
			parser := sitter.NewParser()
			defer parser.Close()
			if err := parser.SetLanguage(cLang); err != nil {
				t.Fatal(err)
			}
			const budget = 10 * time.Second
			parser.SetTimeoutMicros(uint64(budget.Microseconds()))
			measurer := &perfScanLangMeasurer{staticC: oracle, cPsr: parser, budget: budget}
			admission, err := measurer.admitStaticOracle([]byte(tc.source))
			if err != nil {
				t.Fatal(err)
			}
			if !admission.Admitted || admission.StaticDeepSHA256 != admission.ParityDeepSHA256 {
				t.Fatalf("deep admission = %+v", admission)
			}
			if tc.nonzeroRoot {
				tree := parser.Parse([]byte(tc.source), nil)
				if tree == nil {
					t.Fatal("leading-trivia C parse returned nil")
				}
				root := tree.RootNode()
				start, end := root.StartByte(), root.EndByte()
				tree.Close()
				if start == 0 || end != uint(len(tc.source)) {
					t.Fatalf("leading-trivia regression requires nonzero-start complete root: start=%d end=%d len=%d", start, end, len(tc.source))
				}
			}
			measurement := oracle.measure([]byte(tc.source), perfScanAxisFull, 1, 2, budget)
			if measurement.Status != perfScanStatusOK || len(measurement.Samples) != 2 {
				t.Fatalf("measurement = %+v", measurement)
			}
			for _, sample := range measurement.Samples {
				if sample <= 0 {
					t.Fatalf("non-positive static C full-parse sample: %d", sample)
				}
			}
			t.Logf("language=%s artifact=%s admission=%s samples=%v", tc.language, oracle.identity.Language.ArtifactSHA256, admission.StaticDeepSHA256, measurement.Samples)
		})
	}
}

func TestStaticCPerfOracleTimeoutCanary(t *testing.T) {
	if os.Getenv(staticCPerfCanaryEnv) != "1" {
		t.Skip("set " + staticCPerfCanaryEnv + "=1 to build the locked static C timeout canary")
	}
	oracle, err := buildStaticCPerfOracle("go")
	if err != nil {
		t.Fatal(err)
	}
	defer oracle.Close()
	source := append([]byte("package p\n"), bytes.Repeat([]byte("var x = 1\n"), 200_000)...)
	_, sourceSHA, err := oracle.deepDigest(source, time.Microsecond)
	if err == nil || staticCOracleErrorStatus(err) != staticCStatusParserTimeout {
		t.Fatalf("bounded deep admission err=%v status=%q", err, staticCOracleErrorStatus(err))
	}
	if !staticCHashIdentity(sourceSHA) {
		t.Fatalf("timeout lost immutable source identity: %q", sourceSHA)
	}
	// The removed C timeout setter treated zero as no timeout. Keep that
	// behavior in the progress-callback transport as well.
	digest, _, err := oracle.deepDigest([]byte("package p\nvar x = 1\n"), 0)
	if err != nil || !staticCHashIdentity(digest) {
		t.Fatalf("zero timeout changed unlimited parsing: digest=%q err=%v", digest, err)
	}
}

func TestStaticCPerfOracleDistinguishesTransportTimeout(t *testing.T) {
	artifact := filepath.Join(t.TempDir(), "blocking-oracle")
	if err := os.WriteFile(artifact, []byte("#!/bin/sh\nexec sleep 2\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	oracle := &staticCPerfOracle{artifact: artifact, wallGrace: 10 * time.Millisecond}
	if _, _, err := oracle.deepDigest([]byte("source"), 10*time.Millisecond); err == nil || staticCOracleErrorStatus(err) != staticCStatusTransportTimeout {
		t.Fatalf("deep transport timeout err=%v status=%q", err, staticCOracleErrorStatus(err))
	}
	measurement := oracle.measure([]byte("source"), perfScanAxisFull, 0, 1, 10*time.Millisecond)
	if measurement.Status != staticCStatusTransportTimeout {
		t.Fatalf("measurement transport timeout = %+v", measurement)
	}
}

func TestStaticCPerfOracleClosedFailureVocabulary(t *testing.T) {
	writeOracle := func(script string) *staticCPerfOracle {
		artifact := filepath.Join(t.TempDir(), "fixture-oracle")
		if err := os.WriteFile(artifact, []byte("#!/bin/sh\n"+script+"\n"), 0o755); err != nil {
			t.Fatal(err)
		}
		return &staticCPerfOracle{artifact: artifact, wallGrace: 50 * time.Millisecond}
	}
	for _, tc := range []struct {
		name   string
		marker string
		want   string
	}{
		{name: "parser", marker: "status=c_parser_error", want: staticCStatusParserError},
		{name: "digest", marker: "status=c_digest_error", want: staticCStatusDigestError},
		{name: "transport", marker: "transport failed without a protocol status", want: staticCStatusTransportError},
	} {
		t.Run(tc.name+"_deep", func(t *testing.T) {
			oracle := writeOracle("echo '" + tc.marker + "' >&2; exit 2")
			_, _, err := oracle.deepDigest([]byte("source"), time.Second)
			if err == nil || staticCOracleErrorStatus(err) != tc.want {
				t.Fatalf("failure err=%v status=%q want=%q", err, staticCOracleErrorStatus(err), tc.want)
			}
		})
	}
	t.Run("protocol_measurement", func(t *testing.T) {
		oracle := writeOracle("printf 'schema=wrong\\naxis=full\\nstatus=ok\\n'")
		measurement := oracle.measure([]byte("source"), perfScanAxisFull, 0, 1, time.Second)
		if measurement.Status != staticCStatusProtocolError {
			t.Fatalf("protocol measurement=%+v", measurement)
		}
	})
	t.Run("missing_sample_measurement", func(t *testing.T) {
		oracle := writeOracle("printf 'schema=" + staticCPerfSchema + "\\naxis=full\\nstatus=ok\\n'")
		measurement := oracle.measure([]byte("source"), perfScanAxisFull, 0, 1, time.Second)
		if measurement.Status != staticCStatusMeasurementError {
			t.Fatalf("missing-sample measurement=%+v", measurement)
		}
	})
	if !staticCAdmissionFailureStatus(staticCStatusResourceLimit) {
		t.Fatalf("resource status %q is outside the admission vocabulary", staticCStatusResourceLimit)
	}
}

func TestPerfScanValidateCgoResourceAdmissionEvidence(t *testing.T) {
	detail := "cgo parity admission child exceeded 16 MiB"
	file := &perfScanFile{
		OracleAdmission: &perfScanOracleAdmission{
			DigestFormat:    "gts-deep-tree-v1",
			SourceSHA256:    strings.Repeat("a", 64),
			CgoPeakRSSBytes: 17 << 20,
			Stop: &perfScanStop{
				Class:          perfScanStopRSSLimit,
				Implementation: "c",
				Phase:          "oracle_admission",
				Attempt:        1,
				Detail:         detail,
			},
			Status: staticCStatusResourceLimit,
			Detail: detail,
		},
		Classification: &perfScanFileClassification{
			Class:    perfScanClassClean,
			Reason:   "accepted full-span Go tree without ERROR nodes",
			GoStatus: perfScanStatusOK,
			FullSpan: true,
		},
		Axes: map[string]*perfScanFileAxis{
			perfScanAxisFull: {
				Status:     staticCStatusResourceLimit,
				Detail:     detail,
				GoMedianNs: 100,
				GoMinNs:    80,
				GoMaxNs:    120,
				Verdict:    perfScanBucketNoData,
				Stop: &perfScanStop{
					Class:          perfScanStopRSSLimit,
					Implementation: "c",
					Phase:          "oracle_admission",
					Attempt:        1,
					Detail:         detail,
				},
			},
		},
	}
	if err := perfScanValidateFileOracleEvidence(file, "gts-deep-tree-v1"); err != nil {
		t.Fatalf("valid resource admission: %v", err)
	}
	file.OracleAdmission.CgoPeakRSSBytes = 0
	if err := perfScanValidateFileOracleEvidence(file, "gts-deep-tree-v1"); err == nil {
		t.Fatal("resource admission without a measured peak RSS was accepted")
	}
	file.OracleAdmission.CgoPeakRSSBytes = 17 << 20
	file.Axes[perfScanAxisFull].Stop = &perfScanStop{
		Class:          perfScanStopParserBudget,
		Reason:         "memory_budget",
		Implementation: "go",
		Phase:          "warmup",
		Attempt:        1,
		Detail:         "Go parser reached its memory budget",
	}
	if err := perfScanValidateFileOracleEvidence(file, "gts-deep-tree-v1"); err != nil {
		t.Fatalf("valid dual C resource and Go parser stops: %v", err)
	}
	file.OracleAdmission.Stop = nil
	if err := perfScanValidateFileOracleEvidence(file, "gts-deep-tree-v1"); err == nil {
		t.Fatal("resource admission without its typed C stop was accepted")
	}
}

func perfScanOracleBoardFromRows(rows []*perfScanLanguage) (*perfScanOracleBoardIdentity, error) {
	return perfScanOracleBoardFromRowsForMode(rows, perfScanReduceModeCertify)
}

func perfScanOracleBoardFromRowsForMode(rows []*perfScanLanguage, mode string) (*perfScanOracleBoardIdentity, error) {
	allowReportClosures := mode == perfScanReduceModeReport
	board := &perfScanOracleBoardIdentity{}
	for _, row := range rows {
		if allowReportClosures && perfScanReportClosureRow(row) {
			if err := perfScanValidateReportClosureRow(row); err != nil {
				return nil, err
			}
			continue
		}
		if row == nil || row.Oracle == nil {
			return nil, fmt.Errorf("language row is missing static C oracle identity")
		}
		if board.Common.Contract == "" {
			board.Common = row.Oracle.Common
		} else if !reflect.DeepEqual(board.Common, row.Oracle.Common) {
			return nil, fmt.Errorf("language %q static C common identity differs", row.Language)
		}
		board.Languages = append(board.Languages, row.Oracle.Language)
	}
	if board.Common.Contract == "" || len(board.Languages) == 0 {
		return nil, fmt.Errorf("reduced scoreboard has no authenticated static C oracle identity")
	}
	sort.Slice(board.Languages, func(i, j int) bool { return board.Languages[i].Language < board.Languages[j].Language })
	return board, nil
}

func perfScanValidateOracleEvidence(board *perfScanScoreboard) error {
	return perfScanValidateOracleEvidenceForMode(board, perfScanReduceModeCertify)
}

func perfScanValidateOracleEvidenceForMode(board *perfScanScoreboard, mode string) error {
	allowReportClosures := mode == perfScanReduceModeReport
	if board == nil || board.Oracle == nil {
		return fmt.Errorf("scoreboard is missing static C oracle identity")
	}
	common := board.Oracle.Common
	if common.Contract != COracleContractVersion || common.Transport != staticCPerfTransport || common.Linkage != staticCPerfLinkage {
		return fmt.Errorf("oracle contract/transport/linkage=%q/%q/%q", common.Contract, common.Transport, common.Linkage)
	}
	if common.BindingModule != COracleBindingModule || common.BindingVersion != COracleBindingVersion || common.BindingCommit != COracleBindingCommit {
		return fmt.Errorf("oracle binding identity is not the locked %s@%s (%s)", COracleBindingModule, COracleBindingVersion, COracleBindingCommit)
	}
	if common.RuntimeRepo != staticCPerfRuntimeRepo || common.RuntimeVersion != COracleRuntimeVersion || common.RuntimeCommit != COracleRuntimeCommit {
		return fmt.Errorf("oracle runtime identity is not locked to %s", COracleRuntimeCommit)
	}
	if common.OptimizationFlags != staticCPerfOptimizationFlags ||
		common.RuntimeCFlags != staticCPerfRuntimeCFlags ||
		common.GrammarCFlags != staticCPerfGrammarCFlags ||
		common.GrammarCXXFlags != staticCPerfGrammarCXXFlags ||
		common.SourceFlagProtocol != staticCPerfSourceFlagProtocol ||
		common.DriverCFlags != staticCPerfDriverCFlags ||
		common.LinkFlags != staticCPerfLinkFlags ||
		common.TimingRegion != staticCPerfTimingRegion ||
		common.OrderProtocol != staticCPerfOrderProtocol ||
		common.FailureVocabulary != staticCPerfFailureVocabulary ||
		common.DeepDigestFormat != "gts-deep-tree-v1" {
		return fmt.Errorf("oracle compile/link/timing/digest contract differs from the locked static full-parse protocol")
	}
	for label, value := range map[string]string{
		"runtime source tree": common.RuntimeSourceTreeOID,
		"runtime source":      common.RuntimeSourceSHA256,
		"driver":              common.DriverSHA256,
	} {
		if !staticCHashIdentity(value) {
			return fmt.Errorf("oracle %s identity %q is not a full hash", label, value)
		}
	}
	if !filepath.IsAbs(common.CompilerPath) || strings.TrimSpace(common.CompilerVersion) == "" || !staticCHashIdentity(common.CompilerSHA256) {
		return fmt.Errorf("oracle compiler identity is incomplete")
	}
	if common.TargetOS != board.Host.GOOS || common.TargetArch != board.Host.GOARCH {
		return fmt.Errorf("oracle target %s/%s differs from scoreboard host %s/%s", common.TargetOS, common.TargetArch, board.Host.GOOS, board.Host.GOARCH)
	}
	if !reflect.DeepEqual(board.Config.Axes, []string{perfScanAxisFull}) {
		return fmt.Errorf("authoritative static C scoreboard axes=%v, want [%s]", board.Config.Axes, perfScanAxisFull)
	}

	lockPath, err := findParityLockPath()
	if err != nil {
		return err
	}
	lock, err := loadParityLock(lockPath)
	if err != nil {
		return err
	}
	boardByLanguage := make(map[string]perfScanOracleLanguageIdentity, len(board.Oracle.Languages))
	for _, identity := range board.Oracle.Languages {
		if _, duplicate := boardByLanguage[identity.Language]; duplicate {
			return fmt.Errorf("duplicate oracle language identity %q", identity.Language)
		}
		boardByLanguage[identity.Language] = identity
	}
	wantOracleLanguages := len(board.Languages)
	if allowReportClosures {
		for _, row := range board.Languages {
			if perfScanReportClosureRow(row) {
				if err := perfScanValidateReportClosureRow(row); err != nil {
					return fmt.Errorf("language %q report closure: %w", row.Language, err)
				}
				if _, exists := boardByLanguage[row.Language]; exists {
					return fmt.Errorf("language %q typed closure contradicts a scoreboard oracle identity", row.Language)
				}
				wantOracleLanguages--
			}
		}
	}
	if len(boardByLanguage) != wantOracleLanguages {
		return fmt.Errorf("oracle identity has %d languages, scoreboard requires %d", len(boardByLanguage), wantOracleLanguages)
	}
	for _, row := range board.Languages {
		if allowReportClosures && perfScanReportClosureRow(row) {
			continue
		}
		if row == nil || row.Oracle == nil {
			return fmt.Errorf("language row is missing oracle identity")
		}
		if !reflect.DeepEqual(common, row.Oracle.Common) {
			return fmt.Errorf("language %q common oracle identity differs from scoreboard", row.Language)
		}
		identity := row.Oracle.Language
		if identity.Language != row.Language || !reflect.DeepEqual(boardByLanguage[row.Language], identity) {
			return fmt.Errorf("language %q oracle identity does not match its row/board entry", row.Language)
		}
		locked, ok := lock[row.Language]
		if !ok || identity.GrammarRepo != locked.RepoURL || identity.GrammarCommit != locked.Commit {
			return fmt.Errorf("language %q grammar identity is not the languages.lock entry", row.Language)
		}
		if !identity.ArtifactStatic || identity.ArtifactLinkage != staticCPerfLinkageProof || identity.LanguageSymbol == "" || !filepath.IsAbs(identity.LinkerPath) || identity.LinkerVersion == "" || !staticCHashIdentity(identity.LinkerSHA256) {
			return fmt.Errorf("language %q artifact/linker/symbol identity is incomplete", row.Language)
		}
		if identity.GrammarSourceMode != staticCSourceLocked && identity.GrammarSourceMode != staticCSourceGenerated {
			return fmt.Errorf("language %q grammar source mode %q is not recognized", row.Language, identity.GrammarSourceMode)
		}
		parserFlags, err := staticCSourceCompileFlags(locked, identity.Parser.Path, identity.Parser.SHA256)
		if err != nil {
			return fmt.Errorf("language %q parser compile identity: %w", row.Language, err)
		}
		if identity.Parser.Path == "" || identity.Parser.CompileFlags != parserFlags {
			return fmt.Errorf("language %q parser compile identity is not the locked C protocol", row.Language)
		}
		if want := staticCBuildKey(common, identity); identity.BuildKeySHA256 != want {
			return fmt.Errorf("language %q static C build key does not match its serialized inputs", row.Language)
		}
		for label, value := range map[string]string{
			"grammar source tree": identity.GrammarSourceTree,
			"parser":              identity.Parser.SHA256,
			"build key":           identity.BuildKeySHA256,
			"artifact":            identity.ArtifactSHA256,
		} {
			if !staticCHashIdentity(value) {
				return fmt.Errorf("language %q %s identity %q is not a full hash", row.Language, label, value)
			}
		}
		seenSources := map[string]bool{identity.Parser.Path: true}
		for _, scanner := range identity.Scanners {
			if scanner.Path == "" || !staticCHashIdentity(scanner.SHA256) {
				return fmt.Errorf("language %q scanner identity is incomplete", row.Language)
			}
			if seenSources[scanner.Path] {
				return fmt.Errorf("language %q has duplicate oracle source %q", row.Language, scanner.Path)
			}
			seenSources[scanner.Path] = true
			wantFlags, err := staticCSourceCompileFlags(locked, scanner.Path, scanner.SHA256)
			if err != nil {
				return fmt.Errorf("language %q scanner %q compile identity: %w", row.Language, scanner.Path, err)
			}
			if scanner.CompileFlags != wantFlags {
				return fmt.Errorf("language %q scanner %q compile flags are not locked", row.Language, scanner.Path)
			}
		}
		for fileIndex, file := range row.Files {
			if file == nil || file.OracleAdmission == nil {
				return fmt.Errorf("language %q file is missing oracle admission", row.Language)
			}
			wantOrder := "go_then_static_c"
			if staticCFirstFor(row.Language, fileIndex) {
				wantOrder = "static_c_then_go"
			}
			if file.MeasurementOrder != wantOrder {
				return fmt.Errorf("language %q file %q measurement order=%q, want %q", row.Language, file.Path, file.MeasurementOrder, wantOrder)
			}
			if err := perfScanValidateFileOracleEvidence(file, common.DeepDigestFormat); err != nil {
				return fmt.Errorf("language %q file %q: %w", row.Language, file.Path, err)
			}
		}
	}
	return nil
}

func perfScanValidateFileOracleEvidence(file *perfScanFile, digestFormat string) error {
	admission := file.OracleAdmission
	if admission.DigestFormat != digestFormat || !staticCSHA256Identity(admission.SourceSHA256) {
		return fmt.Errorf("invalid static/cgo admission format or source identity")
	}
	if admission.CgoGrammarSHA256 != "" && !staticCSHA256Identity(admission.CgoGrammarSHA256) {
		return fmt.Errorf("invalid cgo grammar identity")
	}
	if admission.Admitted {
		if admission.Status != "" || admission.Detail != "" || admission.Stop != nil ||
			!staticCSHA256Identity(admission.StaticDeepSHA256) ||
			admission.StaticDeepSHA256 != admission.ParityDeepSHA256 {
			return fmt.Errorf("invalid static/cgo deep admission")
		}
		return perfScanValidateAdmittedFullMeasurement(file)
	}

	if !staticCAdmissionFailureStatus(admission.Status) {
		return fmt.Errorf("unrecognized failed oracle admission status %q", admission.Status)
	}
	if strings.TrimSpace(admission.Detail) == "" {
		return fmt.Errorf("failed oracle admission %q has no detail", admission.Status)
	}
	if admission.Status == staticCStatusMismatch {
		if !staticCSHA256Identity(admission.StaticDeepSHA256) ||
			!staticCSHA256Identity(admission.ParityDeepSHA256) ||
			admission.StaticDeepSHA256 == admission.ParityDeepSHA256 {
			return fmt.Errorf("oracle mismatch admission lacks two distinct deep digests")
		}
	} else {
		for label, digest := range map[string]string{"static": admission.StaticDeepSHA256, "parity": admission.ParityDeepSHA256} {
			if digest != "" && !staticCSHA256Identity(digest) {
				return fmt.Errorf("failed oracle admission has invalid %s deep digest %q", label, digest)
			}
		}
		if admission.StaticDeepSHA256 != "" && admission.StaticDeepSHA256 == admission.ParityDeepSHA256 {
			return fmt.Errorf("failed oracle admission contradicts equal completed deep digests")
		}
	}

	full := file.Axes[perfScanAxisFull]
	if full == nil {
		return fmt.Errorf("failed oracle admission has no full-parse Go evidence")
	}
	if full.Status != admission.Status || strings.TrimSpace(full.Detail) == "" || !strings.Contains(full.Detail, admission.Detail) {
		return fmt.Errorf("failed oracle admission is not reflected by the full-parse result")
	}
	if admission.Status == staticCStatusResourceLimit {
		admissionStop := admission.Stop
		if admission.CgoPeakRSSBytes <= 0 || admissionStop == nil ||
			(admissionStop.Class != perfScanStopRSSLimit && admissionStop.Class != perfScanStopOOMOrKill) ||
			admissionStop.Implementation != "c" || admissionStop.Phase != "oracle_admission" ||
			admissionStop.Attempt != 1 || strings.TrimSpace(admissionStop.Detail) == "" {
			return fmt.Errorf("cgo resource admission lacks bounded stop evidence")
		}
		if full.Stop == nil {
			return fmt.Errorf("cgo resource admission lacks a full-parse stop")
		}
		if full.Stop.Implementation == "c" {
			if full.Stop.Class != admissionStop.Class || full.Stop.Phase != admissionStop.Phase {
				return fmt.Errorf("full-parse C stop differs from the cgo resource admission stop")
			}
		} else if !perfScanIsGoStop(full.Stop) {
			return fmt.Errorf("cgo resource admission has an invalid full-parse stop")
		}
	}
	if full.CMedianNs != 0 || full.CMinNs != 0 || full.CMaxNs != 0 || full.Ratio != 0 || full.RatioIsLowerBound || full.Verdict != perfScanBucketNoData {
		return fmt.Errorf("failed oracle admission contains fabricated C timing or ratio evidence")
	}
	if err := perfScanValidateClassification(file.Classification); err != nil {
		return fmt.Errorf("failed oracle admission Go classification: %w", err)
	}
	if full.GoMedianNs > 0 {
		if full.GoMinNs <= 0 || full.GoMaxNs <= 0 || full.GoMinNs > full.GoMedianNs || full.GoMedianNs > full.GoMaxNs || file.Classification.GoStatus != perfScanStatusOK {
			return fmt.Errorf("failed oracle admission has incomplete Go timing evidence")
		}
		return nil
	}
	if file.Classification.GoStatus == perfScanStatusOK {
		return fmt.Errorf("failed oracle admission has neither Go timing nor a typed Go failure")
	}
	if perfScanClassificationStatusIsStopped(file.Classification.GoStatus) && !perfScanIsGoStop(full.Stop) {
		return fmt.Errorf("failed oracle admission stopped Go classification lacks a structured Go stop")
	}
	return nil
}

func perfScanValidateAdmittedFullMeasurement(file *perfScanFile) error {
	full := file.Axes[perfScanAxisFull]
	if full == nil {
		return fmt.Errorf("admitted oracle file has no full-parse measurement")
	}
	if full.Status == perfScanStatusOK {
		return nil
	}
	if !staticCMeasurementFailureStatus(full.Status) {
		switch full.Status {
		case "go_timeout", "go_budget_stop", "go_stopped", "go_error", "go_panic":
		default:
			return fmt.Errorf("admitted oracle file has unrecognized full-parse status %q", full.Status)
		}
	}
	if strings.TrimSpace(full.Detail) == "" {
		return fmt.Errorf("admitted oracle file full-parse failure %q has no detail", full.Status)
	}
	if strings.HasPrefix(full.Status, "c_oracle_") {
		if full.CMedianNs != 0 || full.CMinNs != 0 || full.CMaxNs != 0 || full.Ratio != 0 || full.RatioIsLowerBound || full.Verdict != perfScanBucketNoData {
			return fmt.Errorf("admitted oracle file C measurement failure contains fabricated C timing or ratio evidence")
		}
	}
	return nil
}

func staticCSHA256Identity(value string) bool {
	return len(value) == sha256.Size*2 && staticCHashIdentity(value)
}

func staticCHashIdentity(value string) bool {
	if len(value) != 40 && len(value) != 64 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func staticCFirstFor(language string, fileIndex int) bool {
	sum := sha256.Sum256([]byte(language))
	return (int(sum[0]&1)+fileIndex)%2 == 1
}

var staticLanguageSymbolPattern = regexp.MustCompile(`(?m)const\s+TSLanguage\s*\*\s*(tree_sitter_[A-Za-z0-9_]+)\s*\(\s*(?:void\s*)?\)\s*\{`)

func buildStaticCPerfOracle(language string) (*staticCPerfOracle, error) {
	lockPath, err := findParityLockPath()
	if err != nil {
		return nil, err
	}
	lock, err := loadParityLock(lockPath)
	if err != nil {
		return nil, err
	}
	entry, ok := lock[language]
	if !ok {
		return nil, fmt.Errorf("static C oracle lock has no entry for %q", language)
	}

	cacheRoot := strings.TrimSpace(os.Getenv(staticCPerfCacheEnv))
	if cacheRoot == "" {
		repoRoot := filepath.Dir(filepath.Dir(lockPath))
		cacheRoot = filepath.Join(repoRoot, "harness_out", "c_oracle", "fleet_static")
	}
	cacheRoot, err = filepath.Abs(cacheRoot)
	if err != nil {
		return nil, fmt.Errorf("resolve static C oracle cache: %w", err)
	}
	sourcesRoot := filepath.Join(cacheRoot, "sources")
	runtimeDir := filepath.Join(sourcesRoot, "tree-sitter-"+COracleRuntimeCommit)
	if err := staticCEnsurePinnedRepo("runtime", staticCPerfRuntimeRepo, COracleRuntimeCommit, runtimeDir, ""); err != nil {
		return nil, err
	}
	pinnedGrammarDir := filepath.Join(sourcesRoot, paritySafeName(language)+"-"+entry.Commit)
	if err := staticCEnsurePinnedRepo(language+" grammar", entry.RepoURL, entry.Commit, pinnedGrammarDir, language); err != nil {
		return nil, err
	}
	runtimeSource := filepath.Join(runtimeDir, "lib", "src", "lib.c")
	var lastDrift error
	for attempt := 1; attempt <= 2; attempt++ {
		grammarDir, srcDir, parserPath, sourceMode, cleanupGrammar, resolveErr := staticCResolveGrammarSources(entry, pinnedGrammarDir, cacheRoot)
		if resolveErr != nil {
			return nil, fmt.Errorf("%s: resolve locked parser: %w", language, resolveErr)
		}
		oracle, _, buildErr := func() (*staticCPerfOracle, string, error) {
			defer cleanupGrammar()
			symbol, err := staticCLanguageSymbol(entry, parserPath)
			if err != nil {
				return nil, "", err
			}
			scanners := staticCScannerSources(srcDir)
			hasCXX := false
			for _, scanner := range scanners {
				switch strings.ToLower(filepath.Ext(scanner)) {
				case ".cc", ".cpp", ".cxx":
					hasCXX = true
				}
			}
			driverPath, err := staticCPerfDriverPath()
			if err != nil {
				return nil, "", err
			}
			common, err := staticCCommonIdentity(runtimeDir, runtimeSource, driverPath)
			if err != nil {
				return nil, "", err
			}
			languageIdentity, err := staticCLanguageIdentity(entry, grammarDir, srcDir, parserPath, scanners, symbol, sourceMode, hasCXX)
			if err != nil {
				return nil, "", err
			}
			languageIdentity.BuildKeySHA256 = staticCBuildKey(common, languageIdentity)
			buildDir := filepath.Join(cacheRoot, "artifacts", languageIdentity.BuildKeySHA256)
			artifact := filepath.Join(buildDir, "perf_oracle")
			inputs := staticCBuildInputSnapshot{
				Common: common, Language: languageIdentity,
				RuntimeRepoDir: runtimeDir, RuntimeSource: runtimeSource,
				GrammarRepoDir: pinnedGrammarDir, GrammarTreePath: entry.Subdir,
				SourceRoot: srcDir, ParserPath: parserPath, ScannerPaths: scanners,
				DriverPath: driverPath,
			}
			validateInputs := func() error {
				if err := staticCRevalidateBuildInputs(inputs); err != nil {
					return fmt.Errorf("%w: %v", errStaticCBuildInputDrift, err)
				}
				return nil
			}
			artifactSHA, linkageProof, err := staticCEnsureArtifact(
				buildDir,
				artifact,
				languageIdentity.BuildKeySHA256,
				runtimeDir,
				runtimeSource,
				srcDir,
				parserPath,
				scanners,
				languageIdentity,
				driverPath,
				symbol,
				hasCXX,
				common.CompilerPath,
				languageIdentity.LinkerPath,
				validateInputs,
			)
			if err != nil {
				return nil, buildDir, fmt.Errorf("prepare static C oracle: %w", err)
			}
			if err := validateInputs(); err != nil {
				return nil, buildDir, err
			}
			languageIdentity.ArtifactSHA256 = artifactSHA
			languageIdentity.ArtifactStatic = true
			languageIdentity.ArtifactLinkage = linkageProof
			executable, cleanup, err := staticCPrivateExecutableSnapshot(artifact, artifactSHA, symbol)
			if err != nil {
				return nil, buildDir, fmt.Errorf("snapshot verified static C oracle: %w", err)
			}
			return &staticCPerfOracle{artifact: executable, identity: perfScanOracleIdentity{Common: common, Language: languageIdentity}, cleanup: cleanup}, buildDir, nil
		}()
		if buildErr == nil {
			return oracle, nil
		}
		if !errors.Is(buildErr, errStaticCBuildInputDrift) {
			return nil, fmt.Errorf("%s: %w", language, buildErr)
		}
		lastDrift = buildErr
	}
	return nil, fmt.Errorf("%s: static C build inputs remained unstable after retry: %w", language, lastDrift)
}

func staticCEnsurePinnedRepo(label, repoURL, commit, dest, language string) error {
	if _, err := os.Stat(dest); err == nil {
		if err := verifyPinnedRepo(dest, commit); err != nil {
			return fmt.Errorf("%s cached checkout rejected: %w", label, err)
		}
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}
	tmp, err := os.MkdirTemp(filepath.Dir(dest), filepath.Base(dest)+".tmp.")
	if err != nil {
		return err
	}
	_ = os.Remove(tmp)
	defer os.RemoveAll(tmp)
	if language != "" {
		if cacheDir := parityRepoCacheDir(); cacheDir != "" {
			short := commit
			if len(short) > 12 {
				short = short[:12]
			}
			if cached, err := findCachedParityRepo(cacheDir, language, short); err == nil {
				if err := clonePinnedRepoFromLocalCache(cached, commit, tmp); err != nil {
					return err
				}
				if err := os.Rename(tmp, dest); err != nil {
					if verifyErr := verifyPinnedRepo(dest, commit); verifyErr != nil {
						return err
					}
				}
				return verifyPinnedRepo(dest, commit)
			}
		}
	}
	if err := clonePinnedRepo(repoURL, commit, tmp); err != nil {
		return fmt.Errorf("clone pinned %s: %w", label, err)
	}
	if err := os.Rename(tmp, dest); err != nil {
		if verifyErr := verifyPinnedRepo(dest, commit); verifyErr != nil {
			return fmt.Errorf("install pinned %s: %w", label, err)
		}
	}
	return verifyPinnedRepo(dest, commit)
}

func staticCPerfDriverPath() (string, error) {
	for _, candidate := range []string{
		filepath.Join("pure_c", "perf_oracle.c"),
		filepath.Join("cgo_harness", "pure_c", "perf_oracle.c"),
		filepath.Join("..", "cgo_harness", "pure_c", "perf_oracle.c"),
	} {
		if info, err := os.Stat(candidate); err == nil && info.Mode().IsRegular() {
			return filepath.Abs(candidate)
		}
	}
	return "", fmt.Errorf("could not find pure_c/perf_oracle.c")
}

func staticCResolveGrammarSources(entry parityLockEntry, pinnedDir, cacheRoot string) (grammarDir, srcDir, parserPath, sourceMode string, cleanup func(), err error) {
	cleanup = func() {}
	expectedParser := filepath.Join(pinnedDir, entry.Subdir, "parser.c")
	needsPreparedCopy := false
	if _, statErr := os.Stat(expectedParser); statErr != nil {
		needsPreparedCopy = true
	} else if version, ok := readParserLanguageVersion(expectedParser); ok && (version < parityMinLanguageVersion || version > parityMaxLanguageVersion) {
		needsPreparedCopy = true
	}
	grammarDir = pinnedDir
	if needsPreparedCopy {
		workRoot := filepath.Join(cacheRoot, "work")
		if err = os.MkdirAll(workRoot, 0o755); err != nil {
			return "", "", "", "", cleanup, err
		}
		grammarDir, err = os.MkdirTemp(workRoot, paritySafeName(entry.Name)+"-sources-")
		if err != nil {
			return "", "", "", "", cleanup, err
		}
		cleanup = func() { _ = os.RemoveAll(grammarDir) }
		if err = clonePinnedRepoFromLocalCache(pinnedDir, entry.Commit, grammarDir); err != nil {
			cleanup()
			return "", "", "", "", func() {}, err
		}
	}
	srcDir, parserPath, err = resolveParserSource(entry, grammarDir)
	if err != nil {
		cleanup()
		return "", "", "", "", func() {}, err
	}
	sourceMode = staticCSourceLocked
	if verifyErr := verifyPinnedRepo(grammarDir, entry.Commit); verifyErr != nil {
		if !needsPreparedCopy {
			return "", "", "", "", cleanup, fmt.Errorf("locked grammar checkout was modified while resolving sources: %w", verifyErr)
		}
		sourceMode = staticCSourceGenerated
	}
	return grammarDir, srcDir, parserPath, sourceMode, cleanup, nil
}

func staticCLanguageSymbol(entry parityLockEntry, parserPath string) (string, error) {
	data, err := os.ReadFile(parserPath)
	if err != nil {
		return "", err
	}
	found := make(map[string]bool)
	for _, match := range staticLanguageSymbolPattern.FindAllSubmatch(staticCCodeOnly(data), -1) {
		found[string(match[1])] = true
	}
	for _, candidate := range parityLanguageSymbols(entry) {
		if found[candidate] {
			return candidate, nil
		}
	}
	if len(found) == 1 {
		for symbol := range found {
			return symbol, nil
		}
	}
	var symbols []string
	for symbol := range found {
		symbols = append(symbols, symbol)
	}
	sort.Strings(symbols)
	return "", fmt.Errorf("%s: cannot select language symbol from %v", entry.Name, symbols)
}

// staticCCodeOnly preserves C token boundaries while blanking comments and
// string/character literals. Language entry-point discovery must never accept
// a prototype or text that merely mentions a definition-shaped signature.
func staticCCodeOnly(source []byte) []byte {
	out := append([]byte(nil), source...)
	const (
		code = iota
		lineComment
		blockComment
		stringLiteral
		charLiteral
	)
	state := code
	escaped := false
	for i := 0; i < len(out); i++ {
		current := out[i]
		next := byte(0)
		if i+1 < len(out) {
			next = out[i+1]
		}
		switch state {
		case code:
			switch {
			case current == '/' && next == '/':
				out[i], out[i+1] = ' ', ' '
				i++
				state = lineComment
			case current == '/' && next == '*':
				out[i], out[i+1] = ' ', ' '
				i++
				state = blockComment
			case current == '"':
				out[i] = ' '
				state = stringLiteral
				escaped = false
			case current == '\'':
				out[i] = ' '
				state = charLiteral
				escaped = false
			}
		case lineComment:
			if current == '\n' {
				state = code
			} else {
				out[i] = ' '
			}
		case blockComment:
			if current == '*' && next == '/' {
				out[i], out[i+1] = ' ', ' '
				i++
				state = code
			} else if current != '\n' {
				out[i] = ' '
			}
		case stringLiteral, charLiteral:
			quote := byte('"')
			if state == charLiteral {
				quote = '\''
			}
			if current == '\n' {
				state = code
				escaped = false
				continue
			}
			out[i] = ' '
			if escaped {
				escaped = false
			} else if current == '\\' {
				escaped = true
			} else if current == quote {
				state = code
			}
		}
	}
	return out
}

func staticCScannerSources(srcDir string) []string {
	var sources []string
	for _, name := range []string{"scanner.c", "scanner.cc", "scanner.cpp", "scanner.cxx"} {
		path := filepath.Join(srcDir, name)
		if info, err := os.Stat(path); err == nil && info.Mode().IsRegular() {
			sources = append(sources, path)
		}
	}
	return sources
}

func staticCCommonIdentity(runtimeDir, runtimeSource, driverPath string) (perfScanOracleCommonIdentity, error) {
	compilerPath, compilerVersion, compilerSHA, err := staticCToolIdentity("cc")
	if err != nil {
		return perfScanOracleCommonIdentity{}, err
	}
	runtimeTree, err := parityGitOutput(runtimeDir, "rev-parse", COracleRuntimeCommit+":lib/src")
	if err != nil {
		return perfScanOracleCommonIdentity{}, err
	}
	runtimeSHA, err := fileSHA256(runtimeSource)
	if err != nil {
		return perfScanOracleCommonIdentity{}, err
	}
	driverSHA, err := fileSHA256(driverPath)
	if err != nil {
		return perfScanOracleCommonIdentity{}, err
	}
	return perfScanOracleCommonIdentity{
		Contract:             COracleContractVersion,
		Transport:            staticCPerfTransport,
		Linkage:              staticCPerfLinkage,
		BindingModule:        COracleBindingModule,
		BindingVersion:       COracleBindingVersion,
		BindingCommit:        COracleBindingCommit,
		RuntimeRepo:          staticCPerfRuntimeRepo,
		RuntimeVersion:       COracleRuntimeVersion,
		RuntimeCommit:        COracleRuntimeCommit,
		RuntimeSourceTreeOID: strings.TrimSpace(runtimeTree),
		RuntimeSourceSHA256:  runtimeSHA,
		OptimizationFlags:    staticCPerfOptimizationFlags,
		RuntimeCFlags:        staticCPerfRuntimeCFlags,
		GrammarCFlags:        staticCPerfGrammarCFlags,
		GrammarCXXFlags:      staticCPerfGrammarCXXFlags,
		SourceFlagProtocol:   staticCPerfSourceFlagProtocol,
		DriverCFlags:         staticCPerfDriverCFlags,
		LinkFlags:            staticCPerfLinkFlags,
		TimingRegion:         staticCPerfTimingRegion,
		OrderProtocol:        staticCPerfOrderProtocol,
		FailureVocabulary:    staticCPerfFailureVocabulary,
		TargetOS:             runtime.GOOS,
		TargetArch:           runtime.GOARCH,
		CompilerPath:         compilerPath,
		CompilerVersion:      compilerVersion,
		CompilerSHA256:       compilerSHA,
		DriverSHA256:         driverSHA,
		DeepDigestFormat:     "gts-deep-tree-v1",
	}, nil
}

func staticCSourceCompileFlags(entry parityLockEntry, path, sourceSHA string) (string, error) {
	defaultFlags := ""
	switch strings.ToLower(filepath.Ext(path)) {
	case ".c":
		defaultFlags = staticCPerfGrammarCFlags
	case ".cc", ".cpp", ".cxx":
		defaultFlags = staticCPerfGrammarCXXFlags
	default:
		return "", fmt.Errorf("source %q has an unsupported C oracle type", path)
	}
	key := staticCSourceCompileProfileKey{
		Language:      entry.Name,
		GrammarRepo:   entry.RepoURL,
		GrammarCommit: entry.Commit,
		Path:          filepath.ToSlash(path),
		SHA256:        sourceSHA,
	}
	if flags, ok := staticCSourceCompileProfiles[key]; ok {
		return flags, nil
	}
	return defaultFlags, nil
}

func staticCLanguageIdentity(entry parityLockEntry, grammarDir, srcDir, parserPath string, scanners []string, symbol, sourceMode string, hasCXX bool) (perfScanOracleLanguageIdentity, error) {
	tree, err := parityGitOutput(grammarDir, "rev-parse", entry.Commit+":"+filepath.ToSlash(entry.Subdir))
	if err != nil {
		return perfScanOracleLanguageIdentity{}, err
	}
	parserSHA, err := fileSHA256(parserPath)
	if err != nil {
		return perfScanOracleLanguageIdentity{}, err
	}
	parserRelative := staticCSourceRelative(srcDir, parserPath)
	parserFlags, err := staticCSourceCompileFlags(entry, parserRelative, parserSHA)
	if err != nil {
		return perfScanOracleLanguageIdentity{}, err
	}
	identity := perfScanOracleLanguageIdentity{
		Language:          entry.Name,
		GrammarRepo:       entry.RepoURL,
		GrammarCommit:     entry.Commit,
		GrammarSourceTree: strings.TrimSpace(tree),
		GrammarSourceMode: sourceMode,
		LanguageSymbol:    symbol,
		Parser: perfScanOracleSourceIdentity{
			Path:         parserRelative,
			SHA256:       parserSHA,
			CompileFlags: parserFlags,
		},
	}
	for _, scanner := range scanners {
		sha, err := fileSHA256(scanner)
		if err != nil {
			return perfScanOracleLanguageIdentity{}, err
		}
		relative := staticCSourceRelative(srcDir, scanner)
		flags, err := staticCSourceCompileFlags(entry, relative, sha)
		if err != nil {
			return perfScanOracleLanguageIdentity{}, err
		}
		identity.Scanners = append(identity.Scanners, perfScanOracleSourceIdentity{
			Path: relative, SHA256: sha, CompileFlags: flags,
		})
	}
	linker := "cc"
	if hasCXX {
		linker = "c++"
	}
	identity.LinkerPath, identity.LinkerVersion, identity.LinkerSHA256, err = staticCToolIdentity(linker)
	if err != nil {
		return perfScanOracleLanguageIdentity{}, err
	}
	return identity, nil
}

func staticCSourceRelative(root, path string) string {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return filepath.ToSlash(path)
	}
	return filepath.ToSlash(rel)
}

func staticCToolIdentity(name string) (string, string, string, error) {
	path, err := exec.LookPath(name)
	if err != nil {
		return "", "", "", fmt.Errorf("resolve C oracle tool %s: %w", name, err)
	}
	return staticCToolIdentityAtPath(path)
}

func staticCToolIdentityAtPath(path string) (string, string, string, error) {
	path, err := filepath.Abs(path)
	if err != nil {
		return "", "", "", fmt.Errorf("resolve C oracle tool path %s: %w", path, err)
	}
	out, err := exec.Command(path, "--version").Output()
	if err != nil {
		return "", "", "", fmt.Errorf("read C oracle tool version %s: %w", path, err)
	}
	version := strings.TrimSpace(strings.SplitN(string(out), "\n", 2)[0])
	if version == "" {
		return "", "", "", fmt.Errorf("C oracle tool %s returned an empty version", path)
	}
	sha, err := fileSHA256(path)
	if err != nil {
		return "", "", "", fmt.Errorf("hash C oracle tool %s: %w", path, err)
	}
	return path, version, sha, nil
}

func staticCRevalidateBuildInputs(inputs staticCBuildInputSnapshot) error {
	common := inputs.Common
	language := inputs.Language
	checkTool := func(label, path, wantVersion, wantSHA string) error {
		gotPath, gotVersion, gotSHA, err := staticCToolIdentityAtPath(path)
		if err != nil {
			return fmt.Errorf("%s executable: %w", label, err)
		}
		if gotPath != path || gotVersion != wantVersion || gotSHA != wantSHA {
			return fmt.Errorf("%s executable changed: path/version/sha=%q/%q/%q, want %q/%q/%q", label, gotPath, gotVersion, gotSHA, path, wantVersion, wantSHA)
		}
		return nil
	}
	if err := checkTool("compiler", common.CompilerPath, common.CompilerVersion, common.CompilerSHA256); err != nil {
		return err
	}
	if err := checkTool("linker", language.LinkerPath, language.LinkerVersion, language.LinkerSHA256); err != nil {
		return err
	}
	checkFile := func(label, path, wantSHA string) error {
		gotSHA, err := fileSHA256(path)
		if err != nil {
			return fmt.Errorf("%s source: %w", label, err)
		}
		if gotSHA != wantSHA {
			return fmt.Errorf("%s source changed: sha=%s, want %s", label, gotSHA, wantSHA)
		}
		return nil
	}
	if err := checkFile("runtime", inputs.RuntimeSource, common.RuntimeSourceSHA256); err != nil {
		return err
	}
	if err := checkFile("driver", inputs.DriverPath, common.DriverSHA256); err != nil {
		return err
	}
	if got := staticCSourceRelative(inputs.SourceRoot, inputs.ParserPath); got != language.Parser.Path {
		return fmt.Errorf("parser source path changed: %q, want %q", got, language.Parser.Path)
	}
	if err := checkFile("parser", inputs.ParserPath, language.Parser.SHA256); err != nil {
		return err
	}
	if len(inputs.ScannerPaths) != len(language.Scanners) {
		return fmt.Errorf("scanner source count changed: %d, want %d", len(inputs.ScannerPaths), len(language.Scanners))
	}
	for i, scanner := range inputs.ScannerPaths {
		identity := language.Scanners[i]
		if got := staticCSourceRelative(inputs.SourceRoot, scanner); got != identity.Path {
			return fmt.Errorf("scanner %d source path changed: %q, want %q", i, got, identity.Path)
		}
		if err := checkFile(fmt.Sprintf("scanner %d", i), scanner, identity.SHA256); err != nil {
			return err
		}
	}
	if inputs.RuntimeRepoDir != "" {
		if err := verifyPinnedRepo(inputs.RuntimeRepoDir, common.RuntimeCommit); err != nil {
			return fmt.Errorf("runtime repository: %w", err)
		}
		tree, err := parityGitOutput(inputs.RuntimeRepoDir, "rev-parse", common.RuntimeCommit+":lib/src")
		if err != nil || strings.TrimSpace(tree) != common.RuntimeSourceTreeOID {
			return fmt.Errorf("runtime source tree changed: oid=%q want=%q err=%v", strings.TrimSpace(tree), common.RuntimeSourceTreeOID, err)
		}
	}
	if inputs.GrammarRepoDir != "" {
		if err := verifyPinnedRepo(inputs.GrammarRepoDir, language.GrammarCommit); err != nil {
			return fmt.Errorf("grammar repository: %w", err)
		}
		tree, err := parityGitOutput(inputs.GrammarRepoDir, "rev-parse", language.GrammarCommit+":"+filepath.ToSlash(inputs.GrammarTreePath))
		if err != nil || strings.TrimSpace(tree) != language.GrammarSourceTree {
			return fmt.Errorf("grammar source tree changed: oid=%q want=%q err=%v", strings.TrimSpace(tree), language.GrammarSourceTree, err)
		}
	}
	return nil
}

func staticCScannerKey(scanners []perfScanOracleSourceIdentity) string {
	var parts []string
	for _, scanner := range scanners {
		parts = append(parts, scanner.Path+"="+scanner.SHA256+"@"+scanner.CompileFlags)
	}
	return strings.Join(parts, ";")
}

func staticCBuildKey(common perfScanOracleCommonIdentity, language perfScanOracleLanguageIdentity) string {
	keyInput := strings.Join([]string{
		common.Contract, common.BindingCommit, common.RuntimeCommit,
		common.RuntimeSourceTreeOID, common.RuntimeSourceSHA256,
		common.OptimizationFlags, common.RuntimeCFlags, common.GrammarCFlags,
		common.GrammarCXXFlags, common.SourceFlagProtocol, common.DriverCFlags, common.LinkFlags,
		common.TimingRegion, common.OrderProtocol, common.FailureVocabulary, common.TargetOS, common.TargetArch,
		common.CompilerPath, common.CompilerVersion, common.CompilerSHA256, common.DriverSHA256,
		language.Language, language.GrammarRepo, language.GrammarCommit,
		language.GrammarSourceTree, language.GrammarSourceMode, language.LanguageSymbol,
		language.Parser.Path, language.Parser.SHA256, language.Parser.CompileFlags,
		staticCScannerKey(language.Scanners),
		language.LinkerPath, language.LinkerVersion, language.LinkerSHA256,
	}, "\x00")
	key := sha256.Sum256([]byte(keyInput))
	return hex.EncodeToString(key[:])
}

func staticCEnsureArtifact(buildDir, artifact, buildKey, runtimeDir, runtimeSource, srcDir, parserPath string, scanners []string, language perfScanOracleLanguageIdentity, driverPath, symbol string, hasCXX bool, compilerPath, linkerPath string, validateInputs func() error) (string, string, error) {
	return staticCEnsureCachedArtifactValidated(buildDir, artifact, buildKey, symbol, validateInputs, func() error {
		return staticCBuildArtifact(buildDir, artifact, buildKey, runtimeDir, runtimeSource, srcDir, parserPath, scanners, language, driverPath, symbol, hasCXX, compilerPath, linkerPath, validateInputs)
	})
}

func staticCEnsureCachedArtifact(buildDir, artifact, buildKey, symbol string, build func() error) (string, string, error) {
	return staticCEnsureCachedArtifactValidated(buildDir, artifact, buildKey, symbol, nil, build)
}

func staticCEnsureCachedArtifactValidated(buildDir, artifact, buildKey, symbol string, validateInputs func() error, build func() error) (string, string, error) {
	if artifactSHA, proof, err := staticCValidateCachedArtifact(buildDir, artifact, buildKey, symbol); err == nil {
		if validateInputs != nil {
			if err := validateInputs(); err != nil {
				return "", "", err
			}
		}
		return artifactSHA, proof, nil
	}

	var rejectedDir string
	if _, err := os.Stat(buildDir); err == nil {
		if err := os.MkdirAll(filepath.Dir(buildDir), 0o755); err != nil {
			return "", "", err
		}
		rejectedDir, err = os.MkdirTemp(filepath.Dir(buildDir), filepath.Base(buildDir)+".rejected.")
		if err != nil {
			return "", "", fmt.Errorf("reserve rejected cache path: %w", err)
		}
		if err := os.Remove(rejectedDir); err != nil {
			return "", "", err
		}
		if err := os.Rename(buildDir, rejectedDir); err != nil {
			// A concurrent builder may have replaced the entry between validation
			// and quarantine. Accept it only after the complete manifest/hash/static
			// validation succeeds against the same content key.
			if artifactSHA, proof, validateErr := staticCValidateCachedArtifact(buildDir, artifact, buildKey, symbol); validateErr == nil {
				if validateInputs != nil {
					if err := validateInputs(); err != nil {
						return "", "", err
					}
				}
				return artifactSHA, proof, nil
			}
			return "", "", fmt.Errorf("quarantine invalid static C artifact cache: %w", err)
		}
		defer os.RemoveAll(rejectedDir)
	} else if !os.IsNotExist(err) {
		return "", "", fmt.Errorf("inspect static C artifact cache: %w", err)
	}

	if build == nil {
		return "", "", fmt.Errorf("static C artifact builder is nil")
	}
	if err := build(); err != nil {
		return "", "", fmt.Errorf("build static C oracle: %w", err)
	}
	artifactSHA, proof, err := staticCValidateCachedArtifact(buildDir, artifact, buildKey, symbol)
	if err != nil {
		return "", "", fmt.Errorf("validate installed static C oracle: %w", err)
	}
	if validateInputs != nil {
		if err := validateInputs(); err != nil {
			return "", "", err
		}
	}
	return artifactSHA, proof, nil
}

func staticCBuildArtifact(buildDir, artifact, buildKey, runtimeDir, runtimeSource, srcDir, parserPath string, scanners []string, language perfScanOracleLanguageIdentity, driverPath, symbol string, hasCXX bool, compilerPath, linkerPath string, validateInputs func() error) error {
	if !filepath.IsAbs(compilerPath) || !filepath.IsAbs(linkerPath) {
		return fmt.Errorf("static C compiler/linker paths must be absolute")
	}
	if language.BuildKeySHA256 != buildKey {
		return fmt.Errorf("static C language build key does not match the artifact key")
	}
	if staticCSourceRelative(srcDir, parserPath) != language.Parser.Path {
		return fmt.Errorf("static C parser path does not match its compile identity")
	}
	if len(scanners) != len(language.Scanners) {
		return fmt.Errorf("static C scanner count=%d, compile identities=%d", len(scanners), len(language.Scanners))
	}
	for i, scanner := range scanners {
		if staticCSourceRelative(srcDir, scanner) != language.Scanners[i].Path {
			return fmt.Errorf("static C scanner %d path does not match its compile identity", i)
		}
	}
	if err := os.MkdirAll(filepath.Dir(buildDir), 0o755); err != nil {
		return err
	}
	tmp, err := os.MkdirTemp(filepath.Dir(buildDir), filepath.Base(buildDir)+".tmp.")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)
	runtimeObj := filepath.Join(tmp, "runtime.o")
	runtimeArgs := append(strings.Fields(staticCPerfRuntimeCFlags), "-I", filepath.Join(runtimeDir, "lib", "include"), "-I", filepath.Join(runtimeDir, "lib", "src"), "-c", runtimeSource, "-o", runtimeObj)
	if err := runCommand("", compilerPath, runtimeArgs...); err != nil {
		return err
	}
	var objects []string
	compileGrammar := func(source, object, compileFlags string) error {
		if strings.TrimSpace(compileFlags) == "" {
			return fmt.Errorf("static C source %s has empty compile flags", source)
		}
		ext := strings.ToLower(filepath.Ext(source))
		if ext == ".cc" || ext == ".cpp" || ext == ".cxx" {
			args := append(strings.Fields(compileFlags), "-I", srcDir, "-I", filepath.Join(runtimeDir, "lib", "include"), "-c", source, "-o", object)
			return runCommand("", linkerPath, args...)
		}
		if ext != ".c" {
			return fmt.Errorf("static C source %s has unsupported extension %q", source, ext)
		}
		args := append(strings.Fields(compileFlags), "-I", srcDir, "-I", filepath.Join(runtimeDir, "lib", "include"), "-c", source, "-o", object)
		return runCommand("", compilerPath, args...)
	}
	type grammarBuildInput struct {
		path  string
		flags string
	}
	allGrammar := []grammarBuildInput{{path: parserPath, flags: language.Parser.CompileFlags}}
	for i, scanner := range scanners {
		allGrammar = append(allGrammar, grammarBuildInput{path: scanner, flags: language.Scanners[i].CompileFlags})
	}
	for i, source := range allGrammar {
		object := filepath.Join(tmp, fmt.Sprintf("grammar_%d.o", i))
		if err := compileGrammar(source.path, object, source.flags); err != nil {
			return err
		}
		objects = append(objects, object)
	}
	driverObj := filepath.Join(tmp, "driver.o")
	driverArgs := append(strings.Fields(staticCPerfDriverCFlags), "-DTS_LANG_FN="+symbol, "-I", filepath.Join(runtimeDir, "lib", "include", "tree_sitter"), "-c", driverPath, "-o", driverObj)
	if err := runCommand("", compilerPath, driverArgs...); err != nil {
		return err
	}
	if hasCXX && linkerPath == compilerPath {
		return fmt.Errorf("C++ scanner build did not resolve a distinct C++ linker")
	}
	args := append(strings.Fields(staticCPerfLinkFlags), "-o", filepath.Join(tmp, "perf_oracle"), driverObj)
	args = append(args, objects...)
	args = append(args, runtimeObj)
	if err := runCommand("", linkerPath, args...); err != nil {
		return err
	}
	return staticCInstallBuiltArtifact(tmp, buildDir, artifact, buildKey, symbol, validateInputs)
}

func staticCInstallBuiltArtifact(tmp, buildDir, artifact, buildKey, symbol string, validateInputs func() error) error {
	if validateInputs != nil {
		if err := validateInputs(); err != nil {
			return err
		}
	}
	if err := staticCWriteArtifactManifest(tmp, filepath.Join(tmp, "perf_oracle"), buildKey); err != nil {
		return err
	}
	if err := os.Rename(tmp, buildDir); err != nil {
		// Content-keyed artifacts are immutable. Another shard may have won
		// the same atomic install while this one compiled. It is acceptable
		// only if the winner installed the matching manifest and artifact and
		// the captured build inputs are still current.
		if _, _, validateErr := staticCValidateCachedArtifact(buildDir, artifact, buildKey, symbol); validateErr != nil {
			return fmt.Errorf("install artifact: %w (concurrent cache validation: %v)", err, validateErr)
		}
		if validateInputs != nil {
			if validateErr := validateInputs(); validateErr != nil {
				return validateErr
			}
		}
	}
	return nil
}

func staticCArtifactManifestPath(buildDir string) string {
	return filepath.Join(buildDir, "manifest.json")
}

func staticCWriteArtifactManifest(buildDir, artifact, buildKey string) error {
	if len(buildKey) != sha256.Size*2 || !staticCHashIdentity(buildKey) {
		return fmt.Errorf("invalid static C artifact build key %q", buildKey)
	}
	artifactSHA, err := fileSHA256(artifact)
	if err != nil {
		return fmt.Errorf("hash static C artifact for manifest: %w", err)
	}
	manifest := staticCArtifactManifest{
		Schema:         staticCArtifactManifestSchema,
		BuildKeySHA256: buildKey,
		ArtifactSHA256: artifactSHA,
	}
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	if err := os.WriteFile(staticCArtifactManifestPath(buildDir), data, 0o444); err != nil {
		return fmt.Errorf("write static C artifact manifest: %w", err)
	}
	return nil
}

func staticCValidateCachedArtifact(buildDir, artifact, buildKey, symbol string) (string, string, error) {
	data, err := os.ReadFile(staticCArtifactManifestPath(buildDir))
	if err != nil {
		return "", "", fmt.Errorf("read static C artifact manifest: %w", err)
	}
	var manifest staticCArtifactManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return "", "", fmt.Errorf("decode static C artifact manifest: %w", err)
	}
	if manifest.Schema != staticCArtifactManifestSchema {
		return "", "", fmt.Errorf("static C artifact manifest schema=%q, want %q", manifest.Schema, staticCArtifactManifestSchema)
	}
	if manifest.BuildKeySHA256 != buildKey {
		return "", "", fmt.Errorf("static C artifact manifest build key=%q, want %q", manifest.BuildKeySHA256, buildKey)
	}
	if len(manifest.ArtifactSHA256) != sha256.Size*2 || !staticCHashIdentity(manifest.ArtifactSHA256) {
		return "", "", fmt.Errorf("static C artifact manifest has invalid artifact SHA %q", manifest.ArtifactSHA256)
	}
	artifactSHA, err := fileSHA256(artifact)
	if err != nil {
		return "", "", fmt.Errorf("hash cached static C artifact: %w", err)
	}
	if artifactSHA != manifest.ArtifactSHA256 {
		return "", "", fmt.Errorf("cached static C artifact SHA=%s, manifest=%s", artifactSHA, manifest.ArtifactSHA256)
	}
	proof, err := staticCVerifyArtifact(artifact, symbol)
	if err != nil {
		return "", "", err
	}
	return artifactSHA, proof, nil
}

func staticCVerifyArtifact(artifact, symbol string) (string, error) {
	if strings.TrimSpace(symbol) == "" {
		return "", fmt.Errorf("language symbol is empty")
	}
	nmOut, err := exec.Command("nm", artifact).Output()
	if err != nil {
		return "", fmt.Errorf("nm: %w", err)
	}
	defined := make(map[string]bool)
	for line := range strings.SplitSeq(string(nmOut), "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 3 && !strings.EqualFold(fields[len(fields)-2], "U") {
			defined[fields[len(fields)-1]] = true
		}
	}
	for _, required := range []string{"ts_parser_parse", symbol} {
		if !defined[required] {
			return "", fmt.Errorf("artifact does not define %s", required)
		}
	}
	readelfOut, err := exec.Command("readelf", "-d", artifact).CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("readelf dynamic-section proof: %w: %s", err, strings.TrimSpace(string(readelfOut)))
	}
	if bytes.Contains(readelfOut, []byte("NEEDED")) {
		return "", fmt.Errorf("artifact has dynamic dependencies: %s", strings.TrimSpace(string(readelfOut)))
	}
	programOut, err := exec.Command("readelf", "-l", artifact).CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("readelf program-header proof: %w: %s", err, strings.TrimSpace(string(programOut)))
	}
	if bytes.Contains(programOut, []byte("INTERP")) {
		return "", fmt.Errorf("artifact has a dynamic program interpreter: %s", strings.TrimSpace(string(programOut)))
	}
	return staticCPerfLinkageProof, nil
}

func staticCPrivateExecutableSnapshot(artifact, expectedSHA, symbol string) (string, func(), error) {
	dir, err := os.MkdirTemp("", "gts-static-c-exec-*")
	if err != nil {
		return "", nil, err
	}
	cleanup := func() { _ = os.RemoveAll(dir) }
	source, err := os.Open(artifact)
	if err != nil {
		cleanup()
		return "", nil, err
	}
	info, err := source.Stat()
	if err != nil || !info.Mode().IsRegular() {
		_ = source.Close()
		cleanup()
		if err == nil {
			err = fmt.Errorf("cached static C artifact is not a regular file")
		}
		return "", nil, err
	}
	privatePath := filepath.Join(dir, "perf_oracle")
	destination, err := os.OpenFile(privatePath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o500)
	if err != nil {
		_ = source.Close()
		cleanup()
		return "", nil, err
	}
	_, copyErr := io.Copy(destination, source)
	closeSourceErr := source.Close()
	syncErr := destination.Sync()
	closeDestinationErr := destination.Close()
	for _, candidate := range []error{copyErr, closeSourceErr, syncErr, closeDestinationErr} {
		if candidate != nil {
			cleanup()
			return "", nil, candidate
		}
	}
	snapshotSHA, err := fileSHA256(privatePath)
	if err != nil || snapshotSHA != expectedSHA {
		cleanup()
		if err == nil {
			err = fmt.Errorf("private static C artifact SHA=%s, want %s", snapshotSHA, expectedSHA)
		}
		return "", nil, err
	}
	if _, err := staticCVerifyArtifact(privatePath, symbol); err != nil {
		cleanup()
		return "", nil, err
	}
	return privatePath, cleanup, nil
}

func (oracle *staticCPerfOracle) snapshot(source []byte) (string, string, func(), error) {
	sum := sha256.Sum256(source)
	sourceSHA := hex.EncodeToString(sum[:])
	dir, err := os.MkdirTemp("", "gts-static-c-source-*")
	if err != nil {
		return "", sourceSHA, nil, err
	}
	cleanup := func() { _ = os.RemoveAll(dir) }
	path := filepath.Join(dir, "source.snapshot")
	if err := os.WriteFile(path, source, 0o444); err != nil {
		cleanup()
		return "", sourceSHA, nil, err
	}
	return path, sourceSHA, cleanup, nil
}

func staticCSidecarFailureStatus(detail string, contextErr error, fallback string) string {
	for marker, status := range map[string]string{
		"status=c_timeout":           staticCStatusParserTimeout,
		"status=c_parser_error":      staticCStatusParserError,
		"status=c_transport_error":   staticCStatusTransportError,
		"status=c_digest_error":      staticCStatusDigestError,
		"status=c_measurement_error": staticCStatusMeasurementError,
		"status=c_protocol_error":    staticCStatusProtocolError,
		"status=c_incomplete":        staticCStatusIncomplete,
	} {
		if strings.Contains(detail, marker) {
			return status
		}
	}
	if contextErr != nil {
		return staticCStatusTransportTimeout
	}
	return fallback
}

func (oracle *staticCPerfOracle) deepDigest(source []byte, budget time.Duration) (string, string, error) {
	path, sourceSHA, cleanup, err := oracle.snapshot(source)
	if err != nil {
		return "", sourceSHA, newStaticCOracleError(staticCStatusTransportError, "snapshot static C deep-digest source: "+err.Error())
	}
	defer cleanup()
	ctx, cancel := context.WithTimeout(context.Background(), budget+oracle.transportGrace())
	defer cancel()
	cmd := exec.CommandContext(ctx, oracle.artifact, "dump", path, strconv.FormatInt(budget.Microseconds(), 10))
	h := sha256.New()
	cmd.Stdout = h
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		detail := strings.TrimSpace(stderr.String())
		status := staticCSidecarFailureStatus(detail, ctx.Err(), staticCStatusTransportError)
		return "", sourceSHA, newStaticCOracleError(status, strings.TrimSpace(fmt.Sprintf("static C deep dump: %v: %s", err, detail)))
	}
	if detail := strings.TrimSpace(stderr.String()); detail != "" {
		return "", sourceSHA, newStaticCOracleError(staticCStatusProtocolError, "static C deep dump wrote stderr: "+detail)
	}
	return hex.EncodeToString(h.Sum(nil)), sourceSHA, nil
}

func (oracle *staticCPerfOracle) measure(source []byte, axis string, warmup, reps int, budget time.Duration) staticCPerfMeasurement {
	if axis != perfScanAxisFull {
		return staticCPerfMeasurement{Status: staticCStatusMeasurementError, Detail: "static C publication oracle supports fresh full parse only"}
	}
	path, _, cleanup, err := oracle.snapshot(source)
	if err != nil {
		return staticCPerfMeasurement{Status: staticCStatusTransportError, Detail: err.Error()}
	}
	defer cleanup()
	wall := time.Duration(warmup+reps+1)*budget + oracle.transportGrace()
	ctx, cancel := context.WithTimeout(context.Background(), wall)
	defer cancel()
	cmd := exec.CommandContext(ctx, oracle.artifact, "measure", axis, path, strconv.Itoa(warmup), strconv.Itoa(reps), strconv.FormatInt(budget.Microseconds(), 10))
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return staticCPerfMeasurement{Status: staticCStatusTransportError, Detail: err.Error()}
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		return staticCPerfMeasurement{Status: staticCStatusTransportError, Detail: err.Error()}
	}
	result := staticCPerfMeasurement{}
	var schemaSeen, axisSeen, statusSeen bool
	scanner := bufio.NewScanner(stdout)
	for scanner.Scan() {
		line := scanner.Text()
		switch {
		case strings.HasPrefix(line, "schema="):
			if schemaSeen || strings.TrimPrefix(line, "schema=") != staticCPerfSchema {
				result.Status = staticCStatusProtocolError
				result.Detail = "unexpected static C schema: " + line
			}
			schemaSeen = true
		case strings.HasPrefix(line, "axis="):
			if axisSeen || strings.TrimPrefix(line, "axis=") != axis {
				result.Status = staticCStatusProtocolError
				result.Detail = "unexpected static C axis: " + line
			}
			axisSeen = true
		case strings.HasPrefix(line, "sample_ns="):
			ns, parseErr := strconv.ParseInt(strings.TrimPrefix(line, "sample_ns="), 10, 64)
			if parseErr != nil || ns <= 0 {
				result.Status = staticCStatusMeasurementError
				result.Detail = "invalid static C sample: " + line
			} else {
				result.Samples = append(result.Samples, ns)
			}
		case strings.HasPrefix(line, "status="):
			status := strings.TrimPrefix(line, "status=")
			if statusSeen {
				result.Status = staticCStatusProtocolError
				result.Detail = "duplicate static C status"
			} else if status == "ok" {
				if result.Status == "" {
					result.Status = perfScanStatusOK
				}
			} else if status == "c_timeout" {
				result.Status = staticCStatusParserTimeout
				result.Detail = status
			} else if status == "c_incomplete" {
				result.Status = staticCStatusIncomplete
				result.Detail = status
			} else {
				result.Status = staticCStatusProtocolError
				result.Detail = "unexpected static C status: " + status
			}
			statusSeen = true
		default:
			result.Status = staticCStatusProtocolError
			result.Detail = "unexpected static C protocol line: " + line
		}
	}
	waitErr := cmd.Wait()
	if scanner.Err() != nil && result.Detail == "" {
		result.Status, result.Detail = staticCStatusTransportError, scanner.Err().Error()
	}
	if waitErr != nil {
		result.Detail = strings.TrimSpace(fmt.Sprintf("%v %s", waitErr, stderr.String()))
		result.Status = staticCSidecarFailureStatus(result.Detail, ctx.Err(), staticCStatusTransportError)
	} else if detail := strings.TrimSpace(stderr.String()); detail != "" {
		result.Status = staticCStatusProtocolError
		result.Detail = "static C measurement wrote stderr: " + detail
	}
	if result.Status == perfScanStatusOK && (!schemaSeen || !axisSeen || !statusSeen) {
		result.Status = staticCStatusProtocolError
		result.Detail = "static C response is missing schema, axis, or status"
	}
	if result.Status == perfScanStatusOK && len(result.Samples) != reps {
		result.Status = staticCStatusMeasurementError
		result.Detail = fmt.Sprintf("static C returned %d samples, want %d", len(result.Samples), reps)
	}
	if result.Status == "" {
		result.Status = staticCStatusProtocolError
		result.Detail = "static C returned no status"
	}
	return result
}
