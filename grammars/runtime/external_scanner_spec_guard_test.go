//go:build !grammar_subset

package grammarruntime

import (
	"bufio"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
)

type lockPin struct{ repo, commit string }

// readLanguagesLock parses grammars/languages.lock: "name repo commit ...".
func readLanguagesLock(t *testing.T) map[string]lockPin {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller(0) failed to resolve this test file's path")
	}
	path := filepath.Join(filepath.Dir(thisFile), "..", "languages.lock")
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open %s: %v", path, err)
	}
	defer f.Close()
	pins := map[string]lockPin{}
	s := bufio.NewScanner(f)
	for s.Scan() {
		line := strings.TrimSpace(s.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 3 {
			t.Fatalf("%s: malformed line %q", path, line)
		}
		pins[strings.ToLower(fields[0])] = lockPin{repo: fields[1], commit: fields[2]}
	}
	if err := s.Err(); err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return pins
}

// incompleteExternalScannerSpecs lists specs that predate the full contract
// (upstream commit and source hashes). Every other spec must be complete.
// Remove a row when its spec is completed.
var incompleteExternalScannerSpecs = map[string]bool{"go": true, "python": true}

// TestExternalScannerSpecsMatchLock checks that every registered spec records
// the repository and commit that grammars/languages.lock pins for the same
// language, so a lock bump cannot leave a spec describing an older scanner.
func TestExternalScannerSpecsMatchLock(t *testing.T) {
	pins := readLanguagesLock(t)
	externalScannerSpecRegistry.RLock()
	specs := make([]ExternalScannerSpec, 0, len(externalScannerSpecRegistry.byName))
	for _, spec := range externalScannerSpecRegistry.byName {
		specs = append(specs, spec)
	}
	externalScannerSpecRegistry.RUnlock()
	sort.Slice(specs, func(i, j int) bool { return specs[i].Language < specs[j].Language })
	if len(specs) == 0 {
		t.Fatal("no external scanner specs are registered")
	}
	for _, spec := range specs {
		pin, ok := pins[spec.Language]
		if !ok {
			t.Errorf("%s: spec has no entry in grammars/languages.lock", spec.Language)
			continue
		}
		if spec.UpstreamRepo != pin.repo {
			t.Errorf("%s: spec repo %q, lock repo %q", spec.Language, spec.UpstreamRepo, pin.repo)
		}
		complete := spec.UpstreamCommit != "" && len(spec.SourceFiles) > 0
		switch {
		case incompleteExternalScannerSpecs[spec.Language] && complete:
			t.Errorf("%s: spec is complete; remove it from incompleteExternalScannerSpecs", spec.Language)
		case !incompleteExternalScannerSpecs[spec.Language] && !complete:
			t.Errorf("%s: spec lists commit %q and %d source files; want both", spec.Language, spec.UpstreamCommit, len(spec.SourceFiles))
		}
		if spec.UpstreamCommit != "" && spec.UpstreamCommit != pin.commit {
			t.Errorf("%s: spec commit %q, lock commit %q", spec.Language, spec.UpstreamCommit, pin.commit)
		}
		if len(spec.Externals) == 0 {
			t.Errorf("%s: spec lists no externals", spec.Language)
		}
		for _, file := range spec.SourceFiles {
			if len(file.SHA256) != 64 {
				t.Errorf("%s: %s has SHA-256 %q, want 64 hex digits", spec.Language, file.Path, file.SHA256)
			}
		}
	}
}

// TestEveryRegisteredScannerHasSpec fails when a hand-written external scanner
// is registered without an ExternalScannerSpec.
func TestEveryRegisteredScannerHasSpec(t *testing.T) {
	var missing []string
	for name := range externalScannerRegistry {
		if _, ok := LookupExternalScannerSpec(name); !ok {
			missing = append(missing, name)
		}
	}
	sort.Strings(missing)
	if len(externalScannerRegistry) == 0 {
		t.Fatal("no external scanners are registered")
	}
	if len(missing) != 0 {
		t.Errorf("scanners without an ExternalScannerSpec: %s", strings.Join(missing, ", "))
	}
}
