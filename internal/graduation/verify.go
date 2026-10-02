package graduation

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
)

// VerifySources authenticates an external receipt against this checkout's
// runtime, measurement harness, fixture pins and locked native C identities.
func (matrix *Matrix) VerifySources(root string) error {
	for _, harness := range []bool{false, true} {
		actual, err := SourceFingerprint(root, harness)
		if err != nil {
			return err
		}
		want := matrix.RuntimeSHA256
		if harness {
			want = matrix.HarnessSHA256
		}
		if actual != want {
			return fmt.Errorf("graduation measurements are stale for the current sources")
		}
	}
	fixtures, err := os.ReadFile(filepath.Join(root, "internal/benchfixtures/generated.json"))
	if err != nil {
		return err
	}
	if fmt.Sprintf("%x", sha256.Sum256(fixtures)) != matrix.FixturesSHA256 {
		return fmt.Errorf("graduation fixture pins changed since measurement")
	}
	var manifest struct {
		Entries []struct {
			Grammar     string `json:"language"`
			TargetBytes int    `json:"target_bytes"`
			SourceBytes int    `json:"source_bytes"`
			SHA256      string `json:"sha256"`
		} `json:"entries"`
	}
	if err := json.Unmarshal(fixtures, &manifest); err != nil {
		return err
	}
	for _, language := range matrix.Languages {
		shape := language.Grammar
		if shape == "bash" {
			shape = "sh"
		}
		if language.FixtureShape != shape {
			return fmt.Errorf("%s: fixture shape differs from the pinned language", language.Grammar)
		}
		for _, cell := range language.Cells {
			target := map[string]int{"32k": 32 << 10, "137k": 137 << 10, "1m": 1 << 20}[cell.Size]
			matched := false
			for _, pin := range manifest.Entries {
				if pin.Grammar == shape && pin.TargetBytes == target && pin.SourceBytes == cell.SourceBytes && pin.SHA256 == cell.SourceSHA256 {
					matched = true
				}
			}
			if !matched {
				return fmt.Errorf("%s/%s: source differs from the pinned generator", language.Grammar, cell.Size)
			}
		}
	}
	corpusDigest, err := os.ReadFile(filepath.Join(root, "cgo_harness/perf_scan/corpus_sources.lock.sha256"))
	if err != nil {
		return err
	}
	if fields := strings.Fields(string(corpusDigest)); len(fields) == 0 || fields[0] != matrix.CorpusLockSHA256 {
		return fmt.Errorf("graduation corpus identity differs from the pinned lock digest")
	}
	loader, err := os.ReadFile(filepath.Join(root, "cgo_harness/parity_c_loader_cgo.go"))
	if err != nil {
		return err
	}
	constants := map[string]string{}
	for _, constant := range []string{"COracleBindingCommit", "COracleRuntimeCommit"} {
		matches := regexp.MustCompile(constant + `\s*=\s*"([^"]+)"`).FindSubmatch(loader)
		if len(matches) != 2 {
			return fmt.Errorf("locked C constant %s missing", constant)
		}
		constants[constant] = string(matches[1])
	}
	lock, err := os.ReadFile(filepath.Join(root, "grammars/languages.lock"))
	if err != nil {
		return err
	}
	commits := map[string]string{}
	for _, line := range strings.Split(string(lock), "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 3 && !strings.HasPrefix(fields[0], "#") {
			commits[fields[0]] = fields[2]
		}
	}
	for _, language := range matrix.Languages {
		var oracle map[string]string
		if err := json.Unmarshal(language.OracleIdentity, &oracle); err != nil {
			return err
		}
		if oracle["binding_commit"] != constants["COracleBindingCommit"] || oracle["runtime_commit"] != constants["COracleRuntimeCommit"] || oracle["grammar_commit"] != commits[language.Grammar] {
			return fmt.Errorf("%s: oracle differs from the runtime or grammar lock", language.Grammar)
		}
	}

	return nil
}

// VerifyDefaults requires measured decisions for every enabled default. An
// absent external receipt is accepted only while no language is graduated.
func VerifyDefaults(matrix *Matrix) error {
	actual := Allowlist()
	want := map[string]bool{}
	if matrix != nil {
		for _, grammar := range matrix.Graduated() {
			want[grammar] = true
		}
	}
	if !reflect.DeepEqual(actual, want) {
		return fmt.Errorf("runtime allowlist differs from measured graduation decisions (an external receipt is required for nonempty defaults)")
	}
	return nil
}
