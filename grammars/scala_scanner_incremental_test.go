//go:build !grammar_subset || grammar_subset_scala

package grammars

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"testing"
	_ "unsafe"

	gotreesitter "github.com/odvcencio/gotreesitter"
)

func TestScalaScannerCheckpointCodec(t *testing.T) {
	scanner := ScalaExternalScanner{}
	tests := []scalaState{
		{lastIndentationSize: -1, lastColumn: -1},
		{lastIndentationSize: 7, lastNewlineCount: 3, lastColumn: 19},
		{indents: []int16{0, 2, 4, 32767}, lastIndentationSize: -32768, lastNewlineCount: 32767, lastColumn: -1},
	}
	for index := range tests {
		original := tests[index]
		buffer := make([]byte, 4096)
		size := scanner.Serialize(&original, buffer)
		if size == 0 {
			t.Fatalf("case %d did not produce a checkpoint", index)
		}
		restored := scalaState{indents: []int16{99}, lastIndentationSize: 99, lastNewlineCount: 99, lastColumn: 99}
		scanner.Deserialize(&restored, buffer[:size])
		if !scalaStatesEqual(restored, original) {
			t.Fatalf("case %d restored %+v, want %+v", index, restored, original)
		}
	}

	defaultState := scanner.Create().(*scalaState)
	if size := scanner.Serialize(defaultState, make([]byte, 4096)); size == 0 {
		t.Fatal("the default Scala state serialized as an absent checkpoint")
	}

	boundary := scalaState{indents: []int16{1, 2}, lastIndentationSize: 3, lastNewlineCount: 4, lastColumn: 5}
	required := scalaScannerCheckpointHeader + len(boundary.indents)*2
	if size := scanner.Serialize(&boundary, make([]byte, required)); size != required {
		t.Fatalf("exact-capacity checkpoint size = %d, want %d", size, required)
	}
	if size := scanner.Serialize(&boundary, make([]byte, required-1)); size != 0 {
		t.Fatalf("short checkpoint buffer produced %d bytes, want zero", size)
	}
	maximum := scalaState{indents: make([]int16, (4096-scalaScannerCheckpointHeader)/2)}
	if size := scanner.Serialize(&maximum, make([]byte, 4096)); size != 4096 {
		t.Fatalf("maximum Scala checkpoint size = %d, want 4096", size)
	}
	overflow := scalaState{indents: make([]int16, len(maximum.indents)+1)}
	if size := scanner.Serialize(&overflow, make([]byte, 4096)); size != 0 {
		t.Fatalf("unrepresentable Scala state produced %d bytes, want zero", size)
	}
}

func TestScalaScannerCheckpointCodecIsInjective(t *testing.T) {
	states := []scalaState{
		{lastIndentationSize: -1, lastColumn: -1},
		{indents: []int16{0}, lastIndentationSize: -1, lastColumn: -1},
		{lastIndentationSize: 0, lastColumn: -1},
		{lastIndentationSize: -1, lastNewlineCount: 1, lastColumn: -1},
		{lastIndentationSize: -1, lastColumn: 0},
	}
	seen := make(map[string]int, len(states))
	for index := range states {
		buffer := make([]byte, 4096)
		size := (ScalaExternalScanner{}).Serialize(&states[index], buffer)
		if prior, exists := seen[string(buffer[:size])]; exists {
			t.Fatalf("states %d and %d produced the same checkpoint", prior, index)
		}
		seen[string(buffer[:size])] = index
	}
}

func TestScalaScannerCheckpointRestoreReusesIndentCapacity(t *testing.T) {
	original := scalaState{
		indents:             []int16{0, 2, 4, 6},
		lastIndentationSize: 8,
		lastNewlineCount:    2,
		lastColumn:          12,
	}
	buffer := make([]byte, 4096)
	scanner := ScalaExternalScanner{}
	size := scanner.Serialize(&original, buffer)
	restored := scalaState{indents: make([]int16, 0, len(original.indents))}

	allocations := testing.AllocsPerRun(1000, func() {
		scanner.Deserialize(&restored, buffer[:size])
	})
	if allocations != 0 {
		t.Fatalf("steady-state checkpoint restore allocations = %v, want zero", allocations)
	}
	if !scalaStatesEqual(restored, original) {
		t.Fatalf("restored state = %+v, want %+v", restored, original)
	}
}

