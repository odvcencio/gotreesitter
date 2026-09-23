package gotreesitter

import (
	"crypto/sha256"
	"encoding/gob"
	"fmt"
	"io"
	"os"
	"os/exec"
	"regexp"
	"testing"
)

// TestLanguageBlobEncodingIsProcessHistoryIndependent proves task #37's fix:
// the bytes EncodeLanguageBlob writes for one Language must not depend on
// which other types the process gob-encoded first. encoding/gob assigns
// wire type IDs from a process-global counter the first time a type crosses
// any Encoder, so without the init-time priming in
// language_blob_gob_prime.go a process that encoded unrelated types first
// wrote different bytes (a different SHA-256) for an identical Language.
//
// The parent encodes the language directly. The child is this same test
// binary re-executed with GTS_GOB_PRIME_CHILD=1; it first pushes several
// unrelated types through an Encoder to move the global counter, then
// encodes the same language and prints its SHA-256. The two hashes must
// match.
func TestLanguageBlobEncodingIsProcessHistoryIndependent(t *testing.T) {
	lang := gobPrimeProbeLanguage()
	if os.Getenv("GTS_GOB_PRIME_CHILD") == "1" {
		type junkLeaf struct{ A int32 }
		type junkNode struct {
			Name  string
			Leafs []junkLeaf
			Table map[string]uint16
		}
		enc := gob.NewEncoder(io.Discard)
		for _, v := range []any{junkLeaf{}, junkNode{}, map[uint32][]junkNode{}, [][]string{}, struct{ X, Y float64 }{}} {
			if err := enc.Encode(v); err != nil {
				fmt.Printf("CHILD-ERROR: %v\n", err)
				return
			}
		}
		blob, err := EncodeLanguageBlob(lang)
		if err != nil {
			fmt.Printf("CHILD-ERROR: %v\n", err)
			return
		}
		fmt.Printf("CHILD-SHA=%x\n", sha256.Sum256(blob))
		return
	}

	blob, err := EncodeLanguageBlob(lang)
	if err != nil {
		t.Fatalf("EncodeLanguageBlob: %v", err)
	}
	want := fmt.Sprintf("%x", sha256.Sum256(blob))

	exe, err := os.Executable()
	if err != nil {
		t.Fatalf("os.Executable: %v", err)
	}
	cmd := exec.Command(exe, "-test.run", "^TestLanguageBlobEncodingIsProcessHistoryIndependent$", "-test.v", "-test.count=1")
	cmd.Env = append(os.Environ(), "GTS_GOB_PRIME_CHILD=1")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("child test binary: %v\n%s", err, out)
	}
	m := regexp.MustCompile(`CHILD-SHA=([0-9a-f]+)`).FindSubmatch(out)
	if m == nil {
		t.Fatalf("child printed no SHA:\n%s", out)
	}
	if got := string(m[1]); got != want {
		t.Fatalf("blob bytes depend on process gob history: parent sha256=%s child sha256=%s (child encoded unrelated types first)", want, got)
	}
}

// gobPrimeProbeLanguage is a small synthetic Language with every table kind
// populated, so the encoded stream carries the full wire type graph.
func gobPrimeProbeLanguage() *Language {
	return &Language{
		Name:              "gob-prime-probe",
		SymbolNames:       []string{"end", "a", "b", "source"},
		SymbolMetadata:    []SymbolMetadata{{}, {Visible: true, Named: true}, {Visible: true, Named: true}, {Visible: true, Named: true}},
		ExternalSymbols:   []Symbol{2},
		FieldNames:        []string{"", "name"},
		StateCount:        3,
		LargeStateCount:   0,
		ProductionIDCount: 1,
		ParseActions:      []ParseActionEntry{{Reusable: true}},
	}
}
