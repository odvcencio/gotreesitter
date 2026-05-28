package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// TestGrammarSubsetEmbeddedReducesBinarySize is the build-matrix guard for
// issue #88's embedded-but-selective grammar builds. It builds this probe both
// with the default (all-grammars) tags and with
// `grammar_subset grammar_subset_go`, and asserts the subset binary is
// dramatically smaller because only go.bin is embedded (~20MB of other blobs
// dropped). If someone reintroduces the all-blobs wildcard into a grammar_subset
// build, the subset binary balloons back to ~full size and this test fails.
func TestGrammarSubsetEmbeddedReducesBinarySize(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping binary-size build matrix in -short mode")
	}
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not available")
	}

	build := func(name string, tags ...string) int64 {
		out := filepath.Join(t.TempDir(), name)
		args := []string{"build", "-o", out}
		if len(tags) > 0 {
			args = append(args, "-tags", joinSpace(tags))
		}
		args = append(args, ".")
		cmd := exec.Command("go", args...)
		cmd.Env = append(os.Environ(), "GOMAXPROCS=2")
		if outBytes, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("build %v failed: %v\n%s", args, err, outBytes)
		}
		fi, err := os.Stat(out)
		if err != nil {
			t.Fatalf("stat %s: %v", out, err)
		}
		return fi.Size()
	}

	full := build("probe.full")
	subsetGo := build("probe.subset_go", "grammar_subset", "grammar_subset_go")

	// The unselected ~205 blobs total ~20MB; dropping them must shrink the
	// binary by well over 10MB. Use a conservative floor to stay robust across
	// toolchains and future blob churn.
	const minReduction = 10 << 20 // 10 MiB
	if full-subsetGo < minReduction {
		t.Fatalf("grammar_subset_go binary not meaningfully smaller than default: "+
			"full=%d subset_go=%d (reduction=%d, want >= %d) — is the all-blobs wildcard "+
			"still compiled into grammar_subset builds?", full, subsetGo, full-subsetGo, minReduction)
	}
	t.Logf("OK: full=%d subset_go=%d (%.1fMB smaller)", full, subsetGo, float64(full-subsetGo)/(1<<20))
}

func joinSpace(parts []string) string {
	s := ""
	for i, p := range parts {
		if i > 0 {
			s += " "
		}
		s += p
	}
	return s
}