func TestScalaScannerMalformedCheckpointResetsWithoutPanic(t *testing.T) {
	validState := scalaState{indents: []int16{2}, lastIndentationSize: 3, lastNewlineCount: 4, lastColumn: 5}
	valid := make([]byte, 4096)
	valid = valid[:(ScalaExternalScanner{}).Serialize(&validState, valid)]
	malformed := [][]byte{
		nil,
		{scalaScannerCheckpointMagic},
		append([]byte(nil), valid[:scalaScannerCheckpointHeader-1]...),
		append([]byte{0}, valid[1:]...),
		append([]byte{scalaScannerCheckpointMagic, scalaScannerCheckpointVersion + 1}, valid[2:]...),
		append(append([]byte(nil), valid...), 0),
	}
	badCount := append([]byte(nil), valid...)
	badCount[2]++
	malformed = append(malformed, badCount)

	for index, checkpoint := range malformed {
		state := scalaState{indents: []int16{99}, lastIndentationSize: 99, lastNewlineCount: 99, lastColumn: 99}
		(ScalaExternalScanner{}).Deserialize(&state, checkpoint)
		want := scalaState{lastIndentationSize: -1, lastColumn: -1}
		if !scalaStatesEqual(state, want) {
			t.Fatalf("malformed case %d restored %+v, want %+v", index, state, want)
		}
	}
}

func TestScalaScannerFailedScanRequiresEagerRollback(t *testing.T) {
	state := &scalaState{lastIndentationSize: 7, lastColumn: -1}
	valid := make([]bool, scaTokenCount)
	valid[scaTokAutoSemicolon] = true
	if (ScalaExternalScanner{}).Scan(state, newScalaExternalLexer([]byte("\n."), 0, 0, 0), valid) {
		t.Fatal("Scala automatic-semicolon rejection returned a token")
	}
	if state.lastIndentationSize != -1 {
		t.Fatalf("failed Scala scan preserved state unexpectedly: %+v", state)
	}
}

func scalaStatesEqual(left, right scalaState) bool {
	return slices.Equal(left.indents, right.indents) &&
		left.lastIndentationSize == right.lastIndentationSize &&
		left.lastNewlineCount == right.lastNewlineCount &&
		left.lastColumn == right.lastColumn
}

func TestScalaScannerExactProfile(t *testing.T) {
	t.Cleanup(func() { PurgeEmbeddedLanguageCache() })
	lang := ScalaLanguage()
	scanner := lang.ExternalScanner
	reusable, ok := scanner.(gotreesitter.IncrementalReuseExternalScanner)
	if !ok || reusable.SupportsIncrementalReuse() {
		t.Fatalf("exact Scala scanner capability = %T/%v, want explicit general-reuse rejection", scanner, ok)
	}
	checkpointed, ok := scanner.(gotreesitter.CheckpointedExternalScanner)
	if !ok || !checkpointed.UsesExternalScannerCheckpoints() {
		t.Fatalf("exact Scala scanner is not checkpoint-certified: %T", scanner)
	}
	errorReuse, ok := scanner.(gotreesitter.ErrorTreeIncrementalReuseExternalScanner)
	if !ok || errorReuse.SupportsIncrementalReuseFromErrorTree() {
		t.Fatalf("exact Scala scanner error-tree capability = %T/%v, want explicit rejection", scanner, ok)
	}
	if _, ok := scanner.(gotreesitter.FailurePreservingExternalScanner); ok {
		t.Fatal("Scala advertised failure-state preservation despite mutating failed scans")
	}

	provider, ok := scanner.(gotreesitter.ExternalScannerCheckpointIdentityProvider)
	if !ok {
		t.Fatalf("exact Scala scanner has no checkpoint identity: %T", scanner)
	}
	identity, ok := provider.CheckpointIdentity()
	grammarIdentity, grammarOK := lang.GrammarBlobSHA256()
	if !ok || !grammarOK || !bytes.Equal(identity.Grammar, grammarIdentity[:]) ||
		!bytes.Equal(identity.Scanner, scalaExternalScannerIdentity[:]) {
		t.Fatalf("Scala checkpoint identity is not bound to the exact scanner and grammar")
	}
	identity.Scanner[0]++
	identity.Grammar[0]++
	second, ok := provider.CheckpointIdentity()
	if !ok || bytes.Equal(identity.Scanner, second.Scanner) || bytes.Equal(identity.Grammar, second.Grammar) {
		t.Fatal("Scala checkpoint identity returned mutable aliased storage")
	}

	rawLanguage := &gotreesitter.Language{Name: "scala"}
	raw := ScalaExternalScanner{}.ExternalScannerForLanguage(rawLanguage)
	if _, ok := raw.(gotreesitter.IncrementalReuseExternalScanner); ok {
		t.Fatal("an unverified raw Scala scanner advertised incremental reuse")
	}
	rawLanguage.ExternalScanner = raw
	if attachBuiltinLanguageRuntimeProfile("scala", sha256.Sum256([]byte("wrong Scala blob")), rawLanguage) {
		t.Fatal("a mismatched Scala blob received a runtime profile")
	}
	if _, ok := rawLanguage.ExternalScanner.(gotreesitter.IncrementalReuseExternalScanner); ok {
		t.Fatal("a mismatched Scala blob received scanner reuse certification")
	}
}

func TestScalaScannerAdaptationStaysConservative(t *testing.T) {
	native := ScalaLanguage()
	target := &gotreesitter.Language{
		Name:            "foreign-scala-shape",
		SymbolNames:     append([]string(nil), native.SymbolNames...),
		ExternalSymbols: append([]gotreesitter.Symbol(nil), native.ExternalSymbols...),
	}
	if !AdaptScannerForLanguage("scala", target) || target.ExternalScanner == nil {
		t.Fatal("Scala scanner adaptation failed")
	}
	if _, ok := target.ExternalScanner.(gotreesitter.IncrementalReuseExternalScanner); ok {
		t.Fatalf("adapted Scala scanner advertised exact-blob reuse: %T", target.ExternalScanner)
	}
}

func TestScalaScannerIdentityBindsLocalPortSource(t *testing.T) {
	_, testFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	source, err := os.ReadFile(filepath.Join(filepath.Dir(testFile), "scala_scanner.go"))
	if err != nil {
		t.Fatalf("read Scala scanner source: %v", err)
	}
	begin := []byte("// SCALA_EXTERNAL_SCANNER_LOCAL_PORT_BEGIN\n")
	end := []byte("// SCALA_EXTERNAL_SCANNER_LOCAL_PORT_END\n")
	start := bytes.Index(source, begin)
	if start < 0 {
		t.Fatal("Scala scanner local-port begin marker is missing")
	}
	start += len(begin)
	finish := bytes.Index(source[start:], end)
	if finish < 0 {
		t.Fatal("Scala scanner local-port end marker is missing")
	}
	digest := sha256.Sum256(source[start : start+finish])
	if got := hex.EncodeToString(digest[:]); got != scalaExternalScannerLocalPortSHA256 {
		t.Fatalf("Scala scanner source hash = %s, want %s; update its ABI identity", got, scalaExternalScannerLocalPortSHA256)
	}
}

//go:linkname newScalaExternalLexer github.com/odvcencio/gotreesitter.newExternalLexer
func newScalaExternalLexer(source []byte, pos int, row, col uint32) *gotreesitter.ExternalLexer
